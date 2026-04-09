-- name: GetChildReflectionContext :one
SELECT * FROM child_reflection_context WHERE child_id = $1 LIMIT 1;

-- name: UpsertChildReflectionContext :one
INSERT INTO child_reflection_context (
  child_id,
  total_modules_completed,
  total_quizzes_taken,
  average_quiz_score,
  digital_permit_status,
  digital_permit_score,
  last_activity_date,
  recent_activities,
  completed_module_ids,
  total_sm_sessions,
  avg_daily_sm_minutes,
  most_used_apps,
  frequent_content_categories,
  last_session_end,
  last_reflection_acknowledged,
  reflection_streak,
  total_reflections_responded,
  total_reflections_delivered,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, now()
)
ON CONFLICT (child_id) DO UPDATE SET
  total_modules_completed      = EXCLUDED.total_modules_completed,
  total_quizzes_taken          = EXCLUDED.total_quizzes_taken,
  average_quiz_score           = EXCLUDED.average_quiz_score,
  digital_permit_status        = EXCLUDED.digital_permit_status,
  digital_permit_score         = EXCLUDED.digital_permit_score,
  last_activity_date           = EXCLUDED.last_activity_date,
  recent_activities            = EXCLUDED.recent_activities,
  completed_module_ids         = EXCLUDED.completed_module_ids,
  total_sm_sessions            = EXCLUDED.total_sm_sessions,
  avg_daily_sm_minutes         = EXCLUDED.avg_daily_sm_minutes,
  most_used_apps               = EXCLUDED.most_used_apps,
  frequent_content_categories  = EXCLUDED.frequent_content_categories,
  last_session_end             = EXCLUDED.last_session_end,
  last_reflection_acknowledged = EXCLUDED.last_reflection_acknowledged,
  reflection_streak            = EXCLUDED.reflection_streak,
  total_reflections_responded  = EXCLUDED.total_reflections_responded,
  total_reflections_delivered  = EXCLUDED.total_reflections_delivered,
  updated_at                   = now()
RETURNING *;

-- name: IncrementReflectionsDelivered :exec
UPDATE child_reflection_context
SET total_reflections_delivered = total_reflections_delivered + 1, updated_at = now()
WHERE child_id = $1;

-- name: IncrementReflectionsResponded :exec
UPDATE child_reflection_context
SET total_reflections_responded = total_reflections_responded + 1, updated_at = now()
WHERE child_id = $1;

-- name: UpdateReflectionAcknowledgment :exec
UPDATE child_reflection_context
SET
  last_reflection_acknowledged = $2,
  updated_at                   = now()
WHERE child_id = $1;
