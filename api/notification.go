package api

import (
	"database/sql"
	"errors"
	"net/http"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

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

	// Check if the device already exists
	_, err = server.store.GetDeviceByDeviceID(ctx, req.DeviceID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	if errors.Is(err, sql.ErrNoRows) {
		// Create a new device
		_, err = server.store.CreateDevice(ctx, db.CreateDeviceParams{
			UserID:    userID,
			UserType:  authPayload.Role,
			DeviceID:  req.DeviceID,
			PushToken: pgtype.Text{String: req.PushToken, Valid: true},
			Provider:  pgtype.Text{String: req.Provider, Valid: true},
		})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
	} else {
		// Update existing device's push token
		_, err = server.store.UpdateDevicePushToken(ctx, db.UpdateDevicePushTokenParams{
			UserID:    userID,
			DeviceID:  req.DeviceID,
			PushToken: pgtype.Text{String: req.PushToken, Valid: true},
			Provider:  pgtype.Text{String: req.Provider, Valid: true},
		})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// getNotifications fetches all notifications for the authenticated user.
func (server *Server) getNotifications(ctx *gin.Context) {
	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	recipientID, err := uuid.Parse(authPayload.UserID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	notifications, err := server.store.GetNotificationsForRecipient(ctx, db.GetNotificationsForRecipientParams{
		RecipientID:   recipientID,
		RecipientType: db.NotificationRecipientType(authPayload.Role),
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, notifications)
}

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

	_, err = server.store.MarkNotificationAsRead(ctx, db.MarkNotificationAsReadParams{
		ID:          notificationID,
		RecipientID: recipientID,
	})

	if err != nil {
		if err == sql.ErrNoRows {
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"status": "marked as read"})
}
