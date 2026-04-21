package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// resolveParentInsightCtx verifies ownership and returns (parentID, childID).
// On failure it writes the error response and returns ("", "", false).
func (server *Server) resolveParentInsightCtx(ctx *gin.Context) (parentID string, childID string, ok bool) {
	childID, ok = server.resolveInsightsChild(ctx)
	if !ok {
		return "", "", false
	}

	authPayload, exists := ctx.Get(authorizationPayloadKey)
	if !exists {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("authorization payload not found")))
		return "", "", false
	}
	payload, valid := authPayload.(*token.Payload)
	if !valid {
		ctx.JSON(http.StatusInternalServerError, errorResponse(errors.New("invalid payload type")))
		return "", "", false
	}

	return payload.UserID, childID, true
}

// getParentDailyInsight handles GET /v1/parent/child/:id/insights/daily
// Returns today's insight for the child, generating it on the fly if needed.
func (server *Server) getParentDailyInsight(ctx *gin.Context) {
	parentID, childID, ok := server.resolveParentInsightCtx(ctx)
	if !ok {
		return
	}

	insight, err := server.parentInsightService.GenerateDailyInsight(ctx.Request.Context(), parentID, childID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, insight)
}

// getParentDailyInsightHistory handles GET /v1/parent/child/:id/insights/daily/history
// Query params: limit (default 20), offset (default 0)
func (server *Server) getParentDailyInsightHistory(ctx *gin.Context) {
	parentID, childID, ok := server.resolveParentInsightCtx(ctx)
	if !ok {
		return
	}

	limit := int32(20)
	offset := int32(0)

	if l, err := strconv.Atoi(ctx.DefaultQuery("limit", "20")); err == nil && l > 0 && l <= 100 {
		limit = int32(l)
	}
	if o, err := strconv.Atoi(ctx.DefaultQuery("offset", "0")); err == nil && o >= 0 {
		offset = int32(o)
	}

	history, err := server.parentInsightService.GetInsightHistory(ctx.Request.Context(), parentID, childID, limit, offset)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, history)
}

// markParentInsightRead handles POST /v1/parent/child/:id/insights/daily/:insight_id/read
func (server *Server) markParentInsightRead(ctx *gin.Context) {
	parentID, _, ok := server.resolveParentInsightCtx(ctx)
	if !ok {
		return
	}

	insightIDStr := ctx.Param("insight_id")
	insightID, err := uuid.Parse(insightIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(errors.New("invalid insight_id")))
		return
	}

	updated, err := server.parentInsightService.MarkInsightRead(ctx.Request.Context(), insightID, parentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("insight not found")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, updated)
}
