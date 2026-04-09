-- name: CreateDailyIntention :one
INSERT INTO daily_intentions (
  child_id,
  intention_text,
  intention_date,
  time_limit_minutes,
  specific_goals
) VALUES (
  $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetTodayIntention :one
SELECT * FROM daily_intentions
WHERE child_id = $1
  AND intention_date = CURRENT_DATE
  AND is_active = true
LIMIT 1;

-- name: GetIntentionByID :one
SELECT * FROM daily_intentions WHERE id = $1 LIMIT 1;

-- name: DeactivateIntention :exec
UPDATE daily_intentions
SET is_active = false, updated_at = now()
WHERE id = $1;

-- name: DeactivateTodayIntentionsForChild :exec
UPDATE daily_intentions
SET is_active = false, updated_at = now()
WHERE child_id = $1
  AND intention_date = CURRENT_DATE
  AND is_active = true;

-- name: GetIntentionHistory :many
SELECT * FROM daily_intentions
WHERE child_id = $1
  AND ($2::date IS NULL OR intention_date >= $2)
  AND ($3::date IS NULL OR intention_date <= $3)
ORDER BY intention_date DESC, created_at DESC
LIMIT $4 OFFSET $5;
