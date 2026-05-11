-- name: UpsertMobileConfigCache :one
INSERT INTO mobile_ui_config_cache (
  cache_key,
  config_version,
  payload,
  fetched_at,
  is_stale
)
VALUES ($1, $2, $3, NOW(), FALSE)
ON CONFLICT (cache_key)
DO UPDATE SET
  config_version = EXCLUDED.config_version,
  payload = EXCLUDED.payload,
  fetched_at = NOW(),
  is_stale = FALSE
RETURNING id, cache_key, config_version, payload, fetched_at, is_stale;

-- name: GetMobileConfigCacheByKey :one
SELECT id, cache_key, config_version, payload, fetched_at, is_stale
FROM mobile_ui_config_cache
WHERE cache_key = $1;

-- name: MarkAllMobileConfigCacheStale :exec
UPDATE mobile_ui_config_cache
SET is_stale = TRUE;
