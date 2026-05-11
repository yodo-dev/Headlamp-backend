package api

import (
	"net/http"
	"time"

	"github.com/The-You-School-HeadLamp/headlamp_backend/service"
	"github.com/gin-gonic/gin"
)

type analyticsIdentifyRequest struct {
	UserID     string `json:"user_id" binding:"required"`
	Role       string `json:"role" binding:"required,oneof=parent child"`
	Email      string `json:"email"`
	Plan       string `json:"plan"`
	AppVersion string `json:"app_version"`
	Platform   string `json:"platform" binding:"omitempty,oneof=ios android"`
	DeviceID   string `json:"device_id"`
	PushToken  string `json:"push_token"`
	Locale     string `json:"locale"`
	Timezone   string `json:"timezone"`
}

type analyticsEventAppContext struct {
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
}

type analyticsEventRequest struct {
	EventID    string                   `json:"event_id"`
	EventName  string                   `json:"event_name" binding:"required"`
	EventTime  time.Time                `json:"event_time"`
	UserID     string                   `json:"user_id" binding:"required"`
	Role       string                   `json:"role" binding:"omitempty,oneof=parent child"`
	SessionID  string                   `json:"session_id"`
	ChildID    string                   `json:"child_id"`
	Properties map[string]any           `json:"properties"`
	AppContext analyticsEventAppContext `json:"app_context"`
}

type analyticsBatchEventsRequest struct {
	Events []analyticsEventRequest `json:"events"`
}

type analyticsSessionStartRequest struct {
	SessionID    string    `json:"session_id" binding:"required"`
	UserID       string    `json:"user_id" binding:"required"`
	Role         string    `json:"role" binding:"omitempty,oneof=parent child"`
	ChildID      string    `json:"child_id"`
	StartedAt    time.Time `json:"started_at"`
	SourceScreen string    `json:"source_screen"`
	AppState     string    `json:"app_state"`
}

type analyticsSessionEndRequest struct {
	SessionID       string    `json:"session_id" binding:"required"`
	UserID          string    `json:"user_id" binding:"required"`
	Role            string    `json:"role" binding:"omitempty,oneof=parent child"`
	ChildID         string    `json:"child_id"`
	EndedAt         time.Time `json:"ended_at"`
	DurationSeconds int       `json:"duration_seconds"`
	Reason          string    `json:"reason"`
}

func (server *Server) analyticsIdentify(ctx *gin.Context) {
	if server.analyticsService == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "analytics service unavailable"})
		return
	}

	var req analyticsIdentifyRequest
	if !bindAndValidate(ctx, &req) {
		return
	}

	if err := server.analyticsService.QueueIdentify(ctx.Request.Context(), service.IdentifyInput{
		UserID:     req.UserID,
		Role:       req.Role,
		Email:      req.Email,
		Plan:       req.Plan,
		AppVersion: req.AppVersion,
		Platform:   req.Platform,
		DeviceID:   req.DeviceID,
		PushToken:  req.PushToken,
		Locale:     req.Locale,
		Timezone:   req.Timezone,
	}); err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

func (server *Server) analyticsEvent(ctx *gin.Context) {
	if server.analyticsService == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "analytics service unavailable"})
		return
	}

	var req analyticsEventRequest
	if !bindAndValidate(ctx, &req) {
		return
	}

	if err := server.analyticsService.QueueEvent(ctx.Request.Context(), toEventInput(req)); err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

func (server *Server) analyticsBatchEvents(ctx *gin.Context) {
	if server.analyticsService == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "analytics service unavailable"})
		return
	}

	var req analyticsBatchEventsRequest
	if !bindAndValidate(ctx, &req) {
		return
	}
	if len(req.Events) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "events must not be empty"})
		return
	}

	for _, event := range req.Events {
		if err := server.analyticsService.QueueEvent(ctx.Request.Context(), toEventInput(event)); err != nil {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
	}

	ctx.JSON(http.StatusAccepted, gin.H{"status": "queued", "count": len(req.Events)})
}

func (server *Server) analyticsSessionStart(ctx *gin.Context) {
	if server.analyticsService == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "analytics service unavailable"})
		return
	}

	var req analyticsSessionStartRequest
	if !bindAndValidate(ctx, &req) {
		return
	}

	if err := server.analyticsService.QueueSessionStart(ctx.Request.Context(), service.SessionStartInput{
		SessionID:    req.SessionID,
		UserID:       req.UserID,
		Role:         req.Role,
		ChildID:      req.ChildID,
		StartedAt:    req.StartedAt,
		SourceScreen: req.SourceScreen,
		AppState:     req.AppState,
	}); err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

func (server *Server) analyticsSessionEnd(ctx *gin.Context) {
	if server.analyticsService == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "analytics service unavailable"})
		return
	}

	var req analyticsSessionEndRequest
	if !bindAndValidate(ctx, &req) {
		return
	}

	if err := server.analyticsService.QueueSessionEnd(ctx.Request.Context(), service.SessionEndInput{
		SessionID:       req.SessionID,
		UserID:          req.UserID,
		Role:            req.Role,
		ChildID:         req.ChildID,
		EndedAt:         req.EndedAt,
		DurationSeconds: req.DurationSeconds,
		Reason:          req.Reason,
	}); err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

func toEventInput(req analyticsEventRequest) service.EventInput {
	appContext := map[string]any{}
	if req.AppContext.Platform != "" {
		appContext["platform"] = req.AppContext.Platform
	}
	if req.AppContext.AppVersion != "" {
		appContext["app_version"] = req.AppContext.AppVersion
	}

	return service.EventInput{
		EventID:    req.EventID,
		EventName:  req.EventName,
		EventTime:  req.EventTime,
		UserID:     req.UserID,
		Role:       req.Role,
		SessionID:  req.SessionID,
		ChildID:    req.ChildID,
		Properties: req.Properties,
		AppContext: appContext,
	}
}
