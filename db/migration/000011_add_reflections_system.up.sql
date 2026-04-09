-- Enums for reflection system
CREATE TYPE "reflection_trigger_type" AS ENUM (
  'daily_scheduled',
  'post_session',
  'manual'
);

CREATE TYPE "reflection_response_type" AS ENUM (
  'text',
  'video',
  'audio'
);

-- AI-generated reflection prompts with response tracking
CREATE TABLE "reflections" (
  "id"                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "child_id"                varchar NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  "trigger_type"            reflection_trigger_type NOT NULL,
  "trigger_event_id"        uuid,                         -- references social_media_sessions.id (set after table is created)
  "prompt_content"          jsonb NOT NULL,               -- full GPT response JSON
  "response_text"           text,
  "response_media_url"      text,
  "response_type"           reflection_response_type,
  "responded_at"            timestamptz,
  "is_acknowledged"         boolean NOT NULL DEFAULT false,
  "acknowledgment_feedback" text,
  "delivered_at"            timestamptz NOT NULL DEFAULT now(),
  "metadata"                jsonb NOT NULL DEFAULT '{}',
  "created_at"              timestamptz NOT NULL DEFAULT now(),
  "updated_at"              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ON "reflections" ("child_id", "trigger_type");
CREATE INDEX ON "reflections" ("child_id", "delivered_at" DESC);
CREATE INDEX ON "reflections" ("child_id", "responded_at") WHERE responded_at IS NULL;

-- Pre-session goal / intention setting
CREATE TABLE "daily_intentions" (
  "id"                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "child_id"            varchar NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  "intention_text"      text NOT NULL,
  "intention_date"      date NOT NULL DEFAULT CURRENT_DATE,
  "time_limit_minutes"  int,
  "specific_goals"      jsonb NOT NULL DEFAULT '[]',
  "is_active"           boolean NOT NULL DEFAULT true,
  "created_at"          timestamptz NOT NULL DEFAULT now(),
  "updated_at"          timestamptz NOT NULL DEFAULT now(),
  UNIQUE ("child_id", "intention_date")
);

CREATE INDEX ON "daily_intentions" ("child_id", "intention_date" DESC);

-- Free-form journaling
CREATE TABLE "journal_entries" (
  "id"         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "child_id"   varchar NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  "entry_date" date NOT NULL DEFAULT CURRENT_DATE,
  "entry_text" text NOT NULL,
  "mood"       text,
  "tags"       text[] NOT NULL DEFAULT '{}',
  "media_urls" text[] NOT NULL DEFAULT '{}',
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ON "journal_entries" ("child_id", "entry_date" DESC);

-- Categorised social media sessions for reflection triggering
CREATE TABLE "social_media_sessions" (
  "id"                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "child_id"              varchar NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  "social_media_id"       bigint NOT NULL REFERENCES social_medias(id),
  "intention_id"          uuid REFERENCES daily_intentions(id) ON DELETE SET NULL,
  "session_start"         timestamptz NOT NULL DEFAULT now(),
  "session_end"           timestamptz,
  "duration_minutes"      int,
  "content_categories"    text[] NOT NULL DEFAULT '{}',
  "interaction_count"     int NOT NULL DEFAULT 0,
  "reflection_triggered"  boolean NOT NULL DEFAULT false,
  "reflection_id"         uuid REFERENCES reflections(id) ON DELETE SET NULL,
  "created_at"            timestamptz NOT NULL DEFAULT now(),
  "updated_at"            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ON "social_media_sessions" ("child_id", "session_start" DESC);
CREATE INDEX ON "social_media_sessions" ("child_id", "social_media_id") WHERE session_end IS NULL;

-- Now safe to add the FK trigger_event_id → social_media_sessions
ALTER TABLE "reflections"
  ADD CONSTRAINT "reflections_trigger_event_id_fkey"
  FOREIGN KEY ("trigger_event_id") REFERENCES "social_media_sessions"("id") ON DELETE SET NULL;

-- Materialised context cache for GPT prompt building — one row per child
CREATE TABLE "child_reflection_context" (
  "child_id"                      varchar PRIMARY KEY REFERENCES children(id) ON DELETE CASCADE,
  "total_modules_completed"       int NOT NULL DEFAULT 0,
  "total_quizzes_taken"           int NOT NULL DEFAULT 0,
  "average_quiz_score"            numeric(5,2),
  "digital_permit_status"         text,
  "digital_permit_score"          numeric(5,2),
  "last_activity_date"            timestamptz,
  "recent_activities"             jsonb NOT NULL DEFAULT '[]',
  "completed_module_ids"          text[] NOT NULL DEFAULT '{}',
  "total_sm_sessions"             int NOT NULL DEFAULT 0,
  "avg_daily_sm_minutes"          numeric(6,2) NOT NULL DEFAULT 0,
  "most_used_apps"                jsonb NOT NULL DEFAULT '[]',
  "frequent_content_categories"   text[] NOT NULL DEFAULT '{}',
  "last_session_end"              timestamptz,
  "last_reflection_acknowledged"  boolean NOT NULL DEFAULT false,
  "reflection_streak"             int NOT NULL DEFAULT 0,
  "total_reflections_responded"   int NOT NULL DEFAULT 0,
  "total_reflections_delivered"   int NOT NULL DEFAULT 0,
  "updated_at"                    timestamptz NOT NULL DEFAULT now()
);
