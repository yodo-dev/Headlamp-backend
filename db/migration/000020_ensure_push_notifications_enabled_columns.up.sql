-- Ensure push_notifications_enabled exists even if older migrations were skipped/forced.
ALTER TABLE parents
  ADD COLUMN IF NOT EXISTS push_notifications_enabled BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE children
  ADD COLUMN IF NOT EXISTS push_notifications_enabled BOOLEAN NOT NULL DEFAULT FALSE;
