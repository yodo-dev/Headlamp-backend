package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

type verifyCodeRequest struct {
	Code     string `json:"code" binding:"required"`
	DeviceID string `json:"device_id" binding:"required"`
}

type verifyCodeResponse struct {
	PublicKey   []byte `json:"public_key"`
	AccessToken string `json:"access_token"`
}

// verifyLinkCodeTx manages the transaction for verifying a link code and associating a device.
func (server *Server) verifyLinkCodeTx(ctx context.Context, req verifyCodeRequest) (verifyCodeResponse, error) {
	var response verifyCodeResponse

	err := server.store.ExecTx(ctx, func(q *db.Queries) error {
		// 1. Get the deep link code
		linkCode, err := q.GetDeepLinkCode(ctx, req.Code)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				log.Warn().Err(err).Str("code", req.Code).Msg("link code not found")
				return errors.New("invalid or expired code")
			}
			log.Error().Err(err).Str("code", req.Code).Msg("failed to get deep link code")
			return err
		}

		// 2. Validate the code
		if linkCode.ExpiresAt.Before(time.Now()) {
			log.Warn().Str("code", req.Code).Time("expires_at", linkCode.ExpiresAt).Msg("link code has expired")
			return errors.New("invalid or expired code")
		}

		if linkCode.IsUsed {
			log.Warn().Str("code", req.Code).Msg("link code has already been used")
			return errors.New("invalid or expired code")
		}

		// 3. Get the family to retrieve the public key
		family, err := q.GetFamily(ctx, linkCode.FamilyID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				log.Error().Err(err).Str("family_id", linkCode.FamilyID).Msg("family not found for link code")
				return errors.New("invalid or expired code")
			}
			log.Error().Err(err).Str("family_id", linkCode.FamilyID).Msg("failed to get family")
			return err
		}

		// 4. Deactivate any currently active devices for this child.
		childUUID, err := uuid.Parse(linkCode.ChildID)
		if err != nil {
			return err
		}
		if err := q.DeactivateUserDevices(ctx, childUUID); err != nil {
			log.Error().Err(err).Str("child_id", linkCode.ChildID).Msg("failed to deactivate user devices")
			return err
		}

		// 5. Check if the device is already registered.
		device, err := q.GetDeviceByDeviceID(ctx, req.DeviceID)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				log.Error().Err(err).Str("device_id", req.DeviceID).Msg("failed to get device by device id")
				return err
			}

			// Device does not exist, create it.
			log.Info().Str("device_id", req.DeviceID).Str("child_id", linkCode.ChildID).Msg("creating new device")
			_, err = q.CreateDevice(ctx, db.CreateDeviceParams{
				UserID:   childUUID,
				UserType: "child",
				DeviceID: req.DeviceID,
			})
			if err != nil {
				log.Error().Err(err).Msg("failed to create new device")
				return err
			}
		} else {
			// Device exists, check if it belongs to the correct child.
			if device.UserID != childUUID {
				log.Warn().Str("device_id", req.DeviceID).Str("current_child", device.UserID.String()).Str("new_child", linkCode.ChildID).Msg("device is linked to a different child")
				return errors.New("this device is already linked to another account")
			}
			log.Info().Str("device_id", req.DeviceID).Str("child_id", linkCode.ChildID).Msg("re-linking existing device.")
		}

		// 6. Activate the device (either new or existing).
		_, err = q.ActivateDevice(ctx, db.ActivateDeviceParams{
			DeviceID: req.DeviceID,
			UserID:   childUUID,
		})
		if err != nil {
			log.Error().Err(err).Msg("failed to activate device")
			return err
		}

		// 7. Mark the link code as used
		_, err = q.UseDeepLinkCode(ctx, req.Code)
		if err != nil {
			log.Error().Err(err).Str("code", req.Code).Msg("failed to mark link code as used")
			return err
		}

		// Get child and parent for notification
		child, err := q.GetChild(ctx, linkCode.ChildID)
		if err != nil {
			return err // Fail the transaction if we can't get the child
		}
		parent, err := q.GetParentByFamilyID(ctx, child.FamilyID)
		if err != nil {
			return err // Fail the transaction if we can't get the parent
		}

		// Send notification in a goroutine so it doesn't block the transaction
		go server.sendParentNotification(
			child,
			parent,
			"Account Linked!",
			fmt.Sprintf("%s has successfully linked their account!", child.FirstName),
		)

		// Create Paseto token with child claims
		customToken, _, err := server.tokenMaker.CreateToken(
			linkCode.ChildID,
			linkCode.FamilyID,
			req.DeviceID,
			"child",
			time.Duration(BaseUserAccessTokenDurationInDays)*24*time.Hour,
		)

		if err != nil {
			log.Error().Err(err).Msg("failed to create access token for child")
			return err
		}

		response.PublicKey = family.PublicKey
		response.AccessToken = customToken
		return nil
	})

	return response, err
}

func (server *Server) getChild(ctx *gin.Context) {
	authPayload := ctx.MustGet(authorizationPayloadKey).(db.Child)
	ctx.JSON(http.StatusOK, authPayload)
}

type updateChildProfileRequest struct {
	FirstName                string `form:"first_name"`
	Surname                  string `form:"surname"`
	Age                      int32  `form:"age"`
	Gender                   string `form:"gender"`
	PushNotificationsEnabled *bool  `form:"push_notifications_enabled"`
}

// updateChildProfile godoc
// @Summary Update a child's profile
// @Description Allows an authenticated child to update their own profile information.
// @Tags children
// @Accept  json
// @Produce  json
// @Param   profile  body    updateChildProfileRequest  true  "Profile information"
// @Success 200 {object} db.Child
// @Failure 400 {object} gin.H "Invalid request"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /v1/child/profile [patch]
func (server *Server) updateChildProfile(ctx *gin.Context) {
	var req updateChildProfileRequest
	if err := ctx.ShouldBind(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	authPayload := ctx.MustGet(authorizationPayloadKey).(db.Child)

	arg := db.UpdateChildParams{
		ID:        authPayload.ID,
		FirstName: pgtype.Text{String: req.FirstName, Valid: req.FirstName != ""},
		Surname:   pgtype.Text{String: req.Surname, Valid: req.Surname != ""},
		Age:       pgtype.Int4{Int32: req.Age, Valid: req.Age > 0},
		Gender:    pgtype.Text{String: req.Gender, Valid: req.Gender != ""},
	}

	// Handle the profile image upload
	file, header, err := ctx.Request.FormFile("profile_image")
	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		ctx.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid file upload: %w", err)))
		return
	}

	if file != nil {
		defer file.Close()

		// Upload to the external content provider
		uploadURL, err := server.uploader.UploadFile(header.Filename, file, "app/child_profile_images")
		if err != nil {
			log.Error().Err(err).Msg("failed to upload profile image")
			ctx.JSON(http.StatusInternalServerError, errorResponse(errors.New("failed to process profile image")))
			return
		}
		arg.ProfileImageUrl = pgtype.Text{String: uploadURL, Valid: true}
	}

	if req.PushNotificationsEnabled != nil {
		arg.PushNotificationsEnabled = pgtype.Bool{Bool: *req.PushNotificationsEnabled, Valid: true}
	}

	child, err := server.store.UpdateChild(ctx, arg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, child)
}

func (server *Server) logoutChild(ctx *gin.Context) {
	// The deviceAuthMiddleware has already verified the child and device.
	// We get the child's data from the context.
	authPayload := ctx.MustGet(authorizationPayloadKey).(db.Child)

	// Deactivate all devices for the child in our database.
	childUUID, err := uuid.Parse(authPayload.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	err = server.store.DeactivateDevicesForUser(ctx, childUUID)
	if err != nil {
		log.Error().Err(err).Str("child_id", authPayload.ID).Msg("failed to deactivate devices for child")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Note: Paseto tokens are stateless, so we don't need to revoke them on the server.
	// Device deactivation ensures the child cannot make authenticated requests.

	log.Info().Str("child_id", authPayload.ID).Msg("child logged out and devices deactivated successfully")
	ctx.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func (server *Server) verifyLinkCode(ctx *gin.Context) {
	var req verifyCodeRequest
	if !bindAndValidate(ctx, &req) {
		return
	}

	log.Info().Str("code", req.Code).Str("device_id", req.DeviceID).Msg("verifying link code and device")

	rsp, err := server.verifyLinkCodeTx(ctx, req)
	if err != nil {
		if err.Error() == "invalid or expired code" || err.Error() == "this device is already linked to another account" {
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	log.Info().Str("code", req.Code).Msg("link code verified and device linked successfully")
	ctx.JSON(http.StatusOK, rsp)
}
