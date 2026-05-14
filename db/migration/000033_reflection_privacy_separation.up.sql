-- Reflection Privacy Separation: Separate content from identity with hashed references

-- New table to store reflection content separately (without identity linkage)
CREATE TABLE reflection_content (
  content_hash VARCHAR PRIMARY KEY,
  prompt_content JSONB NOT NULL,
  response_text TEXT,
  response_media_url TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON reflection_content (created_at DESC);

-- Add new columns to reflections table for privacy separation
ALTER TABLE reflections
ADD COLUMN content_hash VARCHAR REFERENCES reflection_content(content_hash) ON DELETE CASCADE,
ADD COLUMN response_summary TEXT,
ADD COLUMN retention_expires_at TIMESTAMPTZ;

-- Backfill: Migrate existing reflections to use hashing (one-time migration)
-- This sets content_hash using SHA256 of (reflection_id + timestamp for randomness)
DO $$
DECLARE
  reflection_record RECORD;
  hash_value VARCHAR;
BEGIN
  FOR reflection_record IN
    SELECT id, prompt_content, response_text, response_media_url, created_at
    FROM reflections
    WHERE content_hash IS NULL
    LIMIT 1000 -- Process in batches to avoid locking issues
  LOOP
    -- Generate hash: SHA256(id || created_at)
    hash_value := encode(digest(reflection_record.id::text || reflection_record.created_at::text, 'sha256'), 'hex');
    
    -- Insert into reflection_content if not exists
    INSERT INTO reflection_content (content_hash, prompt_content, response_text, response_media_url, created_at)
    VALUES (
      hash_value,
      reflection_record.prompt_content,
      reflection_record.response_text,
      reflection_record.response_media_url,
      reflection_record.created_at
    )
    ON CONFLICT (content_hash) DO NOTHING;
    
    -- Update reflections table with hash and set retention (30 days)
    UPDATE reflections
    SET
      content_hash = hash_value,
      response_summary = CASE
        WHEN response_text IS NOT NULL AND response_text != ''
        THEN 'summary: ' || LEFT(response_text, 100)
        ELSE NULL
      END,
      retention_expires_at = reflection_record.created_at + INTERVAL '30 days'
    WHERE id = reflection_record.id;
  END LOOP;
END $$;

-- Trigger to auto-purge expired content
CREATE OR REPLACE FUNCTION purge_expired_reflection_content()
RETURNS void AS $$
BEGIN
  DELETE FROM reflection_content
  WHERE content_hash IN (
    SELECT r.content_hash
    FROM reflections r
    WHERE r.retention_expires_at IS NOT NULL
      AND r.retention_expires_at < now()
  );
END;
$$ LANGUAGE plpgsql;

-- Create index for retention queries
CREATE INDEX ON reflections (retention_expires_at) WHERE retention_expires_at IS NOT NULL;

-- Make columns NOT NULL after backfill (optional, adjust if you want nullable)
-- ALTER TABLE reflections ALTER COLUMN content_hash SET NOT NULL;
