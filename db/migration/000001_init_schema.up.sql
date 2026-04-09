-- Enums for various types used across the database
CREATE TYPE "subscription_status" AS ENUM (
  'active',
  'inactive',
  'past_due',
  'canceled'
);

CREATE TYPE "quiz_question_type" AS ENUM (
  'multiple_choice',
  'true_false',
  'long_answer',
  'reflection'
);


-- Onboarding step types (agnostic to content provider)
CREATE TYPE "onboarding_step_type" AS ENUM (
  'course',
  'quiz',
  'action'
);

CREATE TYPE "auth_provider" AS ENUM (
  'password',
  'google',
  'apple'
);

CREATE TYPE "digital_permit_test_status" AS ENUM (
  'not_started',
  'in_progress',
  'completed'
);

CREATE TYPE "digital_permit_test_result" AS ENUM (
  'pass',
  'not_yet'
);

-- Main tables
CREATE TABLE "families" (
  "id" varchar PRIMARY KEY,
  "private_key" bytea NOT NULL,
  "public_key" bytea NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "parents" (
  "id" bigserial PRIMARY KEY,
  "parent_id" varchar UNIQUE NOT NULL,
  "family_id" varchar NOT NULL,
  "firstname" varchar NOT NULL,
  "surname" varchar NOT NULL,
  "email" varchar UNIQUE NOT NULL,
  "hashed_password" varchar,
  "auth_provider" auth_provider,
  "provider_subject" varchar,
  "email_verified" boolean NOT NULL DEFAULT false,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  "updated_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "children" (
  "id" varchar PRIMARY KEY,
  "family_id" varchar NOT NULL,
  "first_name" varchar NOT NULL,
  "surname" varchar NOT NULL,
  "age" INT,
  "gender" VARCHAR,
  "profile_image_url" VARCHAR,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  "updated_at" timestamptz NOT NULL DEFAULT (now())
);


CREATE TABLE "devices" (
  "id" bigserial PRIMARY KEY,
  "child_id" varchar NOT NULL,
  "device_id" varchar UNIQUE NOT NULL,
  "activated_at" timestamptz,
  "created_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "deep_link_codes" (
  "id" bigserial PRIMARY KEY,
  "family_id" varchar NOT NULL,
  "child_id" varchar NOT NULL,
  "code" varchar UNIQUE NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "is_used" boolean NOT NULL DEFAULT false,
  "created_at" timestamptz NOT NULL DEFAULT (now())
);


CREATE TABLE "parent_sessions" (
  "id" varchar PRIMARY KEY,
  "parent_id" varchar NOT NULL,
  "refresh_token" varchar NOT NULL,
  "user_agent" varchar NOT NULL,
  "client_ip" varchar NOT NULL,
  "is_blocked" boolean NOT NULL DEFAULT false,
  "expires_at" timestamptz NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT (now())
);


CREATE TABLE "subscriptions" (
  "id" bigserial PRIMARY KEY,
  "family_id" varchar UNIQUE NOT NULL,
  "provider" varchar NOT NULL, -- e.g., 'stripe'
  "provider_subscription_id" varchar UNIQUE NOT NULL,
  "status" subscription_status NOT NULL,
  "plan" varchar NOT NULL,
  "current_period_end" timestamptz NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  "updated_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "receipts" (
  "id" bigserial PRIMARY KEY,
  "subscription_id" bigint NOT NULL,
  "amount" decimal(10, 2) NOT NULL,
  "currency" varchar(3) NOT NULL,
  "issued_at" timestamptz NOT NULL DEFAULT (now()),
  "provider_receipt_id" varchar
);


CREATE TABLE "weekly_module_schedule" (
  "id" bigserial PRIMARY KEY,
  "module_id" bigint NOT NULL,
  "week_start_date" date NOT NULL,
  "is_active" boolean NOT NULL DEFAULT true,
  UNIQUE ("week_start_date")
);

CREATE TABLE "quiz_questions" (
  "id" bigserial PRIMARY KEY,
  "module_id" bigint NOT NULL,
  "question_type" quiz_question_type NOT NULL,
  "question_text" text NOT NULL,
  "options" jsonb, -- For multiple choice questions
  "correct_answer" text, -- For true/false and multiple choice
  "order" int NOT NULL
);


CREATE TABLE "child_module_progress" (
  "id" bigserial PRIMARY KEY,
  "child_id" varchar NOT NULL,
  "course_id" varchar NOT NULL,
  "module_id" text NOT NULL,
  "score" decimal(5, 2),
  "is_completed" boolean NOT NULL DEFAULT false,
  "feedback_video_url" varchar,
  "last_attempted_at" timestamptz,
  UNIQUE ("child_id", "module_id", "course_id")
);

CREATE TABLE "social_medias" (
  "id" bigserial PRIMARY KEY,
  "name" varchar UNIQUE NOT NULL,
  "icon_url" varchar
);

CREATE TABLE "accessible_social_media" (
  "id" bigserial PRIMARY KEY,
  "child_id" varchar NOT NULL,
  "social_media_id" bigint NOT NULL,
  "is_accessible" boolean NOT NULL DEFAULT false,
  "access_revoked_at" timestamptz,
  UNIQUE ("child_id", "social_media_id")
);

CREATE TABLE "social_media_usage_stats" (
  "id" bigserial PRIMARY KEY,
  "child_id" varchar NOT NULL,
  "social_media_id" bigint NOT NULL,
  "start_time" timestamptz NOT NULL,
  "end_time" timestamptz NOT NULL,
  "duration_seconds" int NOT NULL
);

CREATE TABLE "onboarding_steps" (
  "onboarding_id" varchar PRIMARY KEY,
  "step_name" varchar NOT NULL,
  "description" text,
  "step_order" int NOT NULL,
  "step_type" onboarding_step_type NOT NULL DEFAULT 'action',
  -- Optional external references (agnostic keys)
  "external_course_key" varchar,
  "external_quiz_id" varchar,
  -- Optional retry limit for quiz steps; NULL means unlimited
  "retry_limit" int,
  "is_active" boolean NOT NULL DEFAULT true,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  "updated_at" timestamptz NOT NULL DEFAULT (now())
);


-- Track per-quiz attempts per child (agnostic external IDs)
CREATE TABLE "child_onboarding_progress" (
  "id" bigserial PRIMARY KEY,
  "child_id" varchar NOT NULL,
  "onboarding_id" varchar NOT NULL,
  "is_completed" boolean NOT NULL DEFAULT false,
  "completed_at" timestamptz,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  UNIQUE ("child_id", "onboarding_id")
);

-- Track per-quiz attempts per child (agnostic external IDs)
CREATE TABLE "child_quiz_attempts" (
  "id" bigserial PRIMARY KEY,
  "child_id" varchar NOT NULL,
  "course_id" varchar NOT NULL,
  "module_id" varchar NOT NULL,
  "external_quiz_id" varchar NOT NULL,
  -- Context to distinguish onboarding vs weekly modules
  "context" varchar NOT NULL,
  -- Reference id within context: onboarding_id or external_module_id
  "context_ref" varchar,
  "attempt_number" int NOT NULL,
  "score" decimal(5, 2) NOT NULL,
  "passed" boolean NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  UNIQUE ("child_id", "course_id", "module_id", "external_quiz_id", "attempt_number")
);

-- Store per-question answers per attempt (no correct answers exposed here)
CREATE TABLE "child_quiz_answers" (
  "id" bigserial PRIMARY KEY,
  "child_id" varchar NOT NULL,
  "course_id" varchar NOT NULL,
  "module_id" varchar NOT NULL,
  "external_quiz_id" varchar NOT NULL,
  "attempt_number" int NOT NULL,
  "external_question_id" varchar NOT NULL,
  -- Support single or multiple choice answers
  "selected_answer_option_ids" varchar[] NOT NULL,
  "is_correct" boolean NOT NULL,
  "score" decimal(5, 2) NOT NULL DEFAULT 0,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  UNIQUE("child_id", "course_id", "module_id", "external_quiz_id", "attempt_number", "external_question_id")
);

-- Track weekly module exposure and completion per child (agnostic external IDs)
CREATE TABLE "child_weekly_modules" (
  "id" bigserial PRIMARY KEY,
  "child_id" varchar NOT NULL,
  "external_module_id" varchar NOT NULL,
  "week_start_date" date NOT NULL,
  "shown_at" timestamptz,
  "completed_at" timestamptz,
  "latest_score" decimal(5, 2),
  UNIQUE ("child_id", "external_module_id")
);

CREATE TABLE "digital_permit_tests" (
  "id" uuid PRIMARY KEY DEFAULT (gen_random_uuid()),
  "child_id" varchar NOT NULL,
  "status" digital_permit_test_status NOT NULL DEFAULT 'not_started',
  "score" float NOT NULL DEFAULT 0,
  "result" digital_permit_test_result,
  "started_at" timestamptz,
  "completed_at" timestamptz,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  "updated_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "digital_permit_test_interactions" (
  "id" uuid PRIMARY KEY DEFAULT (gen_random_uuid()),
  "test_id" uuid NOT NULL,
  "question_text" text,
  "question_type" varchar,
  "question_options" text[],
  "answer_text" text,
  "points_awarded" float,
  "feedback" text,
  "is_final_question" boolean,
  "created_at" timestamptz NOT NULL DEFAULT (now())
);

-- Foreign key constraints
ALTER TABLE "parents" ADD FOREIGN KEY ("family_id") REFERENCES "families" ("id");
ALTER TABLE "children" ADD FOREIGN KEY ("family_id") REFERENCES "families" ("id");
ALTER TABLE "devices" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");
ALTER TABLE "parent_sessions" ADD FOREIGN KEY ("parent_id") REFERENCES "parents" ("parent_id");
ALTER TABLE "deep_link_codes" ADD FOREIGN KEY ("family_id") REFERENCES "families" ("id");
ALTER TABLE "deep_link_codes" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");
ALTER TABLE "subscriptions" ADD FOREIGN KEY ("family_id") REFERENCES "families" ("id");
ALTER TABLE "receipts" ADD FOREIGN KEY ("subscription_id") REFERENCES "subscriptions" ("id");
ALTER TABLE "child_module_progress" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");
ALTER TABLE "accessible_social_media" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");
ALTER TABLE "accessible_social_media" ADD FOREIGN KEY ("social_media_id") REFERENCES "social_medias" ("id");
ALTER TABLE "social_media_usage_stats" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");
ALTER TABLE "social_media_usage_stats" ADD FOREIGN KEY ("social_media_id") REFERENCES "social_medias" ("id");

ALTER TABLE "child_quiz_attempts" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");
ALTER TABLE "child_quiz_answers" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");
ALTER TABLE "child_weekly_modules" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");
ALTER TABLE "child_onboarding_progress" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id");
ALTER TABLE "child_onboarding_progress" ADD FOREIGN KEY ("onboarding_id") REFERENCES "onboarding_steps" ("onboarding_id");

ALTER TABLE "digital_permit_tests" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id") ON DELETE CASCADE;
ALTER TABLE "digital_permit_test_interactions" ADD FOREIGN KEY ("test_id") REFERENCES "digital_permit_tests" ("id") ON DELETE CASCADE;

-- Indexes for performance
CREATE INDEX ON "parents" ("parent_id");
CREATE UNIQUE INDEX ON "parents" ("provider_subject") WHERE "provider_subject" IS NOT NULL;
CREATE INDEX ON "parents" ("email", "auth_provider");

CREATE INDEX ON "devices" ("child_id");
CREATE INDEX ON "devices" ("device_id");
CREATE INDEX ON "deep_link_codes" ("code");
CREATE INDEX ON "subscriptions" ("family_id");
CREATE INDEX ON "child_module_progress" ("child_id", "module_id", "course_id");
CREATE INDEX ON "accessible_social_media" ("child_id", "social_media_id");
CREATE INDEX ON "social_media_usage_stats" ("child_id", "social_media_id");

-- Helpful indexes for new learning features
CREATE INDEX ON "onboarding_steps" ("step_type", "step_order");
CREATE INDEX ON "child_quiz_attempts" ("child_id", "course_id", "module_id", "external_quiz_id");
CREATE INDEX ON "child_quiz_answers" ("child_id", "course_id", "module_id", "external_quiz_id", "attempt_number");
CREATE INDEX ON "child_weekly_modules" ("child_id", "week_start_date");

CREATE INDEX ON "digital_permit_tests" ("child_id");
CREATE INDEX ON "digital_permit_test_interactions" ("test_id");

-- Seed social media platforms
INSERT INTO "social_medias" ("name", "icon_url") VALUES
('Snapchat', 'http://24.199.123.7:1337/uploads/Snapchat_832a5dac9d.png'),
('Twitter', 'http://24.199.123.7:1337/uploads/Twitter.png'),
('Instagram', 'http://24.199.123.7:1337/uploads/Instagram.png'),
('WhatsApp', 'http://24.199.123.7:1337/uploads/WhatsApp.png'),
('YouTube', 'http://24.199.123.7:1337/uploads/YouTube.png'),
('Facebook', 'http://24.199.127.3:1337/uploads/Facebook.png'),
('TikTok', 'http://24.199.127.3:1337/uploads/TikTok.png');

-- Default onboarding steps
INSERT INTO "onboarding_steps" ("onboarding_id", "step_name", "description", "step_order", "step_type", "external_course_key") VALUES
('digital_permit_course', 'digital_permit_course', 'Complete the Digital Permit course to learn about online safety.', 1, 'course', 'digital_permit_course'),
('digital_permit_test', 'digital_permit_test', 'Take the Digital Permit test to unlock social media access.', 2, 'quiz', NULL);