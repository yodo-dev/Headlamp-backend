-- name: GetChildByIDAndFamilyID :one
SELECT * FROM children
WHERE id = $1 AND family_id = $2 AND deleted_at IS NULL;

-- name: UpdateChild :one
UPDATE children
SET 
    first_name = COALESCE(sqlc.narg(first_name), first_name),
    surname = COALESCE(sqlc.narg(surname), surname),
    age = COALESCE(sqlc.narg(age), age),
    gender = COALESCE(sqlc.narg(gender), gender),
    profile_image_url = COALESCE(sqlc.narg(profile_image_url), profile_image_url),
    push_notifications_enabled = COALESCE(sqlc.narg(push_notifications_enabled), push_notifications_enabled),
    updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;
