package api

import (
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

const (
	ParentAccountAlreadyExistsError = "a parent account with this user ID already exists"
)

// Avatar lists for default profile images
var (
	boyAvatars = []string{
		"/uploads/avartar_boy_1_69f06e637b.png",
		"/uploads/avartar_boy_2_6f049a8501.png",
		"/uploads/avartar_boy_3_f727bc3a67.png",
		"/uploads/avartar_boy_4_e8736f03c7.png",
		"/uploads/avartar_boy_5_89d773bd69.png",
		"/uploads/avartar_boy_6_84b6f30f1e.png",
		"/uploads/avartar_boy_7_586f672d35.png",
		"/uploads/avartar_boy_8_274baf51f7.png",
	}
	girlAvatars = []string{
		"/uploads/avartar_girl_1_f7dbe8cde3.png",
		"/uploads/avartar_girl_2_7c4ff5b9f0.png",
		"/uploads/avartar_girl_3_b680f39f15.png",
		"/uploads/avartar_girl_4_eac1f8e55a.png",
		"/uploads/avartar_girl_5_10d7c81ca2.png",
		"/uploads/avartar_girl_6_096ec03624.png",
		"/uploads/avartar_girl_7_cf09bef107.png",
		"/uploads/avartar_girl_8_ddef708a0a.png",
	}
)

type createChildRequest struct {
	FirstName string `form:"firstname" binding:"required"`
	Surname   string `form:"surname"`
	Age       int32  `form:"age" binding:"required,gt=0"`
	Gender    string `form:"gender" binding:"required"`
}

// getDefaultAvatarURL returns a random default avatar URL based on gender
func (server *Server) getDefaultAvatarURL(gender string) string {
	var avatars []string
	if gender == "female" {
		avatars = girlAvatars
	} else {
		avatars = boyAvatars
	}

	// Pick a random avatar from the list
	randomIndex := rand.Intn(len(avatars))
	avatarPath := avatars[randomIndex]

	// Return the full URL with base URL
	return fmt.Sprintf("%s%s", server.config.ExternalContentBaseURL, avatarPath)
}

func (server *Server) createChild(ctx *gin.Context) {
	authPayload, exists := ctx.Get(authorizationPayloadKey)
	fmt.Printf("authPayload: %+v\n", authPayload)
	if !exists {
		err := errors.New("authorization payload not found")
		ctx.JSON(http.StatusUnauthorized, errorResponse(err))
		return
	}

	payload, ok := authPayload.(*token.Payload)
	if !ok {
		err := errors.New("invalid payload type in context")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	parentUserID := payload.UserID
	log.Info().Str("parent_user_id", parentUserID).Msg("creating child")

	var req createChildRequest
	if err := ctx.ShouldBind(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	// Handle the profile image upload
	file, header, err := ctx.Request.FormFile("profile_image")
	// We allow the file to be missing, but if it's provided, it must be valid.
	if err != nil && err != http.ErrMissingFile {
		ctx.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid file upload: %w", err)))
		return
	}

	var profileImageURL sql.NullString
	if file != nil {
		defer file.Close()

		// Upload to the external content provider
		uploadURL, err := server.uploader.UploadFile(header.Filename, file, "app/child_profile_images")
		if err != nil {
			log.Error().Err(err).Msg("failed to upload profile image")
			ctx.JSON(http.StatusInternalServerError, errorResponse(errors.New("failed to process profile image")))
			return
		}
		profileImageURL = sql.NullString{String: uploadURL, Valid: true}
	} else {
		// Use default avatar based on gender
		defaultAvatarURL := server.getDefaultAvatarURL(req.Gender)
		profileImageURL = sql.NullString{String: defaultAvatarURL, Valid: true}
	}

	parent, err := server.store.GetParentByParentID(ctx, parentUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Warn().Err(err).Str("parent_user_id", parentUserID).Msg("parent not found")
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("parent user not found")))
			return
		}
		log.Error().Err(err).Str("parent_user_id", parentUserID).Msg("failed to get parent")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	if req.Surname == "" {
		req.Surname = parent.Surname
	}

	txResult, err := server.store.CreateChildTx(ctx, db.CreateChildTxParams{
		FirstName:       req.FirstName,
		Surname:         req.Surname,
		FamilyID:        parent.FamilyID,
		Age:             sql.NullInt32{Int32: req.Age, Valid: req.Age > 0},
		Gender:          sql.NullString{String: req.Gender, Valid: req.Gender != ""},
		ProfileImageUrl: profileImageURL,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to create child")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	child := txResult.Child
	log.Info().Str("child_id", child.ID).Str("parent_user_id", parentUserID).Msg("child created successfully")

	// Send notification to parent about the new child
	go func() {
		title := "New Child Profile Created"
		message := fmt.Sprintf("You've successfully created a profile for %s. Time to get started!", child.FirstName)
		server.sendParentNotification(child, parent, title, message)
	}()

	ctx.JSON(http.StatusOK, child)
}

type updateChildRequest struct {
	FirstName                *string `json:"first_name"`
	Surname                  *string `json:"surname"`
	Age                      *int32  `json:"age"`
	Gender                   *string `json:"gender"`
	PushNotificationsEnabled *bool   `json:"push_notifications_enabled"`
}

func (server *Server) updateChild(ctx *gin.Context) {
	var uriReq struct {
		ID string `uri:"id" binding:"required"`
	}

	if err := ctx.ShouldBindUri(&uriReq); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	var bodyReq updateChildRequest
	if err := ctx.ShouldBindJSON(&bodyReq); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	parentPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)

	child, err := server.store.GetChildByIDAndFamilyID(ctx, db.GetChildByIDAndFamilyIDParams{
		ID:       uriReq.ID,
		FamilyID: parentPayload.FamilyID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("child not found or does not belong to this family")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Prepare parameters for the update query
	params := db.UpdateChildParams{
		ID:        child.ID,
		FirstName: pgtype.Text{String: child.FirstName, Valid: true},
		Surname:   pgtype.Text{String: child.Surname, Valid: true},
		Age:       child.Age,
		Gender:    child.Gender,
	}

	if bodyReq.FirstName != nil {
		params.FirstName = pgtype.Text{String: *bodyReq.FirstName, Valid: true}
	}
	if bodyReq.Surname != nil {
		params.Surname = pgtype.Text{String: *bodyReq.Surname, Valid: true}
	}
	if bodyReq.Age != nil {
		params.Age = pgtype.Int4{Int32: *bodyReq.Age, Valid: true}
	}
	if bodyReq.Gender != nil {
		params.Gender = pgtype.Text{String: *bodyReq.Gender, Valid: true}
	}
	if bodyReq.PushNotificationsEnabled != nil {
		params.PushNotificationsEnabled = pgtype.Bool{Bool: *bodyReq.PushNotificationsEnabled, Valid: true}
	}

	updatedChild, err := server.store.UpdateChild(ctx, params)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, updatedChild)
}

type childOnboardingProgressResponse struct {
	StepName    string     `json:"step_name"`
	Description string     `json:"description"`
	StepOrder   int32      `json:"step_order"`
	StepType    string     `json:"step_type"`
	IsCompleted bool       `json:"is_completed"`
	CompletedAt *time.Time `json:"completed_at"`
	CTA         string     `json:"cta,omitempty"`
}

type parentChildResponse struct {
	db.Child
	OnboardingProgress []childOnboardingProgressResponse `json:"onboarding_progress"`
}

// checkAndUpdateDigitalPermitCourseCompletion checks if all courses are completed and updates the onboarding step
func (server *Server) checkAndUpdateDigitalPermitCourseCompletion(ctx *gin.Context, childID string) bool {
	// Get all courses for the child
	allCourses, err := server.fetchAllExternalCourses(ctx)
	if err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("failed to fetch all courses for completion check")
		return false
	}

	// Check if all courses are completed
	allCompleted := true
	for _, courseItem := range allCourses {
		courseData, err := server.fetchExternalCourseData(ctx, courseItem.DocumentID)
		if err != nil {
			log.Warn().Err(err).Str("course_id", courseItem.DocumentID).Msg("could not fetch course details for completion check")
			continue
		}

		if len(courseData.Modules) == 0 {
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
			log.Error().Err(err).Str("child_id", childID).Str("course_id", courseItem.DocumentID).Msg("failed to get module progress")
			allCompleted = false
			break
		}

		completedCount := 0
		for _, p := range progress {
			if p.IsCompleted {
				completedCount++
			}
		}

		isCourseCompleted := completedCount == len(courseData.Modules)
		if !isCourseCompleted {
			allCompleted = false
			break
		}
	}

	// If all courses are completed and the onboarding step is not yet marked complete, update it
	if allCompleted {
		_, err := server.store.UpdateChildOnboardingStep(ctx, db.UpdateChildOnboardingStepParams{
			ChildID:      childID,
			OnboardingID: "digital_permit_course",
		})
		if err != nil {
			log.Error().Err(err).Str("child_id", childID).Msg("failed to update digital_permit_course onboarding step")
			return false
		}
		log.Info().Str("child_id", childID).Msg("successfully updated digital_permit_course onboarding step to completed")
		return true
	}

	return false
}

func (server *Server) getParentChild(ctx *gin.Context) {
	authPayload, exists := ctx.Get(authorizationPayloadKey)
	if !exists {
		err := errors.New("authorization payload not found")
		ctx.JSON(http.StatusUnauthorized, errorResponse(err))
		return
	}

	payload, ok := authPayload.(*token.Payload)
	if !ok {
		err := errors.New("invalid payload type in context")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	parent, err := server.store.GetParentByParentID(ctx, payload.UserID)
	if err != nil {
		if err == pgx.ErrNoRows {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("parent not found")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	var req struct {
		ID string `uri:"id" binding:"required"`
	}

	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	child, err := server.store.GetChildByIDAndFamilyID(ctx, db.GetChildByIDAndFamilyIDParams{
		ID:       req.ID,
		FamilyID: parent.FamilyID,
	})

	if err != nil {
		if err == pgx.ErrNoRows {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("child not found or does not belong to this family")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	onboardingProgress, err := server.store.GetChildOnboardingProgress(ctx, child.ID)
	if err != nil {
		log.Error().Err(err).Str("child_id", child.ID).Msg("failed to get child onboarding progress")
		// Non-critical error, so we can still return the child details
		onboardingProgress = []db.GetChildOnboardingProgressRow{}
	}

	resp := parentChildResponse{
		Child:              child,
		OnboardingProgress: []childOnboardingProgressResponse{},
	}

	for _, progress := range onboardingProgress {
		var completedAt *time.Time
		if progress.CompletedAt.Valid {
			completedAt = &progress.CompletedAt.Time
		}

		// Check if this is the digital_permit_course step and it's not yet completed
		isCompleted := progress.IsCompleted
		if progress.StepName == "digital_permit_course" && !isCompleted {
			// Check if all courses are actually completed
			if server.checkAndUpdateDigitalPermitCourseCompletion(ctx, child.ID) {
				isCompleted = true
				now := time.Now()
				completedAt = &now
			}
		}

		var cta string
		stepType := string(progress.StepType)
		if stepType == "course" {
			cta = "/v1/parent/courses"
		} else if stepType == "quiz" && progress.StepName == "digital_permit_test" {
			cta = fmt.Sprintf("/v1/parent/child/%s/digital-permit-test/ws", child.ID)
			log.Info().Str("child_id", child.ID).Str("step_name", progress.StepName).Str("cta", cta).Msg("generated digital permit test websocket cta")
		}

		resp.OnboardingProgress = append(resp.OnboardingProgress, childOnboardingProgressResponse{
			StepName:    progress.StepName,
			Description: progress.Description.String,
			StepOrder:   progress.StepOrder,
			StepType:    string(progress.StepType),
			IsCompleted: isCompleted,
			CompletedAt: completedAt,
			CTA:         cta,
		})
	}

	ctx.JSON(http.StatusOK, resp)
}

type generateLinkCodeResponse struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (server *Server) generateLinkCode(ctx *gin.Context) {
	var uriReq struct {
		ID string `uri:"id" binding:"required"`
	}

	if err := ctx.ShouldBindUri(&uriReq); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	parentPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)

	child, err := server.store.GetChildByIDAndFamilyID(ctx, db.GetChildByIDAndFamilyIDParams{
		ID:       uriReq.ID,
		FamilyID: parentPayload.FamilyID,
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("child not found or does not belong to this family")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	var deepLinkCode db.DeepLinkCode

	// Check if a code already exists for this child
	existingCode, err := server.store.GetDeepLinkCodeByChildID(ctx, child.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Error().Err(err).Msg("failed to check for existing deep link code")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Loop to handle potential collisions on the generated code
	for i := 0; i < 5; i++ {
		code, err := util.GenerateLinkCode()
		if err != nil {
			log.Error().Err(err).Msg("failed to generate link code")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}

		// If code exists, update it. Otherwise, create a new one.
		if existingCode.ID != 0 {
			arg := db.UpdateDeepLinkCodeParams{
				ID:        existingCode.ID,
				Code:      code,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}
			deepLinkCode, err = server.store.UpdateDeepLinkCode(ctx, arg)
		} else {
			arg := db.CreateDeepLinkCodeParams{
				FamilyID:  child.FamilyID,
				ChildID:   child.ID,
				Code:      code,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}
			deepLinkCode, err = server.store.CreateDeepLinkCode(ctx, arg)
		}

		if err == nil {
			break // Success
		}

		if db.ErrorCode(err) == db.UniqueViolation {
			log.Warn().Msg("generated a duplicate link code, retrying...")
			continue
		}

		// If we are here, a non-unique-violation error occurred
		log.Error().Err(err).Msg("failed to create or update deep link code")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Check if the loop completed without success
	if deepLinkCode.ID == 0 {
		log.Error().Msg("failed to generate a unique link code after several attempts")
		ctx.JSON(http.StatusInternalServerError, errorResponse(errors.New("failed to generate a unique link code")))
		return
	}

	rsp := generateLinkCodeResponse{
		Code:      deepLinkCode.Code,
		ExpiresAt: deepLinkCode.ExpiresAt,
	}

	log.Info().Str("family_id", deepLinkCode.FamilyID).Str("child_id", child.ID).Str("code", deepLinkCode.Code).Msg("link code generated successfully")

	ctx.JSON(http.StatusOK, rsp)
}

func (server *Server) getAllChildren(ctx *gin.Context) {
	authPayload, exists := ctx.Get(authorizationPayloadKey)
	if !exists {
		err := errors.New("authorization payload not found")
		ctx.JSON(http.StatusUnauthorized, errorResponse(err))
		return
	}

	payload, ok := authPayload.(*token.Payload)
	if !ok {
		err := errors.New("invalid payload type in context")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	parentUserID := payload.UserID
	log.Info().Str("parent_user_id", parentUserID).Msg("getting all children for parent")

	parent, err := server.store.GetParentByParentID(ctx, parentUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Warn().Err(err).Str("parent_user_id", parentUserID).Msg("parent user not found")
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("parent user not found")))
			return
		}
		log.Error().Err(err).Str("parent_user_id", parentUserID).Msg("failed to get parent by user id")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	children, err := server.store.GetChildrenByFamilyID(ctx, parent.FamilyID)
	if err != nil {
		log.Error().Err(err).Str("family_id", parent.FamilyID).Msg("failed to list children by family")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	if len(children) == 0 {
		log.Info().Str("family_id", parent.FamilyID).Msg("no children found for family, returning empty list")
		ctx.JSON(http.StatusOK, []parentChildResponse{})
		return
	}

	// Create a map to hold child details for easy lookup.
	childResponseMap := make(map[string]*parentChildResponse)
	var childOrder []string // Preserve the original order of children.
	for _, child := range children {
		childOrder = append(childOrder, child.ID)
		childResponseMap[child.ID] = &parentChildResponse{
			Child:              child,
			OnboardingProgress: []childOnboardingProgressResponse{},
		}
	}

	// Fetch all onboarding progress for the family in a single query.
	onboardingProgress, err := server.store.GetOnboardingProgressByFamilyID(ctx, parent.FamilyID)
	if err != nil {
		// This is not a fatal error; we can return children without progress.
		log.Error().Err(err).Str("family_id", parent.FamilyID).Msg("failed to get family onboarding progress")
	} else {
		// Distribute the progress items to the correct child in the map.
		for _, progress := range onboardingProgress {
			if resp, ok := childResponseMap[progress.ChildID]; ok {
				// Create a temporary variable to avoid taking the address of a loop variable.
				p := progress
				var completedAt *time.Time
				if p.CompletedAt.Valid {
					completedAt = &p.CompletedAt.Time
				}

				var cta string
				stepType := string(p.StepType)
				if stepType == "course" {
					cta = "/v1/parent/courses"
				} else if stepType == "quiz" && p.StepName == "digital_permit_test" {
					cta = fmt.Sprintf("/v1/parent/child/%s/digital-permit-test/ws", p.ChildID)
					log.Info().Str("child_id", p.ChildID).Str("step_name", p.StepName).Str("cta", cta).Msg("generated digital permit test websocket cta for child")
				}

				resp.OnboardingProgress = append(resp.OnboardingProgress, childOnboardingProgressResponse{
					StepName:    p.StepName,
					Description: p.Description.String,
					StepOrder:   p.StepOrder,
					StepType:    string(p.StepType),
					IsCompleted: p.IsCompleted,
					CompletedAt: completedAt,
					CTA:         cta,
				})
			}
		}
	}

	// Convert the map back to a slice, preserving the original order.
	finalResponse := make([]parentChildResponse, 0, len(children))
	for _, childID := range childOrder {
		finalResponse = append(finalResponse, *childResponseMap[childID])
	}

	log.Info().Str("family_id", parent.FamilyID).Int("child_count", len(finalResponse)).Msg("successfully retrieved all children and their onboarding progress")
	ctx.JSON(http.StatusOK, finalResponse)
}

type updateOwnParentRequest struct {
	Firstname                *string `json:"firstname"`
	Surname                  *string `json:"surname"`
	PushNotificationsEnabled *bool   `json:"push_notifications_enabled"`
}

type updateParentResponse struct {
	ParentID                 string    `json:"parent_id"`
	Firstname                string    `json:"firstname"`
	Surname                  string    `json:"surname"`
	Email                    string    `json:"email"`
	CreatedAt                time.Time `json:"created_at"`
	PushNotificationsEnabled bool      `json:"push_notifications_enabled"`
}

// getParentProfile godoc
// @Summary Get a parent's profile
// @Description Allows an authenticated parent to fetch their own profile information.
// @Tags parents
// @Accept  json
// @Produce  json
// @Success 200 {object} updateParentResponse
// @Failure 500 {object} gin.H "Internal server error"
// @Router /v1/parent/ [get]
func (server *Server) getParentProfile(ctx *gin.Context) {
	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)

	parent, err := server.store.GetParentByParentID(ctx, authPayload.UserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, updateParentResponse{
		ParentID:                 parent.ParentID,
		Firstname:                parent.Firstname,
		Surname:                  parent.Surname,
		Email:                    parent.Email,
		CreatedAt:                parent.CreatedAt,
		PushNotificationsEnabled: parent.PushNotificationsEnabled,
	})
}

// updateParentProfile godoc
// @Summary Update a parent's profile
// @Description Allows an authenticated parent to update their own profile information.
// @Tags parents
// @Accept  json
// @Produce  json
// @Param   profile  body    updateParentRequest  true  "Profile information"
// @Success 200 {object} db.Parent
// @Failure 400 {object} gin.H "Invalid request"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /v1/parent/profile [patch]
func (server *Server) updateParentProfile(ctx *gin.Context) {
	var req updateOwnParentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)

	arg := db.UpdateParentParams{
		ParentID: authPayload.UserID,
	}

	if req.Firstname != nil {
		arg.Firstname = pgtype.Text{String: *req.Firstname, Valid: true}
	}

	if req.Surname != nil {
		arg.Surname = pgtype.Text{String: *req.Surname, Valid: true}
	}

	if req.PushNotificationsEnabled != nil {
		arg.PushNotificationsEnabled = pgtype.Bool{Bool: *req.PushNotificationsEnabled, Valid: true}
	}

	parent, err := server.store.UpdateParent(ctx, arg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, updateParentResponse{
		ParentID:                 parent.ParentID,
		Firstname:                parent.Firstname,
		Surname:                  parent.Surname,
		Email:                    parent.Email,
		CreatedAt:                parent.CreatedAt,
		PushNotificationsEnabled: parent.PushNotificationsEnabled,
	})
}
