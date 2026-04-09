package db

import (
	"context"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"
)

// CreateQuizAttemptTx creates a quiz attempt and all the associated answers within a single database transaction.
func (store *SQLStore) CreateQuizAttemptTx(ctx context.Context, arg CreateQuizAttemptTxParams) (CreateQuizAttemptTxResult, error) {
	var result CreateQuizAttemptTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		result.Attempt, err = q.CreateChildQuizAttempt(ctx, CreateChildQuizAttemptParams{
			ChildID:        arg.ChildID,
			ExternalQuizID: arg.ExternalQuizID,
			AttemptNumber:  arg.AttemptNumber,
			Score:          pgtype.Numeric{Int: big.NewInt(int64(arg.Score)), Valid: true},
			Passed:         arg.Passed,
			Context:        arg.Context,
			ContextRef:     pgtype.Text{String: arg.ContextRef.String, Valid: arg.ContextRef.Valid},
		})
		if err != nil {
			return fmt.Errorf("failed to create child quiz attempt: %w", err)
		}

		result.Answers = make([]ChildQuizAnswer, len(arg.Answers))
		for i, answer := range arg.Answers {
			result.Answers[i], err = q.CreateChildQuizAnswer(ctx, CreateChildQuizAnswerParams{
				ChildID:                 result.Attempt.ChildID,
				ExternalQuizID:          result.Attempt.ExternalQuizID,
				AttemptNumber:           result.Attempt.AttemptNumber,
				ExternalQuestionID:      answer.QuestionID,
				SelectedAnswerOptionIds: answer.SelectedOptionIDs,
				IsCorrect:               answer.IsCorrect,
			})
			if err != nil {
				return fmt.Errorf("failed to create child quiz answer for question %s: %w", answer.QuestionID, err)
			}
		}

		return nil
	})

	return result, err
}
