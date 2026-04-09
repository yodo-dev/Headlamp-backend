DROP TABLE IF EXISTS "child_reflection_context";

ALTER TABLE "reflections" DROP CONSTRAINT IF EXISTS "reflections_trigger_event_id_fkey";

DROP TABLE IF EXISTS "social_media_sessions";
DROP TABLE IF EXISTS "journal_entries";
DROP TABLE IF EXISTS "daily_intentions";
DROP TABLE IF EXISTS "reflections";

DROP TYPE IF EXISTS "reflection_response_type";
DROP TYPE IF EXISTS "reflection_trigger_type";
