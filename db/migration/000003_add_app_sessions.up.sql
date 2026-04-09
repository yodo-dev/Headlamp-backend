CREATE TABLE "app_sessions" (
    "id" uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    "child_id" varchar NOT NULL,
    "social_media_id" bigint NOT NULL,
    "start_time" timestamptz NOT NULL,
    "end_time" timestamptz,
    "expected_end_time" timestamptz NOT NULL,
    "status" text NOT NULL DEFAULT 'active',
    "created_at" timestamptz NOT NULL DEFAULT (now()),
    "updated_at" timestamptz NOT NULL DEFAULT (now())
);

ALTER TABLE "app_sessions" ADD FOREIGN KEY ("child_id") REFERENCES "children" ("id") ON DELETE CASCADE;
ALTER TABLE "app_sessions" ADD FOREIGN KEY ("social_media_id") REFERENCES "social_medias" ("id") ON DELETE CASCADE;

CREATE INDEX ON "app_sessions" ("child_id");
CREATE INDEX ON "app_sessions" ("social_media_id");
CREATE INDEX ON "app_sessions" ("status", "expected_end_time");
