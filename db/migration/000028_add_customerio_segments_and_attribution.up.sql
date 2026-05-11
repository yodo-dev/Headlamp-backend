CREATE TABLE user_segments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  person_id varchar NOT NULL,
  user_id varchar NOT NULL,
  role varchar NOT NULL,
  segment_name varchar NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  source varchar NOT NULL DEFAULT 'system',
  assigned_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(person_id, segment_name)
);

CREATE INDEX idx_user_segments_person_active
  ON user_segments (person_id, expires_at, assigned_at DESC);

CREATE TABLE customerio_event_attributions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  webhook_event_id uuid REFERENCES customerio_webhook_events(id) ON DELETE SET NULL,
  event_type varchar NOT NULL,
  person_id varchar,
  campaign_id varchar,
  message_id varchar,
  delivery_id varchar,
  link_id varchar,
  action varchar,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_customerio_event_attributions_person_time
  ON customerio_event_attributions (person_id, occurred_at DESC);

CREATE INDEX idx_customerio_event_attributions_event_type
  ON customerio_event_attributions (event_type, occurred_at DESC);