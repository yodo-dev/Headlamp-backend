package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/gpt"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// snapshotTTL is how long a cached snapshot is considered "fresh" before
// triggering a background recomputation.  Keeps p95 dashboard latency low.
const snapshotTTL = 1 * time.Hour

// ModelVersion is stamped on every snapshot so the front-end can gate on it.
const ModelVersion = "ai-insights-v1"

// ValidRangeDays lists the time windows the service accepts.
var ValidRangeDays = []int{1, 7, 30}

// ─── Top-level response type (owned by service layer) ────────────────────────

// DashboardInsightsResponse is the canonical API response returned to parents.
type DashboardInsightsResponse struct {
	ChildID                  string                     `json:"child_id"`
	Range                    string                     `json:"range"`
	GeneratedAt              time.Time                  `json:"generated_at"`
	ModelVersion             string                     `json:"model_version"`
	GuidanceCards            []gpt.GuidanceCard         `json:"guidance_cards"`
	EmotionalTrends          gpt.EmotionalTrends        `json:"emotional_trends"`
	BehavioralInsights       []gpt.BehavioralInsight    `json:"behavioral_insights"`
	EngagementOverview       EngagementOverview         `json:"engagement_overview"`
	DigitalMaturityProfile   gpt.DigitalMaturityProfile `json:"digital_maturity_profile"`
	ContentMonitoringSummary ContentMonitoringSummary   `json:"content_monitoring_summary"`
	// Metadata fields
	DataFreshness string `json:"data_freshness"` // fresh | delayed | stale
	FallbackUsed  bool   `json:"fallback_used"`
	FailureCode   string `json:"failure_code,omitempty"`
}

// EngagementOverview is computed directly from DB data without GPT.
type EngagementOverview struct {
	TotalMinutes          float64         `json:"total_minutes"`
	SessionsCount         int64           `json:"sessions_count"`
	AverageSessionMinutes float64         `json:"average_session_minutes"`
	ByApp                 []AppEngagement `json:"by_app"`
	OverLimitIncidents    int64           `json:"over_limit_incidents"`
	NightUsageCount       int64           `json:"night_usage_count"`
	HealthyUsageScore     int             `json:"healthy_usage_score"` // 0–100
}

// AppEngagement is per-app engagement within a window.
type AppEngagement struct {
	App                   string  `json:"app"`
	Minutes               float64 `json:"minutes"`
	Sessions              int64   `json:"sessions"`
	AverageSessionMinutes float64 `json:"average_session_minutes"`
}

// ContentMonitoringSummary is the parent-facing risk event digest.
type ContentMonitoringSummary struct {
	RiskCounts     ContentRiskCounts   `json:"risk_counts"`
	CategoryCounts map[string]int64    `json:"category_counts"`
	TopPlatforms   []string            `json:"top_platforms"`
	Trend          string              `json:"trend"` // stable | increasing | decreasing
	LatestAlert    *ContentLatestAlert `json:"latest_alert,omitempty"`
}

// ContentRiskCounts holds counts by severity bucket.
type ContentRiskCounts struct {
	Low    int64 `json:"low"`
	Medium int64 `json:"medium"`
	High   int64 `json:"high"`
}

// ContentLatestAlert is the most recent high/medium severity event.
type ContentLatestAlert struct {
	Timestamp time.Time `json:"timestamp"`
	Severity  string    `json:"severity"`
	Category  string    `json:"category"`
	Platform  string    `json:"platform"`
}

// ─── InsightsService ──────────────────────────────────────────────────────────

// InsightsService orchestrates data aggregation, GPT inference, and snapshot
// caching to deliver the parent dashboard insights API.
type InsightsService struct {
	store     db.Store
	gptClient gpt.GptClient
}

// NewInsightsService creates an InsightsService.
func NewInsightsService(store db.Store, gptClient gpt.GptClient) *InsightsService {
	return &InsightsService{store: store, gptClient: gptClient}
}

// GetDashboardInsights is the primary entry point.  It implements a
// cache-first strategy:
//  1. Return a fresh cached snapshot if one exists (< snapshotTTL).
//  2. Otherwise aggregate child data, call GPT, store the result, and return it.
//  3. If GPT fails, return the last stale snapshot or a safe static fallback.
func (s *InsightsService) GetDashboardInsights(ctx context.Context, childID string, rangeDays int) (*DashboardInsightsResponse, error) {
	param := db.GetInsightsSnapshotParams{ChildID: childID, RangeDays: int32(rangeDays)}

	snapshot, snapshotErr := s.store.GetInsightsSnapshot(ctx, param)
	if snapshotErr == nil && db.SnapshotAge(snapshot) < snapshotTTL {
		// Cache hit — fast path.
		return s.deserializeSnapshot(snapshot, "fresh", false), nil
	}

	// Aggregate raw signals.
	rawCtx, err := s.buildInsightsContext(ctx, childID, rangeDays)
	if err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("insights: failed to aggregate child data")
		return s.fallbackFromSnapshot(snapshot, snapshotErr)
	}

	// Compute DB-only sections.
	engagementOverview := s.computeEngagementOverview(rawCtx)
	contentSummary, err := s.computeContentMonitoringSummary(ctx, childID, rawCtx.startTime)
	if err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("insights: failed to compute content summary, using empty")
		contentSummary = ContentMonitoringSummary{
			RiskCounts:     ContentRiskCounts{},
			CategoryCounts: map[string]int64{},
			TopPlatforms:   []string{},
			Trend:          "stable",
		}
	}

	// Call GPT for AI-computed sections.
	gptResp, gptErr := s.gptClient.GenerateInsights(rawCtx.insightsCtx)
	if gptErr != nil {
		log.Error().Err(gptErr).Str("child_id", childID).Msg("insights: GPT generation failed")
		return s.fallbackFromSnapshot(snapshot, snapshotErr)
	}

	// Stamp generated_at on the maturity profile here so it comes from the server.
	gptResp.DigitalMaturity.GeneratedAt = time.Now().UTC()

	resp := &DashboardInsightsResponse{
		ChildID:                  childID,
		Range:                    fmt.Sprintf("%dd", rangeDays),
		GeneratedAt:              time.Now().UTC(),
		ModelVersion:             ModelVersion,
		GuidanceCards:            nilToEmpty(gptResp.GuidanceCards),
		EmotionalTrends:          gptResp.EmotionalTrends,
		BehavioralInsights:       nilToEmpty(gptResp.BehavioralInsights),
		EngagementOverview:       engagementOverview,
		DigitalMaturityProfile:   gptResp.DigitalMaturity,
		ContentMonitoringSummary: contentSummary,
		DataFreshness:            "fresh",
		FallbackUsed:             false,
	}

	// Persist snapshot asynchronously so the HTTP response is not blocked.
	go s.persistSnapshot(childID, rangeDays, resp)

	return resp, nil
}

// ─── Individual sub-endpoints ─────────────────────────────────────────────────

// GetEngagementOverview returns only the engagement section (no GPT call).
func (s *InsightsService) GetEngagementOverview(ctx context.Context, childID string, rangeDays int) (*EngagementOverview, error) {
	rawCtx, err := s.buildInsightsContext(ctx, childID, rangeDays)
	if err != nil {
		return nil, err
	}
	ov := s.computeEngagementOverview(rawCtx)
	return &ov, nil
}

// GetContentMonitoringSummary returns only the content monitoring section (no GPT call).
func (s *InsightsService) GetContentMonitoringSummary(ctx context.Context, childID string, rangeDays int) (*ContentMonitoringSummary, error) {
	startTime := rangeStart(rangeDays)
	summary, err := s.computeContentMonitoringSummary(ctx, childID, startTime)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

// IngestContentMonitoringEvent records a new risk event for a child.
func (s *InsightsService) IngestContentMonitoringEvent(ctx context.Context, arg db.CreateContentMonitoringEventParams) (db.ContentMonitoringEvent, error) {
	event, err := s.store.CreateContentMonitoringEvent(ctx, arg)
	if err != nil {
		return db.ContentMonitoringEvent{}, err
	}
	// Invalidate all cached snapshots for this child so next read recomputes.
	if markErr := s.store.MarkInsightSnapshotStale(ctx, arg.ChildID); markErr != nil {
		log.Warn().Err(markErr).Str("child_id", arg.ChildID).Msg("insights: failed to mark snapshot stale after event ingestion")
	}
	return event, nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// rawInsightsData holds all pre-aggregated data for a single computation run.
type rawInsightsData struct {
	insightsCtx gpt.InsightsContext
	startTime   time.Time
	sessions    []db.AppSessionAggregate
	overLimit   int64
	nightCount  int64
}

func (s *InsightsService) buildInsightsContext(ctx context.Context, childID string, rangeDays int) (*rawInsightsData, error) {
	start := rangeStart(rangeDays)
	param := db.GetInsightAggregateParams{ChildID: childID, StartTime: start}

	child, err := s.store.GetChild(ctx, childID)
	if err != nil {
		return nil, fmt.Errorf("insights: get child: %w", err)
	}

	sessions, err := s.store.GetAppSessionAggregateForChild(ctx, param)
	if err != nil {
		return nil, fmt.Errorf("insights: get session aggregate: %w", err)
	}

	overLimit, err := s.store.GetOverLimitSessionCount(ctx, param)
	if err != nil {
		return nil, fmt.Errorf("insights: get over-limit count: %w", err)
	}

	nightCount, err := s.store.GetNightSessionCount(ctx, param)
	if err != nil {
		return nil, fmt.Errorf("insights: get night count: %w", err)
	}

	quiz, err := s.store.GetQuizAggregateForChild(ctx, param)
	if err != nil {
		return nil, fmt.Errorf("insights: get quiz aggregate: %w", err)
	}

	reflection, err := s.store.GetReflectionAggregateForChild(ctx, param)
	if err != nil {
		return nil, fmt.Errorf("insights: get reflection aggregate: %w", err)
	}

	recentResponses, err := s.store.GetRecentReflectionResponsesForChild(ctx, param)
	if err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("insights: could not fetch recent reflection responses")
		recentResponses = nil
	}

	// Build per-app summary for GPT context.
	byApp := make([]gpt.InsightAppUsage, 0, len(sessions))
	var totalSessions int64
	var totalMinutes float64
	for _, s := range sessions {
		byApp = append(byApp, gpt.InsightAppUsage{
			AppName:           s.AppName,
			Sessions:          s.SessionCount,
			TotalMinutes:      s.TotalMinutes,
			AvgSessionMinutes: s.AvgSessionMinutes,
		})
		totalSessions += s.SessionCount
		totalMinutes += s.TotalMinutes
	}

	var avgSession float64
	if totalSessions > 0 {
		avgSession = totalMinutes / float64(totalSessions)
	}

	var passRate float64
	if quiz.TotalAttempts > 0 {
		passRate = float64(quiz.PassCount) / float64(quiz.TotalAttempts)
	}

	responseTexts := make([]string, 0, len(recentResponses))
	for _, r := range recentResponses {
		responseTexts = append(responseTexts, r.ResponseText)
	}

	age := 0
	if child.Age.Valid {
		age = int(child.Age.Int32)
	}

	insCtx := gpt.InsightsContext{
		ChildID:        childID,
		ChildFirstName: child.FirstName,
		ChildAge:       age,
		RangeDays:      rangeDays,
		Sessions: gpt.InsightSessionData{
			TotalSessions:      totalSessions,
			TotalMinutes:       totalMinutes,
			AvgSessionMinutes:  avgSession,
			ByApp:              byApp,
			OverLimitIncidents: overLimit,
			NightSessionCount:  nightCount,
		},
		Quizzes: gpt.InsightQuizData{
			TotalAttempts: quiz.TotalAttempts,
			PassCount:     quiz.PassCount,
			FailCount:     quiz.FailCount,
			AvgScore:      quiz.AvgScore,
			PassRate:      passRate,
		},
		Reflections: gpt.InsightReflectionData{
			TotalDelivered:  reflection.TotalDelivered,
			TotalResponded:  reflection.TotalResponded,
			CompletionRate:  reflection.CompletionRate,
			RecentResponses: responseTexts,
		},
	}

	return &rawInsightsData{
		insightsCtx: insCtx,
		startTime:   start,
		sessions:    sessions,
		overLimit:   overLimit,
		nightCount:  nightCount,
	}, nil
}

func (s *InsightsService) computeEngagementOverview(raw *rawInsightsData) EngagementOverview {
	byApp := make([]AppEngagement, 0, len(raw.sessions))
	var totalSessions int64
	var totalMinutes float64

	for _, agg := range raw.sessions {
		byApp = append(byApp, AppEngagement{
			App:                   agg.AppName,
			Minutes:               agg.TotalMinutes,
			Sessions:              agg.SessionCount,
			AverageSessionMinutes: agg.AvgSessionMinutes,
		})
		totalSessions += agg.SessionCount
		totalMinutes += agg.TotalMinutes
	}

	var avgSession float64
	if totalSessions > 0 {
		avgSession = totalMinutes / float64(totalSessions)
	}

	healthyScore := computeHealthyUsageScore(totalMinutes, raw.overLimit, raw.nightCount, raw.insightsCtx.RangeDays)

	return EngagementOverview{
		TotalMinutes:          totalMinutes,
		SessionsCount:         totalSessions,
		AverageSessionMinutes: avgSession,
		ByApp:                 byApp,
		OverLimitIncidents:    raw.overLimit,
		NightUsageCount:       raw.nightCount,
		HealthyUsageScore:     healthyScore,
	}
}

func (s *InsightsService) computeContentMonitoringSummary(ctx context.Context, childID string, startTime time.Time) (ContentMonitoringSummary, error) {
	param := db.GetInsightAggregateParams{ChildID: childID, StartTime: startTime}

	counts, err := s.store.GetContentMonitoringCounts(ctx, param)
	if err != nil {
		return ContentMonitoringSummary{}, err
	}

	topPlatformsRaw, err := s.store.GetTopRiskyPlatforms(ctx, param)
	if err != nil {
		return ContentMonitoringSummary{}, err
	}

	riskCounts := ContentRiskCounts{}
	categoryMap := map[string]int64{}
	for _, c := range counts {
		categoryMap[c.Category] += c.EventCount
		switch c.Severity {
		case "low":
			riskCounts.Low += c.EventCount
		case "medium":
			riskCounts.Medium += c.EventCount
		case "high":
			riskCounts.High += c.EventCount
		}
	}

	topPlatforms := make([]string, 0, len(topPlatformsRaw))
	for _, p := range topPlatformsRaw {
		topPlatforms = append(topPlatforms, p.Platform)
	}

	// Fetch latest alert.
	var latestAlert *ContentLatestAlert
	alert, alertErr := s.store.GetLatestContentMonitoringAlert(ctx, childID)
	if alertErr == nil {
		latestAlert = &ContentLatestAlert{
			Timestamp: alert.EventTimestamp,
			Severity:  alert.Severity,
			Category:  alert.Category,
			Platform:  alert.Platform,
		}
	} else if !errors.Is(alertErr, pgx.ErrNoRows) {
		log.Warn().Err(alertErr).Str("child_id", childID).Msg("insights: failed to fetch latest alert")
	}

	return ContentMonitoringSummary{
		RiskCounts:     riskCounts,
		CategoryCounts: categoryMap,
		TopPlatforms:   topPlatforms,
		Trend:          "stable",
		LatestAlert:    latestAlert,
	}, nil
}

// persistSnapshot serialises the response and stores it as a snapshot.
// Called in a goroutine so it never blocks the API response.
func (s *InsightsService) persistSnapshot(childID string, rangeDays int, resp *DashboardInsightsResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("insights: failed to marshal snapshot")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.store.UpsertInsightsSnapshot(ctx, db.UpsertInsightsSnapshotParams{
		ChildID:       childID,
		RangeDays:     int32(rangeDays),
		SnapshotData:  data,
		ModelVersion:  ModelVersion,
		DataFreshness: "fresh",
	}); err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("insights: failed to persist snapshot")
	}
}

// fallbackFromSnapshot returns a stale snapshot if available or a static safe fallback.
func (s *InsightsService) fallbackFromSnapshot(snapshot db.AiInsightsSnapshot, snapshotErr error) (*DashboardInsightsResponse, error) {
	if snapshotErr == nil {
		// We have a stale snapshot.
		return s.deserializeSnapshot(snapshot, "stale", true), nil
	}
	// No snapshot at all — return deterministic safe fallback.
	return s.staticFallback(), nil
}

func (s *InsightsService) deserializeSnapshot(snapshot db.AiInsightsSnapshot, freshness string, fallbackUsed bool) *DashboardInsightsResponse {
	var resp DashboardInsightsResponse
	if err := db.UnmarshalSnapshotData(snapshot.SnapshotData, &resp); err != nil {
		log.Error().Err(err).Str("child_id", snapshot.ChildID).Msg("insights: failed to deserialize snapshot")
		return s.staticFallback()
	}
	resp.DataFreshness = freshness
	resp.FallbackUsed = fallbackUsed
	return &resp
}

// staticFallback returns a minimal safe response with no AI content.
func (s *InsightsService) staticFallback() *DashboardInsightsResponse {
	return &DashboardInsightsResponse{
		GeneratedAt:  time.Now().UTC(),
		ModelVersion: ModelVersion,
		GuidanceCards: []gpt.GuidanceCard{
			{
				ID:         "fallback_1",
				Title:      "Insights temporarily unavailable",
				Message:    "We are still gathering data. Check back shortly for personalised recommendations.",
				Priority:   "low",
				Confidence: 1.0,
				Tags:       []string{"system"},
			},
		},
		EmotionalTrends: gpt.EmotionalTrends{
			Series:         []gpt.EmotionDataPoint{},
			StabilityScore: 50,
			Direction:      "stable",
		},
		BehavioralInsights: []gpt.BehavioralInsight{},
		EngagementOverview: EngagementOverview{
			ByApp: []AppEngagement{},
		},
		DigitalMaturityProfile: gpt.DigitalMaturityProfile{
			OverallScore: 0,
			Band:         "emerging",
			Dimensions:   gpt.MaturityDimensions{},
			NextSteps:    []string{},
		},
		ContentMonitoringSummary: ContentMonitoringSummary{
			RiskCounts:     ContentRiskCounts{},
			CategoryCounts: map[string]int64{},
			TopPlatforms:   []string{},
			Trend:          "stable",
		},
		DataFreshness: "stale",
		FallbackUsed:  true,
		FailureCode:   "no_data",
	}
}

// ─── Pure computation helpers ─────────────────────────────────────────────────

// rangeStart returns the UTC start of the time window for rangeDays.
func rangeStart(rangeDays int) time.Time {
	return time.Now().UTC().Add(-time.Duration(rangeDays) * 24 * time.Hour)
}

// computeHealthyUsageScore produces a 0–100 score from engagement signals.
// Higher over-limit incidents and night usage reduce the score.
func computeHealthyUsageScore(totalMinutes float64, overLimit, nightCount int64, rangeDays int) int {
	base := 100

	// Over-limit penalty: −10 per incident, capped at −50.
	overLimitPenalty := int(overLimit) * 10
	if overLimitPenalty > 50 {
		overLimitPenalty = 50
	}

	// Night-usage penalty: −5 per night session, capped at −30.
	nightPenalty := int(nightCount) * 5
	if nightPenalty > 30 {
		nightPenalty = 30
	}

	// Excessive daily average penalty: > 3h/day → −10.
	avgDailyMinutes := totalMinutes / float64(rangeDays)
	excessivePenalty := 0
	if avgDailyMinutes > 180 {
		excessivePenalty = 10
	}

	score := base - overLimitPenalty - nightPenalty - excessivePenalty
	if score < 0 {
		score = 0
	}
	return score
}

// nilToEmpty ensures nil slices become empty slices for clean JSON output.
func nilToEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
