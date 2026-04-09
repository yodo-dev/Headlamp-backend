-- name: CreateQuizQuestion :one
INSERT INTO quiz_questions (
  module_id,
  question_type,
  question_text,
  options,
  correct_answer,
  "order"
) VALUES (
  $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: ListQuestionsByModule :many
SELECT * FROM quiz_questions
WHERE module_id = $1
ORDER BY "order";
