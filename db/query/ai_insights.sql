-- name: UpsertInsightsSnapshot :one
INSERT INTO ai_insights_snapshots (child_id, range_days, snapshot_data, model_version, data_freshness, generated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (child_id, range_days)
DO UPDATE SET
  snapshot_data  = EXCLUDED.snapshot_data,
  model_version  = EXCLUDED.model_version,
  data_freshness = EXCLUDED.data_freshness,
  generated_at   = now()
RETURNING id, child_id, range_days, snapshot_data, model_version, data_freshness, generated_at, created_at;

-- name: GetInsightsSnapshot :one
SELECT id, child_id, range_days, snapshot_data, model_version, data_freshness, generated_at, created_at
FROM ai_insights_snapshots
WHERE child_id = $1 AND range_days = $2;

-- name: MarkSnapshotStale :exec
UPDATE ai_insights_snapshots
SET data_freshness = 'stale'
WHERE child_id = $1;

-- name: GetAppSessionAggregateForChild :many
-- Returns per-app totals for the child over a date range.
SELECT
  sm.name                                                                                  AS app_name,
  COUNT(s.id)::bigint                                                                      AS session_count,
  COALESCE(SUM(EXTRACT(EPOCH FROM
    (COALESCE(s.end_time, s.last_ping_time) - s.start_time)) / 60), 0)::float8            AS total_minutes,
  COALESCE(AVG(EXTRACT(EPOCH FROM
    (COALESCE(s.end_time, s.last_ping_time) - s.start_time)) / 60), 0)::float8            AS avg_session_minutes
FROM app_sessions s
JOIN social_medias sm ON sm.id = s.social_media_id
WHERE s.child_id = $1
  AND s.start_time >= $2
  AND (s.end_time IS NOT NULL OR s.last_ping_time IS NOT NULL)
GROUP BY sm.name;

-- name: GetOverLimitSessionCount :one
SELECT COUNT(*)::bigint FROM app_sessions
WHERE child_id = $1
  AND start_time >= $2
  AND status = 'expired';

-- name: GetNightSessionCount :one
-- Sessions started between 21:00 and 06:00 local (UTC stored, UI applies offset).
SELECT COUNT(*)::bigint FROM app_sessions
WHERE child_id = $1
  AND start_time >= $2
  AND (EXTRACT(HOUR FROM start_time) >= 21 OR EXTRACT(HOUR FROM start_time) < 6);

-- name: GetReflectionAggregateForChild :one
SELECT
  COUNT(*)::bigint                                               AS total_delivered,
  COUNT(responded_at)::bigint                                   AS total_responded,
  CASE WHEN COUNT(*) > 0
    THEN COUNT(responded_at)::float8 / COUNT(*)::float8
    ELSE 0 END                                                   AS completion_rate
FROM reflections
WHERE child_id = $1
  AND delivered_at >= $2;

-- name: GetRecentReflectionResponsesForChild :many
SELECT response_text, delivered_at
FROM reflections
WHERE child_id = $1
  AND delivered_at >= $2
  AND response_text IS NOT NULL
ORDER BY delivered_at DESC
LIMIT 10;

-- name: GetQuizAggregateForChild :one
SELECT
  COUNT(*)::bigint                                               AS total_attempts,
  COUNT(CASE WHEN passed THEN 1 END)::bigint                    AS pass_count,
  COUNT(CASE WHEN NOT passed THEN 1 END)::bigint                AS fail_count,
  COALESCE(AVG(score::float8), 0)::float8                       AS avg_score
FROM child_quiz_attempts
WHERE child_id = $1
  AND created_at >= $2;
