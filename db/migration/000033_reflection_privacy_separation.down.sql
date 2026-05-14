-- Rollback reflection privacy separation

-- Drop trigger and function
DROP FUNCTION IF EXISTS purge_expired_reflection_content();

-- Remove new columns from reflections table
ALTER TABLE reflections
DROP COLUMN IF EXISTS content_hash,
DROP COLUMN IF EXISTS response_summary,
DROP COLUMN IF EXISTS retention_expires_at;

-- Drop the reflection_content table
DROP TABLE IF EXISTS reflection_content;
