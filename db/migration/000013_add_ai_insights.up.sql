-- ai_insights_snapshots stores pre-computed, cached insight responses per child per time window.
-- Rows are upserted on recomputation; the unique constraint (child_id, range_days) acts as the
-- cache key.  TTL enforcement is done at the application layer.
CREATE TABLE "ai_insights_snapshots" (
  "id"             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
  "child_id"       varchar     NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  "range_days"     int         NOT NULL,          -- 1, 7, or 30
  "snapshot_data"  jsonb       NOT NULL,          -- full DashboardInsightsResponse JSON
  "model_version"  varchar     NOT NULL DEFAULT 'ai-insights-v1',
  "data_freshness" varchar     NOT NULL DEFAULT 'fresh', -- fresh | delayed | stale
  "generated_at"   timestamptz NOT NULL DEFAULT now(),
  "created_at"     timestamptz NOT NULL DEFAULT now(),
  UNIQUE ("child_id", "range_days")
);

CREATE INDEX ON "ai_insights_snapshots" ("child_id", "generated_at" DESC);

-- content_monitoring_events stores policy-compliant risk event metadata reported from
-- the mobile client or detected by backend pipelines.  Raw content is never stored.
CREATE TABLE "content_monitoring_events" (
  "id"              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
  "child_id"        varchar     NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  "platform"        varchar     NOT NULL,              -- e.g. Instagram, TikTok
  "category"        varchar     NOT NULL,              -- toxicity | sexual_content | violence | self_harm | bullying | misinformation
  "severity"        varchar     NOT NULL,              -- low | medium | high
  "event_timestamp" timestamptz NOT NULL DEFAULT now(),
  "metadata"        jsonb       NOT NULL DEFAULT '{}', -- minimum necessary metadata only
  "created_at"      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ON "content_monitoring_events" ("child_id", "event_timestamp" DESC);
CREATE INDEX ON "content_monitoring_events" ("child_id", "severity");
CREATE INDEX ON "content_monitoring_events" ("child_id", "category");
