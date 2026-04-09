-- name: CreateDeepLinkCode :one
INSERT INTO deep_link_codes (
  family_id,
  child_id,
  code,
  expires_at
) VALUES (
  $1, $2, $3, $4
) RETURNING *;

-- name: GetDeepLinkCode :one
SELECT * FROM deep_link_codes
WHERE code = $1 LIMIT 1;

-- name: GetDeepLinkCodeByChildID :one
SELECT * FROM deep_link_codes
WHERE child_id = $1 LIMIT 1;

-- name: UpdateDeepLinkCode :one
UPDATE deep_link_codes
SET
  code = $2,
  expires_at = $3,
  is_used = FALSE
WHERE id = $1
RETURNING *;

-- name: UseDeepLinkCode :one
UPDATE deep_link_codes
SET is_used = TRUE
WHERE code = $1
RETURNING *;
