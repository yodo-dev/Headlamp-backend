-- name: GetChild :one
SELECT * FROM children
WHERE id = $1 LIMIT 1;

-- name: GetChildrenByFamilyID :many
SELECT * FROM children
WHERE family_id = $1
ORDER BY created_at;

-- name: CreateChild :one
INSERT INTO children (
    id,
    family_id,
    first_name,
    surname,
    age,
    gender,
    profile_image_url
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: DeleteChild :exec
DELETE FROM children
WHERE id = $1;
