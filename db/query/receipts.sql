-- name: CreateReceipt :one
INSERT INTO receipts (
  subscription_id,
  amount,
  currency,
  provider_receipt_id
) VALUES (
  $1, $2, $3, $4
) RETURNING *;

-- name: ListReceiptsBySubscription :many
SELECT * FROM receipts
WHERE subscription_id = $1
ORDER BY issued_at DESC;
