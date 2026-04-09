-- name: CreateParentSession :one
INSERT INTO parent_sessions (
  id,
  parent_id,
  refresh_token,
  user_agent,
  client_ip,
  is_blocked,
  expires_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetParentSession :one
SELECT * FROM parent_sessions
WHERE id = $1 LIMIT 1;
