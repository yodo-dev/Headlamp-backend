CREATE TABLE mobile_ui_config_cache (
    id BIGSERIAL PRIMARY KEY,
    cache_key TEXT NOT NULL UNIQUE,
    config_version TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_stale BOOLEAN NOT NULL DEFAULT FALSE
);
