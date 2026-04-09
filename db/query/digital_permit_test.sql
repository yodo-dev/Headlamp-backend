-- name: CreateDigitalPermitTest :one
INSERT INTO digital_permit_tests (
    child_id,
    status,
    started_at
) VALUES (
    $1, 'in_progress', NOW()
) RETURNING *;

-- name: CreateDigitalPermitTestInteraction :one
INSERT INTO digital_permit_test_interactions (
    test_id,
    question_text,
    question_type,
    question_options,
    answer_text,
    points_awarded,
    feedback,
    is_final_question
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetDigitalPermitTestByChildID :one
SELECT * FROM digital_permit_tests
WHERE child_id = $1 AND status = 'in_progress'
LIMIT 1;

-- name: GetLatestCompletedDigitalPermitTestByChildID :one
SELECT * FROM digital_permit_tests
WHERE child_id = $1 AND status = 'completed'
ORDER BY completed_at DESC
LIMIT 1;

-- name: UpdateDigitalPermitTest :one
UPDATE digital_permit_tests
SET
    status = $2,
    score = $3,
    result = $4,
    completed_at = CASE WHEN $2 = 'completed' THEN NOW() ELSE completed_at END,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetDigitalPermitTestInteractions :many
SELECT *
FROM digital_permit_test_interactions
WHERE test_id = $1
ORDER BY created_at ASC;

-- name: UpdateDigitalPermitTestInteraction :one
UPDATE digital_permit_test_interactions
SET 
    answer_text = $2,
    points_awarded = $3,
    feedback = $4
WHERE id = $1
RETURNING *;

-- name: UpdateDigitalPermitTestStatus :one
UPDATE digital_permit_tests
SET status = $2
WHERE id = $1
RETURNING *;

-- name: CreateDigitalPermitTestQuestion :one
INSERT INTO digital_permit_test_interactions (
    test_id,
    question_text,
    question_type,
    question_options
) VALUES (
    $1, $2, $3, $4
) RETURNING *;
