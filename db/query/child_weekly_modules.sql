-- name: UpdateChildWeeklyModuleProgress :one
UPDATE child_weekly_modules
SET 
  completed_at = $1,
  latest_score = $2
WHERE child_id = $3 AND external_module_id = $4
RETURNING *;

-- name: GetChildWeeklyModule :one
SELECT * FROM child_weekly_modules
WHERE child_id = $1 AND external_module_id = $2;
