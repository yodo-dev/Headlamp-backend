ALTER TABLE children
  DROP COLUMN IF EXISTS push_notifications_enabled;

ALTER TABLE parents
  DROP COLUMN IF EXISTS push_notifications_enabled;
