-- Revert Step 3: Drop the notifications table and its related objects.
DROP INDEX IF EXISTS notifications_recipient_id_recipient_type_idx;
DROP TABLE IF EXISTS notifications;
DROP TYPE IF EXISTS notification_recipient_type;

-- Revert Step 2: Revert the devices table to its original structure.
DROP INDEX IF EXISTS devices_user_id_user_type_idx;
ALTER TABLE devices DROP COLUMN IF EXISTS user_id;
ALTER TABLE devices DROP COLUMN IF EXISTS user_type;
ALTER TABLE devices DROP COLUMN IF EXISTS push_token;
ALTER TABLE devices DROP COLUMN IF EXISTS provider;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS child_id UUID REFERENCES children(id) ON DELETE CASCADE;
