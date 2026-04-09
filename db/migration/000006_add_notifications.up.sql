-- Step 1: Remove the old OneSignal-specific columns from parents and children tables.
-- This is done conditionally to prevent errors if the migration is run on a clean schema.
DO $$
BEGIN
  IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='parents' AND column_name='onesignal_player_id') THEN
    ALTER TABLE parents DROP COLUMN onesignal_player_id;
  END IF;
  IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='children' AND column_name='onesignal_player_id') THEN
    ALTER TABLE children DROP COLUMN onesignal_player_id;
  END IF;
END $$;

-- Step 2: Make the devices table polymorphic to support different user types.
ALTER TABLE devices DROP COLUMN IF EXISTS child_id;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS user_id UUID NOT NULL;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS user_type VARCHAR(50) NOT NULL;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS push_token VARCHAR(255);
ALTER TABLE devices ADD COLUMN IF NOT EXISTS provider VARCHAR(50);
ALTER TABLE devices ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE devices ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Add an index for efficiently querying devices by user.
CREATE INDEX ON devices (user_id, user_type);

-- Trigger function to update 'updated_at' timestamp on row update
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply the trigger to the devices table
CREATE TRIGGER set_devices_timestamp
BEFORE UPDATE ON devices
FOR EACH ROW
EXECUTE PROCEDURE trigger_set_timestamp();

-- Step 3: Create the notifications table and its related types and indexes.
CREATE TYPE notification_recipient_type AS ENUM ('parent', 'child');

CREATE TABLE notifications (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  recipient_id UUID NOT NULL, -- This will be the parent's or child's UUID
  recipient_type notification_recipient_type NOT NULL,
  title VARCHAR(255) NOT NULL,
  message TEXT NOT NULL,
  is_read BOOLEAN NOT NULL DEFAULT FALSE,
  sent_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for faster querying of user notifications.
CREATE INDEX ON notifications (recipient_id, recipient_type);

-- Trigger function to update 'updated_at' timestamp on row update
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER set_notifications_timestamp
BEFORE UPDATE ON notifications
FOR EACH ROW
EXECUTE PROCEDURE trigger_set_timestamp();
