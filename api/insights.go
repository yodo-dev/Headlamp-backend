package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// ─── Shared helpers ───────────────────────────────────────────────────────────

// insightsChildURI is the common URI binding for child-scoped insight endpoints.
type insightsChildURI struct {
	ChildID string `uri:"id" binding:"required"`
}

// parseRangeDays converts a ?range query param (e.g. "7d", "30d", "1d") into
// an integer number of days.  Defaults to 7 when absent or unrecognised.
func parseRangeDays(ctx *gin.Context) int {
	raw := strings.TrimSpace(ctx.DefaultQuery("range", "7d"))
	raw = strings.ToLower(strings.TrimSuffix(raw, "d"))
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 7
	}
	switch n {
	case 1, 7, 30:
		return n
	default:
		return 7
	}
}

// resolveInsightsChild verifies that the authenticated parent owns the requested
// child and returns the child ID.  On error it writes the appropriate HTTP
// response and returns ("", false).
func (server *Server) resolveInsightsChild(ctx *gin.Context) (string, bool) {
	authPayload, exists := ctx.Get(authorizationPayloadKey)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("authorization payload not found")))
		return "", false
	}
	payload, ok := authPayload.(*token.Payload)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, errorResponse(errors.New("invalid payload type")))
		return "", false
	}

	parent, err := server.store.GetParentByParentID(ctx, payload.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("parent not found")))
			return "", false
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return "", false
	}

	var uri insightsChildURI
	if err := ctx.ShouldBindUri(&uri); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return "", false
	}

	_, err = server.store.GetChildByIDAndFamilyID(ctx, db.GetChildByIDAndFamilyIDParams{
		ID:       uri.ChildID,
		FamilyID: parent.FamilyID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("child not found or does not belong to this family")))
			return "", false
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return "", false
	}

	return uri.ChildID, true
}

// ─── Dashboard ────────────────────────────────────────────────────────────────

// getDashboardInsights handles GET /v1/parent/child/:id/insights/dashboard
func (server *Server) getDashboardInsights(ctx *gin.Context) {
	childID, ok := server.resolveInsightsChild(ctx)
	if !ok {
		return
	}
	rangeDays := parseRangeDays(ctx)
	resp, err := server.insightsService.GetDashboardInsights(ctx.Request.Context(), childID, rangeDays)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

// ─── Engagement overview ──────────────────────────────────────────────────────

// getEngagementOverview handles GET /v1/parent/child/:id/insights/engagement
func (server *Server) getEngagementOverview(ctx *gin.Context) {
	childID, ok := server.resolveInsightsChild(ctx)
	if !ok {
		return
	}
	rangeDays := parseRangeDays(ctx)
	resp, err := server.insightsService.GetEngagementOverview(ctx.Request.Context(), childID, rangeDays)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

// ─── Content monitoring ───────────────────────────────────────────────────────

// getContentMonitoringSummary handles GET /v1/parent/child/:id/insights/content-monitoring
func (server *Server) getContentMonitoringSummary(ctx *gin.Context) {
	childID, ok := server.resolveInsightsChild(ctx)
	if !ok {
		return
	}
	rangeDays := parseRangeDays(ctx)
	resp, err := server.insightsService.GetContentMonitoringSummary(ctx.Request.Context(), childID, rangeDays)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

// contentMonitoringEventRequest is the POST body for ingesting a content event.
type contentMonitoringEventRequest struct {
	Platform string                 `json:"platform"  binding:"required"`
	Category string                 `json:"category"  binding:"required"`
	Severity string                 `json:"severity"  binding:"required,oneof=low medium high"`
	Metadata map[string]interface{} `json:"metadata"`
}

// postContentMonitoringEvent handles POST /v1/parent/child/:id/insights/content-monitoring/event
func (server *Server) postContentMonitoringEvent(ctx *gin.Context) {
	childID, ok := server.resolveInsightsChild(ctx)
	if !ok {
		return
	}

	var req contentMonitoringEventRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	var metaBytes []byte
	if req.Metadata != nil {
		metaJSON, err := json.Marshal(req.Metadata)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, errorResponse(errors.New("invalid metadata")))
			return
		}
		metaBytes = metaJSON
	}

	event, err := server.insightsService.IngestContentMonitoringEvent(ctx.Request.Context(), db.CreateContentMonitoringEventParams{
		ChildID:  childID,
		Platform: req.Platform,
		Category: req.Category,
		Severity: req.Severity,
		Metadata: metaBytes,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	ctx.JSON(http.StatusCreated, event)
}
