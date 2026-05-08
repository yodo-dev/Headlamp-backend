-- Ensure reflections subsystem schema exists even if migration 000011 was skipped/forced.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'reflection_trigger_type') THEN
    CREATE TYPE reflection_trigger_type AS ENUM ('daily_scheduled', 'post_session', 'manual');
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'reflection_response_type') THEN
    CREATE TYPE reflection_response_type AS ENUM ('text', 'video', 'audio');
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS reflections (
  id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  child_id                varchar NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  trigger_type            reflection_trigger_type NOT NULL,
  trigger_event_id        uuid,
  prompt_content          jsonb NOT NULL,
  response_text           text,
  response_media_url      text,
  response_type           reflection_response_type,
  responded_at            timestamptz,
  is_acknowledged         boolean NOT NULL DEFAULT false,
  acknowledgment_feedback text,
  delivered_at            timestamptz NOT NULL DEFAULT now(),
  metadata                jsonb NOT NULL DEFAULT '{}',
  created_at              timestamptz NOT NULL DEFAULT now(),
  updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS daily_intentions (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  child_id           varchar NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  intention_text     text NOT NULL,
  intention_date     date NOT NULL DEFAULT CURRENT_DATE,
  time_limit_minutes int,
  specific_goals     jsonb NOT NULL DEFAULT '[]',
  is_active          boolean NOT NULL DEFAULT true,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (child_id, intention_date)
);

CREATE TABLE IF NOT EXISTS journal_entries (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  child_id   varchar NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  entry_date date NOT NULL DEFAULT CURRENT_DATE,
  entry_text text NOT NULL,
  mood       text,
  tags       text[] NOT NULL DEFAULT '{}',
  media_urls text[] NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS social_media_sessions (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  child_id             varchar NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  social_media_id      bigint NOT NULL REFERENCES social_medias(id),
  intention_id         uuid REFERENCES daily_intentions(id) ON DELETE SET NULL,
  session_start        timestamptz NOT NULL DEFAULT now(),
  session_end          timestamptz,
  duration_minutes     int,
  content_categories   text[] NOT NULL DEFAULT '{}',
  interaction_count    int NOT NULL DEFAULT 0,
  reflection_triggered boolean NOT NULL DEFAULT false,
  reflection_id        uuid REFERENCES reflections(id) ON DELETE SET NULL,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'reflections_trigger_event_id_fkey') THEN
    ALTER TABLE reflections
      ADD CONSTRAINT reflections_trigger_event_id_fkey
      FOREIGN KEY (trigger_event_id)
      REFERENCES social_media_sessions(id)
      ON DELETE SET NULL;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS child_reflection_context (
  child_id                     varchar PRIMARY KEY REFERENCES children(id) ON DELETE CASCADE,
  total_modules_completed      int NOT NULL DEFAULT 0,
  total_quizzes_taken          int NOT NULL DEFAULT 0,
  average_quiz_score           numeric(5,2),
  digital_permit_status        text,
  digital_permit_score         numeric(5,2),
  last_activity_date           timestamptz,
  recent_activities            jsonb NOT NULL DEFAULT '[]',
  completed_module_ids         text[] NOT NULL DEFAULT '{}',
  total_sm_sessions            int NOT NULL DEFAULT 0,
  avg_daily_sm_minutes         numeric(6,2) NOT NULL DEFAULT 0,
  most_used_apps               jsonb NOT NULL DEFAULT '[]',
  frequent_content_categories  text[] NOT NULL DEFAULT '{}',
  last_session_end             timestamptz,
  last_reflection_acknowledged boolean NOT NULL DEFAULT false,
  reflection_streak            int NOT NULL DEFAULT 0,
  total_reflections_responded  int NOT NULL DEFAULT 0,
  total_reflections_delivered  int NOT NULL DEFAULT 0,
  updated_at                   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_reflections_child_trigger_type
  ON reflections (child_id, trigger_type);

CREATE INDEX IF NOT EXISTS idx_reflections_child_delivered_desc
  ON reflections (child_id, delivered_at DESC);

CREATE INDEX IF NOT EXISTS idx_reflections_child_pending
  ON reflections (child_id, responded_at)
  WHERE responded_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_daily_intentions_child_date_desc
  ON daily_intentions (child_id, intention_date DESC);

CREATE INDEX IF NOT EXISTS idx_journal_entries_child_date_desc
  ON journal_entries (child_id, entry_date DESC);

CREATE INDEX IF NOT EXISTS idx_social_media_sessions_child_start_desc
  ON social_media_sessions (child_id, session_start DESC);

CREATE INDEX IF NOT EXISTS idx_social_media_sessions_child_open
  ON social_media_sessions (child_id, social_media_id)
  WHERE session_end IS NULL;
