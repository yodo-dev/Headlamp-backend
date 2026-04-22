-- name: CreateNotification :one
INSERT INTO notifications (
  recipient_id,
  recipient_type,
  title,
  message,
  sent_at
) VALUES (
  $1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetNotificationsForRecipient :many
SELECT * FROM notifications
WHERE recipient_id = $1 AND recipient_type = $2
ORDER BY created_at DESC;

-- name: MarkNotificationAsRead :one
UPDATE notifications
SET is_read = TRUE
WHERE id = $1 AND recipient_id = $2
RETURNING *;

-- name: GetNotificationSummary :one
SELECT
  COUNT(*)                              AS total,
  COUNT(*) FILTER (WHERE is_read = FALSE) AS unread,
  COUNT(*) FILTER (WHERE is_read = TRUE)  AS read_count
FROM notifications
WHERE recipient_id = $1 AND recipient_type = $2;

-- name: GetNotificationsForRecipientPaginated :many
SELECT * FROM notifications
WHERE recipient_id = $1 AND recipient_type = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: MarkAllNotificationsAsRead :exec
UPDATE notifications
SET is_read = TRUE
WHERE recipient_id = $1 AND recipient_type = $2 AND is_read = FALSE;

