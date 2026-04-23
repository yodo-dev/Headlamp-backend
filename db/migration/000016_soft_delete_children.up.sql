-- Soft-delete support for the children table.
-- A non-NULL deleted_at means the child profile has been removed by the parent.
-- All application queries filter WHERE deleted_at IS NULL so soft-deleted children
-- are invisible to the API while their data is preserved for audit / recovery.

ALTER TABLE "children" ADD COLUMN "deleted_at" TIMESTAMPTZ;

-- Partial index makes WHERE deleted_at IS NULL lookups fast.
CREATE INDEX idx_children_active ON "children" ("id") WHERE deleted_at IS NULL;
CREATE INDEX idx_children_family_active ON "children" ("family_id") WHERE deleted_at IS NULL;
