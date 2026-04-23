package api

import (
	"errors"
	"net/http"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

const (
	BaseUserAccessTokenDurationInDays  = 1
	BaseUserRefreshTokenDurationInDays = 7
	ParentEmailAlreadyExistsError      = "an account with this email already exists"
	ParentNotFoundError                = "parent not found"
	ParentUserProfile                  = "parent"
)

type parentResponse struct {
	ParentID  string    `json:"parent_id"`
	Firstname string    `json:"firstname"`
	Surname   string    `json:"surname"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type familyResponse struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

func newParentResponse(parent db.Parent) parentResponse {
	return parentResponse{
		ParentID:  parent.ParentID,
		Firstname: parent.Firstname,
		Surname:   parent.Surname,
		Email:     parent.Email,
		CreatedAt: parent.CreatedAt,
	}
}

type signUpParentRequest struct {
	Firstname string `json:"firstname" binding:"required"`
	Surname   string `json:"surname" binding:"required"`
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required"`
	// Optional — registers the device for push notifications in the same request
	DeviceID  string `json:"device_id"`
	PushToken string `json:"push_token"`
	Provider  string `json:"provider"`
}

type signUpParentResponse struct {
	AccessToken          string         `json:"access_token"`
	AccessTokenExpiresAt time.Time      `json:"access_token_expires_at"`
	Parent               parentResponse `json:"parent"`
	Family               familyResponse `json:"family"`
}

// Parent Endpoints
func (server *Server) signUpParent(ctx *gin.Context) {
	var req signUpParentRequest
	if !bindAndValidate(ctx, &req) {
		return
	}

	publicKey, privateKey, err := util.GenerateKeyPair()
	if err != nil {
		log.Error().Err(err).Msg("failed to generate key pair")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	user, err := server.store.CreateParentTx(ctx, db.CreateParentTxParams{
		Firstname: req.Firstname,
		Surname:   req.Surname,
		Email:     req.Email,
		Password:  req.Password,
	}, publicKey, privateKey)
	if err != nil {
		if db.ErrorCode(err) == db.UniqueViolation {
			log.Warn().Err(err).Str("email", req.Email).Msg("parent registration failed due to unique email violation")
			ctx.JSON(http.StatusForbidden, gin.H{"error": "parent account already exists"})
			return
		}
		log.Error().Err(err).Str("email", req.Email).Msg("failed to create parent")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	log.Info().Str("family_id", user.Family.ID).Str("parent_id", user.Parent.ParentID).Msg("parent registered successfully")

	accessToken, accessPayload, err := server.tokenMaker.CreateToken(
		user.Parent.ParentID,
		user.Family.ID, // familyID
		"",             // deviceID
		ParentUserProfile,
		time.Duration(BaseUserAccessTokenDurationInDays)*24*time.Hour,
	)
	if err != nil {
		log.Error().Err(err).Str("parent_id", user.Parent.ParentID).Msg("failed to create access token")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	server.upsertDeviceIfProvided(ctx, user.Parent.ParentID, ParentUserProfile, req.DeviceID, req.PushToken, req.Provider)

	// Send welcome email asynchronously so it doesn't block the response.
	go func() {
		if server.emailService != nil {
			if err := server.emailService.SendWelcomeEmail(user.Parent.Email, user.Parent.Firstname); err != nil {
				log.Error().Err(err).Str("parent_id", user.Parent.ParentID).Msg("signUpParent: failed to send welcome email")
			}
		}
	}()

	ctx.JSON(http.StatusOK, signUpParentResponse{
		AccessToken:          accessToken,
		AccessTokenExpiresAt: accessPayload.ExpiredAt,
		Parent:               newParentResponse(user.Parent),
		Family: familyResponse{
			ID:        user.Family.ID,
			CreatedAt: user.Family.CreatedAt,
		},
	})
}

type loginParentRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	// Optional — registers the device for push notifications in the same request
	DeviceID  string `json:"device_id"`
	PushToken string `json:"push_token"`
	Provider  string `json:"provider"`
}

type loginParentResponse struct {
	AccessToken          string         `json:"access_token"`
	AccessTokenExpiresAt time.Time      `json:"access_token_expires_at"`
	Parent               parentResponse `json:"parent"`
	Family               familyResponse `json:"family"`
}

func (server *Server) loginParent(ctx *gin.Context) {
	var req loginParentRequest
	if !bindAndValidate(ctx, &req) {
		return
	}

	log.Info().Str("email", req.Email).Msg("parent login attempt")

	parent, err := server.store.GetParentByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			log.Warn().Str("email", req.Email).Msg("loginParent: parent not found")
			ctx.JSON(http.StatusNotFound, gin.H{"error": "parent not found"})
			return
		}
		log.Error().Err(err).Str("email", req.Email).Msg("loginParent: failed to get parent by email")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	if err = util.CheckPassword(req.Password, parent.HashedPassword.String); err != nil {
		log.Warn().Str("email", req.Email).Msg("invalid password for parent")
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid password"})
		return
	}

	family, err := server.store.GetFamily(ctx, parent.FamilyID)
	if err != nil {
		log.Error().Err(err).Str("family_id", parent.FamilyID).Msg("failed to get family for parent")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	log.Info().Str("parent_id", parent.ParentID).Str("family_id", family.ID).Msg("parent logged in successfully")

	accessToken, accessPayload, err := server.tokenMaker.CreateToken(
		parent.ParentID,
		family.ID,
		"", // deviceID
		ParentUserProfile,
		time.Duration(BaseUserAccessTokenDurationInDays)*24*time.Hour,
	)
	if err != nil {
		log.Error().Err(err).Str("parent_id", parent.ParentID).Msg("failed to create access token")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	server.upsertDeviceIfProvided(ctx, parent.ParentID, ParentUserProfile, req.DeviceID, req.PushToken, req.Provider)

	ctx.JSON(http.StatusOK, loginParentResponse{
		AccessToken:          accessToken,
		AccessTokenExpiresAt: accessPayload.ExpiredAt,
		Parent:               newParentResponse(parent),
		Family: familyResponse{
			ID:        family.ID,
			CreatedAt: family.CreatedAt,
		},
	})
}

// upsertDeviceIfProvided registers or updates a device's push token, then enables
// push notifications on the parent record. It is a best-effort operation: errors
// are logged but do not affect the auth response.
func (server *Server) upsertDeviceIfProvided(ctx *gin.Context, userIDStr, userType, deviceID, pushToken, provider string) {
	if deviceID == "" || pushToken == "" || provider == "" {
		log.Info().
			Str("user_id", userIDStr).
			Str("user_type", userType).
			Msg("upsertDevice: skipping – device_id, push_token or provider not provided")
		return
	}

	log.Info().
		Str("user_id", userIDStr).
		Str("user_type", userType).
		Str("device_id", deviceID).
		Str("provider", provider).
		Msg("upsertDevice: registering device and enabling push notifications")

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Error().Err(err).Str("user_id", userIDStr).Msg("upsertDevice: invalid user id")
		return
	}

	existingDevice, err := server.store.GetDeviceByDeviceID(ctx, deviceID)
	if err != nil && !errors.Is(err, db.ErrRecordNotFound) {
		log.Error().Err(err).Str("device_id", deviceID).Msg("upsertDevice: failed to look up device")
		return
	}

	if errors.Is(err, db.ErrRecordNotFound) {
		// Device has never been seen — create it.
		log.Info().Str("device_id", deviceID).Str("user_id", userIDStr).Msg("upsertDevice: device not found, creating new record")
		_, err = server.store.CreateDevice(ctx, db.CreateDeviceParams{
			UserID:    userID,
			UserType:  userType,
			DeviceID:  deviceID,
			PushToken: pgtype.Text{String: pushToken, Valid: true},
			Provider:  pgtype.Text{String: provider, Valid: true},
		})
		if err != nil {
			log.Error().Err(err).Str("device_id", deviceID).Msg("upsertDevice: failed to create device")
			return
		}
		log.Info().Str("device_id", deviceID).Str("user_id", userIDStr).Msg("upsertDevice: device created successfully")
	} else if existingDevice.UserID != userID {
		// Device exists but belongs to a different user (e.g. reassigned/test device).
		// Delete the stale record and create a fresh one for the current user.
		log.Warn().
			Str("device_id", deviceID).
			Str("previous_user_id", existingDevice.UserID.String()).
			Str("new_user_id", userIDStr).
			Msg("upsertDevice: device belongs to a different user, reassigning")
		if delErr := server.store.DeleteDeviceByID(ctx, deviceID); delErr != nil {
			log.Error().Err(delErr).Str("device_id", deviceID).Msg("upsertDevice: failed to delete stale device record")
			return
		}
		_, err = server.store.CreateDevice(ctx, db.CreateDeviceParams{
			UserID:    userID,
			UserType:  userType,
			DeviceID:  deviceID,
			PushToken: pgtype.Text{String: pushToken, Valid: true},
			Provider:  pgtype.Text{String: provider, Valid: true},
		})
		if err != nil {
			log.Error().Err(err).Str("device_id", deviceID).Msg("upsertDevice: failed to create device after reassignment")
			return
		}
		log.Info().Str("device_id", deviceID).Str("user_id", userIDStr).Msg("upsertDevice: device reassigned and created successfully")
	} else {
		// Device exists and belongs to this user — just update the push token.
		log.Info().Str("device_id", deviceID).Str("user_id", userIDStr).Msg("upsertDevice: device exists, updating push token")
		_, err = server.store.UpdateDevicePushToken(ctx, db.UpdateDevicePushTokenParams{
			UserID:    userID,
			DeviceID:  deviceID,
			PushToken: pgtype.Text{String: pushToken, Valid: true},
			Provider:  pgtype.Text{String: provider, Valid: true},
		})
		if err != nil {
			log.Error().Err(err).Str("device_id", deviceID).Msg("upsertDevice: failed to update device push token")
			return
		}
		log.Info().Str("device_id", deviceID).Str("user_id", userIDStr).Msg("upsertDevice: device push token updated successfully")
	}

	// Device is registered — enable push notifications on the parent record so
	// the sending path knows this user has an active token.
	if userType == ParentUserProfile {
		_, updateErr := server.store.UpdateParent(ctx, db.UpdateParentParams{
			ParentID:                 userIDStr,
			PushNotificationsEnabled: pgtype.Bool{Bool: true, Valid: true},
		})
		if updateErr != nil {
			log.Error().Err(updateErr).Str("parent_id", userIDStr).Msg("upsertDevice: failed to set push_notifications_enabled=true for parent")
		} else {
			log.Info().Str("parent_id", userIDStr).Msg("upsertDevice: push_notifications_enabled set to true for parent")
		}
	}
}

// ─── POST /v1/parent/change-password ─────────────────────────────────────────
// Requires the parent to be logged in (Bearer token).
// Request:  { "current_password": "...", "password": "...", "confirm_password": "..." }
// Success:  200 { "message": "Password changed successfully." }
// Errors:   400 (validation / passwords don't match), 401 (wrong current password)

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	Password        string `json:"password"         binding:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}

func (server *Server) changePassword(ctx *gin.Context) {
	var req changePasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	if req.Password != req.ConfirmPassword {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "password and confirm_password do not match"})
		return
	}

	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)

	parent, err := server.store.GetParentByParentID(ctx, authPayload.UserID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": ParentNotFoundError})
		return
	}

	// Parents who signed up via OAuth may have no password set
	if !parent.HashedPassword.Valid || parent.HashedPassword.String == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "password change is not available for OAuth accounts"})
		return
	}

	// Verify the current password
	if err = util.CheckPassword(req.CurrentPassword, parent.HashedPassword.String); err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "current password is incorrect"})
		return
	}

	// Reject if new password is the same as the current one
	if util.CheckPassword(req.Password, parent.HashedPassword.String) == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "new password must be different from your current password"})
		return
	}

	hashedNew, err := util.HashPassword(req.Password)
	if err != nil {
		log.Error().Err(err).Str("parent_id", parent.ParentID).Msg("changePassword: hash failed")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	if err = server.store.UpdateParentPassword(ctx, hashedNew, parent.ParentID); err != nil {
		log.Error().Err(err).Str("parent_id", parent.ParentID).Msg("changePassword: DB update failed")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Send confirmation email asynchronously
	go func(email string) {
		if server.emailService == nil {
			return
		}
		changedAt := time.Now().UTC().Format("2 Jan 2006, 15:04 UTC")
		if sendErr := server.emailService.SendPasswordResetEmail(email, changedAt); sendErr != nil {
			log.Error().Err(sendErr).Str("email", email).Msg("changePassword: confirmation email failed")
		} else {
			log.Info().Str("email", email).Msg("changePassword: confirmation email sent")
		}
	}(parent.Email)

	ctx.JSON(http.StatusOK, gin.H{"message": "Password changed successfully."})
}
