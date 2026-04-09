-- name: LogChildActivity :one
INSERT INTO child_activity_log (
  child_id,
  activity_type,
  activity_ref_id
) VALUES (
  $1, $2, $3
)
ON CONFLICT (child_id, activity_type, activity_ref_id) DO NOTHING
RETURNING *;

-- name: CheckChildActivityExists :one
SELECT EXISTS(
  SELECT 1 FROM child_activity_log
  WHERE child_id = $1 AND activity_type = $2 AND activity_ref_id = $3
);
