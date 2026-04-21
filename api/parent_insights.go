package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/gpt"
	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// parentInsightResponse is the API response shape — insight_content is decoded
// JSON instead of the raw []byte stored in the DB.
type parentInsightResponse struct {
	ID          uuid.UUID                  `json:"id"`
	ParentID    string                     `json:"parent_id"`
	ChildID     string                     `json:"child_id"`
	Date        string                     `json:"date"`
	Insight     *gpt.ParentInsightResponse `json:"insight"`
	OverallTone string                     `json:"overall_tone"`
	IsRead      bool                       `json:"is_read"`
	GeneratedAt time.Time                  `json:"generated_at"`
	CreatedAt   time.Time                  `json:"created_at"`
}

func toParentInsightResponse(row db.ParentDailyInsight) parentInsightResponse {
	var insight gpt.ParentInsightResponse
	_ = json.Unmarshal(row.InsightContent, &insight)

	dateStr := ""
	if t, err := row.Date.Value(); err == nil && t != nil {
		if d, ok := t.(time.Time); ok {
			dateStr = d.Format("2006-01-02")
		}
	}

	return parentInsightResponse{
		ID:          row.ID,
		ParentID:    row.ParentID,
		ChildID:     row.ChildID,
		Date:        dateStr,
		Insight:     &insight,
		OverallTone: row.OverallTone,
		IsRead:      row.IsRead,
		GeneratedAt: row.GeneratedAt,
		CreatedAt:   row.CreatedAt,
	}
}

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

	ctx.JSON(http.StatusOK, toParentInsightResponse(*insight))
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

	resp := make([]parentInsightResponse, len(history))
	for i, row := range history {
		resp[i] = toParentInsightResponse(row)
	}
	ctx.JSON(http.StatusOK, resp)
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

	ctx.JSON(http.StatusOK, toParentInsightResponse(*updated))
}
