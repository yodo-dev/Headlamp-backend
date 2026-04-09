-- Add push_notifications_enabled column to parents table
ALTER TABLE parents ADD COLUMN IF NOT EXISTS push_notifications_enabled BOOLEAN NOT NULL DEFAULT FALSE;

-- Add push_notifications_enabled column to children table
ALTER TABLE children ADD COLUMN IF NOT EXISTS push_notifications_enabled BOOLEAN NOT NULL DEFAULT FALSE;
