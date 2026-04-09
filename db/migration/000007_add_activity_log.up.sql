CREATE TYPE child_activity_type AS ENUM ('course_started', 'module_started', 'digital_permit_test_started', 'digital_permit_test_completed');

CREATE TABLE child_activity_log (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  child_id VARCHAR(255) NOT NULL REFERENCES children(id) ON DELETE CASCADE,
  activity_type child_activity_type NOT NULL,
  activity_ref_id VARCHAR(255) NOT NULL, -- e.g., course_id or module_id
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(child_id, activity_type, activity_ref_id)
);

CREATE INDEX ON child_activity_log (child_id, activity_type);
