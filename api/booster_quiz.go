package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// getBoosterQuiz godoc
// @Summary Get a quiz for a specific booster
// @Description Retrieves the quiz associated with a specific booster for a child.
// @Tags boosters
// @Accept  json
// @Produce  json
// @Param   booster_id   path    string  true  "Booster ID"
// @Success 200 {object} extQuizWithQuestions
// @Failure 400 {object} gin.H "Invalid request"
// @Failure 404 {object} gin.H "Quiz not found"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /v1/child/booster/{booster_id}/quiz [get]
func (server *Server) getBoosterQuiz(ctx *gin.Context) {
	// The deviceAuthMiddleware has already verified the child and device.
	// We get the child's data from the context.
	authPayload := ctx.MustGet(authorizationPayloadKey).(db.Child)
	boosterID := ctx.Param("booster_id")

	if boosterID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "booster_id is required"})
		return
	}

	// Get the booster to verify it belongs to the child and to get the external module ID
	booster, err := server.store.GetBoosterByID(ctx, boosterID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "booster not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Verify the booster belongs to the authenticated child
	if booster.ChildID != authPayload.ID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "you don't have access to this booster"})
		return
	}

	// Fetch the module data to get the quiz ID
	moduleData, err := server.fetchExternalWeeklyModuleData(ctx, booster.ExternalModuleID)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Str("external_module_id", booster.ExternalModuleID).Msg("failed to fetch external weekly module data")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	if moduleData.Quiz == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "no quiz found for this booster"})
		return
	}

	// Fetch the quiz data
	quizData, err := server.fetchExternalQuizData(ctx, moduleData.Quiz.DocumentID)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Str("quiz_id", moduleData.Quiz.DocumentID).Msg("failed to fetch external quiz data")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Get previous attempts for this quiz
	attempts, err := server.store.GetChildQuizAttempts(ctx, db.GetChildQuizAttemptsParams{
		ChildID:        authPayload.ID,
		ModuleID:       booster.ExternalModuleID,
		ExternalQuizID: moduleData.Quiz.DocumentID,
	})
	if err != nil {
		if err != sql.ErrNoRows {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		// If no attempts, just continue with empty attempts array
		attempts = []db.ChildQuizAttempt{}
	}

	// Check if the quiz has been passed
	var hasPassed bool
	var latestAttempt *db.ChildQuizAttempt
	if len(attempts) > 0 {
		latestAttempt = &attempts[len(attempts)-1]
		for _, attempt := range attempts {
			if attempt.Passed {
				hasPassed = true
				break
			}
		}
	}

	// Prepare the response
	response := gin.H{
		"quiz":           quizData,
		"attempts":       attempts,
		"has_passed":     hasPassed,
		"latest_attempt": latestAttempt,
	}

	ctx.JSON(http.StatusOK, response)
}

// submitBoosterQuiz godoc
// @Summary Submit answers for a booster quiz
// @Description Submits answers for a quiz associated with a specific booster.
// @Tags boosters
// @Accept  json
// @Produce  json
// @Param   booster_id   path    string  true  "Booster ID"
// @Param   answers      body    SubmitQuizAnswersRequest  true  "Quiz answers"
// @Success 200 {object} gin.H
// @Failure 400 {object} gin.H "Invalid request"
// @Failure 404 {object} gin.H "Booster or quiz not found"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /v1/child/booster/{booster_id}/quiz/submit [post]
func (server *Server) submitBoosterQuiz(ctx *gin.Context) {
	// The deviceAuthMiddleware has already verified the child and device.
	// We get the child's data from the context.
	authPayload := ctx.MustGet(authorizationPayloadKey).(db.Child)
	boosterID := ctx.Param("booster_id")

	if boosterID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "booster_id is required"})
		return
	}

	var req SubmitQuizAnswersRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("failed to bind JSON request")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	// Get the booster to verify it belongs to the child and to get the external module ID
	booster, err := server.store.GetBoosterByID(ctx, boosterID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "booster not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Verify the booster belongs to the authenticated child
	if booster.ChildID != authPayload.ID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "you don't have access to this booster"})
		return
	}

	// Fetch the module data to get the quiz ID
	moduleData, err := server.fetchExternalWeeklyModuleData(ctx, booster.ExternalModuleID)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Str("external_module_id", booster.ExternalModuleID).Msg("failed to fetch external weekly module data")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	if moduleData.Quiz == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "no quiz found for this booster"})
		return
	}

	quizID := moduleData.Quiz.DocumentID

	// Fetch the quiz data
	quizData, err := server.fetchExternalQuizData(ctx, quizID)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Str("quiz_id", quizID).Msg("failed to fetch external quiz data")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Get previous attempts for this quiz
	attempts, err := server.store.GetChildQuizAttempts(ctx, db.GetChildQuizAttemptsParams{
		ChildID:        authPayload.ID,
		ModuleID:       booster.ExternalModuleID,
		ExternalQuizID: quizID,
	})
	if err != nil && err != sql.ErrNoRows {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Check if the quiz has already been passed
	var alreadyPassed bool
	for _, attempt := range attempts {
		if attempt.Passed {
			alreadyPassed = true
			break
		}
	}

	// Process the answers
	questionsMap := make(map[string]extQuestion)
	for _, q := range quizData.Questions {
		questionsMap[q.DocumentID] = q
	}

	results := make([]QuizSubmissionResult, 0, len(req.Answers))
	var answerParamsForTx []db.QuizAnswerParams

	for _, answer := range req.Answers {
		targetQuestion, ok := questionsMap[answer.QuestionID]
		if !ok {
			results = append(results, QuizSubmissionResult{QuestionID: answer.QuestionID, Status: "question_not_found"})
			continue
		}

		correctOptionIDs := []string{}
		correctOptionsSet := make(map[string]struct{})
		for _, opt := range targetQuestion.AnswerOptions {
			if opt.IsCorrect {
				correctOptionIDs = append(correctOptionIDs, opt.DocumentID)
				correctOptionsSet[opt.DocumentID] = struct{}{}
			}
		}

		var correctnessStatus string
		var isCorrectForTx bool
		var scoreForTx float64
		submittedCorrectOptions := []string{}

		if targetQuestion.QType == "multiple-choice" && len(correctOptionIDs) > 0 {
			correctlySelectedCount := 0
			for _, submittedOptID := range answer.SelectedOptionIds {
				if _, ok := correctOptionsSet[submittedOptID]; ok {
					correctlySelectedCount++
					submittedCorrectOptions = append(submittedCorrectOptions, submittedOptID)
				}
			}

			if len(correctOptionIDs) > 0 {
				scoreForTx = (float64(correctlySelectedCount) / float64(len(correctOptionIDs))) * 100
			}

			if correctlySelectedCount == len(correctOptionIDs) && len(answer.SelectedOptionIds) == len(correctOptionIDs) {
				correctnessStatus = "true"
				isCorrectForTx = true
			} else if correctlySelectedCount > 0 {
				correctnessStatus = "partial"
				isCorrectForTx = false
			} else {
				correctnessStatus = "false"
				isCorrectForTx = false
			}
		} else {
			isCorrect := false
			if len(answer.SelectedOptionIds) == 1 && len(correctOptionIDs) == 1 && answer.SelectedOptionIds[0] == correctOptionIDs[0] {
				isCorrect = true
				submittedCorrectOptions = correctOptionIDs
			}

			isCorrectForTx = isCorrect
			if isCorrect {
				correctnessStatus = "true"
				scoreForTx = 100
			} else {
				correctnessStatus = "false"
				scoreForTx = 0
			}
		}

		result := QuizSubmissionResult{
			QuestionID:              answer.QuestionID,
			IsCorrect:               correctnessStatus,
			SubmittedCorrectOptions: submittedCorrectOptions,
		}
		if alreadyPassed {
			result.Status = "already_passed"
		}
		results = append(results, result)

		if !alreadyPassed {
			answerParamsForTx = append(answerParamsForTx, db.QuizAnswerParams{
				QuestionID:        answer.QuestionID,
				SelectedOptionIds: answer.SelectedOptionIds,
				IsCorrect:         isCorrectForTx,
				Score:             scoreForTx,
			})
		}
	}

	if !alreadyPassed && len(answerParamsForTx) > 0 {
		txParams := db.SubmitQuizAnswersTxParams{
			ChildID:              authPayload.ID,
			ModuleID:             booster.ExternalModuleID,
			ExternalQuizID:       quizID,
			Answers:              answerParamsForTx,
			Context:              util.ContextBooster,
			ContextRef:           boosterID,
			TotalQuestionsInQuiz: len(quizData.Questions),
			PassingScore:         quizData.Passing,
		}

		attemptResult, err := server.store.SubmitQuizAnswersTx(ctx, txParams)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to submit quiz answers transaction")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}

		// If the quiz was passed, mark the booster as completed
		if attemptResult.Attempt.Passed {
			_, err = server.store.CompleteBooster(ctx, boosterID)
			if err != nil {
				log.Ctx(ctx).Error().Err(err).Str("booster_id", boosterID).Msg("failed to mark booster as completed")
				// Don't return an error to the client, just log it
			} else {
				log.Ctx(ctx).Info().Str("booster_id", boosterID).Msg("booster marked as completed")
			}
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"results": results,
		"message": fmt.Sprintf("Successfully processed %d answers", len(results)),
	})
}
