package db

import (
	"time"

	"github.com/google/uuid"
)

// ─── ai_insights_snapshots ───────────────────────────────────────────────────

// AiInsightsSnapshot maps to the ai_insights_snapshots table.
type AiInsightsSnapshot struct {
	ID            uuid.UUID `json:"id"`
	ChildID       string    `json:"child_id"`
	RangeDays     int32     `json:"range_days"`
	SnapshotData  []byte    `json:"snapshot_data"` // jsonb
	ModelVersion  string    `json:"model_version"`
	DataFreshness string    `json:"data_freshness"`
	GeneratedAt   time.Time `json:"generated_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// ─── content_monitoring_events ───────────────────────────────────────────────

// ContentMonitoringEvent maps to the content_monitoring_events table.
type ContentMonitoringEvent struct {
	ID             uuid.UUID `json:"id"`
	ChildID        string    `json:"child_id"`
	Platform       string    `json:"platform"`
	Category       string    `json:"category"`
	Severity       string    `json:"severity"`
	EventTimestamp time.Time `json:"event_timestamp"`
	Metadata       []byte    `json:"metadata"` // jsonb
	CreatedAt      time.Time `json:"created_at"`
}

// ─── Aggregation result types (returned by insights queries) ─────────────────

// AppSessionAggregate is returned by GetAppSessionAggregateForChild.
type AppSessionAggregate struct {
	AppName           string  `json:"app_name"`
	SessionCount      int64   `json:"session_count"`
	TotalMinutes      float64 `json:"total_minutes"`
	AvgSessionMinutes float64 `json:"avg_session_minutes"`
}

// ReflectionAggregate is returned by GetReflectionAggregateForChild.
type ReflectionAggregate struct {
	TotalDelivered int64   `json:"total_delivered"`
	TotalResponded int64   `json:"total_responded"`
	CompletionRate float64 `json:"completion_rate"`
}

// ReflectionResponseRow is returned by GetRecentReflectionResponsesForChild.
type ReflectionResponseRow struct {
	ResponseText string    `json:"response_text"`
	DeliveredAt  time.Time `json:"delivered_at"`
}

// QuizAggregate is returned by GetQuizAggregateForChild.
type QuizAggregate struct {
	TotalAttempts int64   `json:"total_attempts"`
	PassCount     int64   `json:"pass_count"`
	FailCount     int64   `json:"fail_count"`
	AvgScore      float64 `json:"avg_score"`
}

// ContentCountRow is returned by GetContentMonitoringCountsByCategoryAndSeverity.
type ContentCountRow struct {
	Category   string `json:"category"`
	Severity   string `json:"severity"`
	EventCount int64  `json:"event_count"`
}

// PlatformCountRow is returned by GetTopRiskyPlatforms.
type PlatformCountRow struct {
	Platform   string `json:"platform"`
	EventCount int64  `json:"event_count"`
}

// ─── Store method parameter types ────────────────────────────────────────────

// UpsertInsightsSnapshotParams holds parameters for UpsertInsightsSnapshot.
type UpsertInsightsSnapshotParams struct {
	ChildID       string `json:"child_id"`
	RangeDays     int32  `json:"range_days"`
	SnapshotData  []byte `json:"snapshot_data"`
	ModelVersion  string `json:"model_version"`
	DataFreshness string `json:"data_freshness"`
}

// GetInsightsSnapshotParams holds parameters for GetInsightsSnapshot.
type GetInsightsSnapshotParams struct {
	ChildID   string `json:"child_id"`
	RangeDays int32  `json:"range_days"`
}

// CreateContentMonitoringEventParams holds parameters for CreateContentMonitoringEvent.
type CreateContentMonitoringEventParams struct {
	ChildID        string    `json:"child_id"`
	Platform       string    `json:"platform"`
	Category       string    `json:"category"`
	Severity       string    `json:"severity"`
	EventTimestamp time.Time `json:"event_timestamp"`
	Metadata       []byte    `json:"metadata"`
}

// GetInsightAggregateParams is the shared param type for all child+range aggregate queries.
type GetInsightAggregateParams struct {
	ChildID   string    `json:"child_id"`
	StartTime time.Time `json:"start_time"`
}
