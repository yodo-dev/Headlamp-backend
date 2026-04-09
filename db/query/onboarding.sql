-- name: CreateOnboardingStep :one
INSERT INTO onboarding_steps (
  step_name,
  description,
  step_order
) VALUES (
  $1, $2, $3
) RETURNING *;

-- name: ListActiveOnboardingSteps :many
SELECT * FROM onboarding_steps
WHERE is_active = true
ORDER BY step_order;

-- name: CreateChildOnboardingProgress :exec
INSERT INTO child_onboarding_progress (child_id, onboarding_id)
SELECT $1, onboarding_id FROM onboarding_steps WHERE is_active = true;

-- name: GetChildOnboardingProgress :many
SELECT os.step_name, os.description, os.step_order, os.step_type, cop.is_completed, cop.completed_at
FROM child_onboarding_progress cop
JOIN onboarding_steps os ON cop.onboarding_id = os.onboarding_id
WHERE cop.child_id = $1
ORDER BY os.step_order;

-- name: GetOnboardingProgressByFamilyID :many
SELECT 
    c.id as child_id,
    os.step_name,
    os.description,
    os.step_order,
    os.step_type,
    cop.is_completed,
    cop.completed_at
FROM children c
JOIN child_onboarding_progress cop ON c.id = cop.child_id
JOIN onboarding_steps os ON cop.onboarding_id = os.onboarding_id
WHERE c.family_id = $1
ORDER BY c.id, os.step_order;

-- name: UpdateChildOnboardingStep :one
UPDATE child_onboarding_progress
SET is_completed = true, completed_at = now()
WHERE child_id = $1 AND onboarding_id = $2
RETURNING *;

-- name: UpdateOnboardingStep :one
UPDATE onboarding_steps
SET
  step_name = COALESCE(sqlc.narg(step_name), step_name),
  description = COALESCE(sqlc.narg(description), description),
  step_order = COALESCE(sqlc.narg(step_order), step_order),
  is_active = COALESCE(sqlc.narg(is_active), is_active)
WHERE
  onboarding_id = sqlc.arg(onboarding_id)
RETURNING *;

-- name: DeleteOnboardingStep :exec
DELETE FROM onboarding_steps
WHERE onboarding_id = $1;

-- name: GetOnboardingStep :one
SELECT * FROM onboarding_steps
WHERE onboarding_id = $1;

-- name: GetOnboardingStepByOrder :one
SELECT * FROM onboarding_steps
WHERE step_order = $1;
