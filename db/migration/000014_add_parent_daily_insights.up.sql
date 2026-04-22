-- parent_daily_insights stores one GPT-generated insight digest per parent-child pair per day.
-- The unique constraint (parent_id, child_id, date) ensures idempotency.
CREATE TABLE "parent_daily_insights" (
  "id"              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
  "parent_id"       varchar     NOT NULL REFERENCES parents(parent_id) ON DELETE CASCADE,
  "child_id"        varchar     NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  "date"            date        NOT NULL DEFAULT CURRENT_DATE,
  "insight_content" jsonb       NOT NULL,
  "overall_tone"    varchar     NOT NULL DEFAULT 'neutral', -- positive | neutral | needs_attention
  "is_read"         boolean     NOT NULL DEFAULT false,
  "generated_at"    timestamptz NOT NULL DEFAULT now(),
  "created_at"      timestamptz NOT NULL DEFAULT now(),
  UNIQUE ("parent_id", "child_id", "date")
);

CREATE INDEX ON "parent_daily_insights" ("parent_id", "child_id", "generated_at" DESC);
CREATE INDEX ON "parent_daily_insights" ("parent_id", "child_id", "date" DESC);
