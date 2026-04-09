-- name: GetChildForParent :one
SELECT c.* FROM children c
JOIN parents p ON c.family_id = p.family_id
WHERE p.parent_id = $1 AND c.id = $2;

-- name: UpdateParent :one
UPDATE parents
SET
  firstname = COALESCE(sqlc.narg(firstname), firstname),
  surname = COALESCE(sqlc.narg(surname), surname),
  push_notifications_enabled = COALESCE(sqlc.narg(push_notifications_enabled), push_notifications_enabled),
  updated_at = now()
WHERE
  parent_id = sqlc.arg(parent_id)
RETURNING *;
