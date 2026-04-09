-- Drop all tables, ensuring dependencies are handled correctly by dropping in reverse order of creation.
DROP TABLE IF EXISTS "digital_permit_test_interactions";
DROP TABLE IF EXISTS "digital_permit_tests";
DROP TABLE IF EXISTS "child_weekly_modules";
DROP TABLE IF EXISTS "child_quiz_answers";
DROP TABLE IF EXISTS "child_quiz_attempts";
DROP TABLE IF EXISTS "child_onboarding_progress";
DROP TABLE IF EXISTS "onboarding_steps";
DROP TABLE IF EXISTS "social_media_usage_stats";
DROP TABLE IF EXISTS "accessible_social_media";
DROP TABLE IF EXISTS "child_module_progress";
DROP TABLE IF EXISTS "quiz_questions";
DROP TABLE IF EXISTS "weekly_module_schedule";
DROP TABLE IF EXISTS "receipts";
DROP TABLE IF EXISTS "subscriptions";
DROP TABLE IF EXISTS "parent_sessions";
DROP TABLE IF EXISTS "deep_link_codes";
DROP TABLE IF EXISTS "devices";
DROP TABLE IF EXISTS "children";
DROP TABLE IF EXISTS "parents";
DROP TABLE IF EXISTS "families";

-- Drop all custom enum types
DROP TYPE IF EXISTS "subscription_status";
DROP TYPE IF EXISTS "quiz_question_type";
DROP TYPE IF EXISTS "social_media_platform";
DROP TYPE IF EXISTS "onboarding_step_type";
DROP TYPE IF EXISTS "auth_provider";
DROP TYPE IF EXISTS "digital_permit_test_status";
DROP TYPE IF EXISTS "digital_permit_test_result";