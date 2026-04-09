-- name: GetFamily :one
SELECT * FROM families
WHERE id = $1 LIMIT 1;

-- name: CreateFamily :one
INSERT INTO families (
  id,
  private_key,
  public_key
) VALUES (
  $1, $2, $3
) RETURNING *;
