-- name: CreateParent :one
INSERT INTO parents (
  parent_id,
  family_id,
  firstname,
  surname,
  email,
  hashed_password,
  auth_provider,
  provider_subject,
  email_verified
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetParentByParentID :one
SELECT * FROM parents
WHERE parent_id = $1 LIMIT 1;

-- name: GetParentByEmail :one
SELECT * FROM parents
WHERE email = $1 LIMIT 1;

-- name: GetParentByProvider :one
SELECT * FROM parents
WHERE auth_provider = $1 AND provider_subject = $2 LIMIT 1;

-- name: LinkParentProvider :one
UPDATE parents
SET 
  provider_subject = $2,
  auth_provider = $3,
  email_verified = $4,
  updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetParentByFamilyID :one
SELECT * FROM parents
WHERE family_id = $1
LIMIT 1;

