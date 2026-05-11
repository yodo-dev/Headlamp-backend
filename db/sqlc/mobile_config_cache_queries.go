package db

import (
	"context"
	"time"
)

type MobileConfigCacheRecord struct {
	ID            int64
	CacheKey      string
	ConfigVersion time.Time
	Payload       []byte
	FetchedAt     time.Time
	IsStale       bool
}

type UpsertMobileConfigCacheParams struct {
	CacheKey      string
	ConfigVersion time.Time
	Payload       []byte
}

const upsertMobileConfigCache = `
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
RETURNING id, cache_key, config_version, payload, fetched_at, is_stale
`

func (store *SQLStore) UpsertMobileConfigCache(ctx context.Context, arg UpsertMobileConfigCacheParams) (MobileConfigCacheRecord, error) {
	row := store.connPool.QueryRow(ctx, upsertMobileConfigCache, arg.CacheKey, arg.ConfigVersion, arg.Payload)
	var out MobileConfigCacheRecord
	err := row.Scan(
		&out.ID,
		&out.CacheKey,
		&out.ConfigVersion,
		&out.Payload,
		&out.FetchedAt,
		&out.IsStale,
	)
	return out, err
}

const getMobileConfigCacheByKey = `
SELECT id, cache_key, config_version, payload, fetched_at, is_stale
FROM mobile_ui_config_cache
WHERE cache_key = $1
`

func (store *SQLStore) GetMobileConfigCacheByKey(ctx context.Context, cacheKey string) (MobileConfigCacheRecord, error) {
	row := store.connPool.QueryRow(ctx, getMobileConfigCacheByKey, cacheKey)
	var out MobileConfigCacheRecord
	err := row.Scan(
		&out.ID,
		&out.CacheKey,
		&out.ConfigVersion,
		&out.Payload,
		&out.FetchedAt,
		&out.IsStale,
	)
	return out, err
}

const markAllMobileConfigCacheStale = `
UPDATE mobile_ui_config_cache
SET is_stale = TRUE
`

func (store *SQLStore) MarkAllMobileConfigCacheStale(ctx context.Context) error {
	_, err := store.connPool.Exec(ctx, markAllMobileConfigCacheStale)
	return err
}
