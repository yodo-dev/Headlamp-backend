package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
)

const (
	trainingRoleParent = "parent"
	trainingRoleChild  = "child"

	trainingStatusLocked     = "locked"
	trainingStatusAvailable  = "available"
	trainingStatusInProgress = "in_progress"
	trainingStatusCompleted  = "completed"

	trainingStageIntroReadiness          = "intro_readiness"
	trainingStageIntroReadinessTest      = "intro_readiness_test"
	trainingStageDigitalPermitCourse     = "digital_permit_course"
	trainingStageDigitalPermitTest       = "digital_permit_test"
	trainingStageSocialMediaDriverCourse = "social_media_driver_training"
	trainingStageSocialMediaDriverTest   = "social_media_driver_test"
	trainingMilestoneFirstPermitModule   = "digital_permit_module_1_completed"
	trainingStepTypeVideo                = "video"
	trainingStepTypeQuiz                 = "quiz"
	trainingStepTypeFinalTest            = "final_test"
	trainingStageTypeCourse              = "course"
	trainingStageTypeFinalTest           = "final_test"
	trainingLinkCodeGateBlockedReason    = "Complete Digital Permit Module 1 first"
	trainingSocialConfigurationReason    = "Awaiting social training configuration"
	trainingIntroReadinessLockedReason   = "Parent must complete the first 2 videos"
	trainingPermitTestLockedReason       = "Complete all Digital Permit modules first"
	trainingSocialTrainingLockedReason   = "Pass the Digital Permit Test first"
	trainingSocialDriverTestLockedReason = "Complete Social Media Driver Training first"
)

type trainingHomeResponse struct {
	Role          string                  `json:"role"`
	ChildID       string                  `json:"child_id"`
	ChildName     string                  `json:"child_name,omitempty"`
	ThisWeeksStep *trainingCurrentStep    `json:"this_weeks_step,omitempty"`
	LinkCodeGate  trainingLinkCodeGate    `json:"link_code_gate"`
	ParentActions *trainingParentActions  `json:"parent_actions,omitempty"`
	Stages        []trainingStageResponse `json:"stages"`
}

type trainingLinkCodeGate struct {
	IsReady           bool   `json:"is_ready"`
	RequiredMilestone string `json:"required_milestone"`
}

type trainingParentActions struct {
	CanGenerateLinkCode    bool   `json:"can_generate_link_code"`
	GenerateLinkCodeReason string `json:"generate_link_code_reason,omitempty"`
}

type trainingCurrentStep struct {
	StageKey   string `json:"stage_key"`
	ModuleKey  string `json:"module_key,omitempty"`
	StepKey    string `json:"step_key"`
	StepType   string `json:"step_type"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	IsLocked   bool   `json:"is_locked"`
	LockReason string `json:"lock_reason,omitempty"`
}

type trainingStageResponse struct {
	StageKey     string                   `json:"stage_key"`
	Title        string                   `json:"title"`
	StageType    string                   `json:"stage_type"`
	Status       string                   `json:"status"`
	IsLocked     bool                     `json:"is_locked"`
	IsCompleted  bool                     `json:"is_completed"`
	IsConfigured bool                     `json:"is_configured"`
	LockReason   string                   `json:"lock_reason,omitempty"`
	Progress     *trainingStageProgress   `json:"progress,omitempty"`
	Modules      []trainingModuleResponse `json:"modules,omitempty"`
}

type trainingStageProgress struct {
	ParentVideosWatched          int  `json:"parent_videos_watched"`
	ParentVideosTotal            int  `json:"parent_videos_total"`
	ChildDeviceUnlocked          bool `json:"child_device_unlocked"`
	ChildVideoQuizPairsCompleted int  `json:"child_video_quiz_pairs_completed"`
	ChildVideoQuizPairsTotal     int  `json:"child_video_quiz_pairs_total"`
	ChatTestUnlocked             bool `json:"chat_test_unlocked"`
	ChatTestCompleted            bool `json:"chat_test_completed"`
}

type trainingModuleResponse struct {
	ModuleKey      string                 `json:"module_key"`
	Title          string                 `json:"title"`
	Description    string                 `json:"description,omitempty"`
	Order          int                    `json:"order"`
	Status         string                 `json:"status"`
	IsLocked       bool                   `json:"is_locked"`
	SourceCourseID string                 `json:"source_course_id,omitempty"`
	SocialReward   *trainingSocialReward  `json:"social_reward,omitempty"`
	Steps          []trainingStepResponse `json:"steps"`
}

type trainingSocialReward struct {
	SocialMediaID int64  `json:"social_media_id"`
	SocialMedia   string `json:"social_media"`
	RewardStatus  string `json:"reward_status"`
}

type trainingStepResponse struct {
	StepKey    string `json:"step_key"`
	StepType   string `json:"step_type"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	IsLocked   bool   `json:"is_locked"`
	LockReason string `json:"lock_reason,omitempty"`
	QuizID     string `json:"quiz_id,omitempty"`
}

type flattenedTrainingModule struct {
	CourseID    string
	ModuleID    string
	Title       string
	Description string
	Order       int
	QuizID      string
	VideoDone   bool
	QuizDone    bool
	IsCompleted bool
}

type trainingStateSummary struct {
	firstPermitModuleCompleted bool
	digitalPermitCourseDone    bool
	digitalPermitTestDone      bool
	digitalPermitTestRunning   bool
	socialTrainingConfigured   bool
	socialTrainingDone         bool
}

type completeTrainingVideoStepRequest struct {
	CompletedAt *string `json:"completed_at"`
}

func (server *Server) getChildTrainingHome(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)

	resp, err := server.buildTrainingHome(ctx.Request.Context(), child.ID, trainingRoleChild, child.FirstName)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

func (server *Server) getParentTrainingHome(ctx *gin.Context) {
	payload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	childID := ctx.Param("id")

	child, err := server.store.GetChildByIDAndFamilyID(ctx, db.GetChildByIDAndFamilyIDParams{
		ID:       childID,
		FamilyID: payload.FamilyID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("child not found or does not belong to this family")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	resp, err := server.buildTrainingHome(ctx.Request.Context(), child.ID, trainingRoleParent, child.FirstName)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

func (server *Server) completeTrainingVideoStep(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)
	server.completeTrainingVideoStepForChild(ctx, child)
}

func (server *Server) completeParentTrainingVideoStep(ctx *gin.Context) {
	payload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	childID := ctx.Param("id")

	child, err := server.store.GetChildByIDAndFamilyID(ctx, db.GetChildByIDAndFamilyIDParams{
		ID:       childID,
		FamilyID: payload.FamilyID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("child not found or does not belong to this family")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	server.completeTrainingVideoStepForChild(ctx, child)
}

func (server *Server) completeTrainingVideoStepForChild(ctx *gin.Context, child db.Child) {
	stepKey := ctx.Param("step_key")
	log.Info().
		Str("child_id", child.ID).
		Str("step_key", stepKey).
		Msg("training video completion requested")

	if !strings.HasSuffix(stepKey, "_video") {
		log.Warn().
			Str("child_id", child.ID).
			Str("step_key", stepKey).
			Msg("invalid training step key suffix")
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "step_key must reference a video step"})
		return
	}

	var req completeTrainingVideoStepRequest
	if err := ctx.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		log.Warn().Err(err).
			Str("child_id", child.ID).
			Str("step_key", stepKey).
			Msg("failed to bind training video completion payload")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	moduleID := strings.TrimSuffix(stepKey, "_video")
	moduleID = strings.TrimPrefix(moduleID, "intro_readiness_")
	log.Info().
		Str("child_id", child.ID).
		Str("step_key", stepKey).
		Str("module_id", moduleID).
		Msg("resolved training module id from step key")

	module, err := server.findDigitalPermitModule(ctx.Request.Context(), child.ID, moduleID)
	if err != nil {
		log.Error().Err(err).
			Str("child_id", child.ID).
			Str("step_key", stepKey).
			Str("module_id", moduleID).
			Msg("failed to find training module")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if module == nil {
		log.Warn().
			Str("child_id", child.ID).
			Str("step_key", stepKey).
			Str("module_id", moduleID).
			Msg("training module not found for step")
		ctx.JSON(http.StatusNotFound, gin.H{"error": "training video step not found"})
		return
	}

	accessible, err := server.isDigitalPermitModuleAccessible(ctx.Request.Context(), child.ID, moduleID)
	if err != nil {
		log.Error().Err(err).
			Str("child_id", child.ID).
			Str("step_key", stepKey).
			Str("module_id", moduleID).
			Msg("failed to evaluate module accessibility")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if !accessible {
		log.Warn().
			Str("child_id", child.ID).
			Str("step_key", stepKey).
			Str("module_id", moduleID).
			Msg("training module is locked")
		ctx.JSON(http.StatusForbidden, gin.H{"error": "video step is locked until previous required modules are completed"})
		return
	}

	idempotent := module.VideoDone
	stageKey := trainingStageDigitalPermitCourse
	if strings.HasPrefix(stepKey, "intro_readiness_") {
		stageKey = trainingStageIntroReadiness
	}
	log.Info().
		Str("child_id", child.ID).
		Str("step_key", stepKey).
		Str("module_id", moduleID).
		Str("stage_key", stageKey).
		Bool("idempotent", idempotent).
		Msg("upserting training step progress")

	_, err = server.store.UpsertTrainingStepProgress(ctx.Request.Context(), db.UpsertTrainingStepProgressParams{
		ChildID:   child.ID,
		StageKey:  stageKey,
		ModuleKey: pgtype.Text{String: moduleID, Valid: true},
		StepKey:   stepKey,
		StepType:  trainingStepTypeVideo,
		Status:    trainingStatusCompleted,
	})
	if err != nil {
		log.Error().Err(err).
			Str("child_id", child.ID).
			Str("step_key", stepKey).
			Str("module_id", moduleID).
			Str("stage_key", stageKey).
			Msg("failed to upsert training step progress")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	resp, err := server.buildTrainingHome(ctx.Request.Context(), child.ID, trainingRoleChild, child.FirstName)
	if err != nil {
		log.Error().Err(err).
			Str("child_id", child.ID).
			Str("step_key", stepKey).
			Msg("failed to rebuild training home after step completion")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	log.Info().
		Str("child_id", child.ID).
		Str("step_key", stepKey).
		Str("module_id", moduleID).
		Str("stage_key", stageKey).
		Msg("training video completion succeeded")

	ctx.JSON(http.StatusOK, gin.H{
		"step_key":        stepKey,
		"status":          trainingStatusCompleted,
		"idempotent":      idempotent,
		"this_weeks_step": resp.ThisWeeksStep,
	})
}

func (server *Server) buildTrainingHome(ctx context.Context, childID, role, childName string) (trainingHomeResponse, error) {
	modules, err := server.loadDigitalPermitModules(ctx, childID)
	if err != nil {
		return trainingHomeResponse{}, err
	}

	stepRows, err := server.store.GetTrainingStepProgressForChild(ctx, childID)
	if err != nil {
		return trainingHomeResponse{}, err
	}
	stepStatusByKey := make(map[string]string, len(stepRows))
	for _, row := range stepRows {
		stepStatusByKey[row.StepKey] = row.Status
	}

	socialModules, socialConfigured, err := server.loadSocialDriverModules(ctx, childID)
	if err != nil {
		return trainingHomeResponse{}, err
	}

	summary := server.buildTrainingSummary(ctx, childID, modules)
	summary.socialTrainingConfigured = socialConfigured
	summary.socialTrainingDone = server.isSocialTrainingCompleted(socialModules)
	introStage := server.buildIntroReadinessStage(modules, stepStatusByKey, role)
	thisWeeksStep := server.buildTrainingCurrentStep(modules, socialModules, summary, introStage, stepStatusByKey)

	resp := trainingHomeResponse{
		Role:      role,
		ChildID:   childID,
		ChildName: childName,
		LinkCodeGate: trainingLinkCodeGate{
			IsReady:           summary.firstPermitModuleCompleted,
			RequiredMilestone: trainingMilestoneFirstPermitModule,
		},
		ThisWeeksStep: thisWeeksStep,
		Stages: []trainingStageResponse{
			introStage,
			server.buildDigitalPermitCourseStage(modules),
			server.buildDigitalPermitTestStage(summary),
			server.buildSocialTrainingStage(summary, socialModules),
			server.buildSocialDriverTestStage(summary),
		},
	}

	if role == trainingRoleParent {
		resp.ParentActions = &trainingParentActions{
			CanGenerateLinkCode: summary.firstPermitModuleCompleted,
		}
		if !summary.firstPermitModuleCompleted {
			resp.ParentActions.GenerateLinkCodeReason = trainingLinkCodeGateBlockedReason
		}
	}

	return resp, nil
}

func (server *Server) loadDigitalPermitModules(ctx context.Context, childID string) ([]flattenedTrainingModule, error) {
	allCourses, err := server.fetchExternalCoursesByTrainingCourseText(ctx, trainingCourseTextDigitalPermit)
	if err != nil {
		return nil, err
	}

	stepRows, err := server.store.GetTrainingStepProgressForChild(ctx, childID)
	if err != nil {
		return nil, err
	}
	stepStatusByKey := make(map[string]string, len(stepRows))
	for _, row := range stepRows {
		stepStatusByKey[row.StepKey] = row.Status
	}

	modules := make([]flattenedTrainingModule, 0)
	moduleOrder := 1

	for _, courseItem := range allCourses {
		courseData, err := server.fetchExternalCourseData(nil, courseItem.DocumentID)
		if err != nil {
			return nil, err
		}

		moduleIDs := make([]string, 0, len(courseData.Modules))
		for _, module := range courseData.Modules {
			moduleIDs = append(moduleIDs, module.DocumentID)
		}

		progressRows, err := server.store.GetChildModuleProgressForCourse(ctx, db.GetChildModuleProgressForCourseParams{
			ChildID:   childID,
			CourseID:  courseItem.DocumentID,
			ModuleIds: moduleIDs,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}

		progressMap := make(map[string]bool, len(progressRows))
		for _, row := range progressRows {
			progressMap[row.ModuleID] = row.IsCompleted
		}

		for _, module := range courseData.Modules {
			quizID := ""
			if module.Quiz != nil {
				quizID = module.Quiz.DocumentID
			}
			videoDone := progressMap[module.DocumentID]
			if stepStatusByKey[module.DocumentID+"_video"] == trainingStatusCompleted {
				videoDone = true
			}
			if stepStatusByKey["intro_readiness_"+module.DocumentID+"_video"] == trainingStatusCompleted {
				videoDone = true
			}
			quizDone := progressMap[module.DocumentID]
			if quizID == "" {
				quizDone = true
			}
			if stepStatusByKey[module.DocumentID+"_quiz"] == trainingStatusCompleted {
				quizDone = true
			}
			if stepStatusByKey["intro_readiness_"+module.DocumentID+"_quiz"] == trainingStatusCompleted {
				quizDone = true
			}
			isCompleted := videoDone && quizDone

			modules = append(modules, flattenedTrainingModule{
				CourseID:    courseItem.DocumentID,
				ModuleID:    module.DocumentID,
				Title:       module.Title,
				Description: flattenDescription(module.Desc),
				Order:       moduleOrder,
				QuizID:      quizID,
				VideoDone:   videoDone,
				QuizDone:    quizDone,
				IsCompleted: isCompleted,
			})
			moduleOrder++
		}
	}

	return modules, nil
}

func (server *Server) buildTrainingSummary(ctx context.Context, childID string, modules []flattenedTrainingModule) trainingStateSummary {
	summary := trainingStateSummary{
		firstPermitModuleCompleted: len(modules) == 0,
		digitalPermitCourseDone:    len(modules) > 0,
	}

	if len(modules) > 0 {
		summary.firstPermitModuleCompleted = modules[0].IsCompleted
		for _, module := range modules {
			if !module.IsCompleted {
				summary.digitalPermitCourseDone = false
				break
			}
		}
	}

	if _, err := server.store.GetDigitalPermitTestByChildID(ctx, childID); err == nil {
		summary.digitalPermitTestRunning = true
	}

	completedTest, err := server.store.GetLatestCompletedDigitalPermitTestByChildID(ctx, childID)
	if err == nil && completedTest.CompletedAt.Valid {
		summary.digitalPermitTestDone = true
		summary.digitalPermitTestRunning = false
	}

	return summary
}

func (server *Server) getDigitalPermitProgressSummary(ctx context.Context, childID string) (trainingStateSummary, error) {
	modules, err := server.loadDigitalPermitModules(ctx, childID)
	if err != nil {
		return trainingStateSummary{}, err
	}
	return server.buildTrainingSummary(ctx, childID, modules), nil
}

func (server *Server) isFirstDigitalPermitModuleComplete(ctx context.Context, childID string) (bool, error) {
	summary, err := server.getDigitalPermitProgressSummary(ctx, childID)
	if err != nil {
		return false, err
	}
	return summary.firstPermitModuleCompleted, nil
}

func (server *Server) isDigitalPermitTestUnlocked(ctx context.Context, childID string) (bool, error) {
	summary, err := server.getDigitalPermitProgressSummary(ctx, childID)
	if err != nil {
		return false, err
	}
	return summary.digitalPermitCourseDone, nil
}

func (server *Server) isDigitalPermitCourseAccessible(ctx context.Context, childID, courseID string) (bool, error) {
	modules, err := server.loadDigitalPermitModules(ctx, childID)
	if err != nil {
		return false, err
	}

	accessibleCourses := make(map[string]struct{})
	for _, module := range modules {
		accessibleCourses[module.CourseID] = struct{}{}
		if !module.IsCompleted {
			break
		}
	}

	_, ok := accessibleCourses[courseID]
	return ok, nil
}

func (server *Server) isDigitalPermitModuleAccessible(ctx context.Context, childID, moduleID string) (bool, error) {
	modules, err := server.loadDigitalPermitModules(ctx, childID)
	if err != nil {
		return false, err
	}

	// Check if this moduleID is part of the Digital Permit training curriculum
	isTrainingModule := false
	for _, module := range modules {
		if module.ModuleID == moduleID {
			isTrainingModule = true
			break
		}
	}

	// If this is not a training module, allow access (it's a regular course)
	if !isTrainingModule {
		return true, nil
	}

	// For training modules, enforce sequencing: module must be in order and prior modules complete
	for _, module := range modules {
		if module.ModuleID == moduleID {
			return true, nil
		}
		if !module.IsCompleted {
			break
		}
	}

	return false, nil
}

func (server *Server) loadSocialDriverModules(ctx context.Context, childID string) ([]trainingModuleResponse, bool, error) {
	if err := server.ensureUnlockSystemSeeded(ctx, childID); err != nil {
		return nil, false, err
	}

	courseUnlocks, err := server.store.GetChildCourseUnlocks(ctx, childID)
	if err != nil {
		return nil, false, err
	}
	apps, err := server.store.GetSocialAppAccessForChild(ctx, childID)
	if err != nil {
		return nil, false, err
	}

	sort.Slice(apps, func(i, j int) bool {
		return apps[i].SocialMediaID < apps[j].SocialMediaID
	})

	titles := make(map[string]string)
	allowedCourseIDs := make(map[string]struct{})
	courses, err := server.fetchAllCoursesForUnlock(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, course := range courses {
		allowedCourseIDs[course.DocumentID] = struct{}{}
		titles[course.DocumentID] = course.Title
	}

	socialCourses := make([]db.ChildCourseUnlock, 0)
	for _, item := range courseUnlocks {
		if item.CourseOrder >= 0 {
			if _, ok := allowedCourseIDs[item.CourseID]; !ok {
				continue
			}
			socialCourses = append(socialCourses, item)
		}
	}

	sort.SliceStable(socialCourses, func(i, j int) bool {
		if socialCourses[i].CourseOrder != socialCourses[j].CourseOrder {
			return socialCourses[i].CourseOrder < socialCourses[j].CourseOrder
		}
		return socialCourses[i].CourseID < socialCourses[j].CourseID
	})

	if len(socialCourses) == 0 {
		return []trainingModuleResponse{}, false, nil
	}

	modules := make([]trainingModuleResponse, 0, len(socialCourses))
	for i, item := range socialCourses {
		moduleStatus := trainingStatusLocked
		isLocked := true
		videoStatus := trainingStatusLocked
		quizStatus := trainingStatusLocked

		switch item.Status {
		case db.CourseStatusCompleted:
			moduleStatus = trainingStatusCompleted
			isLocked = false
			videoStatus = trainingStatusCompleted
			quizStatus = trainingStatusCompleted
		case db.CourseStatusUnlocked:
			moduleStatus = trainingStatusAvailable
			isLocked = false
			videoStatus = trainingStatusAvailable
			quizStatus = trainingStatusLocked
		}

		title := titles[item.CourseID]
		if title == "" {
			title = "Social Driver Module"
		}

		var reward *trainingSocialReward
		if i < len(apps) {
			rewardStatus := "locked"
			if apps[i].State != db.SocialAppStateLocked {
				rewardStatus = "earned"
			}
			reward = &trainingSocialReward{
				SocialMediaID: apps[i].SocialMediaID,
				SocialMedia:   apps[i].Name,
				RewardStatus:  rewardStatus,
			}
		}

		modules = append(modules, trainingModuleResponse{
			ModuleKey:      "social_" + item.CourseID,
			Title:          title,
			Order:          int(item.CourseOrder),
			Status:         moduleStatus,
			IsLocked:       isLocked,
			SourceCourseID: item.CourseID,
			SocialReward:   reward,
			Steps: []trainingStepResponse{
				{
					StepKey:    "social_" + item.CourseID + "_video",
					StepType:   trainingStepTypeVideo,
					Title:      "Watch Video",
					Status:     videoStatus,
					IsLocked:   videoStatus == trainingStatusLocked,
					LockReason: conditionalLockReason(videoStatus == trainingStatusLocked, "Complete previous social module first"),
				},
				{
					StepKey:    "social_" + item.CourseID + "_quiz",
					StepType:   trainingStepTypeQuiz,
					Title:      "Complete Quiz",
					Status:     quizStatus,
					IsLocked:   quizStatus == trainingStatusLocked,
					LockReason: conditionalLockReason(quizStatus == trainingStatusLocked, "Complete social module video first"),
				},
			},
		})
	}

	return modules, true, nil
}

func (server *Server) isSocialTrainingCompleted(modules []trainingModuleResponse) bool {
	if len(modules) == 0 {
		return false
	}
	for _, module := range modules {
		if module.Status != trainingStatusCompleted {
			return false
		}
	}
	return true
}

func (server *Server) isDigitalPermitQuizAccessible(ctx context.Context, childID, moduleID string) (bool, error) {
	module, err := server.findDigitalPermitModule(ctx, childID, moduleID)
	if err != nil {
		return false, err
	}
	// If module not found, it's not a training module, so allow access (it's a regular course)
	if module == nil {
		return true, nil
	}
	// For training modules, require video completion before quiz
	if !module.VideoDone {
		return false, nil
	}
	return server.isDigitalPermitModuleAccessible(ctx, childID, moduleID)
}

func (server *Server) findDigitalPermitModule(ctx context.Context, childID, moduleID string) (*flattenedTrainingModule, error) {
	modules, err := server.loadDigitalPermitModules(ctx, childID)
	if err != nil {
		return nil, err
	}
	for _, module := range modules {
		if module.ModuleID == moduleID {
			moduleCopy := module
			return &moduleCopy, nil
		}
	}
	return nil, nil
}

func (server *Server) buildDigitalPermitCourseStage(modules []flattenedTrainingModule) trainingStageResponse {
	stage := trainingStageResponse{
		StageKey:     trainingStageDigitalPermitCourse,
		Title:        "Digital Permit Course",
		StageType:    trainingStageTypeCourse,
		IsConfigured: true,
		Modules:      make([]trainingModuleResponse, 0, len(modules)),
	}

	firstIncompleteFound := false
	completedCount := 0

	for _, module := range modules {
		moduleStatus := trainingStatusLocked
		moduleLocked := true
		videoStatus := trainingStatusLocked
		quizStatus := trainingStatusLocked
		videoLocked := true
		quizLocked := true

		if module.IsCompleted {
			moduleStatus = trainingStatusCompleted
			moduleLocked = false
			videoStatus = trainingStatusCompleted
			quizStatus = trainingStatusCompleted
			videoLocked = false
			quizLocked = false
			completedCount++
		} else if !firstIncompleteFound {
			moduleLocked = false
			if !module.VideoDone {
				moduleStatus = trainingStatusAvailable
				videoStatus = trainingStatusAvailable
				videoLocked = false
				quizStatus = trainingStatusLocked
				quizLocked = true
			} else {
				moduleStatus = trainingStatusInProgress
				videoStatus = trainingStatusCompleted
				videoLocked = false
				quizStatus = trainingStatusAvailable
				quizLocked = false
			}
			firstIncompleteFound = true
		}

		steps := []trainingStepResponse{
			{
				StepKey:    module.ModuleID + "_video",
				StepType:   trainingStepTypeVideo,
				Title:      "Watch Video",
				Status:     videoStatus,
				IsLocked:   videoLocked,
				LockReason: conditionalLockReason(videoLocked, "Complete the previous module first"),
			},
		}

		if module.QuizID != "" {
			steps = append(steps, trainingStepResponse{
				StepKey:    module.ModuleID + "_quiz",
				StepType:   trainingStepTypeQuiz,
				Title:      "Complete Quiz",
				Status:     quizStatus,
				IsLocked:   quizLocked,
				LockReason: conditionalLockReason(quizLocked, "Complete the module video first"),
				QuizID:     module.QuizID,
			})
		}

		stage.Modules = append(stage.Modules, trainingModuleResponse{
			ModuleKey:      module.ModuleID,
			Title:          module.Title,
			Description:    module.Description,
			Order:          module.Order,
			Status:         moduleStatus,
			IsLocked:       moduleLocked,
			SourceCourseID: module.CourseID,
			Steps:          steps,
		})
	}

	switch {
	case len(modules) == 0:
		stage.Status = trainingStatusLocked
		stage.IsLocked = true
		stage.LockReason = "No Digital Permit modules configured"
	case completedCount == len(modules):
		stage.Status = trainingStatusCompleted
		stage.IsCompleted = true
	case completedCount > 0:
		stage.Status = trainingStatusInProgress
	case len(modules) > 0:
		stage.Status = trainingStatusAvailable
	}

	return stage
}

func (server *Server) buildDigitalPermitTestStage(summary trainingStateSummary) trainingStageResponse {
	stage := trainingStageResponse{
		StageKey:     trainingStageDigitalPermitTest,
		Title:        "Digital Permit Test",
		StageType:    trainingStageTypeFinalTest,
		IsConfigured: true,
	}

	switch {
	case summary.digitalPermitTestDone:
		stage.Status = trainingStatusCompleted
		stage.IsCompleted = true
	case summary.digitalPermitTestRunning:
		stage.Status = trainingStatusInProgress
	case summary.digitalPermitCourseDone:
		stage.Status = trainingStatusAvailable
	default:
		stage.Status = trainingStatusLocked
		stage.IsLocked = true
		stage.LockReason = trainingPermitTestLockedReason
	}

	return stage
}

func (server *Server) buildSocialTrainingStage(summary trainingStateSummary, modules []trainingModuleResponse) trainingStageResponse {
	stage := trainingStageResponse{
		StageKey:     trainingStageSocialMediaDriverCourse,
		Title:        "Social Media Driver Training",
		StageType:    trainingStageTypeCourse,
		IsConfigured: summary.socialTrainingConfigured,
		Status:       trainingStatusLocked,
		IsLocked:     true,
		Modules:      modules,
	}

	if !summary.digitalPermitTestDone {
		stage.LockReason = trainingSocialTrainingLockedReason
		return stage
	}

	if !summary.socialTrainingConfigured {
		stage.LockReason = trainingSocialConfigurationReason
		return stage
	}

	if summary.socialTrainingDone {
		stage.Status = trainingStatusCompleted
		stage.IsCompleted = true
		stage.IsLocked = false
		stage.LockReason = ""
		return stage
	}

	hasStarted := false
	hasAvailable := false
	for _, module := range modules {
		if module.Status == trainingStatusCompleted || module.Status == trainingStatusInProgress {
			hasStarted = true
		}
		if module.Status == trainingStatusAvailable {
			hasAvailable = true
		}
	}

	if hasStarted {
		stage.Status = trainingStatusInProgress
	} else if hasAvailable {
		stage.Status = trainingStatusAvailable
	} else {
		stage.Status = trainingStatusLocked
		stage.LockReason = "No social module is unlocked yet"
		return stage
	}

	stage.IsLocked = false
	stage.LockReason = ""
	return stage
}

func (server *Server) buildSocialDriverTestStage(summary trainingStateSummary) trainingStageResponse {
	stage := trainingStageResponse{
		StageKey:     trainingStageSocialMediaDriverTest,
		Title:        "Social Media Driver Test",
		StageType:    trainingStageTypeFinalTest,
		IsConfigured: summary.socialTrainingConfigured,
		Status:       trainingStatusLocked,
		IsLocked:     true,
	}

	if !summary.digitalPermitTestDone {
		stage.LockReason = trainingSocialTrainingLockedReason
	} else if !summary.socialTrainingConfigured {
		stage.LockReason = trainingSocialDriverTestLockedReason
	} else if summary.socialTrainingDone {
		stage.Status = trainingStatusAvailable
		stage.IsLocked = false
		stage.LockReason = ""
	} else {
		stage.LockReason = trainingSocialDriverTestLockedReason
	}

	return stage
}

func (server *Server) buildTrainingCurrentStep(modules []flattenedTrainingModule, socialModules []trainingModuleResponse, summary trainingStateSummary, introStage trainingStageResponse, stepStatusByKey map[string]string) *trainingCurrentStep {
	if introStage.IsConfigured && !introStage.IsCompleted {
		for _, module := range introStage.Modules {
			for _, step := range module.Steps {
				if step.Status == trainingStatusAvailable || step.Status == trainingStatusInProgress {
					return &trainingCurrentStep{
						StageKey:   introStage.StageKey,
						ModuleKey:  module.ModuleKey,
						StepKey:    step.StepKey,
						StepType:   step.StepType,
						Title:      step.Title,
						Status:     step.Status,
						IsLocked:   step.IsLocked,
						LockReason: step.LockReason,
					}
				}
			}
		}
		if stepStatusByKey["intro_readiness_chat_test"] != trainingStatusCompleted {
			return &trainingCurrentStep{
				StageKey:   trainingStageIntroReadiness,
				StepKey:    "intro_readiness_chat_test",
				StepType:   trainingStepTypeFinalTest,
				Title:      "Take Intro and Readiness Chat Test",
				Status:     trainingStatusLocked,
				IsLocked:   true,
				LockReason: "Complete all 10 Intro and Readiness modules first",
			}
		}
	}

	for _, module := range modules {
		if !module.IsCompleted {
			if module.VideoDone {
				return &trainingCurrentStep{
					StageKey:  trainingStageDigitalPermitCourse,
					ModuleKey: module.ModuleID,
					StepKey:   module.ModuleID + "_quiz",
					StepType:  trainingStepTypeQuiz,
					Title:     "Complete Quiz",
					Status:    trainingStatusAvailable,
					IsLocked:  false,
				}
			}
			return &trainingCurrentStep{
				StageKey:  trainingStageDigitalPermitCourse,
				ModuleKey: module.ModuleID,
				StepKey:   module.ModuleID + "_video",
				StepType:  trainingStepTypeVideo,
				Title:     "Watch Video",
				Status:    trainingStatusAvailable,
				IsLocked:  false,
			}
		}
	}

	if !summary.digitalPermitTestDone {
		status := trainingStatusLocked
		locked := true
		lockReason := trainingPermitTestLockedReason
		if summary.digitalPermitTestRunning {
			status = trainingStatusInProgress
			locked = false
			lockReason = ""
		} else if summary.digitalPermitCourseDone {
			status = trainingStatusAvailable
			locked = false
			lockReason = ""
		}

		return &trainingCurrentStep{
			StageKey:   trainingStageDigitalPermitTest,
			StepKey:    trainingStageDigitalPermitTest,
			StepType:   trainingStepTypeFinalTest,
			Title:      "Take Digital Permit Test",
			Status:     status,
			IsLocked:   locked,
			LockReason: lockReason,
		}
	}

	for _, module := range socialModules {
		if module.Status == trainingStatusAvailable || module.Status == trainingStatusInProgress {
			step := module.ModuleKey + "_video"
			if len(module.Steps) > 0 {
				step = module.Steps[0].StepKey
				for _, s := range module.Steps {
					if s.Status == trainingStatusAvailable || s.Status == trainingStatusInProgress {
						step = s.StepKey
						break
					}
				}
			}
			return &trainingCurrentStep{
				StageKey:  trainingStageSocialMediaDriverCourse,
				ModuleKey: module.ModuleKey,
				StepKey:   step,
				StepType:  trainingStepTypeVideo,
				Title:     "Continue Social Media Driver Training",
				Status:    trainingStatusAvailable,
				IsLocked:  false,
			}
		}
	}

	if summary.socialTrainingConfigured && summary.socialTrainingDone {
		return &trainingCurrentStep{
			StageKey: trainingStageSocialMediaDriverTest,
			StepKey:  trainingStageSocialMediaDriverTest,
			StepType: trainingStepTypeFinalTest,
			Title:    "Take Social Media Driver Test",
			Status:   trainingStatusAvailable,
			IsLocked: false,
		}
	}

	return nil
}

func (server *Server) buildIntroReadinessStage(modules []flattenedTrainingModule, stepStatusByKey map[string]string, role string) trainingStageResponse {
	parentModuleCount := 2
	if len(modules) < parentModuleCount {
		parentModuleCount = len(modules)
	}
	childStart := parentModuleCount
	childEnd := childStart + 10
	if childEnd > len(modules) {
		childEnd = len(modules)
	}

	stage := trainingStageResponse{
		StageKey:     trainingStageIntroReadiness,
		Title:        "Intro and Readiness",
		StageType:    trainingStageTypeCourse,
		IsConfigured: true,
		Status:       trainingStatusLocked,
		IsLocked:     true,
		Modules:      []trainingModuleResponse{},
	}

	parentModules := modules[:parentModuleCount]
	childModules := modules[childStart:childEnd]

	parentCompleteCount := 0
	for _, module := range parentModules {
		if module.VideoDone || stepStatusByKey["intro_readiness_"+module.ModuleID+"_video"] == trainingStatusCompleted {
			parentCompleteCount++
		}
	}
	childUnlocked := parentCompleteCount == 2

	for idx, module := range parentModules {
		videoStepKey := "intro_readiness_" + module.ModuleID + "_video"
		videoDone := module.VideoDone || stepStatusByKey[videoStepKey] == trainingStatusCompleted
		videoStatus := trainingStatusAvailable
		if videoDone {
			videoStatus = trainingStatusCompleted
		}
		if !videoDone && idx > 0 {
			firstDone := parentModules[0].VideoDone || stepStatusByKey["intro_readiness_"+parentModules[0].ModuleID+"_video"] == trainingStatusCompleted
			if !firstDone {
				videoStatus = trainingStatusLocked
			}
		}

		stage.Modules = append(stage.Modules, trainingModuleResponse{
			ModuleKey:      "intro_readiness_" + module.ModuleID,
			Title:          module.Title,
			Description:    module.Description,
			Order:          idx + 1,
			Status:         map[bool]string{true: trainingStatusCompleted, false: trainingStatusInProgress}[videoDone],
			IsLocked:       videoStatus == trainingStatusLocked,
			SourceCourseID: module.CourseID,
			Steps: []trainingStepResponse{
				{
					StepKey:    videoStepKey,
					StepType:   trainingStepTypeVideo,
					Title:      "Watch Video",
					Status:     videoStatus,
					IsLocked:   videoStatus == trainingStatusLocked,
					LockReason: conditionalLockReason(videoStatus == trainingStatusLocked, "Complete the previous intro video first"),
				},
			},
		})
	}

	childCompletedCount := 0
	firstChildIncompleteFound := false
	for idx, module := range childModules {
		videoStepKey := "intro_readiness_" + module.ModuleID + "_video"
		quizStepKey := "intro_readiness_" + module.ModuleID + "_quiz"

		videoDone := module.VideoDone || stepStatusByKey[videoStepKey] == trainingStatusCompleted
		quizDone := module.QuizDone || stepStatusByKey[quizStepKey] == trainingStatusCompleted
		moduleDone := videoDone && quizDone
		if moduleDone {
			childCompletedCount++
		}

		moduleStatus := trainingStatusLocked
		moduleLocked := true
		videoStatus := trainingStatusLocked
		videoLocked := true
		quizStatus := trainingStatusLocked
		quizLocked := true

		if moduleDone {
			moduleStatus = trainingStatusCompleted
			moduleLocked = false
			videoStatus = trainingStatusCompleted
			videoLocked = false
			quizStatus = trainingStatusCompleted
			quizLocked = false
		} else if childUnlocked && !firstChildIncompleteFound {
			moduleLocked = false
			if videoDone {
				moduleStatus = trainingStatusInProgress
				videoStatus = trainingStatusCompleted
				videoLocked = false
				quizStatus = trainingStatusAvailable
				quizLocked = false
			} else {
				moduleStatus = trainingStatusAvailable
				videoStatus = trainingStatusAvailable
				videoLocked = false
				quizStatus = trainingStatusLocked
				quizLocked = true
			}
			firstChildIncompleteFound = true
		}

		if !childUnlocked && role == trainingRoleChild {
			moduleStatus = trainingStatusLocked
			moduleLocked = true
			videoStatus = trainingStatusLocked
			videoLocked = true
			quizStatus = trainingStatusLocked
			quizLocked = true
		}

		moduleSteps := []trainingStepResponse{
			{
				StepKey:    videoStepKey,
				StepType:   trainingStepTypeVideo,
				Title:      "Watch Video",
				Status:     videoStatus,
				IsLocked:   videoLocked,
				LockReason: conditionalLockReason(videoLocked, trainingIntroReadinessLockedReason),
			},
		}
		if module.QuizID != "" {
			moduleSteps = append(moduleSteps, trainingStepResponse{
				StepKey:    quizStepKey,
				StepType:   trainingStepTypeQuiz,
				Title:      "Complete Quiz",
				Status:     quizStatus,
				IsLocked:   quizLocked,
				LockReason: conditionalLockReason(quizLocked, "Complete the intro module video first"),
				QuizID:     module.QuizID,
			})
		}

		stage.Modules = append(stage.Modules, trainingModuleResponse{
			ModuleKey:      "intro_readiness_" + module.ModuleID,
			Title:          module.Title,
			Description:    module.Description,
			Order:          idx + 3,
			Status:         moduleStatus,
			IsLocked:       moduleLocked,
			SourceCourseID: module.CourseID,
			Steps:          moduleSteps,
		})
	}

	chatUnlocked := len(childModules) > 0 && childCompletedCount == len(childModules)
	chatDone := stepStatusByKey["intro_readiness_chat_test"] == trainingStatusCompleted

	// Keep the chat test as a stage-level milestone (via ThisWeeksStep/state),
	// not a content module, so module totals match actual Strapi modules/videos.

	stage.Progress = &trainingStageProgress{
		ParentVideosWatched:          parentCompleteCount,
		ParentVideosTotal:            len(parentModules),
		ChildDeviceUnlocked:          childUnlocked,
		ChildVideoQuizPairsCompleted: childCompletedCount,
		ChildVideoQuizPairsTotal:     len(childModules),
		ChatTestUnlocked:             chatUnlocked,
		ChatTestCompleted:            chatDone,
	}

	if chatDone {
		stage.Status = trainingStatusCompleted
		stage.IsCompleted = true
		stage.IsLocked = false
		stage.LockReason = ""
		return stage
	}

	if role == trainingRoleChild && !childUnlocked {
		stage.Status = trainingStatusLocked
		stage.IsLocked = true
		stage.LockReason = trainingIntroReadinessLockedReason
		return stage
	}

	stage.IsLocked = false
	stage.LockReason = ""
	if childCompletedCount > 0 || parentCompleteCount > 0 {
		stage.Status = trainingStatusInProgress
	} else {
		stage.Status = trainingStatusAvailable
	}

	return stage
}

func conditionalLockReason(isLocked bool, reason string) string {
	if !isLocked {
		return ""
	}
	return reason
}
