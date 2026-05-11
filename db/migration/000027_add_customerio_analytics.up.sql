CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE analytics_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_event_id varchar UNIQUE,
  event_type varchar NOT NULL,
  event_name varchar NOT NULL DEFAULT '',
  person_id varchar NOT NULL,
  user_id varchar NOT NULL,
  role varchar NOT NULL,
  session_id varchar,
  child_id varchar,
  event_time timestamptz NOT NULL DEFAULT now(),
  payload jsonb NOT NULL,
  sync_status varchar NOT NULL DEFAULT 'pending',
  attempt_count int NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  last_error text,
  synced_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_analytics_events_pending
  ON analytics_events (sync_status, next_attempt_at, created_at);

CREATE INDEX idx_analytics_events_person_id
  ON analytics_events (person_id, event_time DESC);

CREATE TABLE customerio_webhook_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type varchar NOT NULL,
  signature varchar,
  payload jsonb NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now()
);