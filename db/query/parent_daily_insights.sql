-- name: CreateParentDailyInsight :one
INSERT INTO parent_daily_insights (
  parent_id,
  child_id,
  date,
  insight_content,
  overall_tone
) VALUES (
  $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetTodayParentInsightForChild :one
SELECT * FROM parent_daily_insights
WHERE parent_id = $1
  AND child_id  = $2
  AND date      = CURRENT_DATE
LIMIT 1;

-- name: GetParentInsightHistory :many
SELECT * FROM parent_daily_insights
WHERE parent_id = $1
  AND child_id  = $2
ORDER BY date DESC
LIMIT  $3
OFFSET $4;

-- name: MarkParentInsightRead :one
UPDATE parent_daily_insights
SET is_read = true
WHERE id        = $1
  AND parent_id = $2
RETURNING *;

-- name: GetAllChildrenForParentInsightScheduler :many
-- Returns distinct (parent_id, child_id) pairs that do NOT yet have an insight
-- row for today. Used by the nightly cron to determine what to generate.
SELECT DISTINCT p.parent_id, c.id AS child_id
FROM parents p
JOIN children c ON c.family_id = p.family_id
WHERE NOT EXISTS (
    SELECT 1 FROM parent_daily_insights pdi
    WHERE pdi.parent_id = p.parent_id
      AND pdi.child_id  = c.id
      AND pdi.date      = CURRENT_DATE
  );
