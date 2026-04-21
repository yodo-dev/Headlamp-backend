package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
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
		if errors.Is(err, sql.ErrNoRows) {
			log.Warn().Err(err).Str("email", req.Email).Msg("parent not found")
			ctx.JSON(http.StatusNotFound, gin.H{"error": "parent not found"})
			return
		}
		log.Error().Err(err).Str("email", req.Email).Msg("failed to get parent by email")
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

// upsertDeviceIfProvided registers or updates a device's push token.
// It is a best-effort operation: errors are logged but do not affect the auth response.
func (server *Server) upsertDeviceIfProvided(ctx *gin.Context, userIDStr, userType, deviceID, pushToken, provider string) {
	if deviceID == "" || pushToken == "" || provider == "" {
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Error().Err(err).Str("user_id", userIDStr).Msg("upsertDevice: invalid user id")
		return
	}

	_, err = server.store.GetDeviceByDeviceID(ctx, deviceID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Error().Err(err).Str("device_id", deviceID).Msg("upsertDevice: failed to look up device")
		return
	}

	if errors.Is(err, sql.ErrNoRows) {
		_, err = server.store.CreateDevice(ctx, db.CreateDeviceParams{
			UserID:    userID,
			UserType:  userType,
			DeviceID:  deviceID,
			PushToken: pgtype.Text{String: pushToken, Valid: true},
			Provider:  pgtype.Text{String: provider, Valid: true},
		})
		if err != nil {
			log.Error().Err(err).Str("device_id", deviceID).Msg("upsertDevice: failed to create device")
		}
	} else {
		_, err = server.store.UpdateDevicePushToken(ctx, db.UpdateDevicePushTokenParams{
			UserID:    userID,
			DeviceID:  deviceID,
			PushToken: pgtype.Text{String: pushToken, Valid: true},
			Provider:  pgtype.Text{String: provider, Valid: true},
		})
		if err != nil {
			log.Error().Err(err).Str("device_id", deviceID).Msg("upsertDevice: failed to update device push token")
		}
	}
}
