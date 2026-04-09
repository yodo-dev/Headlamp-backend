-- Rollback migration - restore original foreign keys without CASCADE rules
-- This reverts the schema to the state before migration 000010

-- ============================================================================
-- PARENTS TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "parents" DROP CONSTRAINT IF EXISTS "parents_family_id_fkey";
ALTER TABLE "parents" ADD FOREIGN KEY ("family_id") REFERENCES "families" ("id");

-- ============================================================================
-- CHILDREN TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "children" DROP CONSTRAINT IF EXISTS "children_family_id_fkey";
ALTER TABLE "children" ADD FOREIGN KEY ("family_id") REFERENCES "families" ("id");

-- ============================================================================
-- DEVICES TABLE - SKIP (polymorphic structure, no child_id column)
-- ============================================================================
-- SKIP: No foreign key to revert (polymorphic table)

-- ============================================================================
-- PARENT_SESSIONS TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "parent_sessions" DROP CONSTRAINT IF EXISTS "parent_sessions_parent_id_fkey";
ALTER TABLE "parent_sessions" ADD FOREIGN KEY ("parent_id") REFERENCES "parents" ("parent_id");

-- ============================================================================
-- DEEP_LINK_CODES TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "deep_link_codes" DROP CONSTRAINT IF EXISTS "deep_link_codes_family_id_fkey";
ALTER TABLE "deep_link_codes" DROP CONSTRAINT IF EXISTS "deep_link_codes_child_id_fkey";
ALTER TABLE "deep_link_codes" ADD FOREIGN KEY ("family_id") REFERENCES "families" ("id");
ALTER TABLE "deep_link_codes" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");

-- ============================================================================
-- SUBSCRIPTIONS TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "subscriptions" DROP CONSTRAINT IF EXISTS "subscriptions_family_id_fkey";
ALTER TABLE "subscriptions" ADD FOREIGN KEY ("family_id") REFERENCES "families" ("id");

-- ============================================================================
-- RECEIPTS TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "receipts" DROP CONSTRAINT IF EXISTS "receipts_subscription_id_fkey";
ALTER TABLE "receipts" ADD FOREIGN KEY ("subscription_id") REFERENCES "subscriptions" ("id");

-- ============================================================================
-- CHILD_MODULE_PROGRESS TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "child_module_progress" DROP CONSTRAINT IF EXISTS "child_module_progress_child_id_fkey";
ALTER TABLE "child_module_progress" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");

-- ============================================================================
-- ACCESSIBLE_SOCIAL_MEDIA TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "accessible_social_media" DROP CONSTRAINT IF EXISTS "accessible_social_media_child_id_fkey";
ALTER TABLE "accessible_social_media" DROP CONSTRAINT IF EXISTS "accessible_social_media_social_media_id_fkey";
ALTER TABLE "accessible_social_media" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");
ALTER TABLE "accessible_social_media" ADD FOREIGN KEY ("social_media_id") REFERENCES "social_medias" ("id");

-- ============================================================================
-- SOCIAL_MEDIA_USAGE_STATS TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "social_media_usage_stats" DROP CONSTRAINT IF EXISTS "social_media_usage_stats_child_id_fkey";
ALTER TABLE "social_media_usage_stats" DROP CONSTRAINT IF EXISTS "social_media_usage_stats_social_media_id_fkey";
ALTER TABLE "social_media_usage_stats" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");
ALTER TABLE "social_media_usage_stats" ADD FOREIGN KEY ("social_media_id") REFERENCES "social_medias" ("id");

-- ============================================================================
-- CHILD_QUIZ_ATTEMPTS TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "child_quiz_attempts" DROP CONSTRAINT IF EXISTS "child_quiz_attempts_child_id_fkey";
ALTER TABLE "child_quiz_attempts" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");

-- ============================================================================
-- CHILD_QUIZ_ANSWERS TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "child_quiz_answers" DROP CONSTRAINT IF EXISTS "child_quiz_answers_child_id_fkey";
ALTER TABLE "child_quiz_answers" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");

-- ============================================================================
-- CHILD_WEEKLY_MODULES TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "child_weekly_modules" DROP CONSTRAINT IF EXISTS "child_weekly_modules_child_id_fkey";
ALTER TABLE "child_weekly_modules" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");

-- ============================================================================
-- CHILD_ONBOARDING_PROGRESS TABLE - Remove CASCADE
-- ============================================================================
ALTER TABLE "child_onboarding_progress" DROP CONSTRAINT IF EXISTS "child_onboarding_progress_child_id_fkey";
ALTER TABLE "child_onboarding_progress" DROP CONSTRAINT IF EXISTS "child_onboarding_progress_onboarding_id_fkey";
ALTER TABLE "child_onboarding_progress" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");
ALTER TABLE "child_onboarding_progress" ADD FOREIGN KEY ("onboarding_id") REFERENCES "onboarding_steps" ("onboarding_id");
