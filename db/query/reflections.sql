-- name: CreateReflection :one
INSERT INTO reflections (
  child_id,
  trigger_type,
  trigger_event_id,
  prompt_content,
  metadata
) VALUES (
  $1, $2, $3, $4, $5
) RETURNING *;

-- name: CreateTimedSessionReflection :one
-- Used when a timer-based app_session expires. trigger_event_id is omitted
-- because it references social_media_sessions, not app_sessions.
INSERT INTO reflections (
  child_id,
  trigger_type,
  prompt_content,
  metadata
) VALUES (
  $1, $2, $3, $4
) RETURNING *;

-- name: GetReflection :one
SELECT * FROM reflections WHERE id = $1 LIMIT 1;

-- name: GetTodayDailyReflectionForChild :one
SELECT * FROM reflections
WHERE child_id = $1
  AND trigger_type = 'daily_scheduled'
  AND DATE(delivered_at) = CURRENT_DATE
ORDER BY delivered_at DESC
LIMIT 1;

-- name: CountPostSessionReflectionsToday :one
SELECT COUNT(*) FROM reflections
WHERE child_id = $1
  AND trigger_type = 'post_session'
  AND DATE(delivered_at) = CURRENT_DATE;

-- name: GetPendingReflectionsForChild :many
SELECT * FROM reflections
WHERE child_id = $1
  AND responded_at IS NULL
ORDER BY delivered_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateReflectionTextResponse :one
UPDATE reflections
SET
  response_text  = $2,
  response_type  = $3,
  responded_at   = now(),
  updated_at     = now()
WHERE id = $1
RETURNING *;

-- name: UpdateReflectionMediaResponse :one
UPDATE reflections
SET
  response_media_url = $2,
  response_type      = $3,
  responded_at       = now(),
  updated_at         = now()
WHERE id = $1
RETURNING *;

-- name: AcknowledgeReflection :exec
UPDATE reflections
SET
  is_acknowledged         = true,
  acknowledgment_feedback = $2,
  updated_at              = now()
WHERE id = $1;

-- name: GetReflectionHistory :many
SELECT * FROM reflections
WHERE child_id = $1
  AND ($2::reflection_trigger_type IS NULL OR trigger_type = $2)
  AND ($3::boolean IS NULL OR responded_at IS NOT NULL = $3)
  AND ($4::timestamptz IS NULL OR delivered_at >= $4)
  AND ($5::timestamptz IS NULL OR delivered_at <= $5)
ORDER BY delivered_at DESC
LIMIT $6 OFFSET $7;

-- name: GetReflectionStats :one
SELECT
  COUNT(*)                                                                AS total_reflections,
  COUNT(CASE WHEN responded_at IS NOT NULL THEN 1 END)                   AS total_responded,
  COUNT(CASE WHEN is_acknowledged = true THEN 1 END)                     AS total_acknowledged,
  COUNT(CASE WHEN trigger_type = 'daily_scheduled' THEN 1 END)           AS total_daily,
  COUNT(CASE WHEN trigger_type = 'post_session' THEN 1 END)              AS total_post_session,
  AVG(EXTRACT(EPOCH FROM (responded_at - delivered_at))/60)::numeric(10,2) AS avg_response_time_minutes
FROM reflections
WHERE child_id = $1
  AND ($2::timestamptz IS NULL OR delivered_at >= $2)
  AND ($3::timestamptz IS NULL OR delivered_at <= $3);

-- name: GetRecentReflectionsForChild :many
SELECT * FROM reflections
WHERE child_id = $1
ORDER BY delivered_at DESC
LIMIT $2;

-- name: GetChildrenNeedingDailyReflection :many
SELECT c.id AS child_id, c.first_name, c.age
FROM children c
WHERE c.age >= 13
  AND NOT EXISTS (
    SELECT 1 FROM reflections r
    WHERE r.child_id = c.id
      AND r.trigger_type = 'daily_scheduled'
      AND DATE(r.delivered_at) = CURRENT_DATE
  );

-- name: GetAllEligibleChildrenForReflection :many
-- Returns all children aged 13+ regardless of whether they already have a
-- reflection today. Used in test mode to bypass daily idempotency.
SELECT c.id AS child_id, c.first_name, c.age
FROM children c
WHERE c.age >= 13;

-- name: GetRecentDailyReflectionsWithResponses :many
-- Returns the last 10 daily reflections that the child responded to,
-- used to give GPT conversational context across days.
SELECT prompt_content, response_text, delivered_at
FROM reflections
WHERE child_id = $1
  AND trigger_type = 'daily_scheduled'
  AND responded_at IS NOT NULL
ORDER BY delivered_at DESC
LIMIT 10;
