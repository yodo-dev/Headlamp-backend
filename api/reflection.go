package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// ─── Child: pending reflections ─────────────────────────────────────────────

type getPendingReflectionsRequest struct {
	Limit  int32 `form:"limit,default=10" binding:"min=1,max=50"`
	Offset int32 `form:"offset,default=0"  binding:"min=0"`
}

func (server *Server) getPendingReflections(ctx *gin.Context) {
	payload := server.getAuthPayload(ctx)
	if payload == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("unauthorized")))
		return
	}

	var req getPendingReflectionsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	reflections, err := server.reflectionService.GetPendingReflections(ctx, payload.UserID, req.Limit, req.Offset)
	if err != nil {
		log.Error().Err(err).Str("child_id", payload.UserID).Msg("failed to get pending reflections")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"reflections": reflections})
}

// ─── Child: respond to reflection ───────────────────────────────────────────

type respondToReflectionRequest struct {
	ResponseText string `json:"response_text"`
	MediaURL     string `json:"media_url"`
	ResponseType string `json:"response_type" binding:"required,oneof=text video audio"`
}

func (server *Server) respondToReflection(ctx *gin.Context) {
	payload := server.getAuthPayload(ctx)
	if payload == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("unauthorized")))
		return
	}

	reflectionIDStr := ctx.Param("id")
	reflectionID, err := uuid.Parse(reflectionIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(errors.New("invalid reflection id")))
		return
	}

	var req respondToReflectionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	if req.ResponseText == "" && req.MediaURL == "" {
		ctx.JSON(http.StatusBadRequest, errorResponse(errors.New("response_text or media_url is required")))
		return
	}

	responseType := db.ReflectionResponseType(req.ResponseType)

	var reflection *db.Reflection
	if req.MediaURL != "" {
		reflection, err = server.reflectionService.RespondToReflectionWithMedia(ctx, reflectionID, req.MediaURL, responseType)
	} else {
		reflection, err = server.reflectionService.RespondToReflection(ctx, reflectionID, req.ResponseText, responseType)
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("reflection not found")))
			return
		}
		log.Error().Err(err).Str("reflection_id", reflectionIDStr).Msg("failed to respond to reflection")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, reflection)
}

// ─── Child: acknowledge reflection ──────────────────────────────────────────

type acknowledgeReflectionRequest struct {
	Feedback *string `json:"feedback"`
}

func (server *Server) acknowledgeReflection(ctx *gin.Context) {
	payload := server.getAuthPayload(ctx)
	if payload == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("unauthorized")))
		return
	}

	reflectionIDStr := ctx.Param("id")
	reflectionID, err := uuid.Parse(reflectionIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(errors.New("invalid reflection id")))
		return
	}

	var req acknowledgeReflectionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	if err := server.reflectionService.AcknowledgeReflection(ctx, reflectionID, req.Feedback); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("reflection not found")))
			return
		}
		log.Error().Err(err).Str("reflection_id", reflectionIDStr).Msg("failed to acknowledge reflection")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "acknowledged"})
}

// ─── Child: reflection history ───────────────────────────────────────────────

type getReflectionHistoryRequest struct {
	Limit  int32 `form:"limit,default=20" binding:"min=1,max=100"`
	Offset int32 `form:"offset,default=0"  binding:"min=0"`
}

func (server *Server) getReflectionHistory(ctx *gin.Context) {
	payload := server.getAuthPayload(ctx)
	if payload == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("unauthorized")))
		return
	}

	var req getReflectionHistoryRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	reflections, err := server.reflectionService.GetReflectionHistory(ctx, payload.UserID, req.Limit, req.Offset)
	if err != nil {
		log.Error().Err(err).Str("child_id", payload.UserID).Msg("failed to get reflection history")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"reflections": reflections})
}

// ─── Child: reflection stats ─────────────────────────────────────────────────

func (server *Server) getReflectionStats(ctx *gin.Context) {
	payload := server.getAuthPayload(ctx)
	if payload == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("unauthorized")))
		return
	}

	stats, err := server.reflectionService.GetReflectionStats(ctx, payload.UserID)
	if err != nil {
		log.Error().Err(err).Str("child_id", payload.UserID).Msg("failed to get reflection stats")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, stats)
}

// ─── Parent: view child reflections ─────────────────────────────────────────

type getChildReflectionsParentRequest struct {
	Limit  int32 `form:"limit,default=20" binding:"min=1,max=100"`
	Offset int32 `form:"offset,default=0"  binding:"min=0"`
}

func (server *Server) getChildReflectionsForParent(ctx *gin.Context) {
	payload := server.getAuthPayload(ctx)
	if payload == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("unauthorized")))
		return
	}

	childID := ctx.Param("id")
	if ok, err := server.isParentOfChild(ctx, payload.UserID, childID); err != nil || !ok {
		ctx.JSON(http.StatusForbidden, errorResponse(errors.New("access denied")))
		return
	}

	var req getChildReflectionsParentRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	reflections, err := server.reflectionService.GetReflectionHistory(ctx, childID, req.Limit, req.Offset)
	if err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("parent: failed to get child reflections")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"reflections": reflections})
}

// ─── Parent: manually trigger a daily reflection for a child ─────────────────

func (server *Server) triggerReflectionForChild(ctx *gin.Context) {
	payload := server.getAuthPayload(ctx)
	if payload == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("unauthorized")))
		return
	}

	childID := ctx.Param("id")
	if ok, err := server.isParentOfChild(ctx, payload.UserID, childID); err != nil || !ok {
		ctx.JSON(http.StatusForbidden, errorResponse(errors.New("access denied")))
		return
	}

	reflection, err := server.reflectionService.GenerateDailyReflection(ctx, childID)
	if err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("parent: failed to trigger reflection")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, reflection)
}

// ─── Child: GET /v1/child/reflections/daily ──────────────────────────────────

type dailyReflectionResult struct {
	ReflectionID string    `json:"reflection_id"`
	ChildID      string    `json:"child_id"`
	Date         string    `json:"date"`
	Source       string    `json:"source"` // "quiz_based" | "guidance_based"
	PromptTitle  string    `json:"prompt_title"`
	PromptBody   string    `json:"prompt_body"`
	Tags         []string  `json:"tags"`
	GeneratedAt  time.Time `json:"generated_at"`
}

func (server *Server) getDailyReflection(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)

	dateStr := ctx.DefaultQuery("date", time.Now().UTC().Format("2006-01-02"))
	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(errors.New("invalid date format, use YYYY-MM-DD")))
		return
	}

	forceRegenerate := ctx.DefaultQuery("force_regenerate", "false") == "true"

	reflection, err := server.reflectionService.GenerateDailyReflection(ctx, child.ID)
	if err != nil {
		log.Error().Err(err).Str("child_id", child.ID).Msg("getDailyReflection: failed to generate")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// If force_regenerate is set but we got a cached reflection back, note it.
	_ = forceRegenerate // idempotency is handled inside GenerateDailyReflection

	// Determine source based on whether child has quiz attempts
	source := "guidance_based"
	hasQuiz, err := server.store.CheckQuizAttemptExistsForChild(ctx, child.ID)
	if err == nil && hasQuiz {
		source = "quiz_based"
	}

	// Decode prompt content for structured response
	var promptContent struct {
		PromptText string   `json:"prompt_text"`
		PromptType string   `json:"prompt_type"`
		Tags       []string `json:"tags"`
	}
	if err := json.Unmarshal(reflection.PromptContent, &promptContent); err != nil {
		log.Warn().Err(err).Msg("getDailyReflection: could not decode prompt_content JSON")
	}

	// Derive a human-readable title from the prompt type
	promptTitle := promptTypeTitle(promptContent.PromptType)

	// Derive tags from prompt_type if not embedded in JSON
	tags := promptContent.Tags
	if len(tags) == 0 {
		tags = promptTypeTags(promptContent.PromptType)
	}
	if tags == nil {
		tags = []string{}
	}

	ctx.JSON(http.StatusOK, dailyReflectionResult{
		ReflectionID: reflection.ID.String(),
		ChildID:      child.ID,
		Date:         dateStr,
		Source:       source,
		PromptTitle:  promptTitle,
		PromptBody:   promptContent.PromptText,
		Tags:         tags,
		GeneratedAt:  reflection.CreatedAt,
	})
}

// promptTypeTags maps a GPT prompt_type to human-readable tags.
func promptTypeTags(promptType string) []string {
	m := map[string][]string{
		"gratitude":         {"gratitude", "self-awareness"},
		"growth":            {"growth", "accountability"},
		"mindset":           {"mindset", "resilience"},
		"digital_wellbeing": {"digital wellbeing", "balance"},
		"social_impact":     {"social impact", "empathy"},
		"goals":             {"goals", "intention"},
		"mindfulness":       {"mindfulness", "focus"},
		"social":            {"social", "relationships"},
	}
	if tags, ok := m[promptType]; ok {
		return tags
	}
	return []string{"reflection"}
}

// promptTypeTitle maps a GPT prompt_type to a short display title.
func promptTypeTitle(promptType string) string {
	m := map[string]string{
		"gratitude":         "Today's Gratitude Reflection",
		"growth":            "Growth Check-In",
		"goals":             "Goal Setting Moment",
		"mindfulness":       "Mindfulness Check-In",
		"social":            "Social Reflection",
		"digital_wellbeing": "Digital Wellbeing Check-In",
		"social_impact":     "Social Impact Reflection",
	}
	if title, ok := m[promptType]; ok {
		return title
	}
	return "Daily Reflection"
}
