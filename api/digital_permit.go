package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

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

func (server *Server) resolveDigitalPermitTestMode(ctx *gin.Context, childID string) (debugMode bool, maxQuestions int) {
	debugRequested := false
	if rawDebug, exists := ctx.GetQuery("debug"); exists {
		parsed, err := strconv.ParseBool(rawDebug)
		if err != nil {
			log.Warn().
				Str("child_id", childID).
				Str("debug_query", rawDebug).
				Msg("invalid debug query value; defaulting to production mode")
		} else {
			debugRequested = parsed
		}
	}

	isProduction := strings.EqualFold(server.config.Environment, "production")
	debugMode = debugRequested && !isProduction

	maxQuestions = 50
	if debugMode {
		maxQuestions = 10
		log.Info().Str("child_id", childID).Msg("debug mode enabled - limiting to 10 questions")
	} else if debugRequested && isProduction {
		log.Warn().Str("child_id", childID).Msg("debug mode requested in production; using production limits")
	}

	return debugMode, maxQuestions
}

func sendAssistantStreamStart(conn *websocket.Conn) error {
	return conn.WriteJSON(gin.H{
		"role":         "assistant",
		"status":       "streaming",
		"message_type": "assistant_stream_start",
	})
}

func sendAssistantStreamDelta(conn *websocket.Conn, delta string, streamText string) error {
	return conn.WriteJSON(gin.H{
		"role":         "assistant",
		"status":       "streaming",
		"message_type": "assistant_stream_delta",
		"delta":        delta,
		"stream_text":  streamText,
	})
}

func sendAssistantStreamEnd(conn *websocket.Conn) error {
	return conn.WriteJSON(gin.H{
		"role":         "assistant",
		"status":       "streaming",
		"message_type": "assistant_stream_end",
	})
}

func extractQuestionTextPreview(streamedJSON string) string {
	keyIdx := strings.Index(streamedJSON, `"question_text"`)
	if keyIdx == -1 {
		return ""
	}

	rest := streamedJSON[keyIdx+len(`"question_text"`):]
	colonIdx := strings.Index(rest, ":")
	if colonIdx == -1 {
		return ""
	}

	rest = strings.TrimSpace(rest[colonIdx+1:])
	if rest == "" {
		return ""
	}

	quoteIdx := strings.Index(rest, `"`)
	if quoteIdx == -1 {
		return ""
	}

	content := rest[quoteIdx+1:]
	var b strings.Builder
	escaped := false
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if escaped {
			switch ch {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			default:
				b.WriteByte(ch)
			}
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if ch == '"' {
			break
		}

		b.WriteByte(ch)
	}

	return b.String()
}

func (server *Server) authorizeDigitalPermitTestAccess(ctx *gin.Context, childID string) bool {
	payload := server.getAuthPayload(ctx)
	if payload == nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authorization payload missing"})
		return false
	}

	if payload.Role == "child" {
		if payload.UserID != childID {
			ctx.JSON(http.StatusForbidden, gin.H{"error": "you do not have permission to access this child's data"})
			return false
		}
		return true
	}

	if payload.Role == "parent" {
		_, err := server.store.GetChildByIDAndFamilyID(ctx, db.GetChildByIDAndFamilyIDParams{
			ID:       childID,
			FamilyID: payload.FamilyID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				ctx.JSON(http.StatusForbidden, gin.H{"error": "you do not have permission to access this child's data"})
				return false
			}
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return false
		}
		return true
	}

	ctx.JSON(http.StatusForbidden, gin.H{"error": "user role is not authorized for digital permit test"})
	return false
}

func (server *Server) handleChildTrainingTestWS(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)
	stageKey := ctx.Param("stage_key")

	switch stageKey {
	case trainingStageIntroReadinessTest:
		ctx.Params = append(ctx.Params, gin.Param{Key: "id", Value: child.ID})
		server.handleDigitalPermitTestWSV2(ctx)
	case trainingStageDigitalPermitTest:
		ctx.Params = append(ctx.Params, gin.Param{Key: "id", Value: child.ID})
		server.handleDigitalPermitTestWSV2(ctx)
	case trainingStageSocialMediaDriverTest:
		ctx.JSON(http.StatusNotImplemented, gin.H{"error": "social media driver test is not configured yet"})
	default:
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "unsupported training test stage"})
	}
}

func (server *Server) handleDigitalPermitTestWS(ctx *gin.Context) {
	var req digitalPermitTestUriRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	if !server.authorizeDigitalPermitTestAccess(ctx, req.ChildID) {
		return
	}

	unlocked, err := server.isDigitalPermitTestUnlocked(context.Background(), req.ChildID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if !unlocked {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "digital permit test is locked until all Digital Permit modules are completed"})
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

	if !server.authorizeDigitalPermitTestAccess(ctx, req.ChildID) {
		return
	}

	unlocked, err := server.isDigitalPermitTestUnlocked(context.Background(), req.ChildID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if !unlocked {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "digital permit test is locked until all Digital Permit modules are completed"})
		return
	}

	// Resolve mode based on query parameter and environment.
	debugMode, maxQuestions := server.resolveDigitalPermitTestMode(ctx, req.ChildID)

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

			if err := sendAssistantStreamStart(conn); err != nil {
				log.Error().Err(err).Msg("failed to send assistant stream start")
				return
			}

			streamBuffer := ""
			streamChunks := 0
			streamBytes := 0
			streamStartedAt := time.Now()
			log.Info().Str("child_id", req.ChildID).Str("test_id", testID.String()).Msg("digital permit v2 initial stream started")
			// Get the initial question from GPT (v2 version with clean options)
			initialInteraction, err := server.gptClient.InitiateDigitalPermitTestV2Stream(ctx, curriculumContext, func(delta string) error {
				streamBuffer += delta
				streamChunks++
				streamBytes += len(delta)
				preview := extractQuestionTextPreview(streamBuffer)
				return sendAssistantStreamDelta(conn, delta, preview)
			})
			if err != nil {
				log.Error().Err(err).
					Str("child_id", req.ChildID).
					Str("test_id", testID.String()).
					Int("stream_chunks", streamChunks).
					Int("stream_bytes", streamBytes).
					Int64("stream_elapsed_ms", time.Since(streamStartedAt).Milliseconds()).
					Msg("failed to get initial question from GPT stream")
				conn.WriteJSON(gin.H{"error": "failed to get initial question"})
				return
			}

			log.Info().
				Str("child_id", req.ChildID).
				Str("test_id", testID.String()).
				Int("stream_chunks", streamChunks).
				Int("stream_bytes", streamBytes).
				Int64("stream_elapsed_ms", time.Since(streamStartedAt).Milliseconds()).
				Int("question_text_len", len(initialInteraction.QuestionText)).
				Int("options_count", len(initialInteraction.Options)).
				Msg("digital permit v2 initial stream completed")

			if err := sendAssistantStreamEnd(conn); err != nil {
				log.Error().Err(err).Msg("failed to send assistant stream end")
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

		// If this resumed test already reached the current mode's question limit
		// (e.g., debug=true with maxQuestions=10), complete it immediately.
		const SETUP_QUESTIONS_ON_RESUME = 2
		questionsAnsweredOnResume := len(interactions) - SETUP_QUESTIONS_ON_RESUME
		if questionsAnsweredOnResume < 0 {
			questionsAnsweredOnResume = 0
		}

		var resumedScore float64
		for _, interaction := range interactions {
			if interaction.PointsAwarded.Valid {
				resumedScore += interaction.PointsAwarded.Float64
			}
		}

		if debugMode && resumedScore >= 10.0 {
			_, updateErr := server.store.UpdateDigitalPermitTestStatus(ctx, db.UpdateDigitalPermitTestStatusParams{
				ID:     testID,
				Status: db.DigitalPermitTestStatus("completed"),
			})
			if updateErr != nil {
				log.Error().Err(updateErr).Str("child_id", req.ChildID).Str("test_id", testID.String()).Msg("failed to finalize resumed debug digital permit test")
				conn.WriteJSON(gin.H{"error": "failed to finalize resumed test"})
				return
			}

			_, _ = server.store.UpdateChildOnboardingStep(ctx, db.UpdateChildOnboardingStepParams{
				ChildID:      req.ChildID,
				OnboardingID: "digital_permit_test",
			})

			completionText := fmt.Sprintf("Debug mode complete. Final Score: %.1f", resumedScore)
			_ = conn.WriteJSON(gin.H{
				"role":        "assistant",
				"status":      "complete",
				"text":        completionText,
				"final_score": resumedScore,
				"passed":      true,
			})
			return
		}

		if questionsAnsweredOnResume >= maxQuestions {
			var totalScore float64
			for _, interaction := range interactions {
				if interaction.PointsAwarded.Valid {
					totalScore += interaction.PointsAwarded.Float64
				}
			}

			maxPointsFloat := float64(maxQuestions)
			passPoints := maxPointsFloat * 0.80
			passed := totalScore >= passPoints
			percentage := 0.0
			if maxPointsFloat > 0 {
				percentage = (totalScore / maxPointsFloat) * 100
			}

			status := db.DigitalPermitTestStatus("incomplete")
			if passed {
				status = db.DigitalPermitTestStatus("completed")
			}

			_, updateErr := server.store.UpdateDigitalPermitTestStatus(ctx, db.UpdateDigitalPermitTestStatusParams{
				ID:     testID,
				Status: status,
			})
			if updateErr != nil {
				log.Error().Err(updateErr).Str("child_id", req.ChildID).Str("test_id", testID.String()).Msg("failed to finalize resumed digital permit test")
				conn.WriteJSON(gin.H{"error": "failed to finalize resumed test"})
				return
			}

			if passed {
				_, _ = server.store.UpdateChildOnboardingStep(ctx, db.UpdateChildOnboardingStepParams{
					ChildID:      req.ChildID,
					OnboardingID: "digital_permit_test",
				})
			}

			passStatus := "PASS ✓"
			if !passed {
				passStatus = "NOT YET - Keep Learning!"
			}

			completionText := fmt.Sprintf("You did it—Fantastic job completing all %d questions of the Digital Permit Test!\n\nFinal Score: %.1f/%d (%.1f%%)\nStatus: %s", maxQuestions, totalScore, maxQuestions, percentage, passStatus)

			log.Info().
				Str("child_id", req.ChildID).
				Str("test_id", testID.String()).
				Int("questions_answered", questionsAnsweredOnResume).
				Int("max_questions", maxQuestions).
				Float64("final_score", totalScore).
				Bool("passed", passed).
				Msg("resumed digital permit test auto-finalized")

			_ = conn.WriteJSON(gin.H{
				"role":        "assistant",
				"status":      "complete",
				"text":        completionText,
				"final_score": totalScore,
				"passed":      passed,
			})
			return
		}

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

		if err := sendAssistantStreamStart(conn); err != nil {
			log.Error().Err(err).Msg("failed to send assistant stream start")
			break
		}

		streamBuffer := ""
		streamChunks := 0
		streamBytes := 0
		streamStartedAt := time.Now()
		log.Info().
			Str("child_id", req.ChildID).
			Str("test_id", testID.String()).
			Int("question_number", questionsAsked+1).
			Msg("digital permit v2 answer stream started")
		// Get next question/feedback from GPT (v2 version with clean options)
		gptResponse, err := server.gptClient.ContinueDigitalPermitTestV2Stream(ctx, previousInteractions, msg.Answer, curriculumContext, func(delta string) error {
			streamBuffer += delta
			streamChunks++
			streamBytes += len(delta)
			preview := extractQuestionTextPreview(streamBuffer)
			return sendAssistantStreamDelta(conn, delta, preview)
		})
		if err != nil {
			log.Error().Err(err).
				Str("child_id", req.ChildID).
				Str("test_id", testID.String()).
				Int("question_number", questionsAsked+1).
				Int("stream_chunks", streamChunks).
				Int("stream_bytes", streamBytes).
				Int64("stream_elapsed_ms", time.Since(streamStartedAt).Milliseconds()).
				Msg("failed to get next step from GPT stream")
			conn.WriteJSON(gin.H{"error": "failed to get next step from GPT"})
			continue
		}

		log.Info().
			Str("child_id", req.ChildID).
			Str("test_id", testID.String()).
			Int("question_number", questionsAsked+1).
			Int("stream_chunks", streamChunks).
			Int("stream_bytes", streamBytes).
			Int64("stream_elapsed_ms", time.Since(streamStartedAt).Milliseconds()).
			Int("question_text_len", len(gptResponse.QuestionText)).
			Int("options_count", len(gptResponse.Options)).
			Msg("digital permit v2 answer stream completed")

		if err := sendAssistantStreamEnd(conn); err != nil {
			log.Error().Err(err).Msg("failed to send assistant stream end")
			break
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

		// In debug mode, complete early once score reaches 10.
		if debugMode && totalScore >= 10.0 {
			_, err = server.store.UpdateDigitalPermitTestStatus(ctx, db.UpdateDigitalPermitTestStatusParams{
				ID:     testID,
				Status: "completed",
			})
			if err != nil {
				conn.WriteJSON(gin.H{"error": "failed to update test status"})
				break
			}

			_, err = server.store.UpdateChildOnboardingStep(ctx, db.UpdateChildOnboardingStepParams{
				ChildID:      req.ChildID,
				OnboardingID: "digital_permit_test",
			})
			if err != nil {
				log.Error().Err(err).Str("child_id", req.ChildID).Msg("failed to update digital_permit_test onboarding step")
			}

			finalMessage := gin.H{
				"role":        "assistant",
				"status":      "complete",
				"text":        fmt.Sprintf("Debug mode complete. Final Score: %.1f", totalScore),
				"final_score": totalScore,
				"passed":      true,
			}
			if err := conn.WriteJSON(finalMessage); err != nil {
				break
			}

			go server.logActivityAndNotify(ctx, req.ChildID, "digital_permit_test_completed", testID.String(), "Digital Permit Test")
			go server.triggerDPTUnlockInitialization(req.ChildID)
			break
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
