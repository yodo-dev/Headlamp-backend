package api

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

// weeklyBoosterResponse represents the data for a weekly booster.
// It reuses the CleanModule struct for consistency.
type weeklyBoosterResponse struct {
	CurrentBooster CleanModule  `json:"booster"`                // Keeping the field name as "booster" for backward compatibility
	NextBooster    *CleanModule `json:"next_booster,omitempty"` // Next week's booster, if available
	StreakMessage  string       `json:"streak_message,omitempty"`
}

// getThisWeeksBooster godoc
// @Summary Get the boosters for the current and next week
// @Description Retrieves the assigned weekly boosters for a child for both the current week and the upcoming week.
// @Tags boosters
// @Accept  json
// @Produce  json
// @Param   id   path    string  true  "Child ID"
// @Success 200 {object} weeklyBoosterResponse
// @Failure 400 {object} gin.H "Invalid request"
// @Failure 404 {object} gin.H "Booster not found"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /v1/child/boosters [get]
// getCurrentWeekBooster gets the current week's booster for a child or creates one if it doesn't exist
func (server *Server) getCurrentWeekBooster(ctx *gin.Context, childID string) (db.ChildWeeklyModule, error) {
	// Try to get the existing booster for the current week
	booster, err := server.store.GetCurrentBoosterForChild(ctx, childID)
	if err == nil {
		// Found existing booster
		return booster, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		// Unexpected error
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return db.ChildWeeklyModule{}, err
	}

	// No booster assigned for this week, let's try to assign one.
	log.Info().Msg("No booster found for current week, attempting to assign a new one.")

	// Get all available modules
	allModules, err := server.fetchAllExternalWeeklyModules(ctx)
	if err != nil {
		// Error is handled in the function
		return db.ChildWeeklyModule{}, err
	}
	log.Info().Int("module_count", len(allModules)).Msg("fetched all external weekly modules")

	// Get already assigned module IDs
	assignedModuleIDs, err := server.store.GetAssignedBoosterModuleIDsForChild(ctx, childID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return db.ChildWeeklyModule{}, err
	}
	log.Info().Int("assigned_count", len(assignedModuleIDs)).Msg("fetched assigned booster module IDs for child")

	// Filter out already assigned modules
	assignedSet := make(map[string]struct{}, len(assignedModuleIDs))
	for _, id := range assignedModuleIDs {
		assignedSet[id] = struct{}{}
	}

	var availableModules []extModule
	for _, module := range allModules {
		if _, found := assignedSet[module.DocumentID]; !found {
			availableModules = append(availableModules, module)
		}
	}
	log.Info().Int("available_count", len(availableModules)).Msg("calculated available modules")

	if len(availableModules) == 0 {
		log.Info().Msg("Child has completed all available weekly boosters.")
		ctx.JSON(http.StatusNotFound, gin.H{"message": "Congratulations! You've completed all available boosters."})
		return db.ChildWeeklyModule{}, errors.New("no available modules")
	}

	// Select a random available module
	rand.Seed(time.Now().UnixNano())
	selectedModule := availableModules[rand.Intn(len(availableModules))]
	log.Info().Str("selected_module_id", selectedModule.DocumentID).Msg("selected a random module to assign")

	// Assign the new booster
	now := time.Now().UTC()
	weekStart := now.Truncate(24 * time.Hour)
	if weekStart.Weekday() != time.Monday {
		daysToSubtract := (int(weekStart.Weekday()) - int(time.Monday) + 7) % 7
		weekStart = weekStart.AddDate(0, 0, -daysToSubtract)
	}

	newBoosterID := uuid.New().String()
	assignParams := db.AssignBoosterToChildParams{
		ChildID:          childID,
		ExternalModuleID: selectedModule.DocumentID,
		WeekStartDate:    pgtype.Date{Time: weekStart, Valid: true},
		BoosterID:        newBoosterID,
	}
	log.Info().Interface("params", assignParams).Msg("assigning new booster to child")

	assignedBooster, err := server.store.AssignBoosterToChild(ctx, assignParams)
	if err != nil {
		log.Error().Err(err).Msg("failed to assign new booster to child")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return db.ChildWeeklyModule{}, err
	}
	log.Info().Interface("booster", assignedBooster).Msg("successfully assigned new booster")

	// Send a notification to the child about the new booster
	go server.notifyChildOfNewBooster(assignedBooster.ChildID, selectedModule.Title)

	return assignedBooster, nil
}

func (server *Server) notifyChildOfNewBooster(childID string, boosterTitle string) {
	ctx := context.Background()

	// 1. Get child to find their player ID
	child, err := server.store.GetChild(ctx, childID)
	if err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("failed to get child for new booster notification")
		return
	}

	// 2. Create the notification message
	title := "New Weekly Booster!"
	message := fmt.Sprintf("Your new booster '%s' is ready. Let's check it out!", boosterTitle)

	// 3. Save the notification to our database first
	recipientID, err := uuid.Parse(child.ID)
	if err != nil {
		log.Error().Err(err).Str("child_id", child.ID).Msg("failed to parse child ID for notification")
		return
	}
	_, err = server.store.CreateNotification(ctx, db.CreateNotificationParams{
		RecipientID:   recipientID,
		RecipientType: db.NotificationRecipientTypeChild,
		Title:         title,
		Message:       message,
		SentAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("failed to save new booster notification to database")
	}

	log.Info().Str("child_id", childID).Msg("saved new booster notification")
}

func (server *Server) getThisWeeksBooster(ctx *gin.Context) {
	// The deviceAuthMiddleware has already verified the child and device.
	// We get the child's data from the context.
	authPayload := ctx.MustGet(authorizationPayloadKey).(db.Child)

	log.Info().Str("child_id", authPayload.ID).Msg("getting current week's booster for child")

	// Prepare the response structure
	resp := weeklyBoosterResponse{}

	// 1. Get or create the current week's booster
	currentBooster, err := server.getCurrentWeekBooster(ctx, authPayload.ID)
	if err != nil {
		// Error already handled in getCurrentWeekBooster
		return
	}

	// 2. Fetch the full module data from the external API
	currentModuleData, err := server.fetchExternalWeeklyModuleData(ctx, currentBooster.ExternalModuleID)
	if err != nil {
		// fetchExternalWeeklyModuleData handles sending the error response
		return
	}

	// 3. Calculate the monthly booster streak
	streakMessage := server.calculateBoosterStreak(ctx, authPayload.ID, currentBooster)

	// 4. Get the reflection video for the current booster
	reflectionVideo, err := server.store.GetReflectionVideoForBooster(ctx, currentBooster.BoosterID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		// Log the error but don't fail the request
		log.Error().Err(err).Msg("failed to get reflection video for booster")
	}

	// 5. Construct the response with current week's booster
	resp.CurrentBooster = server.createCleanModule(&currentBooster, currentModuleData, &reflectionVideo)
	resp.StreakMessage = streakMessage

	// 5. Get or create next week's booster
	nextBooster, nextModuleData, err := server.getNextWeekBooster(ctx, authPayload.ID)
	if err == nil && nextBooster != nil && nextModuleData != nil {
		// Only add next booster to response if we have valid data
		// For simplicity, we don't fetch reflection video for the next booster
		nextCleanBooster := server.createCleanModule(nextBooster, nextModuleData, nil)
		resp.NextBooster = &nextCleanBooster
	}

	log.Info().Interface("response", resp).Msg("sending final response")
	ctx.JSON(http.StatusOK, resp)
}

// getNextWeekBooster gets or creates a booster for next week
func (server *Server) getNextWeekBooster(ctx *gin.Context, childID string) (*db.ChildWeeklyModule, *extModule, error) {
	// Try to get the existing booster for next week
	nextBooster, err := server.store.GetNextBoosterForChild(ctx, childID)

	// Check if we have a valid next booster with a valid external module ID
	if err == nil && nextBooster.ExternalModuleID != "" {
		log.Info().Interface("next_booster", nextBooster).Msg("found existing next week's booster")

		// Fetch the module data
		nextModuleData, nextModuleErr := server.fetchExternalWeeklyModuleData(ctx, nextBooster.ExternalModuleID)
		if nextModuleErr == nil && nextModuleData != nil {
			log.Info().Interface("next_module_data", nextModuleData).Msg("successfully fetched next week's module data")
			return &nextBooster, nextModuleData, nil
		} else {
			log.Warn().Err(nextModuleErr).Msg("failed to fetch external data for next booster")
			return nil, nil, nextModuleErr
		}
	}

	// No valid next week booster found, create one
	log.Info().Msg("No booster found for next week, attempting to assign a new one")

	// Get all available modules
	allModules, err := server.fetchAllExternalWeeklyModules(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to fetch all external weekly modules for next week assignment")
		return nil, nil, err
	}

	// Get already assigned module IDs
	assignedModuleIDs, err := server.store.GetAssignedBoosterModuleIDsForChild(ctx, childID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get assigned booster module IDs for next week assignment")
		return nil, nil, err
	}

	// Filter out already assigned modules
	assignedSet := make(map[string]struct{}, len(assignedModuleIDs))
	for _, id := range assignedModuleIDs {
		assignedSet[id] = struct{}{}
	}

	var availableModules []extModule
	for _, module := range allModules {
		if _, found := assignedSet[module.DocumentID]; !found {
			availableModules = append(availableModules, module)
		}
	}

	if len(availableModules) == 0 {
		log.Info().Msg("No available modules for next week's booster")
		return nil, nil, errors.New("no available modules for next week")
	}

	// Select a random available module for next week
	rand.Seed(time.Now().UnixNano())
	selectedModule := availableModules[rand.Intn(len(availableModules))]
	log.Info().Str("selected_module_id", selectedModule.DocumentID).Msg("selected a random module for next week")

	// Calculate next week's start date (Monday of next week)
	nextWeekStart := time.Now().UTC().AddDate(0, 0, 7)
	nextWeekStart = nextWeekStart.Truncate(24 * time.Hour)
	if nextWeekStart.Weekday() != time.Monday {
		daysToSubtract := (int(nextWeekStart.Weekday()) - int(time.Monday) + 7) % 7
		nextWeekStart = nextWeekStart.AddDate(0, 0, -daysToSubtract)
	}

	// Create the new booster for next week
	newBoosterID := uuid.New().String()
	assignParams := db.AssignBoosterToChildParams{
		ChildID:          childID,
		ExternalModuleID: selectedModule.DocumentID,
		WeekStartDate:    pgtype.Date{Time: nextWeekStart, Valid: true},
		BoosterID:        newBoosterID,
	}

	log.Info().Interface("params", assignParams).Msg("assigning new booster for next week")

	nextBooster, err = server.store.AssignBoosterToChild(ctx, assignParams)
	if err != nil {
		log.Error().Err(err).Msg("failed to assign new booster for next week")
		return nil, nil, err
	}

	log.Info().Interface("next_booster", nextBooster).Msg("successfully assigned new booster for next week")
	return &nextBooster, &selectedModule, nil
}

// calculateBoosterStreak calculates the monthly booster streak for a child
func (server *Server) calculateBoosterStreak(ctx *gin.Context, childID string, currentBooster db.ChildWeeklyModule) string {
	// Get all boosters completed this month
	monthlyBoosters, err := server.store.GetBoostersForChildInMonth(ctx, childID)
	if err != nil {
		// Log the error but don't fail the request
		log.Error().Err(err).Msg("failed to get monthly boosters for streak calculation")
		return "" // Return empty streak message on error
	}

	// Count completed boosters
	completedCount := 0
	for _, mb := range monthlyBoosters {
		if mb.CompletedAt.Valid {
			completedCount++
		}
	}

	// Make sure we count the current booster if it's completed
	// This ensures we don't miss the current booster's completion status
	// which might not yet be reflected in the monthlyBoosters query result
	currentBoosterAlreadyCounted := false
	for _, mb := range monthlyBoosters {
		if mb.BoosterID == currentBooster.BoosterID {
			currentBoosterAlreadyCounted = true
			break
		}
	}

	if currentBooster.CompletedAt.Valid && !currentBoosterAlreadyCounted {
		completedCount++
		log.Info().Str("booster_id", currentBooster.BoosterID).Msg("adding current completed booster to streak count")
	}

	// This is a simplification. A more accurate way would be to calculate the number of weeks in the month.
	totalWeeksInMonth := 4
	streakMessage := fmt.Sprintf("You've completed %d of %d boosters this month.", completedCount, totalWeeksInMonth)
	if completedCount > 0 {
		streakMessage = fmt.Sprintf("You're on a streak! 🎉 %s", streakMessage)
	}

	return streakMessage
}

// createCleanModule creates a clean module representation from a booster and its module data
func (server *Server) createCleanModule(booster *db.ChildWeeklyModule, moduleData *extModule, reflectionVideo *db.ReflectionVideo) CleanModule {
	cleanBooster := CleanModule{
		ID:          booster.BoosterID,
		Title:       moduleData.Title,
		Description: flattenDescription(moduleData.Description),
		IsCompleted: booster.CompletedAt.Valid,
	}

	if moduleData.Video != nil {
		cleanBooster.Video = &CleanVideo{
			URL:  server.absoluteURL(moduleData.Video.URL),
			Mime: moduleData.Video.Mime,
			Size: moduleData.Video.Size,
		}
	}

	if moduleData.Quiz != nil {
		cleanBooster.Quiz = &CleanQuiz{
			ID:           moduleData.Quiz.DocumentID,
			Title:        moduleData.Quiz.Title,
			PassingScore: moduleData.Quiz.Passing,
		}
	}

	if reflectionVideo != nil {
		cleanBooster.ReflectionVideoURL = reflectionVideo.VideoUrl
		cleanBooster.ReflectionSubmittedAt = &reflectionVideo.CreatedAt
	}

	return cleanBooster
}

type reflectionVideoRequest struct {
	VideoURL      string `form:"video_url"`
	StrapiAssetID string `form:"strapi_asset_id"`
}

type parentBoosterResponse struct {
	CleanModule
	ReflectionVideoURL    string    `json:"reflection_video_url"`
	ReflectionSubmittedAt time.Time `json:"reflection_submitted_at"`
}

// getBoostersForChildByParent godoc
// @Summary Get all boosters for a child for a parent
// @Description Retrieves all assigned weekly boosters for a child up to the current week, including reflection videos.
// @Tags parents,boosters
// @Accept  json
// @Produce  json
// @Param   id   path    string  true  "Child ID"
// @Success 200 {array} parentBoosterResponse
// @Failure 400 {object} gin.H "Invalid request"
// @Failure 403 {object} gin.H "Forbidden"
// @Failure 404 {object} gin.H "Not found"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /v1/parent/child/{id}/boosters [get]
func (server *Server) getBoostersForChildByParent(ctx *gin.Context) {
	childID := ctx.Param("id")

	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)

	// Check if the parent has access to this child
	_, err := server.store.GetChildForParent(ctx, db.GetChildForParentParams{
		ParentID: authPayload.UserID,
		ID:       childID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusForbidden, errorResponse(errors.New("parent does not have access to this child")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	boosters, err := server.store.GetBoostersForChildByParent(ctx, childID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	resp := []parentBoosterResponse{}
	for _, booster := range boosters {
		moduleData, err := server.fetchExternalWeeklyModuleData(ctx, booster.ExternalModuleID)
		if err != nil {
			// Log the error but continue, so one failed module doesn't break the whole list
			log.Error().Err(err).Str("external_module_id", booster.ExternalModuleID).Msg("failed to fetch external module data for booster")
			continue
		}

		// Manually construct the ChildWeeklyModule struct from the GetBoostersForChildByParentRow
		childWeeklyModule := db.ChildWeeklyModule{
			ID:               booster.ID,
			ChildID:          booster.ChildID,
			ExternalModuleID: booster.ExternalModuleID,
			WeekStartDate:    booster.WeekStartDate,
			ShownAt:          booster.ShownAt,
			CompletedAt:      booster.CompletedAt,
			LatestScore:      booster.LatestScore,
			BoosterID:        booster.BoosterID,
		}

		cleanBooster := server.createCleanModule(&childWeeklyModule, moduleData, nil)

		parentBooster := parentBoosterResponse{
			CleanModule: cleanBooster,
		}

		if booster.ReflectionVideoUrl.Valid {
			parentBooster.ReflectionVideoURL = booster.ReflectionVideoUrl.String
		}

		if booster.ReflectionSubmittedAt.Valid {
			parentBooster.ReflectionSubmittedAt = booster.ReflectionSubmittedAt.Time
		}

		resp = append(resp, parentBooster)
	}

	ctx.JSON(http.StatusOK, resp)
}

// addReflectionVideo godoc
// @Summary Add a reflection video for a booster
// @Description Adds a reflection video URL for a specific completed booster.
// @Tags boosters
// @Accept  json
// @Produce  json
// @Param   id         path    string  true  "Child ID"
// @Param   booster_id path    string  true  "Booster ID"
// @Param   video      body    reflectionVideoRequest  true  "Video information"
// @Success 200 {object} db.ReflectionVideo
// @Failure 400 {object} gin.H "Invalid request"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /v1/child/{id}/booster/{booster_id}/reflection [post]
type reflectionVideoResponse struct {
	ID               int64     `json:"id"`
	BoosterID        string    `json:"booster_id"`
	VideoURL         string    `json:"video_url"`
	SubmittedAt      time.Time `json:"submitted_at"`
	ExternalModuleID string    `json:"external_module_id"`
	BoosterTitle     string    `json:"booster_title"`
}

// getReflectionVideosForChild godoc
// @Summary Get all reflection videos for a child for a parent
// @Description Retrieves all reflection videos a child has ever uploaded.
// @Tags parents,boosters
// @Accept  json
// @Produce  json
// @Param   id   path    string  true  "Child ID"
// @Success 200 {array} reflectionVideoResponse
// @Failure 403 {object} gin.H "Forbidden"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /v1/parent/child/{id}/reflections [get]
func (server *Server) getReflectionVideosForChild(ctx *gin.Context) {
	childID := ctx.Param("id")

	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)

	// Check if the parent has access to this child
	_, err := server.store.GetChildForParent(ctx, db.GetChildForParentParams{
		ParentID: authPayload.UserID,
		ID:       childID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusForbidden, errorResponse(errors.New("parent does not have access to this child")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	videos, err := server.store.GetReflectionVideosForChild(ctx, childID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	resp := []reflectionVideoResponse{}
	for _, video := range videos {
		moduleData, err := server.fetchExternalWeeklyModuleData(ctx, video.ExternalModuleID)
		if err != nil {
			log.Error().Err(err).Str("external_module_id", video.ExternalModuleID).Msg("failed to fetch external module data for reflection video's booster")
			continue
		}

		resp = append(resp, reflectionVideoResponse{
			ID:               video.ID,
			BoosterID:        video.BoosterID,
			VideoURL:         video.VideoUrl,
			SubmittedAt:      video.CreatedAt,
			ExternalModuleID: video.ExternalModuleID,
			BoosterTitle:     moduleData.Title,
		})
	}

	ctx.JSON(http.StatusOK, resp)
}

func (server *Server) addReflectionVideo(ctx *gin.Context) {
	// The deviceAuthMiddleware has already verified the child and device.
	// We get the child's data from the context.
	authPayload := ctx.MustGet(authorizationPayloadKey).(db.Child)
	boosterID := ctx.Param("booster_id")

	var req reflectionVideoRequest
	if err := ctx.ShouldBind(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	// Handle the video file upload
	file, header, err := ctx.Request.FormFile("video_file")
	// We allow either a direct URL or a file upload
	var videoURL string
	var strapiAssetID string

	if err != nil && err != http.ErrMissingFile {
		ctx.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid file upload: %w", err)))
		return
	}

	if file != nil {
		defer file.Close()

		// Upload to the external content provider
		uploadURL, err := server.uploader.UploadFile(header.Filename, file, "app/reflection_videos")
		if err != nil {
			log.Error().Err(err).Msg("failed to upload reflection video")
			ctx.JSON(http.StatusInternalServerError, errorResponse(errors.New("failed to process reflection video")))
			return
		}
		videoURL = uploadURL
		// Extract the Strapi asset ID from the response if needed
		// This would depend on how your uploader returns the ID
	} else if req.VideoURL != "" {
		// Use the provided URL if no file was uploaded
		videoURL = req.VideoURL
		strapiAssetID = req.StrapiAssetID
	} else {
		ctx.JSON(http.StatusBadRequest, errorResponse(errors.New("either video_url or video_file must be provided")))
		return
	}

	arg := db.CreateReflectionVideoParams{
		ChildID:   authPayload.ID,
		BoosterID: boosterID,
		VideoUrl:  videoURL,
		StrapiAssetID: pgtype.Text{
			String: strapiAssetID,
			Valid:  strapiAssetID != "",
		},
	}

	video, err := server.store.CreateReflectionVideo(ctx, arg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, video)
}
