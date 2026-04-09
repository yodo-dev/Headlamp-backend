-- name: CreateSocialMediaSession :one
INSERT INTO social_media_sessions (
  child_id,
  social_media_id,
  intention_id,
  session_start,
  content_categories,
  interaction_count
) VALUES (
  $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetSocialMediaSession :one
SELECT * FROM social_media_sessions WHERE id = $1 LIMIT 1;

-- name: GetActiveSocialMediaSession :one
SELECT * FROM social_media_sessions
WHERE child_id = $1
  AND social_media_id = $2
  AND session_end IS NULL
ORDER BY session_start DESC
LIMIT 1;

-- name: EndSocialMediaSession :one
UPDATE social_media_sessions
SET
  session_end        = now(),
  duration_minutes   = GREATEST(1, EXTRACT(EPOCH FROM (now() - session_start))::int / 60),
  content_categories = CASE WHEN array_length($2::text[], 1) > 0 THEN $2::text[] ELSE content_categories END,
  interaction_count  = CASE WHEN $3::int > 0 THEN $3::int ELSE interaction_count END,
  updated_at         = now()
WHERE id = $1
RETURNING *;

-- name: MarkSessionReflectionTriggered :exec
UPDATE social_media_sessions
SET
  reflection_triggered = true,
  reflection_id        = $2,
  updated_at           = now()
WHERE id = $1;

-- name: GetRecentSessionsNeedingReflection :many
SELECT * FROM social_media_sessions
WHERE child_id = $1
  AND session_end IS NOT NULL
  AND reflection_triggered = false
  AND session_end >= now() - INTERVAL '2 hours'
ORDER BY session_end DESC;

-- name: GetDailyUsageStats :many
SELECT
  DATE(session_start)              AS usage_date,
  SUM(duration_minutes)::int       AS total_minutes,
  COUNT(*)                         AS session_count,
  array_agg(DISTINCT social_media_id) AS apps_used
FROM social_media_sessions
WHERE child_id      = $1
  AND session_start >= $2
  AND session_start <= $3
  AND session_end IS NOT NULL
GROUP BY DATE(session_start)
ORDER BY usage_date DESC;

-- name: GetSessionsByChild :many
SELECT * FROM social_media_sessions
WHERE child_id = $1
ORDER BY session_start DESC
LIMIT $2 OFFSET $3;
