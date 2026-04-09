-- Add parent-configured session duration to per-child app access rules.
-- Defaults to 3600 seconds (1 hour) for existing rows.
ALTER TABLE accessible_social_media
    ADD COLUMN session_duration_seconds INT NOT NULL DEFAULT 3600;
