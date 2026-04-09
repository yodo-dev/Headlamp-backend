package db

import (
	"context"
	"fmt"
	"math/big"

	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

// QuizAnswerParams contains the parameters for a single answer in a bulk submission.
type QuizAnswerParams struct {
	QuestionID        string   `json:"question_id"`
	SelectedOptionIds []string `json:"selected_option_ids"`
	IsCorrect         bool     `json:"is_correct"`
	Score             float64  `json:"score"`
}

// SubmitQuizAnswersTxParams contains the input parameters for the bulk quiz answer submission transaction.
type SubmitQuizAnswersTxParams struct {
	ChildID              string             `json:"child_id"`
	CourseID             string             `json:"course_id"`
	ModuleID             string             `json:"module_id"`
	ExternalQuizID       string             `json:"external_quiz_id"`
	Answers              []QuizAnswerParams `json:"answers"`
	Context              string             `json:"context"`
	ContextRef           string             `json:"context_ref"`
	TotalQuestionsInQuiz int                `json:"total_questions_in_quiz"`
	PassingScore         int                `json:"passing_score"`
}

// SubmitQuizAnswersTxResult contains the result of the bulk quiz answer submission transaction.
type SubmitQuizAnswersTxResult struct {
	Attempt        ChildQuizAttempt  `json:"attempt"`
	Answers        []ChildQuizAnswer `json:"answers"`
	ModuleUpdated  bool              `json:"module_updated"`
	BoosterUpdated bool              `json:"booster_updated"`
	Child          Child             `json:"child"`
	Parent         Parent            `json:"parent"`
}

// SubmitQuizAnswersTx handles the submission of multiple quiz answers in a single transaction.
func (store *SQLStore) SubmitQuizAnswersTx(ctx context.Context, arg SubmitQuizAnswersTxParams) (SubmitQuizAnswersTxResult, error) {
		var result SubmitQuizAnswersTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		// Get the latest attempt number for this quiz
		attempts, err := q.GetChildQuizAttempts(ctx, GetChildQuizAttemptsParams{
			ChildID:        arg.ChildID,
			CourseID:       arg.CourseID,
			ModuleID:       arg.ModuleID,
			ExternalQuizID: arg.ExternalQuizID,
		})
		if err != nil && err.Error() != "sql: no rows in result set" {
			return fmt.Errorf("failed to get child quiz attempts: %w", err)
		}

		var currentAttemptNumber int32 = 1
		if len(attempts) > 0 {
			currentAttemptNumber = attempts[0].AttemptNumber + 1
		}

		result.Answers = make([]ChildQuizAnswer, 0, len(arg.Answers))
		for _, answer := range arg.Answers {
			answerRecord, err := q.CreateChildQuizAnswer(ctx, CreateChildQuizAnswerParams{
				ChildID:                 arg.ChildID,
				CourseID:                arg.CourseID,
				ModuleID:                arg.ModuleID,
				ExternalQuizID:          arg.ExternalQuizID,
				AttemptNumber:           currentAttemptNumber,
				ExternalQuestionID:      answer.QuestionID,
				SelectedAnswerOptionIds: answer.SelectedOptionIds,
				IsCorrect:               answer.IsCorrect,
				Score: pgtype.Numeric{
					Int:   big.NewInt(int64(answer.Score * 100)),
					Exp:   -2,
					Valid: true,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to create child quiz answer for question %s: %w", answer.QuestionID, err)
			}
			result.Answers = append(result.Answers, answerRecord)
		}

		// Check if the quiz is complete
		answersForAttempt, err := q.GetChildQuizAnswersByAttempt(ctx, GetChildQuizAnswersByAttemptParams{
			ChildID:        arg.ChildID,
			CourseID:       arg.CourseID,
			ModuleID:       arg.ModuleID,
			ExternalQuizID: arg.ExternalQuizID,
			AttemptNumber:  currentAttemptNumber,
		})
		if err != nil {
			return fmt.Errorf("failed to get answers for attempt: %w", err)
		}

		if len(answersForAttempt) >= arg.TotalQuestionsInQuiz {
			// Quiz is complete, calculate score and create attempt record
			totalScore := 0.0
			for _, ans := range answersForAttempt {
				if ans.Score.Valid {
					ansScore, err := ans.Score.Float64Value()
					if err == nil {
						totalScore += ansScore.Float64
					}
				}
			}

			score := 0
			if arg.TotalQuestionsInQuiz > 0 {
				score = int(totalScore / float64(arg.TotalQuestionsInQuiz))
			}

						
			passed := score >= arg.PassingScore

			attemptParams := CreateChildQuizAttemptParams{
				ChildID:        arg.ChildID,
				CourseID:       arg.CourseID,
				ModuleID:       arg.ModuleID,
				ExternalQuizID: arg.ExternalQuizID,
				AttemptNumber:  currentAttemptNumber,
				Score:          pgtype.Numeric{Int: big.NewInt(int64(score)), Valid: true},
				Passed:         passed,
				Context:        arg.Context,
				ContextRef:     pgtype.Text{String: arg.ContextRef, Valid: arg.ContextRef != ""},
			}

									result.Attempt, err = q.CreateChildQuizAttempt(ctx, attemptParams)
			if err != nil {
				return fmt.Errorf("failed to create child quiz attempt: %w", err)
			}

			if passed && arg.Context == util.ContextModule && arg.ContextRef != "" {
				_, err := q.CreateOrUpdateModuleProgress(ctx, CreateOrUpdateModuleProgressParams{
					ChildID:     arg.ChildID,
					ModuleID:    arg.ContextRef,
					CourseID:    arg.CourseID,
					Score:       pgtype.Numeric{Int: big.NewInt(int64(score)), Valid: true},
					IsCompleted: true,
				})
				if err != nil {
					log.Ctx(ctx).Error().Err(err).Msg("failed to update module progress")
				} else {
					result.ModuleUpdated = true

					// Get child and parent for notification
					child, err := q.GetChild(ctx, arg.ChildID)
					if err != nil {
						return fmt.Errorf("failed to get child for notification: %w", err)
					}
					result.Child = child

					parent, err := q.GetParentByFamilyID(ctx, child.FamilyID)
					if err != nil {
						return fmt.Errorf("failed to get parent for notification: %w", err)
					}
					result.Parent = parent
				}
			} else if passed && arg.Context == util.ContextBooster && arg.ContextRef != "" {
				_, err := q.CompleteBooster(ctx, arg.ContextRef)
				if err != nil {
					log.Ctx(ctx).Error().Err(err).Msg("failed to update booster progress")
				} else {
					result.BoosterUpdated = true
				}
			}
		}

		return nil
	})

	return result, err
}
