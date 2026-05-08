-- Ensure soft-delete schema exists for children even if older migrations were skipped/forced.
ALTER TABLE children
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_children_active
  ON children (id)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_children_family_active
  ON children (family_id)
  WHERE deleted_at IS NULL;
