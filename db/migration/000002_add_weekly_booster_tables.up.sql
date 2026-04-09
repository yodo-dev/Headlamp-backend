-- Add a stable, unique ID for each weekly module assigned to a child
ALTER TABLE "child_weekly_modules" ADD COLUMN "booster_id" VARCHAR;

-- Make booster_id not null and unique after populating it
UPDATE "child_weekly_modules" SET "booster_id" = gen_random_uuid()::varchar WHERE "booster_id" IS NULL;
ALTER TABLE "child_weekly_modules" ALTER COLUMN "booster_id" SET NOT NULL;
ALTER TABLE "child_weekly_modules" ADD UNIQUE ("booster_id");

-- Table to store reflection videos uploaded by children for completed boosters
CREATE TABLE "reflection_videos" (
  "id" bigserial PRIMARY KEY,
  "child_id" varchar NOT NULL,
  "booster_id" varchar NOT NULL,
  "video_url" varchar NOT NULL,
  "strapi_asset_id" varchar, -- To store the ID from Strapi
  "created_at" timestamptz NOT NULL DEFAULT (now())
);

-- Foreign key constraints
ALTER TABLE "reflection_videos" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id") ON DELETE CASCADE;
ALTER TABLE "reflection_videos" ADD FOREIGN KEY ("booster_id") REFERENCES "child_weekly_modules" ("booster_id") ON DELETE CASCADE;

-- Indexes for performance
CREATE INDEX ON "reflection_videos" ("child_id", "booster_id");
