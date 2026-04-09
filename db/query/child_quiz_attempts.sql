-- name: CreateChildQuizAttempt :one
INSERT INTO child_quiz_attempts (
  child_id,
  course_id,
  module_id,
  external_quiz_id,
  context,
  context_ref,
  attempt_number,
  score,
  passed
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetChildQuizAttempts :many
SELECT * FROM child_quiz_attempts
WHERE child_id = $1 AND course_id = $2 AND module_id = $3 AND external_quiz_id = $4
ORDER BY attempt_number DESC;

-- name: CheckQuizAttemptExistsForChild :one
SELECT EXISTS (
    SELECT 1
    FROM child_quiz_attempts
    WHERE child_id = $1
);
