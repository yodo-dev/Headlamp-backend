-- name: CreateSubscription :one
INSERT INTO subscriptions (
  family_id,
  provider,
  provider_subscription_id,
  status,
  plan,
  current_period_end
) VALUES (
  $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetSubscriptionByFamilyID :one
SELECT * FROM subscriptions
WHERE family_id = $1 LIMIT 1;

-- name: UpdateSubscriptionStatus :one
UPDATE subscriptions
SET status = $2, updated_at = now()
WHERE id = $1
RETURNING *;
