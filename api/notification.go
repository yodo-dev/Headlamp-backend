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

	// Check if the device already exists
	_, err = server.store.GetDeviceByDeviceID(ctx, req.DeviceID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	if errors.Is(err, sql.ErrNoRows) {
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
		if err == sql.ErrNoRows {
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
