package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type TrainingStepProgress struct {
	ID          int64              `json:"id"`
	ChildID     string             `json:"child_id"`
	StageKey    string             `json:"stage_key"`
	ModuleKey   pgtype.Text        `json:"module_key"`
	StepKey     string             `json:"step_key"`
	StepType    string             `json:"step_type"`
	Status      string             `json:"status"`
	CompletedAt pgtype.Timestamptz `json:"completed_at"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

type UpsertTrainingStepProgressParams struct {
	ChildID   string
	StageKey  string
	ModuleKey pgtype.Text
	StepKey   string
	StepType  string
	Status    string
}

const upsertTrainingStepProgress = `
WITH input AS (
	SELECT
		$1::varchar AS child_id,
		$2::varchar AS stage_key,
		$3::text AS module_key,
		$4::text AS step_key,
		$5::varchar AS step_type,
		$6::varchar AS status
)
INSERT INTO child_training_step_progress (
  child_id,
  stage_key,
  module_key,
  step_key,
  step_type,
  status,
  completed_at
) SELECT
	input.child_id,
	input.stage_key,
	input.module_key,
	input.step_key,
	input.step_type,
	input.status,
	CASE WHEN input.status = 'completed' THEN NOW() ELSE NULL END
FROM input
ON CONFLICT (child_id, step_key)
DO UPDATE SET
  stage_key = EXCLUDED.stage_key,
  module_key = COALESCE(EXCLUDED.module_key, child_training_step_progress.module_key),
  step_type = EXCLUDED.step_type,
  status = CASE
    WHEN child_training_step_progress.status = 'completed' THEN child_training_step_progress.status
    ELSE EXCLUDED.status
  END,
  completed_at = CASE
    WHEN EXCLUDED.status = 'completed' THEN COALESCE(child_training_step_progress.completed_at, NOW())
    ELSE child_training_step_progress.completed_at
  END,
  updated_at = NOW()
RETURNING id, child_id, stage_key, module_key, step_key, step_type, status, completed_at, created_at, updated_at
`

func (store *SQLStore) UpsertTrainingStepProgress(ctx context.Context, arg UpsertTrainingStepProgressParams) (TrainingStepProgress, error) {
	row := store.connPool.QueryRow(ctx, upsertTrainingStepProgress,
		arg.ChildID,
		arg.StageKey,
		arg.ModuleKey,
		arg.StepKey,
		arg.StepType,
		arg.Status,
	)

	var progress TrainingStepProgress
	err := row.Scan(
		&progress.ID,
		&progress.ChildID,
		&progress.StageKey,
		&progress.ModuleKey,
		&progress.StepKey,
		&progress.StepType,
		&progress.Status,
		&progress.CompletedAt,
		&progress.CreatedAt,
		&progress.UpdatedAt,
	)
	return progress, err
}

const getTrainingStepProgressForChild = `
SELECT id, child_id, stage_key, module_key, step_key, step_type, status, completed_at, created_at, updated_at
FROM child_training_step_progress
WHERE child_id = $1
ORDER BY created_at ASC
`

func (store *SQLStore) GetTrainingStepProgressForChild(ctx context.Context, childID string) ([]TrainingStepProgress, error) {
	rows, err := store.connPool.Query(ctx, getTrainingStepProgressForChild, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]TrainingStepProgress, 0)
	for rows.Next() {
		var progress TrainingStepProgress
		if err := rows.Scan(
			&progress.ID,
			&progress.ChildID,
			&progress.StageKey,
			&progress.ModuleKey,
			&progress.StepKey,
			&progress.StepType,
			&progress.Status,
			&progress.CompletedAt,
			&progress.CreatedAt,
			&progress.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, progress)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}
