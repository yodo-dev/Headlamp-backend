package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all connections for development purposes.
		// In production, you should implement a proper origin check.
		return true
	},
}

type digitalPermitTestUriRequest struct {
	ChildID string `uri:"id" binding:"required"`
}

type digitalPermitTestAnswer struct {
	Answer string `json:"answer"`
}

func (server *Server) handleDigitalPermitTestWS(ctx *gin.Context) {
	var req digitalPermitTestUriRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to upgrade connection")
		return
	}
	defer conn.Close()

	log.Info().Str("child_id", req.ChildID).Msg("websocket connection established for digital permit test")

	// Check for an existing in-progress test
	existingTest, err := server.store.GetDigitalPermitTestByChildID(ctx, req.ChildID)
	var testID uuid.UUID

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// --- Path 1: New Test ---
			log.Info().Str("child_id", req.ChildID).Msg("no existing test found, creating a new one")
			newTest, err := server.store.CreateDigitalPermitTest(ctx, req.ChildID)
			if err != nil {
				log.Error().Err(err).Str("child_id", req.ChildID).Msg("failed to create new digital permit test")
				conn.WriteJSON(gin.H{"error": "failed to create test"})
				return
			}
			testID = newTest.ID

			// Notify parent that the test has started
			go server.logActivityAndNotify(ctx, req.ChildID, "digital_permit_test_started", testID.String(), "Digital Permit Test")

			// Get the initial question from GPT
			initialInteraction, err := server.gptClient.InitiateDigitalPermitTest(ctx)
			if err != nil {
				log.Error().Err(err).Msg("failed to get initial question from GPT")
				conn.WriteJSON(gin.H{"error": "failed to get initial question"})
				return
			}

			// Save the initial interaction
			arg := db.CreateDigitalPermitTestInteractionParams{
				TestID:          testID,
				QuestionText:    pgtype.Text{String: initialInteraction.QuestionText, Valid: true},
				QuestionType:    pgtype.Text{String: initialInteraction.QuestionType, Valid: true},
				QuestionOptions: initialInteraction.Options,
			}
			_, err = server.store.CreateDigitalPermitTestInteraction(ctx, arg)
			if err != nil {
				log.Error().Err(err).Msg("failed to save initial interaction")
				conn.WriteJSON(gin.H{"error": "failed to save initial question"})
				return
			}

			// Send the initial question to the client
			initialQuestion := gin.H{
				"role":           "assistant",
				"status":         "question",
				"text":           initialInteraction.QuestionText,
				"points_awarded": nil,
				"total_score":    nil,
			}
			if err := conn.WriteJSON(initialQuestion); err != nil {
				log.Error().Err(err).Msg("failed to send initial question to client")
				return
			}
		} else {
			// Any other error is unexpected.
			log.Error().Err(err).Str("child_id", req.ChildID).Msg("failed to get existing digital permit test")
			conn.WriteJSON(gin.H{"error": "failed to get existing test"})
			return
		}
	} else {
		// --- Path 2: Resume Test ---
		log.Info().Str("child_id", req.ChildID).Str("test_id", existingTest.ID.String()).Msg("digital permit test already in progress, resuming")
		testID = existingTest.ID
		interactions, err := server.store.GetDigitalPermitTestInteractions(ctx, existingTest.ID)
		if err != nil {
			log.Error().Err(err).Str("test_id", existingTest.ID.String()).Msg("failed to get interactions for existing test")
			conn.WriteJSON(gin.H{"error": "failed to get interactions for existing test"})
			return
		}

		// Sort interactions by CreatedAt timestamp to ensure chronological order.
		sort.Slice(interactions, func(i, j int) bool {
			return interactions[i].CreatedAt.Before(interactions[j].CreatedAt)
		})

		// Build and send the history to the client
		if len(interactions) > 0 {
			history := []gin.H{}
			var currentScore float64 = 0

			for _, interaction := range interactions {
				// Add assistant's question to history
				questionMessage := gin.H{
					"role":       "assistant",
					"text":       interaction.QuestionText.String,
					"created_at": interaction.CreatedAt,
				}
				history = append(history, questionMessage)

				// If there's an answer, add user's answer and assistant's feedback to history
				if interaction.AnswerText.Valid {
					userMessage := gin.H{
						"role":       "user",
						"answer":     interaction.AnswerText.String,
						"created_at": interaction.CreatedAt, // Use the same timestamp as the interaction was updated in place
					}
					history = append(history, userMessage)

					if interaction.Feedback.Valid {
						currentScore += interaction.PointsAwarded.Float64
						feedbackMessage := gin.H{
							"role":           "assistant",
							"text":           interaction.Feedback.String,
							"points_awarded": interaction.PointsAwarded.Float64,
							"total_score":    currentScore,
							"created_at":     interaction.CreatedAt, // Use the same timestamp
						}
						history = append(history, feedbackMessage)
					}
				}
			}

			resumptionMessage := gin.H{
				"role":    "assistant",
				"status":  "resuming",
				"history": history,
			}

			if err := conn.WriteJSON(resumptionMessage); err != nil {
				log.Error().Err(err).Msg("failed to send resumption message to client")
				return
			}
		}
	}

	// --- Main Message Loop (for both new and resumed tests) ---
	for {
		var msg digitalPermitTestAnswer
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Msg("websocket error")
			}
			break // Exit loop on error
		}

		// Send a 'thinking' message to the client
		if err := conn.WriteJSON(gin.H{"role": "assistant", "status": "thinking"}); err != nil {
			log.Error().Err(err).Msg("failed to send 'thinking' status")
			break
		}

		// Get previous interactions to build context for GPT
		previousInteractions, err := server.store.GetDigitalPermitTestInteractions(ctx, testID)
		if err != nil {
			conn.WriteJSON(gin.H{"error": "failed to retrieve previous interactions"})
			continue
		}

		// Get next question/feedback from GPT
		gptResponse, err := server.gptClient.ContinueDigitalPermitTest(ctx, previousInteractions, msg.Answer)
		if err != nil {
			conn.WriteJSON(gin.H{"error": "failed to get next step from GPT"})
			continue
		}

		// Update the previous interaction
		updateArg := db.UpdateDigitalPermitTestInteractionParams{
			ID:            previousInteractions[len(previousInteractions)-1].ID,
			AnswerText:    pgtype.Text{String: msg.Answer, Valid: true},
			PointsAwarded: pgtype.Float8{Float64: gptResponse.PointsAwarded, Valid: true},
			Feedback:      pgtype.Text{String: gptResponse.Feedback, Valid: true},
		}
		updatedInteraction, err := server.store.UpdateDigitalPermitTestInteraction(ctx, updateArg)
		if err != nil {
			conn.WriteJSON(gin.H{"error": "failed to update interaction"})
			continue
		}

		// After updating, calculate the total score
		allInteractions, err := server.store.GetDigitalPermitTestInteractions(ctx, testID)
		if err != nil {
			conn.WriteJSON(gin.H{"error": "failed to retrieve interactions for score calculation"})
			continue
		}

		var totalScore float64
		for _, interaction := range allInteractions {
			if interaction.PointsAwarded.Valid {
				totalScore += interaction.PointsAwarded.Float64
			}
		}

		// If there's a next question, combine feedback with it and send as one message.
		if gptResponse.QuestionText != "" {
			// Save the new question interaction first
			newQuestionArg := db.CreateDigitalPermitTestQuestionParams{
				TestID:          testID,
				QuestionText:    pgtype.Text{String: gptResponse.QuestionText, Valid: true},
				QuestionType:    pgtype.Text{String: gptResponse.QuestionType, Valid: true},
				QuestionOptions: gptResponse.Options,
			}
			_, err := server.store.CreateDigitalPermitTestQuestion(ctx, newQuestionArg)
			if err != nil {
				conn.WriteJSON(gin.H{"error": "failed to save new question"})
				continue
			}

			// Combine feedback from the last answer with the new question.
			combinedText := fmt.Sprintf("%s %s", updatedInteraction.Feedback.String, gptResponse.QuestionText)

			// Send the combined message
			combinedResponse := gin.H{
				"role":           "assistant",
				"status":         "question",
				"text":           combinedText,
				"points_awarded": updatedInteraction.PointsAwarded.Float64,
				"total_score":    totalScore,
			}
			if err := conn.WriteJSON(combinedResponse); err != nil {
				log.Error().Err(err).Msg("failed to send combined message to client")
				break
			}
		} else {
			// Final question, update test status and send summary
			_, err = server.store.UpdateDigitalPermitTestStatus(ctx, db.UpdateDigitalPermitTestStatusParams{
				ID:     testID,
				Status: "completed",
			})
			if err != nil {
				conn.WriteJSON(gin.H{"error": "failed to update test status"})
				continue
			}

			// Mark the onboarding step as complete
			_, err = server.store.UpdateChildOnboardingStep(ctx, db.UpdateChildOnboardingStepParams{
				ChildID:      req.ChildID,
				OnboardingID: "digital_permit_test",
			})
			if err != nil {
				// Log the error, but don't block the user flow since the test itself is complete.
				log.Error().Err(err).Str("child_id", req.ChildID).Msg("failed to update digital_permit_test onboarding step")
			} else {
				log.Info().Str("child_id", req.ChildID).Msg("successfully updated digital_permit_test onboarding step")
			}

			// Recalculate final score to be sure
			finalInteractions, _ := server.store.GetDigitalPermitTestInteractions(ctx, testID)
			var finalScore float64
			for _, interaction := range finalInteractions {
				if interaction.PointsAwarded.Valid {
					finalScore += interaction.PointsAwarded.Float64
				}
			}

			finalMessage := gin.H{
				"role":        "assistant",
				"status":      "complete",
				"text":        gptResponse.FinalSummary,
				"final_score": finalScore,
			}
			if err := conn.WriteJSON(finalMessage); err != nil {
				break
			}
			// Test is complete. The client will close the connection.

			// Notify parent that the test has been completed
			go server.logActivityAndNotify(ctx, req.ChildID, "digital_permit_test_completed", testID.String(), "Digital Permit Test")
			go server.triggerDPTUnlockInitialization(req.ChildID)
		}
	}
}

// handleDigitalPermitTestWSV2 is the v2 endpoint with clean question text and separate options array
func (server *Server) handleDigitalPermitTestWSV2(ctx *gin.Context) {
	var req digitalPermitTestUriRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	// Check for debug mode query parameter
	debugMode := ctx.Query("debug") == "true"
	var maxQuestions int
	if debugMode {
		maxQuestions = 7
		log.Info().Str("child_id", req.ChildID).Msg("debug mode enabled - limiting to 7 questions")
	} else {
		maxQuestions = 50
	}

	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to upgrade connection")
		return
	}
	defer conn.Close()

	log.Info().Str("child_id", req.ChildID).Int("max_questions", maxQuestions).Msg("websocket connection established for digital permit test v2")

	// Check for an existing in-progress test
	existingTest, err := server.store.GetDigitalPermitTestByChildID(ctx, req.ChildID)
	var testID uuid.UUID

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// --- Path 1: New Test ---
			log.Info().Str("child_id", req.ChildID).Msg("no existing test found, creating a new one")
			newTest, err := server.store.CreateDigitalPermitTest(ctx, req.ChildID)
			if err != nil {
				log.Error().Err(err).Str("child_id", req.ChildID).Msg("failed to create new digital permit test")
				conn.WriteJSON(gin.H{"error": "failed to create test"})
				return
			}
			testID = newTest.ID

			// Notify parent that the test has started
			go server.logActivityAndNotify(ctx, req.ChildID, "digital_permit_test_started", testID.String(), "Digital Permit Test")

			// Fetch curriculum context for the v2 prompt
			curriculumContent, err := server.aggregateAllContentForPrompt(ctx)
			if err != nil {
				log.Warn().Err(err).Msg("failed to fetch curriculum content, proceeding without it")
			}
			curriculumContext := formatCurriculumForPrompt(curriculumContent)

			// Get the initial question from GPT (v2 version with clean options)
			initialInteraction, err := server.gptClient.InitiateDigitalPermitTestV2(ctx, curriculumContext)
			if err != nil {
				log.Error().Err(err).Msg("failed to get initial question from GPT")
				conn.WriteJSON(gin.H{"error": "failed to get initial question"})
				return
			}

			// Save the initial interaction
			arg := db.CreateDigitalPermitTestInteractionParams{
				TestID:          testID,
				QuestionText:    pgtype.Text{String: initialInteraction.QuestionText, Valid: true},
				QuestionType:    pgtype.Text{String: initialInteraction.QuestionType, Valid: true},
				QuestionOptions: initialInteraction.Options,
			}
			_, err = server.store.CreateDigitalPermitTestInteraction(ctx, arg)
			if err != nil {
				log.Error().Err(err).Msg("failed to save initial interaction")
				conn.WriteJSON(gin.H{"error": "failed to save initial question"})
				return
			}

			// Send the initial question to the client (v2 format with separate options)
			initialQuestion := gin.H{
				"role":           "assistant",
				"status":         "question",
				"text":           initialInteraction.QuestionText,
				"question_type":  initialInteraction.QuestionType,
				"options":        initialInteraction.Options,
				"current_score":  initialInteraction.CurrentScore,
				"points_awarded": nil,
				"total_score":    nil,
			}
			if err := conn.WriteJSON(initialQuestion); err != nil {
				log.Error().Err(err).Msg("failed to send initial question to client")
				return
			}
		} else {
			// Any other error is unexpected.
			log.Error().Err(err).Str("child_id", req.ChildID).Msg("failed to get existing digital permit test")
			conn.WriteJSON(gin.H{"error": "failed to get existing test"})
			return
		}
	} else {
		// --- Path 2: Resume Test ---
		log.Info().Str("child_id", req.ChildID).Str("test_id", existingTest.ID.String()).Msg("digital permit test already in progress, resuming")
		testID = existingTest.ID
		interactions, err := server.store.GetDigitalPermitTestInteractions(ctx, existingTest.ID)
		if err != nil {
			log.Error().Err(err).Str("test_id", existingTest.ID.String()).Msg("failed to get interactions for existing test")
			conn.WriteJSON(gin.H{"error": "failed to get interactions for existing test"})
			return
		}

		// Sort interactions by CreatedAt timestamp to ensure chronological order.
		sort.Slice(interactions, func(i, j int) bool {
			return interactions[i].CreatedAt.Before(interactions[j].CreatedAt)
		})

		// Build and send the history to the client
		if len(interactions) > 0 {
			history := []gin.H{}
			var currentScore float64 = 0

			for _, interaction := range interactions {
				// Add assistant's question to history
				questionMessage := gin.H{
					"role":          "assistant",
					"text":          interaction.QuestionText.String,
					"question_type": interaction.QuestionType.String,
					"options":       interaction.QuestionOptions,
					"created_at":    interaction.CreatedAt,
				}
				history = append(history, questionMessage)

				// If there's an answer, add user's answer and assistant's feedback to history
				if interaction.AnswerText.Valid {
					userMessage := gin.H{
						"role":       "user",
						"answer":     interaction.AnswerText.String,
						"created_at": interaction.CreatedAt,
					}
					history = append(history, userMessage)

					if interaction.Feedback.Valid {
						currentScore += interaction.PointsAwarded.Float64
						feedbackMessage := gin.H{
							"role":           "assistant",
							"text":           interaction.Feedback.String,
							"points_awarded": interaction.PointsAwarded.Float64,
							"total_score":    currentScore,
							"created_at":     interaction.CreatedAt,
						}
						history = append(history, feedbackMessage)
					}
				}
			}

			resumptionMessage := gin.H{
				"role":    "assistant",
				"status":  "resuming",
				"history": history,
			}

			if err := conn.WriteJSON(resumptionMessage); err != nil {
				log.Error().Err(err).Msg("failed to send resumption message to client")
				return
			}
		}
	}

	// --- Main Message Loop (for both new and resumed tests) ---
	const SETUP_QUESTIONS = 2 // Age and device ownership questions
	const PASS_PERCENTAGE = 0.80

	// Calculate pass points based on max questions
	maxPointsFloat := float64(maxQuestions)
	passPoints := maxPointsFloat * PASS_PERCENTAGE

	for {
		var msg digitalPermitTestAnswer
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Msg("websocket error")
			}
			break // Exit loop on error
		}

		// Send a 'thinking' message to the client
		if err := conn.WriteJSON(gin.H{"role": "assistant", "status": "thinking"}); err != nil {
			log.Error().Err(err).Msg("failed to send 'thinking' status")
			break
		}

		// Get previous interactions to build context for GPT
		previousInteractions, err := server.store.GetDigitalPermitTestInteractions(ctx, testID)
		if err != nil {
			conn.WriteJSON(gin.H{"error": "failed to retrieve previous interactions"})
			continue
		}

		// Count how many actual questions have been asked (excluding setup questions)
		questionsAsked := len(previousInteractions) - SETUP_QUESTIONS
		if questionsAsked < 0 {
			questionsAsked = 0
		}

		// Fetch curriculum context for the v2 prompt (cache this if needed for performance)
		curriculumContent, err := server.aggregateAllContentForPrompt(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("failed to fetch curriculum content, proceeding without it")
		}
		curriculumContext := formatCurriculumForPrompt(curriculumContent)

		// Get next question/feedback from GPT (v2 version with clean options)
		gptResponse, err := server.gptClient.ContinueDigitalPermitTestV2(ctx, previousInteractions, msg.Answer, curriculumContext)
		if err != nil {
			conn.WriteJSON(gin.H{"error": "failed to get next step from GPT"})
			continue
		}

		// Update the previous interaction
		updateArg := db.UpdateDigitalPermitTestInteractionParams{
			ID:            previousInteractions[len(previousInteractions)-1].ID,
			AnswerText:    pgtype.Text{String: msg.Answer, Valid: true},
			PointsAwarded: pgtype.Float8{Float64: gptResponse.PointsAwarded, Valid: true},
			Feedback:      pgtype.Text{String: gptResponse.Feedback, Valid: true},
		}
		updatedInteraction, err := server.store.UpdateDigitalPermitTestInteraction(ctx, updateArg)
		if err != nil {
			conn.WriteJSON(gin.H{"error": "failed to update interaction"})
			continue
		}

		// After updating, calculate the total score
		allInteractions, err := server.store.GetDigitalPermitTestInteractions(ctx, testID)
		if err != nil {
			conn.WriteJSON(gin.H{"error": "failed to retrieve interactions for score calculation"})
			continue
		}

		var totalScore float64
		for _, interaction := range allInteractions {
			if interaction.PointsAwarded.Valid {
				totalScore += interaction.PointsAwarded.Float64
			}
		}

		// Check if we've reached the max questions (deterministic check)
		questionsAnswered := len(allInteractions) - SETUP_QUESTIONS
		if questionsAnswered < 0 {
			questionsAnswered = 0
		}

		// If we've reached max questions, force completion regardless of GPT response
		if questionsAnswered >= maxQuestions {
			// Calculate pass/fail
			passed := totalScore >= passPoints
			percentage := (totalScore / maxPointsFloat) * 100

			// Only mark as completed if they passed
			if passed {
				_, err = server.store.UpdateDigitalPermitTestStatus(ctx, db.UpdateDigitalPermitTestStatusParams{
					ID:     testID,
					Status: "completed",
				})
				if err != nil {
					conn.WriteJSON(gin.H{"error": "failed to update test status"})
					break
				}

				// Mark the onboarding step as complete
				_, err = server.store.UpdateChildOnboardingStep(ctx, db.UpdateChildOnboardingStepParams{
					ChildID:      req.ChildID,
					OnboardingID: "digital_permit_test",
				})
				if err != nil {
					log.Error().Err(err).Str("child_id", req.ChildID).Msg("failed to update digital_permit_test onboarding step")
				} else {
					log.Info().Str("child_id", req.ChildID).Msg("successfully updated digital_permit_test onboarding step")
				}
			} else {
				// Mark as incomplete if they didn't pass
				_, err = server.store.UpdateDigitalPermitTestStatus(ctx, db.UpdateDigitalPermitTestStatusParams{
					ID:     testID,
					Status: "incomplete",
				})
				if err != nil {
					log.Error().Err(err).Msg("failed to update test status to incomplete")
				}
			}

			// Send completion message with final score and pass/fail status
			passStatus := "PASS ✓"
			if !passed {
				passStatus = "NOT YET - Keep Learning!"
			}
			completionText := fmt.Sprintf("You did it—Fantastic job completing all %d questions of the Digital Permit Test!\n\nFinal Score: %.1f/%d (%.1f%%)\nStatus: %s", maxQuestions, totalScore, maxQuestions, percentage, passStatus)

			finalMessage := gin.H{
				"role":        "assistant",
				"status":      "complete",
				"text":        completionText,
				"final_score": totalScore,
				"passed":      passed,
			}
			if err := conn.WriteJSON(finalMessage); err != nil {
				break
			}

			// Notify parent that the test has been completed
			go server.logActivityAndNotify(ctx, req.ChildID, "digital_permit_test_completed", testID.String(), "Digital Permit Test")
			go server.triggerDPTUnlockInitialization(req.ChildID)
			break
		}

		// If there's a next question, combine feedback with it and send as one message.
		if gptResponse.QuestionText != "" {
			// Save the new question interaction first
			newQuestionArg := db.CreateDigitalPermitTestQuestionParams{
				TestID:          testID,
				QuestionText:    pgtype.Text{String: gptResponse.QuestionText, Valid: true},
				QuestionType:    pgtype.Text{String: gptResponse.QuestionType, Valid: true},
				QuestionOptions: gptResponse.Options,
			}
			_, err := server.store.CreateDigitalPermitTestQuestion(ctx, newQuestionArg)
			if err != nil {
				conn.WriteJSON(gin.H{"error": "failed to save new question"})
				continue
			}

			// Combine feedback from the last answer with the new question.
			combinedText := fmt.Sprintf("%s %s", updatedInteraction.Feedback.String, gptResponse.QuestionText)

			// Send the combined message (v2 format with separate options)
			combinedResponse := gin.H{
				"role":           "assistant",
				"status":         "question",
				"text":           combinedText,
				"question_type":  gptResponse.QuestionType,
				"options":        gptResponse.Options,
				"current_score":  gptResponse.CurrentScore,
				"points_awarded": updatedInteraction.PointsAwarded.Float64,
				"total_score":    totalScore,
			}
			if err := conn.WriteJSON(combinedResponse); err != nil {
				log.Error().Err(err).Msg("failed to send combined message to client")
				break
			}
		} else {
			// Final question, update test status and send summary
			_, err = server.store.UpdateDigitalPermitTestStatus(ctx, db.UpdateDigitalPermitTestStatusParams{
				ID:     testID,
				Status: "completed",
			})
			if err != nil {
				conn.WriteJSON(gin.H{"error": "failed to update test status"})
				continue
			}

			// Mark the onboarding step as complete
			_, err = server.store.UpdateChildOnboardingStep(ctx, db.UpdateChildOnboardingStepParams{
				ChildID:      req.ChildID,
				OnboardingID: "digital_permit_test",
			})
			if err != nil {
				// Log the error, but don't block the user flow since the test itself is complete.
				log.Error().Err(err).Str("child_id", req.ChildID).Msg("failed to update digital_permit_test onboarding step")
			} else {
				log.Info().Str("child_id", req.ChildID).Msg("successfully updated digital_permit_test onboarding step")
			}

			// Recalculate final score to be sure
			finalInteractions, _ := server.store.GetDigitalPermitTestInteractions(ctx, testID)
			var finalScore float64
			for _, interaction := range finalInteractions {
				if interaction.PointsAwarded.Valid {
					finalScore += interaction.PointsAwarded.Float64
				}
			}

			finalMessage := gin.H{
				"role":        "assistant",
				"status":      "complete",
				"text":        gptResponse.FinalSummary,
				"final_score": finalScore,
			}
			if err := conn.WriteJSON(finalMessage); err != nil {
				break
			}
			// Test is complete. The client will close the connection.

			// Notify parent that the test has been completed
			go server.logActivityAndNotify(ctx, req.ChildID, "digital_permit_test_completed", testID.String(), "Digital Permit Test")
			go server.triggerDPTUnlockInitialization(req.ChildID)
		}
	}
}
