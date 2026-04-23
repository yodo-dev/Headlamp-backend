DROP INDEX IF EXISTS idx_children_family_active;
DROP INDEX IF EXISTS idx_children_active;
ALTER TABLE "children" DROP COLUMN IF EXISTS "deleted_at";
