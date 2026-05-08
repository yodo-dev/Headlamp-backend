-- Ensure children.date_of_birth exists even if older migrations were skipped/forced.
ALTER TABLE children
  ADD COLUMN IF NOT EXISTS date_of_birth DATE;
