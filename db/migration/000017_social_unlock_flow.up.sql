-- Migration 000017: Social Access Unlock Flow
-- Adds tables for the gated progression system:
--   child_progress_gate    → per-child DPT completion + config
--   child_course_unlock    → per-child per-course lock state
--   social_app_access      → gated social app state machine
--   unlock_audit_events    → immutable audit trail

-- Per-child unlock configuration and DPT completion record
CREATE TABLE IF NOT EXISTS child_progress_gate (
  child_id                         varchar PRIMARY KEY,
  digital_permit_test_completed_at timestamptz,
  unlock_after_courses             int NOT NULL DEFAULT 1,
  created_at                       timestamptz NOT NULL DEFAULT (now()),
  updated_at                       timestamptz NOT NULL DEFAULT (now()),
  CONSTRAINT fk_cpg_child FOREIGN KEY (child_id) REFERENCES children(id) ON DELETE CASCADE
);

-- Per-child per-course unlock state (LOCKED → UNLOCKED → COMPLETED)
CREATE TABLE IF NOT EXISTS child_course_unlock (
  id           bigserial PRIMARY KEY,
  child_id     varchar NOT NULL,
  course_id    varchar NOT NULL,
  course_order int     NOT NULL DEFAULT 0,
  status       varchar NOT NULL DEFAULT 'LOCKED',
  unlocked_at  timestamptz,
  completed_at timestamptz,
  created_at   timestamptz NOT NULL DEFAULT (now()),
  updated_at   timestamptz NOT NULL DEFAULT (now()),
  UNIQUE (child_id, course_id),
  CONSTRAINT fk_ccu_child FOREIGN KEY (child_id) REFERENCES children(id) ON DELETE CASCADE
);

-- Per-child per-platform gated access state (LOCKED → ELIGIBLE_PENDING_PARENT_APPROVAL → ENABLED)
CREATE TABLE IF NOT EXISTS social_app_access (
  id                     bigserial PRIMARY KEY,
  child_id               varchar NOT NULL,
  social_media_id        bigint  NOT NULL,
  state                  varchar NOT NULL DEFAULT 'LOCKED',
  eligibility_granted_at timestamptz,
  enabled_by_parent_id   varchar,
  enabled_at             timestamptz,
  created_at             timestamptz NOT NULL DEFAULT (now()),
  updated_at             timestamptz NOT NULL DEFAULT (now()),
  UNIQUE (child_id, social_media_id),
  CONSTRAINT fk_saa_child        FOREIGN KEY (child_id)       REFERENCES children(id)      ON DELETE CASCADE,
  CONSTRAINT fk_saa_social_media FOREIGN KEY (social_media_id) REFERENCES social_medias(id) ON DELETE CASCADE
);

-- Immutable audit trail for all unlock events
CREATE TABLE IF NOT EXISTS unlock_audit_events (
  id         bigserial PRIMARY KEY,
  child_id   varchar NOT NULL,
  event_type varchar NOT NULL,
  metadata   jsonb,
  created_at timestamptz NOT NULL DEFAULT (now()),
  CONSTRAINT fk_uae_child FOREIGN KEY (child_id) REFERENCES children(id) ON DELETE CASCADE
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_child_course_unlock_child_status ON child_course_unlock (child_id, status);
CREATE INDEX IF NOT EXISTS idx_social_app_access_child_state    ON social_app_access (child_id, state);
CREATE INDEX IF NOT EXISTS idx_unlock_audit_events_child        ON unlock_audit_events (child_id, created_at DESC);
