-- name: CreateDevice :one
INSERT INTO devices (
  user_id,
  user_type,
  device_id,
  push_token,
  provider
) VALUES (
  $1, $2, $3, $4, $5
) RETURNING *;

-- name: UpdateDevicePushToken :one
UPDATE devices
SET 
    push_token = $3,
    provider = $4
WHERE user_id = $1 AND device_id = $2
RETURNING *;

-- name: GetDeviceByDeviceID :one
SELECT * FROM devices
WHERE device_id = $1 LIMIT 1;

-- name: ListDevicesByUser :many
SELECT * FROM devices
WHERE user_id = $1
ORDER BY created_at;

-- name: DeleteDeviceByID :exec
DELETE FROM devices
WHERE device_id = $1;

-- name: DeactivateUserDevices :exec
UPDATE devices
SET activated_at = NULL
WHERE user_id = $1 AND activated_at IS NOT NULL;

-- name: GetActiveDeviceByDeviceID :one
SELECT * FROM devices
WHERE device_id = $1 AND activated_at IS NOT NULL;

-- name: GetActiveDeviceByUserAndDeviceID :one
SELECT * FROM devices
WHERE user_id = $1 AND device_id = $2 AND activated_at IS NOT NULL;

-- name: DeactivateDevicesForUser :exec
UPDATE devices
SET activated_at = NULL
WHERE user_id = $1;

-- name: ActivateDevice :one
UPDATE devices
SET activated_at = now()
WHERE device_id = $1 AND user_id = $2
RETURNING *;

-- name: ListPushTokensForUser :many
SELECT push_token FROM devices
WHERE user_id = $1 AND push_token IS NOT NULL;
