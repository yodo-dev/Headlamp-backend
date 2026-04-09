-- name: CreateOrUpdateModuleProgress :one
INSERT INTO child_module_progress (
  child_id,
  course_id,
  module_id,
  score,
  is_completed,
  feedback_video_url
)
VALUES (
  $1, $2, $3, $4, $5, $6
)
ON CONFLICT (child_id, module_id, course_id)
DO UPDATE SET 
  score = GREATEST(COALESCE(child_module_progress.score, 0), EXCLUDED.score),
  is_completed = child_module_progress.is_completed OR EXCLUDED.is_completed,
  feedback_video_url = EXCLUDED.feedback_video_url,
  last_attempted_at = now()
RETURNING *;

-- name: GetChildModuleProgress :one
SELECT * FROM child_module_progress
WHERE child_id = $1 AND module_id = $2 AND course_id = $3;

-- name: GetChildModuleProgressForCourse :many
SELECT module_id, is_completed FROM child_module_progress
WHERE child_id = $1 AND course_id = $2 AND module_id = ANY(sqlc.arg(module_ids)::text[]);

