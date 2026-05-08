-- Ensure parent_daily_insights exists even if earlier migrations were skipped/forced.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS parent_daily_insights (
  id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
  parent_id       varchar     NOT NULL REFERENCES parents(parent_id) ON DELETE CASCADE,
  child_id        varchar     NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  date            date        NOT NULL DEFAULT CURRENT_DATE,
  insight_content jsonb       NOT NULL,
  overall_tone    varchar     NOT NULL DEFAULT 'neutral',
  is_read         boolean     NOT NULL DEFAULT false,
  generated_at    timestamptz NOT NULL DEFAULT now(),
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (parent_id, child_id, date)
);

CREATE INDEX IF NOT EXISTS idx_parent_daily_insights_parent_child_generated
  ON parent_daily_insights (parent_id, child_id, generated_at DESC);

CREATE INDEX IF NOT EXISTS idx_parent_daily_insights_parent_child_date
  ON parent_daily_insights (parent_id, child_id, date DESC);
