CREATE TABLE IF NOT EXISTS child_training_step_progress (
  id           bigserial PRIMARY KEY,
  child_id     varchar NOT NULL,
  stage_key    varchar NOT NULL,
  module_key   text,
  step_key     text NOT NULL,
  step_type    varchar NOT NULL,
  status       varchar NOT NULL DEFAULT 'available',
  completed_at timestamptz,
  created_at   timestamptz NOT NULL DEFAULT (now()),
  updated_at   timestamptz NOT NULL DEFAULT (now()),
  UNIQUE (child_id, step_key),
  CONSTRAINT fk_training_step_progress_child FOREIGN KEY (child_id) REFERENCES children(id) ON DELETE CASCADE ON UPDATE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_training_step_progress_child_stage
  ON child_training_step_progress (child_id, stage_key);

CREATE INDEX IF NOT EXISTS idx_training_step_progress_child_module
  ON child_training_step_progress (child_id, module_key);