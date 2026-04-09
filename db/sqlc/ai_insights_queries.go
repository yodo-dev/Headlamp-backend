package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ─── ai_insights_snapshots ───────────────────────────────────────────────────

const upsertInsightsSnapshot = `
INSERT INTO ai_insights_snapshots (child_id, range_days, snapshot_data, model_version, data_freshness, generated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (child_id, range_days)
DO UPDATE SET
  snapshot_data  = EXCLUDED.snapshot_data,
  model_version  = EXCLUDED.model_version,
  data_freshness = EXCLUDED.data_freshness,
  generated_at   = now()
RETURNING id, child_id, range_days, snapshot_data, model_version, data_freshness, generated_at, created_at
`

func (store *SQLStore) UpsertInsightsSnapshot(ctx context.Context, arg UpsertInsightsSnapshotParams) (AiInsightsSnapshot, error) {
	row := store.connPool.QueryRow(ctx, upsertInsightsSnapshot,
		arg.ChildID,
		arg.RangeDays,
		arg.SnapshotData,
		arg.ModelVersion,
		arg.DataFreshness,
	)
	var i AiInsightsSnapshot
	err := row.Scan(
		&i.ID,
		&i.ChildID,
		&i.RangeDays,
		&i.SnapshotData,
		&i.ModelVersion,
		&i.DataFreshness,
		&i.GeneratedAt,
		&i.CreatedAt,
	)
	return i, err
}

const getInsightsSnapshot = `
SELECT id, child_id, range_days, snapshot_data, model_version, data_freshness, generated_at, created_at
FROM ai_insights_snapshots
WHERE child_id = $1 AND range_days = $2
`

func (store *SQLStore) GetInsightsSnapshot(ctx context.Context, arg GetInsightsSnapshotParams) (AiInsightsSnapshot, error) {
	row := store.connPool.QueryRow(ctx, getInsightsSnapshot, arg.ChildID, arg.RangeDays)
	var i AiInsightsSnapshot
	err := row.Scan(
		&i.ID,
		&i.ChildID,
		&i.RangeDays,
		&i.SnapshotData,
		&i.ModelVersion,
		&i.DataFreshness,
		&i.GeneratedAt,
		&i.CreatedAt,
	)
	return i, err
}

const markSnapshotStale = `
UPDATE ai_insights_snapshots SET data_freshness = 'stale' WHERE child_id = $1
`

func (store *SQLStore) MarkInsightSnapshotStale(ctx context.Context, childID string) error {
	_, err := store.connPool.Exec(ctx, markSnapshotStale, childID)
	return err
}

// ─── content_monitoring_events ───────────────────────────────────────────────

const createContentMonitoringEvent = `
INSERT INTO content_monitoring_events (child_id, platform, category, severity, event_timestamp, metadata)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, child_id, platform, category, severity, event_timestamp, metadata, created_at
`

func (store *SQLStore) CreateContentMonitoringEvent(ctx context.Context, arg CreateContentMonitoringEventParams) (ContentMonitoringEvent, error) {
	metaJSON := arg.Metadata
	if metaJSON == nil {
		metaJSON = []byte("{}")
	}
	row := store.connPool.QueryRow(ctx, createContentMonitoringEvent,
		arg.ChildID,
		arg.Platform,
		arg.Category,
		arg.Severity,
		arg.EventTimestamp,
		metaJSON,
	)
	var i ContentMonitoringEvent
	err := row.Scan(
		&i.ID,
		&i.ChildID,
		&i.Platform,
		&i.Category,
		&i.Severity,
		&i.EventTimestamp,
		&i.Metadata,
		&i.CreatedAt,
	)
	return i, err
}

const getContentMonitoringEventsForChild = `
SELECT id, child_id, platform, category, severity, event_timestamp, metadata, created_at
FROM content_monitoring_events
WHERE child_id = $1 AND event_timestamp >= $2
ORDER BY event_timestamp DESC
`

func (store *SQLStore) GetContentMonitoringEventsForChild(ctx context.Context, arg GetInsightAggregateParams) ([]ContentMonitoringEvent, error) {
	rows, err := store.connPool.Query(ctx, getContentMonitoringEventsForChild, arg.ChildID, arg.StartTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ContentMonitoringEvent
	for rows.Next() {
		var i ContentMonitoringEvent
		if err := rows.Scan(
			&i.ID,
			&i.ChildID,
			&i.Platform,
			&i.Category,
			&i.Severity,
			&i.EventTimestamp,
			&i.Metadata,
			&i.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const getLatestContentMonitoringAlert = `
SELECT id, child_id, platform, category, severity, event_timestamp, metadata, created_at
FROM content_monitoring_events
WHERE child_id = $1
ORDER BY event_timestamp DESC
LIMIT 1
`

func (store *SQLStore) GetLatestContentMonitoringAlert(ctx context.Context, childID string) (ContentMonitoringEvent, error) {
	row := store.connPool.QueryRow(ctx, getLatestContentMonitoringAlert, childID)
	var i ContentMonitoringEvent
	err := row.Scan(
		&i.ID,
		&i.ChildID,
		&i.Platform,
		&i.Category,
		&i.Severity,
		&i.EventTimestamp,
		&i.Metadata,
		&i.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return ContentMonitoringEvent{}, pgx.ErrNoRows
	}
	return i, err
}

const getContentMonitoringCountsByCategoryAndSeverity = `
SELECT category, severity, COUNT(*)::bigint AS event_count
FROM content_monitoring_events
WHERE child_id = $1 AND event_timestamp >= $2
GROUP BY category, severity
ORDER BY event_count DESC
`

func (store *SQLStore) GetContentMonitoringCounts(ctx context.Context, arg GetInsightAggregateParams) ([]ContentCountRow, error) {
	rows, err := store.connPool.Query(ctx, getContentMonitoringCountsByCategoryAndSeverity, arg.ChildID, arg.StartTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ContentCountRow
	for rows.Next() {
		var r ContentCountRow
		if err := rows.Scan(&r.Category, &r.Severity, &r.EventCount); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

const getTopRiskyPlatforms = `
SELECT platform, COUNT(*)::bigint AS event_count
FROM content_monitoring_events
WHERE child_id = $1
  AND event_timestamp >= $2
  AND severity IN ('medium', 'high')
GROUP BY platform
ORDER BY event_count DESC
LIMIT 5
`

func (store *SQLStore) GetTopRiskyPlatforms(ctx context.Context, arg GetInsightAggregateParams) ([]PlatformCountRow, error) {
	rows, err := store.connPool.Query(ctx, getTopRiskyPlatforms, arg.ChildID, arg.StartTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []PlatformCountRow
	for rows.Next() {
		var r PlatformCountRow
		if err := rows.Scan(&r.Platform, &r.EventCount); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

// ─── Session aggregation ─────────────────────────────────────────────────────

const getAppSessionAggregateForChild = `
SELECT
  sm.name                                                                                AS app_name,
  COUNT(s.id)::bigint                                                                    AS session_count,
  COALESCE(SUM(EXTRACT(EPOCH FROM
    (COALESCE(s.end_time, s.last_ping_time, s.expected_end_time) - s.start_time)) / 60),
    0)::float8                                                                           AS total_minutes,
  COALESCE(AVG(EXTRACT(EPOCH FROM
    (COALESCE(s.end_time, s.last_ping_time, s.expected_end_time) - s.start_time)) / 60),
    0)::float8                                                                           AS avg_session_minutes
FROM app_sessions s
JOIN social_medias sm ON sm.id = s.social_media_id
WHERE s.child_id = $1
  AND s.start_time >= $2
GROUP BY sm.name
`

func (store *SQLStore) GetAppSessionAggregateForChild(ctx context.Context, arg GetInsightAggregateParams) ([]AppSessionAggregate, error) {
	rows, err := store.connPool.Query(ctx, getAppSessionAggregateForChild, arg.ChildID, arg.StartTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []AppSessionAggregate
	for rows.Next() {
		var r AppSessionAggregate
		if err := rows.Scan(&r.AppName, &r.SessionCount, &r.TotalMinutes, &r.AvgSessionMinutes); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

const getOverLimitSessionCount = `
SELECT COUNT(*)::bigint FROM app_sessions
WHERE child_id = $1
  AND start_time >= $2
  AND COALESCE(end_time, last_ping_time, now()) > expected_end_time
`

func (store *SQLStore) GetOverLimitSessionCount(ctx context.Context, arg GetInsightAggregateParams) (int64, error) {
	row := store.connPool.QueryRow(ctx, getOverLimitSessionCount, arg.ChildID, arg.StartTime)
	var count int64
	return count, row.Scan(&count)
}

const getNightSessionCount = `
SELECT COUNT(*)::bigint FROM app_sessions
WHERE child_id = $1
  AND start_time >= $2
  AND (EXTRACT(HOUR FROM start_time AT TIME ZONE 'UTC') >= 21
    OR EXTRACT(HOUR FROM start_time AT TIME ZONE 'UTC') < 6)
`

func (store *SQLStore) GetNightSessionCount(ctx context.Context, arg GetInsightAggregateParams) (int64, error) {
	row := store.connPool.QueryRow(ctx, getNightSessionCount, arg.ChildID, arg.StartTime)
	var count int64
	return count, row.Scan(&count)
}

// ─── Reflection aggregation ──────────────────────────────────────────────────

const getReflectionAggregateForChild = `
SELECT
  COUNT(*)::bigint                                                        AS total_delivered,
  COUNT(responded_at)::bigint                                            AS total_responded,
  CASE WHEN COUNT(*) > 0
    THEN COUNT(responded_at)::float8 / COUNT(*)::float8
    ELSE 0 END                                                            AS completion_rate
FROM reflections
WHERE child_id = $1 AND delivered_at >= $2
`

func (store *SQLStore) GetReflectionAggregateForChild(ctx context.Context, arg GetInsightAggregateParams) (ReflectionAggregate, error) {
	row := store.connPool.QueryRow(ctx, getReflectionAggregateForChild, arg.ChildID, arg.StartTime)
	var r ReflectionAggregate
	err := row.Scan(&r.TotalDelivered, &r.TotalResponded, &r.CompletionRate)
	return r, err
}

const getRecentReflectionResponsesForChild = `
SELECT response_text, delivered_at
FROM reflections
WHERE child_id = $1
  AND delivered_at >= $2
  AND response_text IS NOT NULL
ORDER BY delivered_at DESC
LIMIT 10
`

func (store *SQLStore) GetRecentReflectionResponsesForChild(ctx context.Context, arg GetInsightAggregateParams) ([]ReflectionResponseRow, error) {
	rows, err := store.connPool.Query(ctx, getRecentReflectionResponsesForChild, arg.ChildID, arg.StartTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ReflectionResponseRow
	for rows.Next() {
		var r ReflectionResponseRow
		if err := rows.Scan(&r.ResponseText, &r.DeliveredAt); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

// ─── Quiz aggregation ────────────────────────────────────────────────────────

const getQuizAggregateForChild = `
SELECT
  COUNT(*)::bigint                                                AS total_attempts,
  COUNT(CASE WHEN passed THEN 1 END)::bigint                     AS pass_count,
  COUNT(CASE WHEN NOT passed THEN 1 END)::bigint                 AS fail_count,
  COALESCE(AVG(score::float8), 0)::float8                        AS avg_score
FROM child_quiz_attempts
WHERE child_id = $1 AND created_at >= $2
`

func (store *SQLStore) GetQuizAggregateForChild(ctx context.Context, arg GetInsightAggregateParams) (QuizAggregate, error) {
	row := store.connPool.QueryRow(ctx, getQuizAggregateForChild, arg.ChildID, arg.StartTime)
	var r QuizAggregate
	err := row.Scan(&r.TotalAttempts, &r.PassCount, &r.FailCount, &r.AvgScore)
	return r, err
}

// ─── Compile-time guard: ensure all new methods are on *SQLStore ─────────────
// (unmarshalling utility used by the service layer)

// UnmarshalSnapshotData is a convenience helper to parse the JSON blob stored in
// a snapshot back into a Go value.
func UnmarshalSnapshotData(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// snapshotAge returns the age of a snapshot relative to now.
func SnapshotAge(s AiInsightsSnapshot) time.Duration {
	return time.Since(s.GeneratedAt)
}

// ensure uuid is imported (used indirectly via ContentMonitoringEvent.ID)
var _ = uuid.Nil
