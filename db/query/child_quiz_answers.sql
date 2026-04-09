-- name: CreateChildQuizAnswer :one
INSERT INTO child_quiz_answers (
  child_id,
  course_id,
  module_id,
  external_quiz_id,
  attempt_number,
  external_question_id,
  selected_answer_option_ids,
  is_correct,
  score
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetChildQuizAnswersByQuiz :many
SELECT * FROM child_quiz_answers
WHERE child_id = $1 AND course_id = $2 AND module_id = $3 AND external_quiz_id = $4;

-- name: GetChildQuizAnswersByAttempt :many
SELECT * FROM child_quiz_answers
WHERE child_id = $1 AND course_id = $2 AND module_id = $3 AND external_quiz_id = $4 AND attempt_number = $5;
