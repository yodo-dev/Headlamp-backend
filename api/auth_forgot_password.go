package api

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"time"
)

const otpExpiryDuration = 15 * time.Minute

// ─── Request / Response types ────────────────────────────────────────────────

type forgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type resendOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type verifyOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
	OTP   string `json:"otp"   binding:"required,min=6,max=6"`
}

type verifyOTPResponse struct {
	ResetToken string    `json:"reset_token"`
	ExpiresAt  time.Time `json:"expires_at"`
	Message    string    `json:"message"`
}

type resetPasswordRequest struct {
	ResetToken      string `json:"reset_token"      binding:"required"`
	Password        string `json:"password"         binding:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" binding:"required,min=8"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// generateOTP produces a cryptographically random 6-digit numeric string.
func generateOTP() (string, error) {
	const digits = "0123456789"
	var otp [6]byte
	for i := range otp {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", fmt.Errorf("failed to generate OTP digit: %w", err)
		}
		otp[i] = digits[n.Int64()]
	}
	return string(otp[:]), nil
}

// rateLimitedOTPSend is the shared logic for forgot-password and resend-otp.
// isResend distinguishes which email template to use.
// It:
//  1. Rate-limits by email (3 per 15 min).
//  2. Checks the parent exists (silently succeeds if not, to prevent enumeration).
//  3. Invalidates existing OTPs, generates a new one, stores it, and emails it.
func (server *Server) rateLimitedOTPSend(ctx *gin.Context, email string, isResend bool) {
	// ── Rate limiting ─────────────────────────────────────────────────────────
	if !server.otpSendLimiter.Allow(email) {
		remaining := server.otpSendLimiter.RemainingSeconds(email)
		ctx.JSON(http.StatusTooManyRequests, gin.H{
			"error":            "Too many OTP requests. Please wait before trying again.",
			"retry_after_secs": remaining,
		})
		return
	}

	// ── Look up parent (don't reveal whether email exists) ───────────────────
	_, err := server.store.GetParentByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, db.ErrRecordNotFound) {
			// Return the same success-looking response to prevent email enumeration
			log.Info().Str("email", email).Msg("forgot-password: email not found, responding generically")
			ctx.JSON(http.StatusOK, gin.H{"message": "If an account with this email exists, a verification code has been sent."})
			return
		}
		log.Error().Err(err).Str("email", email).Msg("forgot-password: DB error looking up parent")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// ── Generate & hash OTP ───────────────────────────────────────────────────
	otp, err := generateOTP()
	if err != nil {
		log.Error().Err(err).Msg("forgot-password: failed to generate OTP")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	otpHash, err := util.HashPassword(otp)
	if err != nil {
		log.Error().Err(err).Msg("forgot-password: failed to hash OTP")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// ── Invalidate any existing pending OTPs for this email ───────────────────
	if err = server.store.InvalidateOTPsByEmail(ctx, email); err != nil {
		log.Warn().Err(err).Str("email", email).Msg("forgot-password: failed to invalidate old OTPs (non-fatal)")
	}

	// ── Persist new OTP ───────────────────────────────────────────────────────
	expiresAt := time.Now().Add(otpExpiryDuration)
	if _, err = server.store.CreatePasswordResetOTP(ctx, db.CreatePasswordResetOTPParams{
		Email:     email,
		OtpHash:   otpHash,
		ExpiresAt: expiresAt,
	}); err != nil {
		log.Error().Err(err).Str("email", email).Msg("forgot-password: failed to store OTP")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// ── Send email (async so we don't block the HTTP response) ────────────────
	go func() {
		if server.emailService == nil {
			log.Warn().Str("email", email).Msg("forgot-password: email service not configured, OTP not sent")
			return
		}
		var sendErr error
		if isResend {
			sendErr = server.emailService.SendResendOTPEmail(email, otp)
		} else {
			sendErr = server.emailService.SendForgotPasswordEmail(email, otp)
		}
		if sendErr != nil {
			log.Error().Err(sendErr).Str("email", email).Msg("forgot-password: failed to send OTP email")
		} else {
			log.Info().Str("email", email).Bool("resend", isResend).Msg("forgot-password: OTP email dispatched")
		}
	}()

	ctx.JSON(http.StatusOK, gin.H{
		"message": "If an account with this email exists, a verification code has been sent.",
	})
}

// ─── POST /v1/auth/parent/forgot-password ────────────────────────────────────
//
// Request:  { "email": "user@example.com" }
// Success:  200 { "message": "..." }
// Errors:   400 (invalid email), 429 (rate limited)

func (server *Server) forgotPassword(ctx *gin.Context) {
	var req forgotPasswordRequest
	if !bindAndValidate(ctx, &req) {
		return
	}
	server.rateLimitedOTPSend(ctx, strings.ToLower(strings.TrimSpace(req.Email)), false)
}

// ─── POST /v1/auth/parent/resend-otp ─────────────────────────────────────────
//
// Request:  { "email": "user@example.com" }
// Success:  200 { "message": "..." }
// Errors:   400 (invalid email), 429 (rate limited)

func (server *Server) resendOTP(ctx *gin.Context) {
	var req resendOTPRequest
	if !bindAndValidate(ctx, &req) {
		return
	}
	server.rateLimitedOTPSend(ctx, strings.ToLower(strings.TrimSpace(req.Email)), true)
}

// ─── POST /v1/auth/parent/verify-otp ─────────────────────────────────────────
//
// Request:  { "email": "user@example.com", "otp": "123456" }
// Success:  200 { "reset_token": "uuid", "expires_at": "...", "message": "..." }
// Errors:   400 (bad input), 401 (wrong OTP), 404 (no pending OTP), 429 (rate limited)

func (server *Server) verifyOTP(ctx *gin.Context) {
	var req verifyOTPRequest
	if !bindAndValidate(ctx, &req) {
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))

	// ── Rate limiting (OTP verification attempts) ─────────────────────────────
	if !server.otpVerifyLimiter.Allow(email) {
		remaining := server.otpVerifyLimiter.RemainingSeconds(email)
		ctx.JSON(http.StatusTooManyRequests, gin.H{
			"error":            "Too many verification attempts. Please wait before trying again.",
			"retry_after_secs": remaining,
		})
		return
	}

	// ── Fetch latest valid (unexpired, unverified, unused) OTP ───────────────
	record, err := server.store.GetLatestValidOTPByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, db.ErrRecordNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "No pending verification code found. Please request a new one."})
			return
		}
		log.Error().Err(err).Str("email", email).Msg("verify-otp: DB error fetching OTP")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// ── Verify OTP hash ───────────────────────────────────────────────────────
	if err = util.CheckPassword(req.OTP, record.OtpHash); err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid verification code."})
		return
	}

	// ── Mark OTP verified & generate reset token ──────────────────────────────
	verified, err := server.store.MarkOTPVerified(ctx, record.ID)
	if err != nil {
		log.Error().Err(err).Str("email", email).Msg("verify-otp: failed to mark OTP verified")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// reset_token is a UUID generated by the DB when verified_at is set
	resetToken := uuid.UUID(verified.ResetToken.Bytes)

	ctx.JSON(http.StatusOK, verifyOTPResponse{
		ResetToken: resetToken.String(),
		ExpiresAt:  verified.ExpiresAt,
		Message:    "OTP verified. Use the reset_token to set a new password.",
	})
}

// ─── POST /v1/auth/parent/reset-password ─────────────────────────────────────
//
// Request:  { "reset_token": "uuid", "password": "...", "confirm_password": "..." }
// Success:  200 { "message": "Password reset successfully. Please log in." }
// Errors:   400 (bad input / passwords don't match), 401 (invalid/expired token)

func (server *Server) resetPassword(ctx *gin.Context) {
	var req resetPasswordRequest
	if !bindAndValidate(ctx, &req) {
		return
	}

	// ── Validate passwords match ──────────────────────────────────────────────
	if req.Password != req.ConfirmPassword {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Password and confirm password do not match."})
		return
	}

	// ── Parse reset token ─────────────────────────────────────────────────────
	tokenUUID, err := uuid.Parse(strings.TrimSpace(req.ResetToken))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reset token format."})
		return
	}

	// ── Fetch the OTP record by reset token ───────────────────────────────────
	record, err := server.store.GetOTPByResetToken(ctx, tokenUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, db.ErrRecordNotFound) {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Reset token is invalid or has expired. Please request a new code."})
			return
		}
		log.Error().Err(err).Msg("reset-password: DB error fetching OTP by reset token")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// ── Hash new password ─────────────────────────────────────────────────────
	hashedPassword, err := util.HashPassword(req.Password)
	if err != nil {
		log.Error().Err(err).Msg("reset-password: failed to hash new password")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// ── Look up parent by email ───────────────────────────────────────────────
	parent, err := server.store.GetParentByEmail(ctx, record.Email)
	if err != nil {
		log.Error().Err(err).Str("email", record.Email).Msg("reset-password: parent not found by email")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// ── Update password in DB ─────────────────────────────────────────────────
	if err = server.store.UpdateParentPassword(ctx, hashedPassword, parent.ParentID); err != nil {
		log.Error().Err(err).Str("parent_id", parent.ParentID).Msg("reset-password: failed to update password")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// ── Consume the reset token so it can't be reused ────────────────────────
	if err = server.store.MarkOTPUsed(ctx, record.ID); err != nil {
		// Non-fatal: password is already updated. Log and continue.
		log.Warn().Err(err).Msg("reset-password: failed to mark OTP used (non-fatal)")
	}

	// ── Send password-changed confirmation email (async) ─────────────────────
	changedAt := time.Now().UTC().Format("2 Jan 2006, 15:04 UTC")
	go func() {
		if server.emailService != nil {
			if err := server.emailService.SendPasswordResetEmail(record.Email, changedAt); err != nil {
				log.Error().Err(err).Str("email", record.Email).Msg("reset-password: failed to send confirmation email")
			}
		}
	}()

	log.Info().Str("parent_id", parent.ParentID).Msg("reset-password: password updated successfully")
	ctx.JSON(http.StatusOK, gin.H{"message": "Password reset successfully. Please log in with your new password."})
}
