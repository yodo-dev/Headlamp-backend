-- Remove push_notifications_enabled column from children table
ALTER TABLE children DROP COLUMN IF EXISTS push_notifications_enabled;

-- Remove push_notifications_enabled column from parents table
ALTER TABLE parents DROP COLUMN IF EXISTS push_notifications_enabled;
