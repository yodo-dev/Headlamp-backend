package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/gpt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

// ReflectionService handles all business logic for generating, storing, and
// retrieving GPT-powered reflections.
type ReflectionService struct {
	store     db.Store
	gptClient gpt.GptClient
}

func NewReflectionService(store db.Store, gptClient gpt.GptClient) *ReflectionService {
	return &ReflectionService{store: store, gptClient: gptClient}
}

// GenerateDailyReflection generates (or returns the existing) daily scheduled
// reflection for the given child. It is idempotent — safe to call multiple times.
func (s *ReflectionService) GenerateDailyReflection(ctx context.Context, childID string) (*db.Reflection, error) {
	// Idempotency: return existing daily reflection if already generated today
	existing, err := s.store.GetTodayDailyReflectionForChild(ctx, childID)
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	child, err := s.store.GetChild(ctx, childID)
	if err != nil {
		return nil, err
	}

	// Age gate: only 13+
	if !child.Age.Valid || child.Age.Int32 < 13 {
		return nil, errors.New("child does not meet age requirement for reflections")
	}

	childCtx, err := s.buildOrGetContext(ctx, childID)
	if err != nil {
		return nil, err
	}
	childCtx.FirstName = child.FirstName
	childCtx.RecentDailyReflections = s.fetchRecentDailyHistory(ctx, childID)

	gptResp, err := s.gptClient.GenerateDailyReflection(ctx, *childCtx)
	if err != nil {
		return nil, err
	}

	promptJSON, err := json.Marshal(gptResp)
	if err != nil {
		return nil, err
	}

	reflection, err := s.store.CreateReflection(ctx, db.CreateReflectionParams{
		ChildID:       childID,
		TriggerType:   db.ReflectionTriggerTypeDailyScheduled,
		PromptContent: promptJSON,
		Metadata:      []byte(`{}`),
	})
	if err != nil {
		return nil, err
	}

	// Increment delivered count (best-effort, do not fail the request)
	go func() {
		bgCtx := context.Background()
		if err := s.store.IncrementReflectionsDelivered(bgCtx, childID); err != nil {
			log.Error().Err(err).Str("child_id", childID).Msg("failed to increment reflections delivered")
		}
	}()

	return &reflection, nil
}

// GenerateDailyReflectionForced skips the one-per-day idempotency check.
// Use only when REFLECTION_TEST_MODE is enabled.
func (s *ReflectionService) GenerateDailyReflectionForced(ctx context.Context, childID string) (*db.Reflection, error) {
	child, err := s.store.GetChild(ctx, childID)
	if err != nil {
		return nil, err
	}
	if !child.Age.Valid || child.Age.Int32 < 13 {
		return nil, errors.New("child does not meet age requirement for reflections")
	}
	childCtx, err := s.buildOrGetContext(ctx, childID)
	if err != nil {
		return nil, err
	}
	childCtx.FirstName = child.FirstName
	childCtx.RecentDailyReflections = s.fetchRecentDailyHistory(ctx, childID)
	gptResp, err := s.gptClient.GenerateDailyReflection(ctx, *childCtx)
	if err != nil {
		return nil, err
	}
	promptJSON, err := json.Marshal(gptResp)
	if err != nil {
		return nil, err
	}
	reflection, err := s.store.CreateReflection(ctx, db.CreateReflectionParams{
		ChildID:       childID,
		TriggerType:   db.ReflectionTriggerTypeDailyScheduled,
		PromptContent: promptJSON,
		Metadata:      []byte(`{}`),
	})
	if err != nil {
		return nil, err
	}
	go func() {
		if err := s.store.IncrementReflectionsDelivered(context.Background(), childID); err != nil {
			log.Error().Err(err).Str("child_id", childID).Msg("failed to increment reflections delivered")
		}
	}()
	return &reflection, nil
}

// GenerateTimedSessionReflection generates a post-session reflection from an
// app_sessions (timer-based) record. It is used when the session timer expires
// and we need to prompt the child to record a video reflection.
func (s *ReflectionService) GenerateTimedSessionReflection(ctx context.Context, childID string, sessionID uuid.UUID, socialMediaID int64, durationMinutes int) (*db.Reflection, error) {
	// Rate limit: max 5 post-session reflections per day
	count, err := s.store.CountPostSessionReflectionsToday(ctx, childID)
	if err != nil {
		return nil, err
	}
	if count >= 5 {
		return nil, errors.New("daily post-session reflection limit reached")
	}

	child, err := s.store.GetChild(ctx, childID)
	if err != nil {
		return nil, err
	}

	childCtx, err := s.buildOrGetContext(ctx, childID)
	if err != nil {
		return nil, err
	}
	childCtx.FirstName = child.FirstName

	sessCtx := gpt.PostSessionContext{
		Child:          *childCtx,
		SessionAppName: getAppName(socialMediaID),
		SessionMinutes: durationMinutes,
	}

	gptResp, err := s.gptClient.GeneratePostSessionReflection(ctx, sessCtx)
	if err != nil {
		return nil, err
	}

	promptJSON, err := json.Marshal(gptResp)
	if err != nil {
		return nil, err
	}

	// trigger_event_id FK references social_media_sessions.id, but sessionID here
	// is an app_sessions.id — a different table. Pass NULL and store the reference
	// in metadata to avoid the FK violation.
	metadataJSON, err := json.Marshal(map[string]any{
		"app_session_id":   sessionID.String(),
		"social_media_id":  socialMediaID,
		"duration_minutes": durationMinutes,
	})
	if err != nil {
		return nil, err
	}

	reflection, err := s.store.CreateTimedSessionReflection(ctx, db.CreateTimedSessionReflectionParams{
		ChildID:       childID,
		TriggerType:   db.ReflectionTriggerTypePostSession,
		PromptContent: promptJSON,
		Metadata:      metadataJSON,
	})
	if err != nil {
		return nil, err
	}

	go func() {
		bgCtx := context.Background()
		if err := s.store.IncrementReflectionsDelivered(bgCtx, childID); err != nil {
			log.Error().Err(err).Str("child_id", childID).Msg("failed to increment reflections delivered")
		}
	}()

	return &reflection, nil
}

// GeneratePostSessionReflection generates a post-session reflection after a
// social media session ends. Rate-limited to 5 per day.
func (s *ReflectionService) GeneratePostSessionReflection(ctx context.Context, childID string, sessionID uuid.UUID) (*db.Reflection, error) {
	// Rate limit: max 5 post-session reflections per day
	count, err := s.store.CountPostSessionReflectionsToday(ctx, childID)
	if err != nil {
		return nil, err
	}
	if count >= 5 {
		return nil, errors.New("daily post-session reflection limit reached")
	}

	session, err := s.store.GetSocialMediaSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	child, err := s.store.GetChild(ctx, childID)
	if err != nil {
		return nil, err
	}

	childCtx, err := s.buildOrGetContext(ctx, childID)
	if err != nil {
		return nil, err
	}
	childCtx.FirstName = child.FirstName

	// Get today's intention text (best-effort)
	intentionText := ""
	if intention, err := s.store.GetTodayIntention(ctx, childID); err == nil {
		intentionText = intention.IntentionText
	}

	sessionMinutes := 0
	if session.DurationMinutes.Valid {
		sessionMinutes = int(session.DurationMinutes.Int32)
	}

	sessCtx := gpt.PostSessionContext{
		Child:             *childCtx,
		SessionAppName:    getAppName(session.SocialMediaID),
		SessionMinutes:    sessionMinutes,
		ContentCategories: session.ContentCategories,
		IntentionText:     intentionText,
	}

	gptResp, err := s.gptClient.GeneratePostSessionReflection(ctx, sessCtx)
	if err != nil {
		return nil, err
	}

	promptJSON, err := json.Marshal(gptResp)
	if err != nil {
		return nil, err
	}

	reflectionID := uuid.New()
	reflection, err := s.store.CreateReflection(ctx, db.CreateReflectionParams{
		ChildID:        childID,
		TriggerType:    db.ReflectionTriggerTypePostSession,
		TriggerEventID: pgtype.UUID{Bytes: sessionID, Valid: true},
		PromptContent:  promptJSON,
		Metadata:       []byte(`{}`),
	})
	if err != nil {
		return nil, err
	}
	_ = reflectionID

	// Mark the session as having triggered a reflection
	go func() {
		bgCtx := context.Background()
		if err := s.store.MarkSessionReflectionTriggered(bgCtx, db.MarkSessionReflectionTriggeredParams{
			ID:           sessionID,
			ReflectionID: pgtype.UUID{Bytes: reflection.ID, Valid: true},
		}); err != nil {
			log.Error().Err(err).Str("session_id", sessionID.String()).Msg("failed to mark session reflection triggered")
		}
		if err := s.store.IncrementReflectionsDelivered(bgCtx, childID); err != nil {
			log.Error().Err(err).Str("child_id", childID).Msg("failed to increment reflections delivered")
		}
	}()

	return &reflection, nil
}

// RespondToReflection stores a text response to a reflection and auto-acknowledges it.
func (s *ReflectionService) RespondToReflection(ctx context.Context, reflectionID uuid.UUID, responseText string, responseType db.ReflectionResponseType) (*db.Reflection, error) {
	reflection, err := s.store.UpdateReflectionTextResponse(ctx, db.UpdateReflectionTextResponseParams{
		ID:           reflectionID,
		ResponseText: pgtype.Text{String: responseText, Valid: true},
		ResponseType: db.NullReflectionResponseType{ReflectionResponseType: responseType, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	go s.postResponseUpdates(reflection.ChildID)
	go func(id uuid.UUID) {
		if err := s.store.AcknowledgeReflection(context.Background(), db.AcknowledgeReflectionParams{
			ID:                     id,
			AcknowledgmentFeedback: pgtype.Text{},
		}); err != nil {
			log.Error().Err(err).Str("reflection_id", id.String()).Msg("failed to auto-acknowledge reflection")
		}
	}(reflection.ID)

	return &reflection, nil
}

// RespondToReflectionWithMedia stores a media URL response to a reflection and auto-acknowledges it.
func (s *ReflectionService) RespondToReflectionWithMedia(ctx context.Context, reflectionID uuid.UUID, mediaURL string, responseType db.ReflectionResponseType) (*db.Reflection, error) {
	reflection, err := s.store.UpdateReflectionMediaResponse(ctx, db.UpdateReflectionMediaResponseParams{
		ID:               reflectionID,
		ResponseMediaUrl: pgtype.Text{String: mediaURL, Valid: true},
		ResponseType:     db.NullReflectionResponseType{ReflectionResponseType: responseType, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	go s.postResponseUpdates(reflection.ChildID)
	go func(id uuid.UUID) {
		if err := s.store.AcknowledgeReflection(context.Background(), db.AcknowledgeReflectionParams{
			ID:                     id,
			AcknowledgmentFeedback: pgtype.Text{},
		}); err != nil {
			log.Error().Err(err).Str("reflection_id", id.String()).Msg("failed to auto-acknowledge reflection")
		}
	}(reflection.ID)

	return &reflection, nil
}

// AcknowledgeReflection marks a reflection as acknowledged by the parent.
func (s *ReflectionService) AcknowledgeReflection(ctx context.Context, reflectionID uuid.UUID, feedback *string) error {
	feedbackText := pgtype.Text{}
	if feedback != nil {
		feedbackText = pgtype.Text{String: *feedback, Valid: true}
	}
	return s.store.AcknowledgeReflection(ctx, db.AcknowledgeReflectionParams{
		ID:                     reflectionID,
		AcknowledgmentFeedback: feedbackText,
	})
}

// GetPendingReflections returns unanswered reflections for a child.
func (s *ReflectionService) GetPendingReflections(ctx context.Context, childID string, limit, offset int32) ([]db.Reflection, error) {
	return s.store.GetPendingReflectionsForChild(ctx, db.GetPendingReflectionsForChildParams{
		ChildID: childID,
		Limit:   limit,
		Offset:  offset,
	})
}

// GetReflectionHistory returns all reflections for a child (paginated).
func (s *ReflectionService) GetReflectionHistory(ctx context.Context, childID string, limit, offset int32) ([]db.Reflection, error) {
	return s.store.GetReflectionHistory(ctx, db.GetReflectionHistoryParams{
		ChildID: childID,
		// Column2-5 are nullable filter params — pass nil/zero to match all
		Column2: nil,
		Column3: false,
		Column4: time.Time{},
		Column5: time.Time{},
		Limit:   limit,
		Offset:  offset,
	})
}

// GetReflectionStats returns aggregate reflection statistics for a child.
func (s *ReflectionService) GetReflectionStats(ctx context.Context, childID string) (db.GetReflectionStatsRow, error) {
	return s.store.GetReflectionStats(ctx, db.GetReflectionStatsParams{
		ChildID: childID,
		Column2: time.Time{},
		Column3: time.Time{},
	})
}

// postResponseUpdates runs asynchronous updates after a child responds.
func (s *ReflectionService) postResponseUpdates(childID string) {
	bgCtx := context.Background()
	if err := s.store.IncrementReflectionsResponded(bgCtx, childID); err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("failed to increment reflections responded")
	}
	// Refresh context cache async
	if _, err := s.buildContextFromDB(bgCtx, childID); err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("failed to refresh reflection context")
	}
}

// fetchRecentDailyHistory returns the last 10 daily reflections the child
// responded to, used to give GPT conversational continuity across days.
func (s *ReflectionService) fetchRecentDailyHistory(ctx context.Context, childID string) []gpt.PastReflectionEntry {
	rows, err := s.store.GetRecentDailyReflectionsWithResponses(ctx, childID)
	if err != nil {
		return nil
	}
	entries := make([]gpt.PastReflectionEntry, 0, len(rows))
	for _, row := range rows {
		var resp gpt.DailyReflectionResponse
		_ = json.Unmarshal(row.PromptContent, &resp)
		responseText := ""
		if row.ResponseText.Valid {
			responseText = row.ResponseText.String
		}
		entries = append(entries, gpt.PastReflectionEntry{
			Date:         row.DeliveredAt.Format("2006-01-02"),
			PromptText:   resp.PromptText,
			ResponseText: responseText,
		})
	}
	return entries
}

// buildOrGetContext returns the cached reflection context if fresh (<1hr),
// otherwise rebuilds it from the database.
func (s *ReflectionService) buildOrGetContext(ctx context.Context, childID string) (*gpt.ChildReflectionContext, error) {
	cached, err := s.store.GetChildReflectionContext(ctx, childID)
	if err == nil && time.Since(cached.UpdatedAt) < time.Hour {
		return dbContextToGPT(cached), nil
	}
	return s.buildContextFromDB(ctx, childID)
}

// buildContextFromDB builds a fresh ChildReflectionContext by querying all
// relevant tables and upserts it into child_reflection_context.
func (s *ReflectionService) buildContextFromDB(ctx context.Context, childID string) (*gpt.ChildReflectionContext, error) {
	child, err := s.store.GetChild(ctx, childID)
	if err != nil {
		return nil, err
	}

	age := 0
	if child.Age.Valid {
		age = int(child.Age.Int32)
	}

	gptCtx := &gpt.ChildReflectionContext{
		ChildID:   childID,
		FirstName: child.FirstName,
		Age:       age,
	}

	// Digital permit
	if permit, err := s.store.GetLatestCompletedDigitalPermitTestByChildID(ctx, childID); err == nil {
		gptCtx.DigitalPermitStatus = string(permit.Status)
		if permit.Result.Valid {
			gptCtx.DigitalPermitStatus = string(permit.Result.DigitalPermitTestResult)
		}
		gptCtx.DigitalPermitScore = permit.Score
	} else {
		gptCtx.DigitalPermitStatus = "not_started"
	}

	// Reflection stats from context cache (best-effort)
	if cached, err := s.store.GetChildReflectionContext(ctx, childID); err == nil {
		gptCtx.TotalModulesCompleted = int(cached.TotalModulesCompleted)
		gptCtx.TotalQuizzesTaken = int(cached.TotalQuizzesTaken)
		if cached.AverageQuizScore.Valid {
			f, _ := cached.AverageQuizScore.Float64Value()
			gptCtx.AverageQuizScore = f.Float64
		}
		gptCtx.TotalSMSessions = int(cached.TotalSmSessions)
		if cached.AvgDailySmMinutes.Valid {
			f, _ := cached.AvgDailySmMinutes.Float64Value()
			gptCtx.AvgDailyMinutes = f.Float64
		}
		gptCtx.FrequentContentCategories = cached.FrequentContentCategories
		gptCtx.CompletedModuleIDs = cached.CompletedModuleIds
		gptCtx.ReflectionStreak = int(cached.ReflectionStreak)
		gptCtx.TotalReflectionsResponded = int(cached.TotalReflectionsResponded)
		gptCtx.TotalReflectionsDelivered = int(cached.TotalReflectionsDelivered)
		gptCtx.LastReflectionAcknowledged = cached.LastReflectionAcknowledged

		// Deserialise most_used_apps JSON
		if len(cached.MostUsedApps) > 0 {
			_ = json.Unmarshal(cached.MostUsedApps, &gptCtx.MostUsedApps)
		}
		if len(cached.RecentActivities) > 0 {
			_ = json.Unmarshal(cached.RecentActivities, &gptCtx.RecentActivities)
		}
	}

	return gptCtx, nil
}

// dbContextToGPT converts a cached db.ChildReflectionContext to the GPT type.
func dbContextToGPT(c db.ChildReflectionContext) *gpt.ChildReflectionContext {
	ctx := &gpt.ChildReflectionContext{
		ChildID:                    c.ChildID,
		TotalModulesCompleted:      int(c.TotalModulesCompleted),
		TotalQuizzesTaken:          int(c.TotalQuizzesTaken),
		TotalSMSessions:            int(c.TotalSmSessions),
		FrequentContentCategories:  c.FrequentContentCategories,
		CompletedModuleIDs:         c.CompletedModuleIds,
		ReflectionStreak:           int(c.ReflectionStreak),
		TotalReflectionsResponded:  int(c.TotalReflectionsResponded),
		TotalReflectionsDelivered:  int(c.TotalReflectionsDelivered),
		LastReflectionAcknowledged: c.LastReflectionAcknowledged,
	}
	if c.AverageQuizScore.Valid {
		f, _ := c.AverageQuizScore.Float64Value()
		ctx.AverageQuizScore = f.Float64
	}
	if c.AvgDailySmMinutes.Valid {
		f, _ := c.AvgDailySmMinutes.Float64Value()
		ctx.AvgDailyMinutes = f.Float64
	}
	if c.DigitalPermitStatus.Valid {
		ctx.DigitalPermitStatus = c.DigitalPermitStatus.String
	}
	if len(c.MostUsedApps) > 0 {
		_ = json.Unmarshal(c.MostUsedApps, &ctx.MostUsedApps)
	}
	if len(c.RecentActivities) > 0 {
		_ = json.Unmarshal(c.RecentActivities, &ctx.RecentActivities)
	}
	return ctx
}

// getAppName returns a friendly app name for a social_media_id.
// In a real system this would be a lookup; here we return an identifier.
func getAppName(socialMediaID int64) string {
	names := map[int64]string{
		1: "Instagram",
		2: "TikTok",
		3: "YouTube",
		4: "Twitter",
		5: "WhatsApp",
		6: "Facebook",
	}
	if name, ok := names[socialMediaID]; ok {
		return name
	}
	return "social media"
}
