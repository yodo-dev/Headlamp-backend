-- Migration to add proper CASCADE rules to foreign keys
-- This ensures referential integrity and automatic cleanup of orphaned records

-- ============================================================================
-- PARENTS TABLE - Cascade when family is deleted
-- ============================================================================
ALTER TABLE "parents" DROP CONSTRAINT IF EXISTS "parents_family_id_fkey";
ALTER TABLE "parents" ADD FOREIGN KEY ("family_id") REFERENCES "families" ("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- ============================================================================
-- CHILDREN TABLE - Cascade when family is deleted
-- ============================================================================
ALTER TABLE "children" DROP CONSTRAINT IF EXISTS "children_family_id_fkey";
ALTER TABLE "children" ADD FOREIGN KEY ("family_id") REFERENCES "families" ("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- ============================================================================
-- DEVICES TABLE - Note: Polymorphic structure (user_id, user_type) - no direct FK
-- Migration 000006 changed devices to be polymorphic, removing child_id column
-- ============================================================================
-- SKIP: No foreign key to update (polymorphic table)

-- ============================================================================
-- PARENT_SESSIONS TABLE - Cascade when parent is deleted
-- ============================================================================
ALTER TABLE "parent_sessions" DROP CONSTRAINT IF EXISTS "parent_sessions_parent_id_fkey";
ALTER TABLE "parent_sessions" ADD FOREIGN KEY ("parent_id") REFERENCES "parents" ("parent_id") ON DELETE CASCADE ON UPDATE CASCADE;

-- ============================================================================
-- DEEP_LINK_CODES TABLE - Cascade when family or child is deleted
-- ============================================================================
ALTER TABLE "deep_link_codes" DROP CONSTRAINT IF EXISTS "deep_link_codes_family_id_fkey";
ALTER TABLE "deep_link_codes" DROP CONSTRAINT IF EXISTS "deep_link_codes_child_id_fkey";
ALTER TABLE "deep_link_codes" ADD FOREIGN KEY ("family_id") REFERENCES "families" ("id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "deep_link_codes" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- ============================================================================
-- SUBSCRIPTIONS TABLE - Cascade when family is deleted
-- ============================================================================
ALTER TABLE "subscriptions" DROP CONSTRAINT IF EXISTS "subscriptions_family_id_fkey";
ALTER TABLE "subscriptions" ADD FOREIGN KEY ("family_id") REFERENCES "families" ("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- ============================================================================
-- RECEIPTS TABLE - Cascade when subscription is deleted
-- ============================================================================
ALTER TABLE "receipts" DROP CONSTRAINT IF EXISTS "receipts_subscription_id_fkey";
ALTER TABLE "receipts" ADD FOREIGN KEY ("subscription_id") REFERENCES "subscriptions" ("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- ============================================================================
-- CHILD_MODULE_PROGRESS TABLE - Cascade when child is deleted
-- ============================================================================
ALTER TABLE "child_module_progress" DROP CONSTRAINT IF EXISTS "child_module_progress_child_id_fkey";
ALTER TABLE "child_module_progress" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- ============================================================================
-- ACCESSIBLE_SOCIAL_MEDIA TABLE - Cascade when child is deleted, RESTRICT on social_media
-- ============================================================================
ALTER TABLE "accessible_social_media" DROP CONSTRAINT IF EXISTS "accessible_social_media_child_id_fkey";
ALTER TABLE "accessible_social_media" DROP CONSTRAINT IF EXISTS "accessible_social_media_social_media_id_fkey";
ALTER TABLE "accessible_social_media" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "accessible_social_media" ADD FOREIGN KEY ("social_media_id") REFERENCES "social_medias" ("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- ============================================================================
-- SOCIAL_MEDIA_USAGE_STATS TABLE - Cascade when child is deleted, RESTRICT on social_media
-- ============================================================================
ALTER TABLE "social_media_usage_stats" DROP CONSTRAINT IF EXISTS "social_media_usage_stats_child_id_fkey";
ALTER TABLE "social_media_usage_stats" DROP CONSTRAINT IF EXISTS "social_media_usage_stats_social_media_id_fkey";
ALTER TABLE "social_media_usage_stats" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "social_media_usage_stats" ADD FOREIGN KEY ("social_media_id") REFERENCES "social_medias" ("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- ============================================================================
-- CHILD_QUIZ_ATTEMPTS TABLE - Cascade when child is deleted
-- ============================================================================
ALTER TABLE "child_quiz_attempts" DROP CONSTRAINT IF EXISTS "child_quiz_attempts_child_id_fkey";
ALTER TABLE "child_quiz_attempts" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- ============================================================================
-- CHILD_QUIZ_ANSWERS TABLE - Cascade when child is deleted
-- ============================================================================
ALTER TABLE "child_quiz_answers" DROP CONSTRAINT IF EXISTS "child_quiz_answers_child_id_fkey";
ALTER TABLE "child_quiz_answers" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- ============================================================================
-- CHILD_WEEKLY_MODULES TABLE - Cascade when child is deleted
-- ============================================================================
ALTER TABLE "child_weekly_modules" DROP CONSTRAINT IF EXISTS "child_weekly_modules_child_id_fkey";
ALTER TABLE "child_weekly_modules" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- ============================================================================
-- CHILD_ONBOARDING_PROGRESS TABLE - Cascade when child is deleted, RESTRICT on onboarding
-- ============================================================================
ALTER TABLE "child_onboarding_progress" DROP CONSTRAINT IF EXISTS "child_onboarding_progress_child_id_fkey";
ALTER TABLE "child_onboarding_progress" DROP CONSTRAINT IF EXISTS "child_onboarding_progress_onboarding_id_fkey";
ALTER TABLE "child_onboarding_progress" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "child_onboarding_progress" ADD FOREIGN KEY ("onboarding_id") REFERENCES "onboarding_steps" ("onboarding_id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- ============================================================================
-- Note: The following tables already have proper CASCADE rules from previous migrations:
-- - digital_permit_tests (child_id -> children.id ON DELETE CASCADE)
-- - digital_permit_test_interactions (test_id -> digital_permit_tests.id ON DELETE CASCADE)
-- - reflection_videos (child_id -> children.id ON DELETE CASCADE)
-- - reflection_videos (booster_id -> child_weekly_modules.booster_id ON DELETE CASCADE)
-- - app_sessions (child_id -> children.id ON DELETE CASCADE)
-- - app_sessions (social_media_id -> social_medias.id ON DELETE CASCADE)
-- - child_activity_log (child_id -> children.id ON DELETE CASCADE)
-- ============================================================================
