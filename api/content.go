package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
)

// Cleaned response models (provider-agnostic)
type CleanCourse struct {
	ID                   string        `json:"id"`
	Title                string        `json:"title"`
	Description          string        `json:"description"`
	Modules              []CleanModule `json:"modules"`
	TotalNumberOfModules int           `json:"total_number_of_modules"`
	TotalNumberOfQuizzes int           `json:"total_number_of_quizzes"`
}

// Build absolute URL if external file uses relative path
func (server *Server) absoluteURL(rel string) string {
	if rel == "" {
		return ""
	}
	if strings.HasPrefix(rel, "http://") || strings.HasPrefix(rel, "https://") {
		return rel
	}
	base := strings.TrimRight(server.config.ExternalContentBaseURL, "/")
	if base == "" {
		return rel
	}
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	return base + rel
}

type CleanModule struct {
	ID                    string      `json:"id"`
	Title                 string      `json:"title"`
	Order                 int         `json:"order"`
	Description           string      `json:"description,omitempty"`
	IsCompleted           bool        `json:"is_completed"`
	Video                 *CleanVideo `json:"video,omitempty"`
	Quiz                  *CleanQuiz  `json:"quiz,omitempty"`
	ReflectionVideoURL    string      `json:"reflection_video_url,omitempty"`
	ReflectionSubmittedAt *time.Time  `json:"reflection_submitted_at,omitempty"`
}

type CleanQuizBrief struct {
	ID                            string `json:"id"`
	Title                         string `json:"title"`
	PassingScore                  int    `json:"passing_score"`
	EstimatedCompletionTimeInMins int    `json:"estimated_completion_time_in_mins"`
}

type CleanQuiz struct {
	ID                            string          `json:"id"`
	Title                         string          `json:"title"`
	Format                        string          `json:"format"`
	PassingScore                  int             `json:"passing_score"`
	EstimatedCompletionTimeInMins int             `json:"estimated_completion_time_in_mins"`
	Questions                     []CleanQuestion `json:"questions"`
}

type CleanVideo struct {
	URL  string  `json:"url"`
	Mime string  `json:"mime"`
	Size float64 `json:"size"`
}

// External API response minimal structs (only fields that we use)
type extCourseResponse struct {
	Data extCourse `json:"data"`
}

// For the /api/courses endpoint from the external service
type extAllCoursesResponse struct {
	Data []extCourseItem `json:"data"`
	Meta extMeta         `json:"meta"`
}

type extCourseItem struct {
	ID          int64         `json:"id"`
	DocumentID  string        `json:"documentId"`
	Title       string        `json:"title"`
	Description []extRichText `json:"description"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
	PublishedAt time.Time     `json:"publishedAt"`
}

type extMeta struct {
	Pagination extPagination `json:"pagination"`
}

type extPagination struct {
	Page      int `json:"page"`
	PageSize  int `json:"pageSize"`
	PageCount int `json:"pageCount"`
	Total     int `json:"total"`
}

type extCourse struct {
	ID          int64             `json:"id"`
	DocumentID  string            `json:"documentId"`
	Title       string            `json:"title"`
	Description []extRichText     `json:"description"`
	Modules     []extCourseModule `json:"course_modules"`
}

type extCourseModule struct {
	ID         int64         `json:"id"`
	DocumentID string        `json:"documentId"`
	Title      string        `json:"title"`
	Desc       []extRichText `json:"description"`
	Order      int           `json:"order_in_course"`
	Quiz       *extQuizBrief `json:"quiz"`
	Video      *extFile      `json:"video"`
}

type extQuizBrief struct {
	ID                            int64  `json:"id"`
	DocumentID                    string `json:"documentId"`
	Title                         string `json:"title"`
	Passing                       int    `json:"passing_score"`
	EstimatedCompletionTimeInMins int    `json:"estimated_completion_time_in_mins"`
}

type extModuleResp struct {
	Data extModule `json:"data"`
}

type extModuleList struct {
	Data []extModule `json:"data"`
}

type extModule struct {
	ID          int64         `json:"id"`
	DocumentID  string        `json:"documentId"`
	Title       string        `json:"title"`
	Description []extRichText `json:"description"`
	Type        string        `json:"type"`
	Order       int           `json:"order_in_course"`
	Video       *extFile      `json:"video"`
	Quiz        *extQuizFull  `json:"quiz"`
}

type extFile struct {
	ID         int64   `json:"id"`
	DocumentID string  `json:"documentId"`
	Name       string  `json:"name"`
	Mime       string  `json:"mime"`
	Size       float64 `json:"size"`
	URL        string  `json:"url"`
}

type extQuizFull struct {
	ID                            int64  `json:"id"`
	DocumentID                    string `json:"documentId"`
	Title                         string `json:"title"`
	QuizType                      string `json:"quiz_type"`
	Format                        string `json:"format"`
	Passing                       int    `json:"passing_score"`
	EstimatedCompletionTimeInMins int    `json:"estimated_completion_time_in_mins"`
}

type extQuizResp struct {
	Data extQuizWithQuestions `json:"data"`
}

type extQuizWithQuestions struct {
	ID                            int64         `json:"id"`
	DocumentID                    string        `json:"documentId"`
	Title                         string        `json:"title"`
	QuizType                      string        `json:"quiz_type"`
	Format                        string        `json:"format"`
	Passing                       int           `json:"passing_score"`
	EstimatedCompletionTimeInMins int           `json:"estimated_completion_time_in_mins"`
	Questions                     []extQuestion `json:"questions"`
}

type extQuestion struct {
	ID            int64             `json:"id"`
	DocumentID    string            `json:"documentId"`
	Prompt        string            `json:"prompt"`
	QType         string            `json:"question_type"`
	Explanation   string            `json:"explanation"`
	AnswerOptions []extAnswerOption `json:"answer_options"`
}

type extAnswerOption struct {
	ID         int64  `json:"id"`
	DocumentID string `json:"documentId"`
	Text       string `json:"text"`
	IsCorrect  bool   `json:"is_correct"`
}

type CleanQuestion struct {
	ID            string              `json:"id"`
	Prompt        string              `json:"prompt"`
	QuestionType  string              `json:"question_type"`
	Explanation   string              `json:"explanation"`
	AnswerOptions []CleanAnswerOption `json:"answer_options"`
}

type CleanAnswerOption struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	IsCorrect bool   `json:"is_correct"`
}

type extRichText struct {
	Type     string         `json:"type"`
	Children []extRichChild `json:"children"`
}

type extRichChild struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type getChildCourseRequest struct {
	ID       string `uri:"id" binding:"required"`
	CourseID string `uri:"course_id" binding:"required"`
}

func (server *Server) getChildCourse(ctx *gin.Context) {
	var getCourseReq getChildCourseRequest
	if !bindAndValidateUri(ctx, &getCourseReq) {
		return
	}
	server.renderCourseForChild(ctx, getCourseReq.ID, getCourseReq.CourseID)
}

// renderCourseForChild fetches and renders a course for the given childID + courseID.
// Shared by both the parent-auth (getChildCourse) and child-auth (getMyCourse) handlers.
func (server *Server) renderCourseForChild(ctx *gin.Context, childID, courseID string) {
	base := strings.TrimRight(server.config.ExternalContentBaseURL, "/")
	if base == "" {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "external content base URL not configured"})
		return
	}

	reqURL := fmt.Sprintf("%s/api/courses/%s?populate[course_modules][populate]=quiz", base, courseID)

	// HTTP client with timeout
	timeout := server.config.ExternalRequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to build external request")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if server.config.ExternalContentToken != "" {
		req.Header.Set("Authorization", "Bearer "+server.config.ExternalContentToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Str("url", reqURL).Msg("external request failed")
		ctx.JSON(http.StatusBadGateway, errorResponse(err))
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Str("body", string(body)).Msg("external non-200")
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed fetching course", "status": resp.StatusCode})
		return
	}

	var ext extCourseResponse
	if err := json.Unmarshal(body, &ext); err != nil {
		log.Error().Err(err).Str("body", string(body)).Msg("failed to parse external response")
		ctx.JSON(http.StatusBadGateway, errorResponse(err))
		return
	}

	c := ext.Data

	// Log that the child has started the course and notify the parent
	go server.logActivityAndNotify(ctx, childID, "course_started", courseID, c.Title)

	// Get module IDs from the external response
	moduleIDs := make([]string, len(c.Modules))
	for i, m := range c.Modules {
		moduleIDs[i] = m.DocumentID
	}

	// Get module progress from our DB
	progress, err := server.store.GetChildModuleProgressForCourse(ctx, db.GetChildModuleProgressForCourseParams{
		ChildID:   childID,
		CourseID:  courseID,
		ModuleIds: moduleIDs,
	})
	if err != nil && err != sql.ErrNoRows {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Create a map for quick lookup of completion status
	progressMap := make(map[string]bool)
	for _, p := range progress {
		progressMap[p.ModuleID] = p.IsCompleted
	}

	totalQuizzes := 0
	for _, m := range c.Modules {
		if m.Quiz != nil {
			totalQuizzes++
		}
	}

	clean := CleanCourse{
		ID:                   c.DocumentID,
		Title:                c.Title,
		Description:          flattenDescription(c.Description),
		TotalNumberOfModules: len(c.Modules),
		TotalNumberOfQuizzes: totalQuizzes,
	}

	clean.Modules = make([]CleanModule, len(c.Modules))
	for i, m := range c.Modules {
		// Fetch full module data to get description and video
		moduleData, err := server.fetchExternalModuleData(ctx, m.DocumentID)
		if err != nil {
			// Log the error but don't fail the entire course request
			log.Error().Err(err).Str("module_id", m.DocumentID).Msg("failed to fetch external module data")
			clean.Modules[i] = CleanModule{
				ID:          m.DocumentID,
				Title:       m.Title,
				Order:       m.Order,
				IsCompleted: progressMap[m.DocumentID],
			}
			continue
		}

		var cleanVideo *CleanVideo
		if moduleData.Video != nil {
			cleanVideo = &CleanVideo{
				URL:  server.absoluteURL(moduleData.Video.URL),
				Mime: moduleData.Video.Mime,
				Size: moduleData.Video.Size,
			}
		}

		clean.Modules[i] = CleanModule{
			ID:          m.DocumentID,
			Title:       m.Title,
			Order:       m.Order,
			Description: flattenDescription(moduleData.Description),
			IsCompleted: progressMap[m.DocumentID],
			Video:       cleanVideo,
		}
	}

	log.Info().Str("course_id", c.DocumentID).Str("title", c.Title).Msg("successfully retrieved course")
	ctx.JSON(http.StatusOK, gin.H{"course": clean})
}

// getMyCourse is the child-auth equivalent of getChildCourse.
// GET /v1/child/course/:course_id — child uses their own device token.
func (server *Server) getMyCourse(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)

	var req struct {
		CourseID string `uri:"course_id" binding:"required"`
	}
	if !bindAndValidateUri(ctx, &req) {
		return
	}

	server.renderCourseForChild(ctx, child.ID, req.CourseID)
}

// GET /v1/child/course/:course_id/module/:module_id
func (server *Server) getMyModule(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)
	var req struct {
		CourseID string `uri:"course_id" binding:"required"`
		ModuleID string `uri:"module_id" binding:"required,alphanum"`
	}
	if !bindAndValidateUri(ctx, &req) {
		return
	}

	module, err := server.fetchExternalModuleData(ctx, req.ModuleID)
	if err != nil {
		return
	}

	progress, err := server.store.GetChildModuleProgressForCourse(ctx, db.GetChildModuleProgressForCourseParams{
		ChildID:   child.ID,
		CourseID:  req.CourseID,
		ModuleIds: []string{req.ModuleID},
	})
	if err != nil && err != sql.ErrNoRows {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	isCompleted := false
	if len(progress) > 0 {
		isCompleted = progress[0].IsCompleted
	}

	clean := CleanModule{
		ID:          module.DocumentID,
		Title:       module.Title,
		Order:       module.Order,
		Description: flattenDescription(module.Description),
		IsCompleted: isCompleted,
	}

	go server.logActivityAndNotify(ctx, child.ID, "module_started", req.ModuleID, module.Title)

	if module.Video != nil {
		clean.Video = &CleanVideo{
			URL:  server.absoluteURL(module.Video.URL),
			Mime: module.Video.Mime,
			Size: module.Video.Size,
		}
	}
	if module.Quiz != nil {
		clean.Quiz = &CleanQuiz{
			ID:                            module.Quiz.DocumentID,
			Title:                         module.Quiz.Title,
			Format:                        module.Quiz.Format,
			PassingScore:                  module.Quiz.Passing,
			EstimatedCompletionTimeInMins: module.Quiz.EstimatedCompletionTimeInMins,
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"module": clean})
}

// GET /v1/child/course/:course_id/module/:module_id/quiz/:quiz_id
func (server *Server) getMyQuiz(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)
	var req struct {
		CourseID string `uri:"course_id" binding:"required"`
		ModuleID string `uri:"module_id" binding:"required"`
		QuizID   string `uri:"quiz_id" binding:"required"`
	}
	if !bindAndValidateUri(ctx, &req) {
		return
	}

	quizData, err := server.fetchExternalQuizData(ctx, req.QuizID)
	if err != nil {
		return
	}

	cleanQuiz := CleanQuiz{
		ID:                            quizData.DocumentID,
		Title:                         quizData.Title,
		Format:                        quizData.Format,
		PassingScore:                  quizData.Passing,
		EstimatedCompletionTimeInMins: quizData.EstimatedCompletionTimeInMins,
	}
	for _, q := range quizData.Questions {
		cq := CleanQuestion{
			ID:           q.DocumentID,
			Prompt:       q.Prompt,
			QuestionType: q.QType,
			Explanation:  q.Explanation,
		}
		for _, ao := range q.AnswerOptions {
			cq.AnswerOptions = append(cq.AnswerOptions, CleanAnswerOption{
				ID:        ao.DocumentID,
				Text:      ao.Text,
				IsCorrect: ao.IsCorrect,
			})
		}
		cleanQuiz.Questions = append(cleanQuiz.Questions, cq)
	}

	attempts, err := server.store.GetChildQuizAttempts(ctx, db.GetChildQuizAttemptsParams{
		ChildID:        child.ID,
		CourseID:       req.CourseID,
		ModuleID:       req.ModuleID,
		ExternalQuizID: req.QuizID,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	var lastAttemptScore float64
	var lastAttemptPassed bool
	if len(attempts) > 0 {
		lastAttemptPassed = attempts[0].Passed
		if attempts[0].Score.Valid {
			if v, err2 := attempts[0].Score.Float64Value(); err2 == nil {
				lastAttemptScore = v.Float64
			}
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"attempts_count":      len(attempts),
		"last_attempt_score":  lastAttemptScore,
		"last_attempt_passed": lastAttemptPassed,
		"passing_score":       cleanQuiz.PassingScore,
		"attempts":            attempts,
		"quiz":                cleanQuiz,
	})
}

// POST /v1/child/course/:course_id/module/:module_id/quiz/:quiz_id/submit
func (server *Server) submitMyQuizAnswers(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)
	var req SubmitQuizAnswersRequest
	var uri struct {
		CourseID string `uri:"course_id"`
		ModuleID string `uri:"module_id"`
		QuizID   string `uri:"quiz_id" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	if err := ctx.ShouldBindUri(&uri); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	if uri.ModuleID != "" {
		moduleData, err := server.fetchExternalModuleData(ctx, uri.ModuleID)
		if err != nil {
			return
		}
		if moduleData.Quiz == nil || moduleData.Quiz.DocumentID != uri.QuizID {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "quiz does not belong to the specified module"})
			return
		}
	}

	quizData, err := server.fetchExternalQuizData(ctx, uri.QuizID)
	if err != nil {
		return
	}

	questionsMap := make(map[string]extQuestion)
	for _, q := range quizData.Questions {
		questionsMap[q.DocumentID] = q
	}

	attempts, err := server.store.GetChildQuizAttempts(ctx, db.GetChildQuizAttemptsParams{
		ChildID:        child.ID,
		CourseID:       uri.CourseID,
		ModuleID:       uri.ModuleID,
		ExternalQuizID: uri.QuizID,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	var alreadyPassed bool
	for _, attempt := range attempts {
		if attempt.Passed {
			alreadyPassed = true
			break
		}
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
			} else {
				correctnessStatus = "false"
			}
		} else {
			if len(answer.SelectedOptionIds) == 1 && len(correctOptionIDs) == 1 && answer.SelectedOptionIds[0] == correctOptionIDs[0] {
				isCorrectForTx = true
				correctnessStatus = "true"
				scoreForTx = 100
				submittedCorrectOptions = correctOptionIDs
			} else {
				correctnessStatus = "false"
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
			ChildID:              child.ID,
			CourseID:             uri.CourseID,
			ModuleID:             uri.ModuleID,
			ExternalQuizID:       uri.QuizID,
			Answers:              answerParamsForTx,
			Context:              util.ContextModule,
			ContextRef:           uri.ModuleID,
			TotalQuestionsInQuiz: len(quizData.Questions),
			PassingScore:         quizData.Passing,
		}
		txResult, err := server.store.SubmitQuizAnswersTx(ctx, txParams)
		if err != nil {
			log.Error().Err(err).Msg("failed to submit quiz answers transaction")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		if txResult.Attempt.Passed {
			go server.sendParentNotification(
				txResult.Child, txResult.Parent,
				"Quiz Completed!",
				fmt.Sprintf("%s just passed the '%s' quiz!", txResult.Child.FirstName, quizData.Title),
			)
		}
		if txResult.ModuleUpdated && uri.ModuleID != "" {
			moduleData, err := server.fetchExternalModuleData(ctx, uri.ModuleID)
			if err == nil {
				go server.sendParentNotification(
					txResult.Child, txResult.Parent,
					"Module Completed!",
					fmt.Sprintf("%s just completed the '%s' module!", txResult.Child.FirstName, moduleData.Title),
				)
			}
			go server.tryAutoCompleteCourseIfEligible(context.Background(), child.ID, uri.CourseID)
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"results": results})
}

func (server *Server) getChildCourseModule(ctx *gin.Context) {
	var req getChildCourseModuleRequest
	if !bindAndValidateUri(ctx, &req) {
		return
	}

	// Fetch module from external API
	module, err := server.fetchExternalModuleData(ctx, req.ModuleID)
	if err != nil {
		// fetchExternalModuleData handles the response
		return
	}

	// Get module progress from our DB
	progress, err := server.store.GetChildModuleProgressForCourse(ctx, db.GetChildModuleProgressForCourseParams{
		ChildID:   req.ID,
		CourseID:  req.CourseID,
		ModuleIds: []string{req.ModuleID},
	})
	if err != nil && err != sql.ErrNoRows {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	isCompleted := false
	if len(progress) > 0 {
		isCompleted = progress[0].IsCompleted
	}

	clean := CleanModule{
		ID:          module.DocumentID,
		Title:       module.Title,
		Order:       module.Order,
		Description: flattenDescription(module.Description),
		IsCompleted: isCompleted,
	}

	// Log that the child has started the module and notify the parent
	go server.logActivityAndNotify(ctx, req.ID, "module_started", req.ModuleID, module.Title)

	if module.Video != nil {
		clean.Video = &CleanVideo{
			URL:  server.absoluteURL(module.Video.URL),
			Mime: module.Video.Mime,
			Size: module.Video.Size,
		}
	}
	if module.Quiz != nil {
		clean.Quiz = &CleanQuiz{
			ID:                            module.Quiz.DocumentID,
			Title:                         module.Quiz.Title,
			Format:                        module.Quiz.Format,
			PassingScore:                  module.Quiz.Passing,
			EstimatedCompletionTimeInMins: module.Quiz.EstimatedCompletionTimeInMins,
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"module": clean})
}

type ExternalQuizResponseWithExt struct {
	Data extQuizWithQuestions `json:"data"`
}

type QuizSubmission struct {
	QuestionID        string   `json:"question_id" binding:"required"`
	SelectedOptionIds []string `json:"selected_option_ids" binding:"required"`
}

type SubmitQuizAnswersRequest struct {
	Answers []QuizSubmission `json:"answers" binding:"required"`
}

type submitQuizUri struct {
	ID       string `uri:"id" binding:"required"`
	CourseID string `uri:"course_id"` // Optional
	ModuleID string `uri:"module_id"` // Optional
	QuizID   string `uri:"quiz_id" binding:"required"`
}

type QuizSubmissionResult struct {
	QuestionID              string   `json:"question_id"`
	IsCorrect               string   `json:"is_correct"`
	Status                  string   `json:"status,omitempty"`
	SubmittedCorrectOptions []string `json:"submitted_correct_options"`
}

func (server *Server) submitQuizAnswers(ctx *gin.Context) {
	var req SubmitQuizAnswersRequest
	var uri submitQuizUri

	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("failed to bind JSON request")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	if err := ctx.ShouldBindUri(&uri); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("failed to bind URI")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	contextType := util.ContextModule
	contextRef := uri.ModuleID

	boosterID := ctx.Query("booster_id")
	if boosterID != "" {
		contextType = util.ContextBooster
		contextRef = boosterID
		// If it's a booster, we need to find its external module ID to validate the quiz
		booster, err := server.store.GetBoosterByID(ctx, boosterID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				ctx.JSON(http.StatusNotFound, gin.H{"error": "booster not found"})
				return
			}
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		uri.ModuleID = booster.ExternalModuleID // Use the booster's module ID for the check below
	}

	if uri.ModuleID != "" {
		var moduleData *extModule
		var err error

		if contextType == util.ContextBooster {
			moduleData, err = server.fetchExternalWeeklyModuleData(ctx, uri.ModuleID)
		} else {
			moduleData, err = server.fetchExternalModuleData(ctx, uri.ModuleID)
		}

		if err != nil {
			return // Error is handled and response sent by the helper function
		}
		if moduleData.Quiz == nil || moduleData.Quiz.DocumentID != uri.QuizID {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "quiz does not belong to the specified module"})
			return
		}
	}

	quizData, err := server.fetchExternalQuizData(ctx, uri.QuizID)
	if err != nil {
		return // Error is handled and response sent by the helper function
	}

	questionsMap := make(map[string]extQuestion)
	for _, q := range quizData.Questions {
		questionsMap[q.DocumentID] = q
	}

	attempts, err := server.store.GetChildQuizAttempts(ctx, db.GetChildQuizAttemptsParams{
		ChildID:        uri.ID,
		CourseID:       uri.CourseID,
		ModuleID:       uri.ModuleID,
		ExternalQuizID: uri.QuizID,
	})
	if err != nil && err != sql.ErrNoRows {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	var alreadyPassed bool
	for _, attempt := range attempts {
		if attempt.Passed {
			alreadyPassed = true
			break
		}
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
			ChildID:              uri.ID,
			CourseID:             uri.CourseID,
			ModuleID:             uri.ModuleID,
			ExternalQuizID:       uri.QuizID,
			Answers:              answerParamsForTx,
			Context:              contextType,
			ContextRef:           contextRef,
			TotalQuestionsInQuiz: len(quizData.Questions),
			PassingScore:         quizData.Passing,
		}
		txResult, err := server.store.SubmitQuizAnswersTx(ctx, txParams)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to submit quiz answers transaction")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}

		if txResult.Attempt.Passed {
			go server.sendParentNotification(
				txResult.Child,
				txResult.Parent,
				"Quiz Completed!",
				fmt.Sprintf("%s just passed the '%s' quiz!", txResult.Child.FirstName, quizData.Title),
			)
		}

		if txResult.ModuleUpdated {
			moduleData, err := server.fetchExternalModuleData(ctx, uri.ModuleID)
			if err == nil {
				go server.sendParentNotification(
					txResult.Child,
					txResult.Parent,
					"Module Completed!",
					fmt.Sprintf("%s just completed the '%s' module!", txResult.Child.FirstName, moduleData.Title),
				)
			}
			go server.tryAutoCompleteCourseIfEligible(context.Background(), uri.ID, uri.CourseID)
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"results": results})
}

// sendParentNotification is a helper function to send notifications to a parent.
func (server *Server) logActivityAndNotify(ctx *gin.Context, childID, activityType, activityRefID, activityTitle string) {
	// Log the activity. ON CONFLICT DO NOTHING ensures this only runs once.
	activity, err := server.store.LogChildActivity(ctx, db.LogChildActivityParams{
		ChildID:       childID,
		ActivityType:  db.ChildActivityType(activityType),
		ActivityRefID: activityRefID,
	})
	if err != nil || activity.ID == uuid.Nil {
		// If err is not nil, or if the returned activity ID is nil, it means the activity was already logged.
		return
	}

	// Get child and parent details for the notification
	child, err := server.store.GetChild(ctx, childID)
	if err != nil {
		return
	}
	parent, err := server.store.GetParentByFamilyID(ctx, child.FamilyID)
	if err != nil {
		return
	}

	var title, message string
	switch activityType {
	case "course_started":
		title = "Course Started!"
		message = fmt.Sprintf("%s has just started the '%s' course!", child.FirstName, activityTitle)
	case "module_started":
		title = "Module Started!"
		message = fmt.Sprintf("%s has just started the '%s' module!", child.FirstName, activityTitle)
	case "digital_permit_test_completed":
		title = "Digital Permit Test Completed!"
		message = fmt.Sprintf("%s has completed the Digital Permit Test!", child.FirstName)
	case "digital_permit_test_started":
		title = "Digital Permit Test Started!"
		message = fmt.Sprintf("%s has started the Digital Permit Test!", child.FirstName)
	}

	server.sendParentNotification(child, parent, title, message)
}

func (server *Server) sendParentNotification(_ db.Child, parent db.Parent, title, message string) {
	ctx := context.Background()

	log.Info().
		Str("parent_id", parent.ParentID).
		Str("title", title).
		Msg("sendParentNotification: saving DB record and sending FCM push")

	parentUUID, err := uuid.Parse(parent.ParentID)
	if err != nil {
		log.Error().Err(err).Str("parent_id", parent.ParentID).Msg("sendParentNotification: invalid parent UUID")
		return
	}

	// Always persist the notification record in the database.
	_, err = server.store.CreateNotification(ctx, db.CreateNotificationParams{
		RecipientID:   parentUUID,
		RecipientType: db.NotificationRecipientTypeParent,
		Title:         title,
		Message:       message,
		SentAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		log.Error().Err(err).Str("parent_id", parent.ParentID).Msg("sendParentNotification: failed to save notification to database")
		// Continue — still attempt push delivery even if DB write failed.
	} else {
		log.Info().Str("parent_id", parent.ParentID).Msg("sendParentNotification: notification record saved to database")
	}

	// Deliver the FCM push notification via the notification service.
	if server.notificationService == nil {
		log.Warn().Str("parent_id", parent.ParentID).Msg("sendParentNotification: notification service not initialised, FCM push skipped")
		return
	}

	if err := server.notificationService.SendPush(ctx, parentUUID, title, message); err != nil {
		log.Error().Err(err).Str("parent_id", parent.ParentID).Msg("sendParentNotification: FCM push failed")
	} else {
		log.Info().Str("parent_id", parent.ParentID).Msg("sendParentNotification: FCM push dispatched successfully")
	}
}

func (server *Server) fetchAllExternalWeeklyModules(ctx *gin.Context) ([]extModule, error) {
	base := strings.TrimRight(server.config.ExternalContentBaseURL, "/")
	if base == "" {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "external content base URL not configured"})
		return nil, fmt.Errorf("external content base URL not configured")
	}

	reqURL := fmt.Sprintf("%s/api/course-modules?filters[type][$eq]=weekly&populate[0]=video&populate[1]=quiz", base)
	timeout := server.config.ExternalRequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to build external request for all weekly modules")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return nil, err
	}
	if server.config.ExternalContentToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+server.config.ExternalContentToken)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Str("url", reqURL).Msg("external all weekly modules request failed")
		ctx.JSON(http.StatusBadGateway, errorResponse(err))
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Str("body", string(body)).Msg("external all weekly modules request returned non-200 status")
		ctx.JSON(resp.StatusCode, gin.H{"error": "failed to fetch all weekly modules from external source"})
		return nil, fmt.Errorf("failed to fetch all weekly modules")
	}

	var extModuleListResp extModuleList
	if err := json.Unmarshal(body, &extModuleListResp); err != nil {
		log.Error().Err(err).Str("body", string(body)).Msg("failed to unmarshal external all weekly modules list response")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return nil, err
	}

	return extModuleListResp.Data, nil
}

func (server *Server) fetchExternalWeeklyModuleData(ctx *gin.Context, moduleID string) (*extModule, error) {
	base := strings.TrimRight(server.config.ExternalContentBaseURL, "/")
	if base == "" {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "external content base URL not configured"})
		return nil, fmt.Errorf("external content base URL not configured")
	}

	reqURL := fmt.Sprintf("%s/api/course-modules?filters[type][$eq]=weekly&filters[documentId][$eq]=%s&populate[0]=video&populate[1]=quiz", base, url.PathEscape(moduleID))
	timeout := server.config.ExternalRequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to build external request for weekly module")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return nil, err
	}
	if server.config.ExternalContentToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+server.config.ExternalContentToken)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Str("url", reqURL).Msg("external weekly module request failed")
		ctx.JSON(http.StatusBadGateway, errorResponse(err))
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Str("body", string(body)).Msg("external weekly module request returned non-200 status")
		ctx.JSON(resp.StatusCode, gin.H{"error": "failed to fetch weekly module from external source"})
		return nil, fmt.Errorf("failed to fetch weekly module")
	}

	var extModuleListResp extModuleList
	if err := json.Unmarshal(body, &extModuleListResp); err != nil {
		log.Error().Err(err).Str("body", string(body)).Msg("failed to unmarshal external weekly module list response")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return nil, err
	}

	if len(extModuleListResp.Data) == 0 {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "weekly module not found"})
		return nil, fmt.Errorf("weekly module with id %s not found", moduleID)
	}

	return &extModuleListResp.Data[0], nil
}

func (server *Server) fetchExternalModuleData(ctx *gin.Context, moduleID string) (*extModule, error) {
	base := strings.TrimRight(server.config.ExternalContentBaseURL, "/")
	if base == "" {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "external content base URL not configured"})
		return nil, fmt.Errorf("external content base URL not configured")
	}

	reqURL := fmt.Sprintf("%s/api/course-modules/%s?populate[0]=video&populate[1]=quiz", base, url.PathEscape(moduleID))
	timeout := server.config.ExternalRequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to build external request for module")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return nil, err
	}
	if server.config.ExternalContentToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+server.config.ExternalContentToken)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Str("url", reqURL).Msg("external module request failed")
		ctx.JSON(http.StatusBadGateway, errorResponse(err))
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "module not found"})
			return nil, fmt.Errorf("module not found")
		}
		log.Error().Int("status", resp.StatusCode).Str("body", string(body)).Msg("external module request returned non-200 status")
		ctx.JSON(resp.StatusCode, gin.H{"error": "failed to fetch module from external source"})
		return nil, fmt.Errorf("failed to fetch module")
	}

	var extModuleResp extModuleResp
	if err := json.Unmarshal(body, &extModuleResp); err != nil {
		log.Error().Err(err).Str("body", string(body)).Msg("failed to unmarshal external module response")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return nil, err
	}

	return &extModuleResp.Data, nil
}

type getChildCourseModuleRequest struct {
	ID       string `uri:"id" binding:"required"`
	CourseID string `uri:"course_id" binding:"required"`
	ModuleID string `uri:"module_id" binding:"required,alphanum"`
}

type getChildQuizAttemptsRequest struct {
	ID       string `uri:"id" binding:"required"`
	CourseID string `uri:"course_id" binding:"required"`
	ModuleID string `uri:"module_id" binding:"required"`
	QuizID   string `uri:"quiz_id" binding:"required"`
}

func (server *Server) getQuizAttemptsForChild(ctx *gin.Context) {
	var req getChildQuizAttemptsRequest
	if !bindAndValidateUri(ctx, &req) {
		return
	}

	log.Info().Str("child_id", req.ID).Str("quiz_id", req.QuizID).Msg("getting child quiz attempts")

	authPayload := server.getAuthPayload(ctx)
	if authPayload == nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authorization payload not found"})
		return
	}

	isParent, err := server.isParentOfChild(ctx, authPayload.UserID, req.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check parentage"})
		return
	}
	if !isParent {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "you are not authorized to view this child's data"})
		return
	}

	// Fetch quiz from external provider to get passing_score
	base := strings.TrimRight(server.config.ExternalContentBaseURL, "/")
	if base == "" {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "external content base URL not configured"})
		return
	}

	reqURL := fmt.Sprintf("%s/api/quizzes/%s?populate[questions][populate]=answer_options", base, url.PathEscape(req.QuizID))

	timeout := server.config.ExternalRequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to build external request")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if server.config.ExternalContentToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+server.config.ExternalContentToken)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Str("url", reqURL).Msg("external request failed")
		ctx.JSON(http.StatusBadGateway, errorResponse(err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "quiz not found"})
			return
		}
		log.Error().Int("status", resp.StatusCode).Str("body", string(body)).Msg("external request returned non-200 status")
		ctx.JSON(resp.StatusCode, gin.H{"error": "failed to fetch quiz from external source"})
		return
	}

	var extQuizResp ExternalQuizResponseWithExt
	if err := json.Unmarshal(body, &extQuizResp); err != nil {
		log.Error().Err(err).Str("body", string(body)).Msg("failed to unmarshal external quiz response")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	q := extQuizResp.Data
	cleanQuiz := CleanQuiz{
		ID:                            q.DocumentID,
		Title:                         q.Title,
		Format:                        q.Format,
		PassingScore:                  q.Passing,
		EstimatedCompletionTimeInMins: q.EstimatedCompletionTimeInMins,
	}
	for _, que := range q.Questions {
		cq := CleanQuestion{
			ID:           que.DocumentID,
			Prompt:       que.Prompt,
			QuestionType: que.QType,
			Explanation:  que.Explanation,
		}
		for _, ao := range que.AnswerOptions {
			cq.AnswerOptions = append(cq.AnswerOptions, CleanAnswerOption{
				ID:        ao.DocumentID,
				Text:      ao.Text,
				IsCorrect: ao.IsCorrect,
			})
		}
		cleanQuiz.Questions = append(cleanQuiz.Questions, cq)
	}

	attempts, err := server.store.GetChildQuizAttempts(ctx, db.GetChildQuizAttemptsParams{
		ChildID:        req.ID,
		CourseID:       req.CourseID,
		ModuleID:       req.ModuleID,
		ExternalQuizID: req.QuizID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Info().Str("child_id", req.ID).Str("quiz_id", req.QuizID).Msg("no quiz attempts found for child")
			ctx.JSON(http.StatusOK, gin.H{
				"attempts_count": 0,
				"quiz":           cleanQuiz,
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	log.Info().Int("attempt_count", len(attempts)).Msg("retrieved quiz attempts")

	var lastAttemptScore float64 = 0
	var lastAttemptPassed bool = false
	if len(attempts) > 0 {
		// The first attempt in the slice is the latest one because of the ORDER BY clause.
		lastAttempt := attempts[0]

		lastAttemptPassed = lastAttempt.Passed
		if lastAttempt.Score.Valid {
			float8Val, err := lastAttempt.Score.Float64Value()
			if err == nil {
				lastAttemptScore = float8Val.Float64
			}
		}
	}

	response := gin.H{
		"attempts_count":      len(attempts),
		"last_attempt_score":  lastAttemptScore,
		"last_attempt_passed": lastAttemptPassed,
		"passing_score":       cleanQuiz.PassingScore,
		"attempts":            attempts,
		"quiz":                cleanQuiz,
	}

	log.Info().Str("child_id", req.ID).Str("quiz_id", req.QuizID).Int("attempts", len(attempts)).Msg("successfully retrieved child quiz attempts")
	ctx.JSON(http.StatusOK, response)
}

// Response model for a single course in the list of all courses
// This is a simplified version of CleanCourse
type CleanCourseListItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// getAllCourses fetches all available courses from the external content API.
// GET /v1/parent/child/:id/courses/stats
type getCourseStatsForChildRequest struct {
	ID string `uri:"id" binding:"required,uuid"`
}

type CourseStatsResponse struct {
	EstimatedTotalTimeTakenInMins int64   `json:"estimated_total_time_taken_in_mins"`
	TotalCourses                  int     `json:"total_courses"`
	TotalQuizzes                  int     `json:"total_quizzes"`
	ModuleCompletionRate          float64 `json:"module_completion_rate"`
	IsStarted                     bool    `json:"is_started"`
}

type courseDetailResult struct {
	courseData *extCourse
	err        error
}

func (server *Server) buildCourseStatsResponse(ctx *gin.Context, childID string) (*CourseStatsResponse, error) {
	allCourses, err := server.fetchAllExternalCourses(ctx)
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	resultsChan := make(chan courseDetailResult, len(allCourses))

	for _, courseItem := range allCourses {
		wg.Add(1)
		go func(courseID string) {
			defer wg.Done()
			courseData, err := server.fetchExternalCourseData(ctx, courseID)
			resultsChan <- courseDetailResult{courseData: courseData, err: err}
		}(courseItem.DocumentID)
	}

	wg.Wait()
	close(resultsChan)

	var totalTimeTakenMins float64
	var totalQuizzes, totalModules, completedModules int

	for result := range resultsChan {
		if result.err != nil {
			log.Warn().Err(result.err).Msg("failed to fetch course details for stats")
			continue
		}

		courseData := result.courseData
		totalModules += len(courseData.Modules)
		moduleIDs := make([]string, len(courseData.Modules))
		completedModuleIDs := make(map[string]struct{}, len(courseData.Modules))

		for i, module := range courseData.Modules {
			moduleIDs[i] = module.DocumentID
			if module.Quiz != nil {
				totalQuizzes++
				totalTimeTakenMins += float64(module.Quiz.EstimatedCompletionTimeInMins)
			}
			if module.Video != nil {
				totalTimeTakenMins += module.Video.Size / 10
			}
		}

		if len(moduleIDs) > 0 {
			progress, err := server.store.GetChildModuleProgressForCourse(ctx, db.GetChildModuleProgressForCourseParams{
				ChildID:   childID,
				CourseID:  courseData.DocumentID,
				ModuleIds: moduleIDs,
			})
			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}
			for _, p := range progress {
				if p.IsCompleted {
					completedModuleIDs[p.ModuleID] = struct{}{}
				}
			}

			passedModuleIDs, err := server.store.GetPassedModuleIDsForCourse(ctx, db.GetPassedModuleIDsForCourseParams{
				ChildID:   childID,
				CourseID:  courseData.DocumentID,
				ModuleIDs: moduleIDs,
			})
			if err != nil {
				return nil, err
			}
			for _, moduleID := range passedModuleIDs {
				completedModuleIDs[moduleID] = struct{}{}
			}

			completedModules += len(completedModuleIDs)
		}
	}

	var moduleCompletionRate float64
	if totalModules > 0 {
		moduleCompletionRate = math.Round((float64(completedModules)/float64(totalModules))*100*100) / 100
	}

	isStarted, err := server.store.CheckQuizAttemptExistsForChild(ctx, childID)
	if err != nil {
		return nil, err
	}
	if !isStarted {
		isStarted, err = server.store.CheckChildModuleProgressExists(ctx, childID)
		if err != nil {
			return nil, err
		}
	}

	stats := &CourseStatsResponse{
		EstimatedTotalTimeTakenInMins: int64(totalTimeTakenMins),
		TotalCourses:                  len(allCourses),
		TotalQuizzes:                  totalQuizzes,
		ModuleCompletionRate:          moduleCompletionRate,
		IsStarted:                     isStarted,
	}

	return stats, nil
}

func (server *Server) getCoursesStatsForChild(ctx *gin.Context) {
	var req getCourseStatsForChildRequest
	if !bindAndValidateUri(ctx, &req) {
		return
	}

	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	child, err := server.store.GetChildForParent(ctx, db.GetChildForParentParams{
		ParentID: authPayload.UserID,
		ID:       req.ID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ctx.JSON(http.StatusForbidden, errorResponse(fmt.Errorf("you do not have access to this child")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	stats, err := server.buildCourseStatsResponse(ctx, child.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, stats)
}

// GET /v1/parent/child/:id/courses
type getAllCoursesForChildRequest struct {
	ID string `uri:"id" binding:"required"`
}

type CleanCourseWithStatus struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Description          string `json:"description"`
	IsCompleted          bool   `json:"is_completed"`
	IsLocked             bool   `json:"is_locked"`
	TotalNumberOfModules int    `json:"total_number_of_modules"`
	CompletedModules     int    `json:"completed_modules"`
}

func (server *Server) getAllCoursesForChild(ctx *gin.Context) {
	var req getAllCoursesForChildRequest
	if !bindAndValidateUri(ctx, &req) {
		return
	}

	allCourses, err := server.fetchAllExternalCourses(ctx)
	if err != nil {
		return
	}

	var responseCourses []CleanCourseWithStatus
	var firstIncompleteCourseFound bool

	for _, courseItem := range allCourses {
		courseData, err := server.fetchExternalCourseData(ctx, courseItem.DocumentID)
		if err != nil {
			log.Warn().Err(err).Str("course_id", courseItem.DocumentID).Msg("could not fetch course details, skipping")
			continue
		}

		if len(courseData.Modules) == 0 {
			responseCourses = append(responseCourses, CleanCourseWithStatus{
				ID:                   courseItem.DocumentID,
				Title:                courseItem.Title,
				Description:          flattenDescription(courseItem.Description),
				IsCompleted:          true,
				IsLocked:             firstIncompleteCourseFound,
				TotalNumberOfModules: 0,
				CompletedModules:     0,
			})
			continue
		}

		moduleIDs := make([]string, len(courseData.Modules))
		for i, m := range courseData.Modules {
			moduleIDs[i] = m.DocumentID
		}

		progress, err := server.store.GetChildModuleProgressForCourse(ctx, db.GetChildModuleProgressForCourseParams{
			ChildID:   req.ID,
			CourseID:  courseItem.DocumentID,
			ModuleIds: moduleIDs,
		})
		if err != nil && err != sql.ErrNoRows {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}

		completedCount := 0
		for _, p := range progress {
			if p.IsCompleted {
				completedCount++
			}
		}

		isCourseCompleted := completedCount == len(courseData.Modules)
		isLocked := false
		if firstIncompleteCourseFound {
			isLocked = true
		} else if !isCourseCompleted {
			firstIncompleteCourseFound = true
		}

		responseCourses = append(responseCourses, CleanCourseWithStatus{
			ID:                   courseItem.DocumentID,
			Title:                courseItem.Title,
			Description:          flattenDescription(courseItem.Description),
			IsCompleted:          isCourseCompleted,
			IsLocked:             isLocked,
			TotalNumberOfModules: len(courseData.Modules),
			CompletedModules:     completedCount,
		})
	}

	ctx.JSON(http.StatusOK, gin.H{"courses": responseCourses})
}

// GET /v1/parent/courses
type getLatestCourseForChildRequest struct {
	ID string `uri:"id" binding:"required"`
}

func (server *Server) getLatestCourseForChild(ctx *gin.Context) {
	var req getLatestCourseForChildRequest
	if !bindAndValidateUri(ctx, &req) {
		return
	}

	// 1. Fetch all courses from the external API
	allCourses, err := server.fetchAllExternalCourses(ctx)
	if err != nil {
		// error is handled in the function
		return
	}

	// 2. Iterate through courses to find the first one with incomplete modules
	for _, courseItem := range allCourses {
		// Fetch full course details
		courseData, err := server.fetchExternalCourseData(ctx, courseItem.DocumentID)
		if err != nil {
			log.Warn().Err(err).Str("course_id", courseItem.DocumentID).Msg("could not fetch course details, skipping")
			continue // or handle error more gracefully
		}

		if len(courseData.Modules) == 0 {
			continue // Skip courses with no modules
		}

		// Get module IDs from the external response
		moduleIDs := make([]string, len(courseData.Modules))
		for i, m := range courseData.Modules {
			moduleIDs[i] = m.DocumentID
		}

		// Get module progress from our DB
		progress, err := server.store.GetChildModuleProgressForCourse(ctx, db.GetChildModuleProgressForCourseParams{
			ChildID:   req.ID,
			CourseID:  courseItem.DocumentID,
			ModuleIds: moduleIDs,
		})
		if err != nil && err != sql.ErrNoRows {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}

		// Count completed modules
		completedCount := 0
		for _, p := range progress {
			if p.IsCompleted {
				completedCount++
			}
		}

		// If not all modules are complete, this is our target course
		if completedCount < len(courseData.Modules) {
			// We found the course, now we build the response similar to getChildCourse
			progressMap := make(map[string]bool)
			for _, p := range progress {
				progressMap[p.ModuleID] = p.IsCompleted
			}

			totalQuizzes := 0
			for _, m := range courseData.Modules {
				if m.Quiz != nil {
					totalQuizzes++
				}
			}

			clean := CleanCourse{
				ID:                   courseData.DocumentID,
				Title:                courseData.Title,
				Description:          flattenDescription(courseData.Description),
				TotalNumberOfModules: len(courseData.Modules),
				TotalNumberOfQuizzes: totalQuizzes,
			}

			clean.Modules = make([]CleanModule, len(courseData.Modules))
			for i, m := range courseData.Modules {
				clean.Modules[i] = CleanModule{
					ID:          m.DocumentID,
					Title:       m.Title,
					Order:       m.Order,
					IsCompleted: progressMap[m.DocumentID], // Defaults to false if not in map
				}
			}

			ctx.JSON(http.StatusOK, gin.H{"course": clean})
			return
		}
	}

	// 3. If all courses are completed or no courses exist
	ctx.JSON(http.StatusOK, gin.H{"message": "All courses completed"})
}

func (server *Server) fetchAllExternalCourses(ctx *gin.Context) ([]extCourseItem, error) {
	base := strings.TrimRight(server.config.ExternalContentBaseURL, "/")
	if base == "" {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "external content base URL not configured"})
		return nil, fmt.Errorf("external content base URL not configured")
	}

	reqURL := fmt.Sprintf("%s/api/courses", base)

	timeout := server.config.ExternalRequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to build external request for all courses")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return nil, err
	}
	if server.config.ExternalContentToken != "" {
		req.Header.Set("Authorization", "Bearer "+server.config.ExternalContentToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Str("url", reqURL).Msg("external all courses request failed")
		ctx.JSON(http.StatusBadGateway, errorResponse(err))
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Str("body", string(body)).Msg("external all courses non-200")
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed fetching all courses", "status": resp.StatusCode})
		return nil, fmt.Errorf("failed fetching all courses")
	}

	var extResp extAllCoursesResponse
	if err := json.Unmarshal(body, &extResp); err != nil {
		log.Error().Err(err).Msg("failed to parse external all courses response")
		ctx.JSON(http.StatusBadGateway, errorResponse(err))
		return nil, err
	}

	return extResp.Data, nil
}

func (server *Server) fetchExternalCourseData(_ *gin.Context, courseID string) (*extCourse, error) {
	base := strings.TrimRight(server.config.ExternalContentBaseURL, "/")
	if base == "" {
		return nil, fmt.Errorf("external content base URL not configured")
	}

	reqURL := fmt.Sprintf("%s/api/courses/%s?populate[course_modules][populate][0]=quiz&populate[course_modules][populate][1]=video", base, courseID)

	timeout := server.config.ExternalRequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	if server.config.ExternalContentToken != "" {
		req.Header.Set("Authorization", "Bearer "+server.config.ExternalContentToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("external request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var ext extCourseResponse
	if err := json.Unmarshal(body, &ext); err != nil {
		return nil, err
	}

	return &ext.Data, nil
}

func (server *Server) getAllCourses(ctx *gin.Context) {
	log.Info().Msg("getting all courses")

	base := strings.TrimRight(server.config.ExternalContentBaseURL, "/")
	if base == "" {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "external content base URL not configured"})
		return
	}

	reqURL := fmt.Sprintf("%s/api/courses", base)

	// HTTP client with timeout
	timeout := server.config.ExternalRequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to build external request")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if server.config.ExternalContentToken != "" {
		req.Header.Set("Authorization", "Bearer "+server.config.ExternalContentToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Str("url", reqURL).Msg("external request failed")
		ctx.JSON(http.StatusBadGateway, errorResponse(err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Str("body", string(body)).Msg("external non-200")
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed fetching courses", "status": resp.StatusCode})
		return
	}

	var extResp extAllCoursesResponse
	if err := json.Unmarshal(body, &extResp); err != nil {
		log.Error().Err(err).Msg("failed to parse external response for all courses")
		ctx.JSON(http.StatusBadGateway, errorResponse(err))
		return
	}

	cleanedCourses := make([]CleanCourseListItem, len(extResp.Data))
	for i, c := range extResp.Data {
		cleanedCourses[i] = CleanCourseListItem{
			ID:          c.DocumentID,
			Title:       c.Title,
			Description: flattenDescription(c.Description),
		}
	}

	log.Info().Int("count", len(cleanedCourses)).Msg("successfully retrieved all courses")
	ctx.JSON(http.StatusOK, gin.H{"courses": cleanedCourses, "meta": extResp.Meta})
}

func (server *Server) fetchExternalQuizData(ctx *gin.Context, quizID string) (*extQuizWithQuestions, error) {
	base := strings.TrimRight(server.config.ExternalContentBaseURL, "/")
	if base == "" {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "external content base URL not configured"})
		return nil, fmt.Errorf("external content base URL not configured")
	}

	reqURL := fmt.Sprintf("%s/api/quizzes/%s?populate[questions][populate]=answer_options", base, url.PathEscape(quizID))
	timeout := server.config.ExternalRequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to build external request")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return nil, err
	}
	if server.config.ExternalContentToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+server.config.ExternalContentToken)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Str("url", reqURL).Msg("external request failed")
		ctx.JSON(http.StatusBadGateway, errorResponse(err))
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "quiz not found"})
		} else {
			log.Error().Int("status", resp.StatusCode).Str("body", string(body)).Msg("external request returned non-200 status")
			ctx.JSON(resp.StatusCode, gin.H{"error": "failed to fetch quiz from external source"})
		}
		return nil, fmt.Errorf("external API error: status %d", resp.StatusCode)
	}

	var extQuizResp extQuizResp
	if err := json.Unmarshal(body, &extQuizResp); err != nil {
		log.Error().Err(err).Str("body", string(body)).Msg("failed to unmarshal external quiz response")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return nil, err
	}

	return &extQuizResp.Data, nil
}

// GET /v1/child/courses
// Returns all courses with per-course progress for the authenticated child.
func (server *Server) getMyCoursesForChild(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)
	childID := child.ID

	allCourses, err := server.fetchAllExternalCourses(ctx)
	if err != nil {
		return
	}

	var responseCourses []CleanCourseWithStatus
	var firstIncompleteCourseFound bool

	for _, courseItem := range allCourses {
		courseData, err := server.fetchExternalCourseData(ctx, courseItem.DocumentID)
		if err != nil {
			log.Warn().Err(err).Str("course_id", courseItem.DocumentID).Msg("could not fetch course details, skipping")
			continue
		}

		if len(courseData.Modules) == 0 {
			responseCourses = append(responseCourses, CleanCourseWithStatus{
				ID:                   courseItem.DocumentID,
				Title:                courseItem.Title,
				Description:          flattenDescription(courseItem.Description),
				IsCompleted:          true,
				IsLocked:             firstIncompleteCourseFound,
				TotalNumberOfModules: 0,
				CompletedModules:     0,
			})
			continue
		}

		moduleIDs := make([]string, len(courseData.Modules))
		for i, m := range courseData.Modules {
			moduleIDs[i] = m.DocumentID
		}

		progress, err := server.store.GetChildModuleProgressForCourse(ctx, db.GetChildModuleProgressForCourseParams{
			ChildID:   childID,
			CourseID:  courseItem.DocumentID,
			ModuleIds: moduleIDs,
		})
		if err != nil && err != sql.ErrNoRows {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}

		completedCount := 0
		for _, p := range progress {
			if p.IsCompleted {
				completedCount++
			}
		}

		isCourseCompleted := completedCount == len(courseData.Modules)
		isLocked := false
		if firstIncompleteCourseFound {
			isLocked = true
		} else if !isCourseCompleted {
			firstIncompleteCourseFound = true
		}

		responseCourses = append(responseCourses, CleanCourseWithStatus{
			ID:                   courseItem.DocumentID,
			Title:                courseItem.Title,
			Description:          flattenDescription(courseItem.Description),
			IsCompleted:          isCourseCompleted,
			IsLocked:             isLocked,
			TotalNumberOfModules: len(courseData.Modules),
			CompletedModules:     completedCount,
		})
	}

	ctx.JSON(http.StatusOK, gin.H{"courses": responseCourses})
}

// GET /v1/child/courses/stats
// Returns aggregate course stats for the authenticated child.
func (server *Server) getMyCoursesStatsForChild(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)
	stats, err := server.buildCourseStatsResponse(ctx, child.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, stats)
}
