package api

import (
	"errors"
	"net/http"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

// ─── Device registration ─────────────────────────────────────────────────────

type registerDeviceRequest struct {
	DeviceID  string `json:"device_id" binding:"required"`
	PushToken string `json:"push_token" binding:"required"`
	Provider  string `json:"provider" binding:"required"`
}

// registerDevice registers a device for push notifications.
func (server *Server) registerDevice(ctx *gin.Context) {
	var req registerDeviceRequest
	if !bindAndValidate(ctx, &req) {
		return
	}

	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	userID, err := uuid.Parse(authPayload.UserID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	log.Info().
		Str("user_id", authPayload.UserID).
		Str("role", authPayload.Role).
		Str("device_id", req.DeviceID).
		Str("provider", req.Provider).
		Msg("registerDevice: upserting device record")

	// Check if the device already exists.
	existingDevice, err := server.store.GetDeviceByDeviceID(ctx, req.DeviceID)
	if err != nil && !errors.Is(err, db.ErrRecordNotFound) {
		log.Error().Err(err).Str("device_id", req.DeviceID).Msg("registerDevice: failed to look up device")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	if errors.Is(err, db.ErrRecordNotFound) {
		// Device has never been seen — create it.
		log.Info().Str("device_id", req.DeviceID).Str("user_id", authPayload.UserID).Msg("registerDevice: device not found, creating new record")
		_, err = server.store.CreateDevice(ctx, db.CreateDeviceParams{
			UserID:    userID,
			UserType:  authPayload.Role,
			DeviceID:  req.DeviceID,
			PushToken: pgtype.Text{String: req.PushToken, Valid: true},
			Provider:  pgtype.Text{String: req.Provider, Valid: true},
		})
		if err != nil {
			log.Error().Err(err).Str("device_id", req.DeviceID).Msg("registerDevice: failed to create device")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		log.Info().Str("device_id", req.DeviceID).Str("user_id", authPayload.UserID).Msg("registerDevice: device created successfully")
	} else if existingDevice.UserID != userID {
		// Device exists but belongs to a different user (e.g. reassigned/test device).
		// Delete the stale record and create a fresh one for the current user.
		log.Warn().
			Str("device_id", req.DeviceID).
			Str("previous_user_id", existingDevice.UserID.String()).
			Str("new_user_id", authPayload.UserID).
			Msg("registerDevice: device belongs to a different user, reassigning")
		if delErr := server.store.DeleteDeviceByID(ctx, req.DeviceID); delErr != nil {
			log.Error().Err(delErr).Str("device_id", req.DeviceID).Msg("registerDevice: failed to delete stale device record")
			ctx.JSON(http.StatusInternalServerError, errorResponse(delErr))
			return
		}
		_, err = server.store.CreateDevice(ctx, db.CreateDeviceParams{
			UserID:    userID,
			UserType:  authPayload.Role,
			DeviceID:  req.DeviceID,
			PushToken: pgtype.Text{String: req.PushToken, Valid: true},
			Provider:  pgtype.Text{String: req.Provider, Valid: true},
		})
		if err != nil {
			log.Error().Err(err).Str("device_id", req.DeviceID).Msg("registerDevice: failed to create device after reassignment")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		log.Info().Str("device_id", req.DeviceID).Str("user_id", authPayload.UserID).Msg("registerDevice: device reassigned and created successfully")
	} else {
		// Device exists and belongs to this user — just update the push token.
		log.Info().Str("device_id", req.DeviceID).Str("user_id", authPayload.UserID).Msg("registerDevice: device exists, updating push token")
		_, err = server.store.UpdateDevicePushToken(ctx, db.UpdateDevicePushTokenParams{
			UserID:    userID,
			DeviceID:  req.DeviceID,
			PushToken: pgtype.Text{String: req.PushToken, Valid: true},
			Provider:  pgtype.Text{String: req.Provider, Valid: true},
		})
		if err != nil {
			log.Error().Err(err).Str("device_id", req.DeviceID).Msg("registerDevice: failed to update device push token")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		log.Info().Str("device_id", req.DeviceID).Str("user_id", authPayload.UserID).Msg("registerDevice: push token updated successfully")
	}

	// Enable push notifications on the user record so the sending path sees this
	// user as opted-in. Best-effort: log errors but do not fail the request.
	switch authPayload.Role {
	case "parent":
		_, updateErr := server.store.UpdateParent(ctx, db.UpdateParentParams{
			ParentID:                 authPayload.UserID,
			PushNotificationsEnabled: pgtype.Bool{Bool: true, Valid: true},
		})
		if updateErr != nil {
			log.Error().Err(updateErr).Str("parent_id", authPayload.UserID).Msg("registerDevice: failed to set push_notifications_enabled=true for parent")
		} else {
			log.Info().Str("parent_id", authPayload.UserID).Msg("registerDevice: push_notifications_enabled set to true for parent")
		}
	case "child":
		_, updateErr := server.store.UpdateChild(ctx, db.UpdateChildParams{
			ID:                       authPayload.UserID,
			PushNotificationsEnabled: pgtype.Bool{Bool: true, Valid: true},
		})
		if updateErr != nil {
			log.Error().Err(updateErr).Str("child_id", authPayload.UserID).Msg("registerDevice: failed to set push_notifications_enabled=true for child")
		} else {
			log.Info().Str("child_id", authPayload.UserID).Msg("registerDevice: push_notifications_enabled set to true for child")
		}
	default:
		log.Warn().Str("role", authPayload.Role).Msg("registerDevice: unknown role, skipping push toggle update")
	}

	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ─── List notifications (paginated) ─────────────────────────────────────────

type listNotificationsRequest struct {
	Limit  int32 `form:"limit,default=20" binding:"min=1,max=100"`
	Offset int32 `form:"offset,default=0"  binding:"min=0"`
}

type notificationsResponse struct {
	Total  int64             `json:"total"`
	Unread int64             `json:"unread"`
	Items  []db.Notification `json:"items"`
}

// getNotifications returns a paginated list of notifications for the caller,
// together with total and unread counts in the same response envelope.
func (server *Server) getNotifications(ctx *gin.Context) {
	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	recipientID, err := uuid.Parse(authPayload.UserID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	recipientType := db.NotificationRecipientType(authPayload.Role)

	var req listNotificationsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	summaryArg := db.GetNotificationSummaryParams{
		RecipientID:   recipientID,
		RecipientType: recipientType,
	}
	summary, err := server.store.GetNotificationSummary(ctx, summaryArg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	items, err := server.store.GetNotificationsForRecipientPaginated(ctx, db.GetNotificationsForRecipientPaginatedParams{
		RecipientID:   recipientID,
		RecipientType: recipientType,
		Limit:         req.Limit,
		Offset:        req.Offset,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	if items == nil {
		items = []db.Notification{}
	}

	ctx.JSON(http.StatusOK, notificationsResponse{
		Total:  summary.Total,
		Unread: summary.Unread,
		Items:  items,
	})
}

// ─── Notification summary (counts only) ──────────────────────────────────────

type notificationSummaryResponse struct {
	Total     int64 `json:"total"`
	Unread    int64 `json:"unread"`
	ReadCount int64 `json:"read_count"`
}

// getNotificationSummary returns total, unread, and read counts only.
// Lightweight — ideal for badge counts on mobile.
func (server *Server) getNotificationSummary(ctx *gin.Context) {
	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	recipientID, err := uuid.Parse(authPayload.UserID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	summary, err := server.store.GetNotificationSummary(ctx, db.GetNotificationSummaryParams{
		RecipientID:   recipientID,
		RecipientType: db.NotificationRecipientType(authPayload.Role),
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, notificationSummaryResponse{
		Total:     summary.Total,
		Unread:    summary.Unread,
		ReadCount: summary.ReadCount,
	})
}

// ─── Mark single notification as read ───────────────────────────────────────

type markNotificationReadRequest struct {
	NotificationID string `uri:"id" binding:"required,uuid"`
}

// markNotificationAsRead marks a specific notification as read.
func (server *Server) markNotificationAsRead(ctx *gin.Context) {
	var req markNotificationReadRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	notificationID, _ := uuid.Parse(req.NotificationID)
	recipientID, err := uuid.Parse(authPayload.UserID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	updated, err := server.store.MarkNotificationAsRead(ctx, db.MarkNotificationAsReadParams{
		ID:          notificationID,
		RecipientID: recipientID,
	})
	if err != nil {
		if err == db.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("notification not found")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, updated)
}

// ─── Mark all notifications as read ─────────────────────────────────────────

// markAllNotificationsAsRead marks every unread notification for the caller as read.
func (server *Server) markAllNotificationsAsRead(ctx *gin.Context) {
	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	recipientID, err := uuid.Parse(authPayload.UserID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	if err := server.store.MarkAllNotificationsAsRead(ctx, db.MarkAllNotificationsAsReadParams{
		RecipientID:   recipientID,
		RecipientType: db.NotificationRecipientType(authPayload.Role),
	}); err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"status": "all notifications marked as read"})
}
