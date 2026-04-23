package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
)

// ─── fetchAllCoursesForUnlock ────────────────────────────────────────────────
// A context.Context-based (non-gin) variant of fetchAllExternalCourses, safe
// to call from background goroutines where no HTTP response writer is available.

func (server *Server) fetchAllCoursesForUnlock(ctx context.Context) ([]extCourseItem, error) {
	base := strings.TrimRight(server.config.ExternalContentBaseURL, "/")
	if base == "" {
		return nil, fmt.Errorf("external content base URL not configured")
	}

	reqURL := fmt.Sprintf("%s/api/courses", base)

	timeout := server.config.ExternalRequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build courses request: %w", err)
	}
	if server.config.ExternalContentToken != "" {
		req.Header.Set("Authorization", "Bearer "+server.config.ExternalContentToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("external courses request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("external courses returned status %d", resp.StatusCode)
	}

	var extResp extAllCoursesResponse
	if err := json.Unmarshal(body, &extResp); err != nil {
		return nil, fmt.Errorf("failed to parse courses response: %w", err)
	}
	return extResp.Data, nil
}

// ─── ensureUnlockSystemSeeded ────────────────────────────────────────────────
// Idempotently seeds the three unlock tables for a child. Safe to call on every
// request; all upserts use ON CONFLICT DO NOTHING.

func (server *Server) ensureUnlockSystemSeeded(ctx context.Context, childID string) error {
	// 1. Determine DPT completion from existing digital_permit_tests table
	var dptCompletedAt pgtype.Timestamptz
	dpt, err := server.store.GetLatestCompletedDigitalPermitTestByChildID(ctx, childID)
	if err == nil && dpt.CompletedAt.Valid {
		dptCompletedAt = pgtype.Timestamptz{Time: dpt.CompletedAt.Time, Valid: true}
	}

	// 2. Upsert child_progress_gate (default: 1 course unlocks 1 social slot)
	_, err = server.store.UpsertChildProgressGate(ctx, db.UpsertChildProgressGateParams{
		ChildID:                      childID,
		DigitalPermitTestCompletedAt: dptCompletedAt,
		UnlockAfterCourses:           1,
	})
	if err != nil {
		return fmt.Errorf("upsert child_progress_gate: %w", err)
	}

	// 3. Seed child_course_unlock from Strapi
	courses, err := server.fetchAllCoursesForUnlock(ctx)
	if err != nil {
		return fmt.Errorf("fetch courses for seeding: %w", err)
	}

	for i, course := range courses {
		status := db.CourseStatusLocked
		if i == 0 {
			// First course starts UNLOCKED (DPT is the actual gate, not the course order)
			status = db.CourseStatusUnlocked
		}
		if err := server.store.UpsertChildCourseUnlock(ctx, db.UpsertChildCourseUnlockParams{
			ChildID:     childID,
			CourseID:    course.DocumentID,
			CourseOrder: int32(i),
			Status:      status,
		}); err != nil {
			return fmt.Errorf("upsert course unlock %s: %w", course.DocumentID, err)
		}
	}

	// 4. Seed social_app_access from social_medias table
	platforms, err := server.store.ListSocialMedias(ctx)
	if err != nil {
		return fmt.Errorf("list social medias for seeding: %w", err)
	}
	for _, platform := range platforms {
		if err := server.store.UpsertSocialAppAccess(ctx, db.UpsertSocialAppAccessParams{
			ChildID:       childID,
			SocialMediaID: platform.ID,
		}); err != nil {
			return fmt.Errorf("upsert social_app_access for platform %d: %w", platform.ID, err)
		}
	}

	return nil
}

// ─── triggerDPTUnlockInitialization ─────────────────────────────────────────
// Called as a goroutine when the Digital Permit Test completes. Seeds the unlock
// system so it's ready when the child views their courses/social access.

func (server *Server) triggerDPTUnlockInitialization(childID string) {
	ctx := context.Background()
	if err := server.ensureUnlockSystemSeeded(ctx, childID); err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("triggerDPTUnlockInitialization: seed failed")
		return
	}

	// Mark DPT as completed in child_progress_gate
	dpt, err := server.store.GetLatestCompletedDigitalPermitTestByChildID(ctx, childID)
	if err != nil || !dpt.CompletedAt.Valid {
		return
	}
	if _, err := server.store.UpsertChildProgressGate(ctx, db.UpsertChildProgressGateParams{
		ChildID:                      childID,
		DigitalPermitTestCompletedAt: pgtype.Timestamptz{Time: dpt.CompletedAt.Time, Valid: true},
		UnlockAfterCourses:           1,
	}); err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("triggerDPTUnlockInitialization: mark DPT failed")
	}

	// Log audit event
	metaJSON, _ := json.Marshal(map[string]string{
		"test_id": dpt.ID.String(),
		"result":  string(dpt.Result.DigitalPermitTestResult),
	})
	_ = server.store.InsertUnlockAuditEvent(ctx, db.InsertUnlockAuditEventParams{
		ChildID:   childID,
		EventType: db.UnlockEventDPTCompleted,
		Metadata:  json.RawMessage(metaJSON),
	})
	log.Info().Str("child_id", childID).Msg("triggerDPTUnlockInitialization: complete")
}

// ─── recalculateUnlockSlots ──────────────────────────────────────────────────
// Calculates how many social app slots the child has earned and promotes the
// appropriate number of LOCKED apps to ELIGIBLE_PENDING_PARENT_APPROVAL.
// Returns the newly promoted apps (delta > 0 case).

func (server *Server) recalculateUnlockSlots(ctx context.Context, childID string) ([]db.SocialAppAccess, error) {
	gate, err := server.store.GetChildProgressGate(ctx, childID)
	if err != nil {
		return nil, fmt.Errorf("get child_progress_gate: %w", err)
	}

	// Must have completed DPT first
	if !gate.DigitalPermitTestCompletedAt.Valid {
		return nil, nil
	}

	completedCount, err := server.store.CountCompletedCoursesForChild(ctx, childID)
	if err != nil {
		return nil, fmt.Errorf("count completed courses: %w", err)
	}

	unlockAfter := int64(gate.UnlockAfterCourses)
	if unlockAfter <= 0 {
		unlockAfter = 1
	}

	earnedSlots := completedCount / unlockAfter

	nonLockedCount, err := server.store.CountNonLockedSocialAppsForChild(ctx, childID)
	if err != nil {
		return nil, fmt.Errorf("count non-locked social apps: %w", err)
	}

	delta := earnedSlots - nonLockedCount
	if delta <= 0 {
		return nil, nil
	}

	var promoted []db.SocialAppAccess
	for i := int64(0); i < delta; i++ {
		app, err := server.store.GetFirstLockedSocialAppForChild(ctx, childID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				break // No more locked apps
			}
			return promoted, fmt.Errorf("get first locked social app: %w", err)
		}
		updated, err := server.store.MakeSocialAppEligible(ctx, db.MakeSocialAppEligibleParams{
			ChildID:       childID,
			SocialMediaID: app.SocialMediaID,
		})
		if err != nil {
			log.Warn().Err(err).Int64("social_media_id", app.SocialMediaID).Msg("recalculateUnlockSlots: MakeSocialAppEligible failed (possibly already promoted)")
			continue
		}
		promoted = append(promoted, updated)
	}
	return promoted, nil
}

// ─── completeCourse ──────────────────────────────────────────────────────────
// POST /v1/child/courses/:course_id/complete

func (server *Server) completeCourse(ctx *gin.Context) {
	authPayload := server.getAuthPayload(ctx)
	if authPayload == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("unauthorized")))
		return
	}
	childID := authPayload.UserID
	courseID := ctx.Param("course_id")
	if courseID == "" {
		ctx.JSON(http.StatusBadRequest, errorResponse(errors.New("course_id is required")))
		return
	}

	bgCtx := context.Background()

	// Seed on first call
	if err := server.ensureUnlockSystemSeeded(bgCtx, childID); err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("completeCourse: seed failed")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Ensure DPT is completed before any course can be completed
	gate, err := server.store.GetChildProgressGate(bgCtx, childID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if !gate.DigitalPermitTestCompletedAt.Valid {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "digital permit test must be completed before completing courses"})
		return
	}

	// Verify course is in UNLOCKED state
	courseUnlock, err := server.store.GetChildCourseUnlockByCourse(bgCtx, db.GetChildCourseUnlockByCourseParams{
		ChildID:  childID,
		CourseID: courseID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "course not found for this child"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if courseUnlock.Status == db.CourseStatusCompleted {
		ctx.JSON(http.StatusConflict, gin.H{"error": "course already completed"})
		return
	}
	if courseUnlock.Status == db.CourseStatusLocked {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "course is locked — complete previous courses first"})
		return
	}

	// Verify all modules in the course are completed in child_module_progress
	courseData, err := server.fetchExternalCourseData(nil, courseID)
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch course data", "detail": err.Error()})
		return
	}
	for _, mod := range courseData.Modules {
		progress, err := server.store.GetChildModuleProgress(bgCtx, db.GetChildModuleProgressParams{
			ChildID:  childID,
			ModuleID: mod.DocumentID,
		})
		if err != nil || !progress.IsCompleted {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"error":     "not all modules are completed",
				"module_id": mod.DocumentID,
				"title":     mod.Title,
			})
			return
		}
	}

	// Mark course as COMPLETED
	_, err = server.store.UpdateChildCourseUnlockStatus(bgCtx, db.UpdateChildCourseUnlockStatusParams{
		ChildID:  childID,
		CourseID: courseID,
		Status:   db.CourseStatusCompleted,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Recalculate social slots
	newlyEligible, err := server.recalculateUnlockSlots(bgCtx, childID)
	if err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("completeCourse: recalculate slots error (non-fatal)")
	}

	// Unlock next locked course
	var nextCourseID string
	nextLocked, err := server.store.GetFirstLockedCourseForChild(bgCtx, childID)
	if err == nil {
		unlocked, err := server.store.UpdateChildCourseUnlockStatus(bgCtx, db.UpdateChildCourseUnlockStatusParams{
			ChildID:  childID,
			CourseID: nextLocked.CourseID,
			Status:   db.CourseStatusUnlocked,
		})
		if err == nil {
			nextCourseID = unlocked.CourseID
		}
	}

	// Audit events
	completedMeta, _ := json.Marshal(map[string]string{"course_id": courseID})
	_ = server.store.InsertUnlockAuditEvent(bgCtx, db.InsertUnlockAuditEventParams{
		ChildID:   childID,
		EventType: db.UnlockEventCourseCompleted,
		Metadata:  json.RawMessage(completedMeta),
	})
	for _, app := range newlyEligible {
		slotMeta, _ := json.Marshal(map[string]interface{}{"social_media_id": app.SocialMediaID, "triggered_by": "course_completed", "course_id": courseID})
		_ = server.store.InsertUnlockAuditEvent(bgCtx, db.InsertUnlockAuditEventParams{
			ChildID:   childID,
			EventType: db.UnlockEventSocialSlotEligible,
			Metadata:  json.RawMessage(slotMeta),
		})
	}

	// Notify parent if new slots became eligible
	parentNotified := false
	if len(newlyEligible) > 0 {
		parentNotified = true
		go func() {
			bgCtx2 := context.Background()
			parentUID, childName, ok := server.lookupParentForChild(bgCtx2, childID)
			if !ok {
				return
			}
			if server.notificationService != nil {
				_ = server.notificationService.CreateAndSend(
					bgCtx2,
					parentUID,
					db.NotificationRecipientTypeParent,
					fmt.Sprintf("%s earned a new app slot!", childName),
					fmt.Sprintf("%s completed a course and can now unlock a social app. Approve it now.", childName),
				)
			}
		}()
	}

	// Build response
	completedCount, _ := server.store.CountCompletedCoursesForChild(bgCtx, childID)
	eligibleSlots, _ := server.store.CountNonLockedSocialAppsForChild(bgCtx, childID)

	type newlyEligibleItem struct {
		SocialMediaID int64  `json:"social_media_id"`
		State         string `json:"state"`
	}
	var eligibleList []newlyEligibleItem
	for _, app := range newlyEligible {
		eligibleList = append(eligibleList, newlyEligibleItem{
			SocialMediaID: app.SocialMediaID,
			State:         app.State,
		})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"child_id":                    childID,
		"course_id":                   courseID,
		"courses_completed_count":     completedCount,
		"eligible_social_slots_count": eligibleSlots,
		"newly_eligible_apps":         eligibleList,
		"next_unlocked_course_id":     nextCourseID,
		"parent_notification_queued":  parentNotified,
	})
}

// ─── getChildSocialAccess ────────────────────────────────────────────────────
// GET /v1/child/social-access

func (server *Server) getChildSocialAccess(ctx *gin.Context) {
	authPayload := server.getAuthPayload(ctx)
	if authPayload == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("unauthorized")))
		return
	}
	childID := authPayload.UserID
	bgCtx := context.Background()

	if err := server.ensureUnlockSystemSeeded(bgCtx, childID); err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("getChildSocialAccess: seed failed")
	}

	gate, err := server.store.GetChildProgressGate(bgCtx, childID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	apps, err := server.store.GetSocialAppAccessForChild(bgCtx, childID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	completedCount, _ := server.store.CountCompletedCoursesForChild(bgCtx, childID)

	type appItem struct {
		SocialMediaID int64   `json:"social_media_id"`
		Name          string  `json:"name"`
		IconURL       *string `json:"icon_url"`
		State         string  `json:"state"`
	}
	var appList []appItem
	for _, a := range apps {
		item := appItem{
			SocialMediaID: a.SocialMediaID,
			Name:          a.Name,
			State:         a.State,
		}
		if a.IconUrl.Valid {
			item.IconURL = &a.IconUrl.String
		}
		appList = append(appList, item)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"digital_permit_test_completed": gate.DigitalPermitTestCompletedAt.Valid,
		"courses_completed_count":       completedCount,
		"social_apps":                   appList,
	})
}

// ─── getUnlockStatus ─────────────────────────────────────────────────────────
// GET /v1/parent/child/:id/unlock-status

func (server *Server) getUnlockStatus(ctx *gin.Context) {
	childID := ctx.Param("id")
	// Parent ownership already verified by authMiddleware
	bgCtx := context.Background()

	if err := server.ensureUnlockSystemSeeded(bgCtx, childID); err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("getUnlockStatus: seed failed")
	}

	gate, err := server.store.GetChildProgressGate(bgCtx, childID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	courseUnlocks, err := server.store.GetChildCourseUnlocks(bgCtx, childID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Enrich with titles from Strapi (best-effort)
	allCourses, _ := server.fetchAllCoursesForUnlock(bgCtx)
	titleMap := make(map[string]string, len(allCourses))
	for _, c := range allCourses {
		titleMap[c.DocumentID] = c.Title
	}

	type courseItem struct {
		CourseID    string  `json:"course_id"`
		Title       string  `json:"title"`
		Status      string  `json:"status"`
		UnlockedAt  *string `json:"unlocked_at,omitempty"`
		CompletedAt *string `json:"completed_at,omitempty"`
	}
	var courseList []courseItem
	for _, cu := range courseUnlocks {
		item := courseItem{
			CourseID: cu.CourseID,
			Title:    titleMap[cu.CourseID],
			Status:   cu.Status,
		}
		if cu.UnlockedAt.Valid {
			s := cu.UnlockedAt.Time.Format(time.RFC3339)
			item.UnlockedAt = &s
		}
		if cu.CompletedAt.Valid {
			s := cu.CompletedAt.Time.Format(time.RFC3339)
			item.CompletedAt = &s
		}
		courseList = append(courseList, item)
	}

	apps, err := server.store.GetSocialAppAccessForChild(bgCtx, childID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	type appItem struct {
		SocialMediaID int64   `json:"social_media_id"`
		Name          string  `json:"name"`
		IconURL       *string `json:"icon_url,omitempty"`
		State         string  `json:"state"`
		EnabledAt     *string `json:"enabled_at,omitempty"`
	}
	var appList []appItem
	pendingParentActions := 0
	for _, a := range apps {
		item := appItem{
			SocialMediaID: a.SocialMediaID,
			Name:          a.Name,
			State:         a.State,
		}
		if a.IconUrl.Valid {
			item.IconURL = &a.IconUrl.String
		}
		if a.EnabledAt.Valid {
			s := a.EnabledAt.Time.Format(time.RFC3339)
			item.EnabledAt = &s
		}
		if a.State == db.SocialAppStateEligiblePendingApproval {
			pendingParentActions++
		}
		appList = append(appList, item)
	}

	completedCount, _ := server.store.CountCompletedCoursesForChild(bgCtx, childID)
	eligibleSlots, _ := server.store.CountNonLockedSocialAppsForChild(bgCtx, childID)

	resp := gin.H{
		"child_id":                      childID,
		"digital_permit_test_completed": gate.DigitalPermitTestCompletedAt.Valid,
		"unlock_after_courses":          gate.UnlockAfterCourses,
		"courses_completed_count":       completedCount,
		"total_courses":                 len(courseList),
		"eligible_social_slots_count":   eligibleSlots,
		"courses":                       courseList,
		"social_apps":                   appList,
		"pending_parent_actions":        pendingParentActions,
	}
	if gate.DigitalPermitTestCompletedAt.Valid {
		s := gate.DigitalPermitTestCompletedAt.Time.Format(time.RFC3339)
		resp["digital_permit_test_completed_at"] = s
	}

	ctx.JSON(http.StatusOK, resp)
}

// ─── parentEnableSocialApp ────────────────────────────────────────────────────
// POST /v1/parent/child/:id/social-apps/:social_media_id/enable

func (server *Server) parentEnableSocialApp(ctx *gin.Context) {
	childID := ctx.Param("id")
	// Parent ownership already verified by authMiddleware

	authPayload := server.getAuthPayload(ctx)
	if authPayload == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("unauthorized")))
		return
	}
	parentID := authPayload.UserID

	socialMediaIDStr := ctx.Param("social_media_id")
	socialMediaID, err := strconv.ParseInt(socialMediaIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(errors.New("invalid social_media_id")))
		return
	}

	bgCtx := context.Background()

	// Fetch current state
	access, err := server.store.GetSocialAppAccessByChildAndSocialMedia(bgCtx, db.GetSocialAppAccessByChildAndSocialMediaParams{
		ChildID:       childID,
		SocialMediaID: socialMediaID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "social app record not found for this child"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Must be ELIGIBLE or already ENABLED (idempotent)
	if access.State == db.SocialAppStateLocked {
		ctx.JSON(http.StatusForbidden, gin.H{
			"error": "app is locked — child must complete more courses to earn this slot",
			"state": access.State,
		})
		return
	}

	// If already enabled, return success idempotently
	appName := ""
	var enabledAtStr string
	if access.State == db.SocialAppStateEnabled {
		if access.EnabledAt.Valid {
			enabledAtStr = access.EnabledAt.Time.Format(time.RFC3339)
		}
		ctx.JSON(http.StatusOK, gin.H{
			"child_id":                  childID,
			"social_media_id":           socialMediaID,
			"state":                     access.State,
			"enabled_at":                enabledAtStr,
			"child_notification_queued": false,
		})
		return
	}

	// Verify available slot budget: earned slots >= non-locked count + 1 (the one we're enabling)
	gate, err := server.store.GetChildProgressGate(bgCtx, childID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	completedCount, _ := server.store.CountCompletedCoursesForChild(bgCtx, childID)
	unlockAfter := int64(gate.UnlockAfterCourses)
	if unlockAfter <= 0 {
		unlockAfter = 1
	}
	earnedSlots := completedCount / unlockAfter
	nonLockedCount, _ := server.store.CountNonLockedSocialAppsForChild(bgCtx, childID)
	if nonLockedCount > earnedSlots {
		ctx.JSON(http.StatusForbidden, gin.H{
			"error":        "no available slots — child must complete more courses",
			"earned_slots": earnedSlots,
			"used_slots":   nonLockedCount,
		})
		return
	}

	// Enable the app
	updated, err := server.store.EnableSocialApp(bgCtx, db.EnableSocialAppParams{
		ChildID:           childID,
		SocialMediaID:     socialMediaID,
		EnabledByParentID: parentID,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Audit event
	auditMeta, _ := json.Marshal(map[string]interface{}{
		"social_media_id":   socialMediaID,
		"enabled_by_parent": parentID,
	})
	_ = server.store.InsertUnlockAuditEvent(bgCtx, db.InsertUnlockAuditEventParams{
		ChildID:   childID,
		EventType: db.UnlockEventParentEnabledSocialApp,
		Metadata:  json.RawMessage(auditMeta),
	})

	// Notify child
	go func(cid string, smID int64, name string) {
		bgCtx2 := context.Background()
		childUUID, parseErr := uuid.Parse(cid)
		if parseErr != nil {
			return
		}
		msg := "Your parent enabled a new app for you!"
		if name != "" {
			msg = fmt.Sprintf("Your parent enabled %s for you!", name)
		}
		if server.notificationService != nil {
			_ = server.notificationService.CreateAndSend(
				bgCtx2,
				childUUID,
				db.NotificationRecipientTypeChild,
				"New app unlocked!",
				msg,
			)
		}
	}(childID, socialMediaID, appName)

	if updated.EnabledAt.Valid {
		enabledAtStr = updated.EnabledAt.Time.Format(time.RFC3339)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"child_id":                  childID,
		"social_media_id":           socialMediaID,
		"state":                     updated.State,
		"enabled_at":                enabledAtStr,
		"child_notification_queued": true,
	})
}
