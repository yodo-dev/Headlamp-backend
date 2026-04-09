package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

// ─── Session response (matches mobile app contract) ─────────────────────────

type sessionResponse struct {
	ID              string    `json:"id"`
	ChildID         string    `json:"child_id"`
	SocialMediaID   int64     `json:"social_media_id"`
	StartTime       time.Time `json:"start_time"`
	ExpectedEndTime time.Time `json:"expected_end_time"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func toSessionResponse(s db.AppSession) sessionResponse {
	return sessionResponse{
		ID:              s.ID.String(),
		ChildID:         s.ChildID,
		SocialMediaID:   s.SocialMediaID,
		StartTime:       s.StartTime,
		ExpectedEndTime: s.ExpectedEndTime,
		Status:          s.Status,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
}

// ─── POST /v1/child/activity/session/start ───────────────────────────────────

type startSessionRequest struct {
	SocialMediaID int64 `json:"social_media_id" binding:"required,min=1"`
}

func (server *Server) startSession(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)

	var req startSessionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	// Check if app is accessible and get session duration from parent config
	rule, err := server.store.GetSocialMediaAccessRule(ctx, db.GetSocialMediaAccessRuleParams{
		ChildID:       child.ID,
		SocialMediaID: req.SocialMediaID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusForbidden, gin.H{
				"error":   "app_not_configured",
				"message": "this app has not been configured by a parent",
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	if !rule.IsAccessible {
		ctx.JSON(http.StatusForbidden, gin.H{
			"error":   "app_not_accessible",
			"message": "this app is not currently accessible",
		})
		return
	}

	// Idempotency: return existing active session for this app
	existing, err := server.store.GetActiveSessionByChildAndApp(ctx, db.GetActiveSessionByChildAndAppParams{
		ChildID:       child.ID,
		SocialMediaID: req.SocialMediaID,
	})
	if err == nil {
		ctx.JSON(http.StatusOK, toSessionResponse(existing))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Daily limit: if the child already had a session for this app today that
	// expired or ended, deny access until tomorrow (UTC calendar day).
	_, err = server.store.GetTodayExpiredSessionByChildAndApp(ctx, db.GetTodayExpiredSessionByChildAndAppParams{
		ChildID:       child.ID,
		SocialMediaID: req.SocialMediaID,
	})
	if err == nil {
		// A completed session was found today — block until tomorrow
		ctx.JSON(http.StatusForbidden, gin.H{
			"error":   "daily_limit_reached",
			"message": "you have used this app for today, come back tomorrow",
		})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Create new session with parent-configured duration
	duration := time.Duration(rule.SessionDurationSeconds) * time.Second
	if duration <= 0 {
		duration = time.Hour // fallback
	}
	now := time.Now().UTC()
	session, err := server.store.CreateAppSession(ctx, db.CreateAppSessionParams{
		ChildID:         child.ID,
		SocialMediaID:   req.SocialMediaID,
		StartTime:       now,
		ExpectedEndTime: now.Add(duration),
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, toSessionResponse(session))
}

// ─── POST /v1/child/activity/session/end ────────────────────────────────────

type endSessionRequest struct {
	SocialMediaID int64 `json:"social_media_id" binding:"required,min=1"`
}

func (server *Server) endSession(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)

	var req endSessionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	session, err := server.store.MarkSessionEnded(ctx, db.MarkSessionEndedParams{
		ChildID:       child.ID,
		SocialMediaID: req.SocialMediaID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No active session found — treat as success (idempotent)
			ctx.JSON(http.StatusOK, gin.H{"message": "no active session found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, toSessionResponse(session))
}

// ─── GET /v1/child/activity/session/:id ─────────────────────────────────────

func (server *Server) getSessionStatus(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)

	sessionIDStr := ctx.Param("id")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(errors.New("invalid session id")))
		return
	}

	session, err := server.store.GetAppSessionByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, errorResponse(errors.New("session not found")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	if session.ChildID != child.ID {
		ctx.JSON(http.StatusForbidden, errorResponse(errors.New("access denied")))
		return
	}

	ctx.JSON(http.StatusOK, toSessionResponse(session))
}

// ─── Legacy ping (kept for backward compatibility) ───────────────────────────

type pingRequest struct {
	SocialMediaID int64 `json:"social_media_id" binding:"required"`
}

func (server *Server) ping(ctx *gin.Context) {
	var req pingRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	authPayload := ctx.MustGet(authorizationPayloadKey).(db.Child)
	log.Info().Str("child_id", authPayload.ID).Int64("social_media_id", req.SocialMediaID).Msg("Received ping")

	activeSession, err := server.store.GetActiveSessionForChild(ctx, authPayload.ID)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			now := time.Now().UTC()
			arg := db.CreateAppSessionParams{
				ChildID:         authPayload.ID,
				SocialMediaID:   req.SocialMediaID,
				StartTime:       now,
				ExpectedEndTime: now.Add(1 * time.Hour),
			}
			newSession, createErr := server.store.CreateAppSession(ctx, arg)
			if createErr != nil {
				ctx.JSON(http.StatusInternalServerError, errorResponse(createErr))
				return
			}
			ctx.JSON(http.StatusOK, gin.H{"status": "session_started", "session": newSession})
		} else {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		}
		return
	}

	if activeSession.SocialMediaID != req.SocialMediaID {
		_, closeErr := server.store.CloseSessionsForChild(ctx, authPayload.ID)
		if closeErr != nil {
			ctx.JSON(http.StatusInternalServerError, errorResponse(closeErr))
			return
		}
		now := time.Now().UTC()
		arg := db.CreateAppSessionParams{
			ChildID:         authPayload.ID,
			SocialMediaID:   req.SocialMediaID,
			StartTime:       now,
			ExpectedEndTime: now.Add(1 * time.Hour),
		}
		newSession, createErr := server.store.CreateAppSession(ctx, arg)
		if createErr != nil {
			ctx.JSON(http.StatusInternalServerError, errorResponse(createErr))
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"status": "session_started", "session": newSession})
	} else {
		updatedSession, updateErr := server.store.UpdateSessionPing(ctx, activeSession.ID)
		if updateErr != nil {
			ctx.JSON(http.StatusInternalServerError, errorResponse(updateErr))
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"status": "session_updated", "session": updatedSession})
	}
}

// ─── Activity Reporting Endpoints ────────────────────────────────────────────

type activitySummaryResponse struct {
	Date     string          `json:"date"`
	Summary  summaryBlock    `json:"summary"`
	AppUsage []appUsageBlock `json:"app_usage"`
}

type summaryBlock struct {
	TotalTimeMinutes float64  `json:"total_time_minutes"`
	FormattedTime    string   `json:"formatted_time"`
	TimeOnlineToday  string   `json:"time_online_today"`
	TopApps          []string `json:"top_apps"`
	Insight          string   `json:"insight"`
}

type appUsageBlock struct {
	SocialMediaID   int64   `json:"social_media_id"`
	SocialMediaName string  `json:"social_media_name"`
	SocialMediaLogo string  `json:"social_media_logo"`
	UsageMinutes    float64 `json:"usage_minutes"`
	FormattedTime   string  `json:"formatted_time"`
}

func (server *Server) getActivitySummary(ctx *gin.Context) {
	childID := ctx.Param("id")
	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)

	_, err := server.store.GetChildForParent(ctx, db.GetChildForParentParams{
		ParentID: authPayload.UserID,
		ID:       childID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ctx.JSON(http.StatusForbidden, errorResponse(fmt.Errorf("you do not have access to this child")))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	dateStr := ctx.DefaultQuery("date", time.Now().UTC().Format("2006-01-02"))
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid date format: %w", err)))
		return
	}

	startOfDay := date
	endOfDay := startOfDay.Add(24 * time.Hour)

	usageSummary, err := server.store.GetUsageSummaryForDate(ctx, db.GetUsageSummaryForDateParams{
		ChildID:   childID,
		StartTime: startOfDay,
		EndTime:   pgtype.Timestamptz{Time: endOfDay, Valid: true},
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	var totalMinutes float64
	var topApps []string
	var appUsageBlocks []appUsageBlock

	for _, usage := range usageSummary {
		totalMinutes += usage.TotalMinutes
		topApps = append(topApps, usage.SocialMediaName)
		appUsageBlocks = append(appUsageBlocks, appUsageBlock{
			SocialMediaID:   usage.SocialMediaID,
			SocialMediaName: usage.SocialMediaName,
			SocialMediaLogo: usage.SocialMediaLogo.String,
			UsageMinutes:    usage.TotalMinutes,
			FormattedTime:   formatDuration(usage.TotalMinutes),
		})
	}

	startOfYesterday := startOfDay.Add(-24 * time.Hour)
	yesterdayUsageSummary, err := server.store.GetUsageSummaryForDate(ctx, db.GetUsageSummaryForDateParams{
		ChildID:   childID,
		StartTime: startOfYesterday,
		EndTime:   pgtype.Timestamptz{Time: startOfDay, Valid: true},
	})
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("failed to get yesterday's usage for insight")
	}

	var yesterdayTotalMinutes float64
	for _, usage := range yesterdayUsageSummary {
		yesterdayTotalMinutes += usage.TotalMinutes
	}

	insight := generateInsight(totalMinutes, yesterdayTotalMinutes)

	ctx.JSON(http.StatusOK, activitySummaryResponse{
		Date: dateStr,
		Summary: summaryBlock{
			TotalTimeMinutes: totalMinutes,
			FormattedTime:    formatDuration(totalMinutes),
			TimeOnlineToday:  formatDuration(totalMinutes),
			TopApps:          topApps,
			Insight:          insight,
		},
		AppUsage: appUsageBlocks,
	})
}

func generateInsight(today, yesterday float64) string {
	if yesterday == 0 {
		return fmt.Sprintf("You spent %s on social media today.", formatDuration(today))
	}
	diff := today - yesterday
	if diff > 0 {
		return fmt.Sprintf("You spent %s more than yesterday.", formatDuration(diff))
	}
	if diff < 0 {
		return fmt.Sprintf("Great job! You spent %s less than yesterday.", formatDuration(-diff))
	}
	return "Your screen time today is the same as yesterday."
}

type weeklyActivitySummaryResponse struct {
	WeeklyUsage []weeklyAppUsage `json:"weekly_usage"`
}

type weeklyAppUsage struct {
	SocialMediaID   int64            `json:"social_media_id"`
	SocialMediaName string           `json:"social_media_name"`
	SocialMediaLogo string           `json:"social_media_logo"`
	TotalMinutes    float64          `json:"total_minutes"`
	FormattedTime   string           `json:"formatted_time"`
	DailyUsage      []dailyUsageData `json:"daily_usage"`
}

type dailyUsageData struct {
	DayOfWeek string  `json:"day_of_week"`
	Minutes   float64 `json:"minutes"`
}

func (server *Server) getWeeklyActivitySummary(ctx *gin.Context) {
	childID := ctx.Param("id")

	weeklyUsage, err := server.store.GetWeeklyUsageSummary(ctx, childID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	usageByApp := make(map[int64]*weeklyAppUsage)
	for _, row := range weeklyUsage {
		if _, ok := usageByApp[row.SocialMediaID]; !ok {
			usageByApp[row.SocialMediaID] = &weeklyAppUsage{
				SocialMediaID:   row.SocialMediaID,
				SocialMediaName: row.SocialMediaName,
				SocialMediaLogo: row.SocialMediaLogo.String,
				DailyUsage:      make([]dailyUsageData, 0, 7),
			}
		}
		appUsage := usageByApp[row.SocialMediaID]
		appUsage.TotalMinutes += row.TotalMinutes
		appUsage.DailyUsage = append(appUsage.DailyUsage, dailyUsageData{
			DayOfWeek: row.UsageDay.Weekday().String(),
			Minutes:   row.TotalMinutes,
		})
	}

	response := weeklyActivitySummaryResponse{
		WeeklyUsage: make([]weeklyAppUsage, 0, len(usageByApp)),
	}
	for _, appUsage := range usageByApp {
		appUsage.FormattedTime = formatDuration(appUsage.TotalMinutes)
		response.WeeklyUsage = append(response.WeeklyUsage, *appUsage)
	}

	ctx.JSON(http.StatusOK, response)
}

func formatDuration(minutes float64) string {
	hours := int(minutes) / 60
	mins := int(minutes) % 60
	return fmt.Sprintf("%d hr %d min", hours, mins)
}

// ─── Session expiry background worker ────────────────────────────────────────

// sessionExpiredPayload is the push notification data payload sent to the
// child device when the session timer elapses.
type sessionExpiredPayload struct {
	Type            string `json:"type"`
	SessionID       string `json:"session_id"`
	ChildID         string `json:"child_id"`
	SocialMediaID   int64  `json:"social_media_id"`
	ExpectedEndTime string `json:"expected_end_time"`
	Action          string `json:"action"`
}

func (server *Server) startSessionExpiryWorker() {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			<-ticker.C
			server.processExpiredSessions()
		}
	}()
}

func (server *Server) processExpiredSessions() {
	ctx := context.Background()

	expired, err := server.store.GetExpiredActiveSessions(ctx)
	if err != nil {
		log.Error().Err(err).Msg("session expiry worker: failed to query expired sessions")
		return
	}

	for _, session := range expired {
		s := session // capture
		go func() {
			bgCtx := context.Background()

			// Idempotently mark as expired
			updated, err := server.store.MarkSessionExpired(bgCtx, s.ID)
			if err != nil {
				log.Error().Err(err).Str("session_id", s.ID.String()).Msg("session expiry: failed to mark expired")
				return
			}

			log.Info().
				Str("session_id", updated.ID.String()).
				Str("child_id", updated.ChildID).
				Msg("session marked expired")

			// Generate post-session reflection asynchronously (best-effort)
			go func() {
				durationMinutes := int(updated.ExpectedEndTime.Sub(updated.StartTime).Minutes())
				if _, err := server.reflectionService.GenerateTimedSessionReflection(
					context.Background(),
					updated.ChildID,
					updated.ID,
					updated.SocialMediaID,
					durationMinutes,
				); err != nil {
					log.Error().Err(err).
						Str("session_id", updated.ID.String()).
						Msg("session expiry: failed to generate post-session reflection")
				}
			}()

			// Send SESSION_EXPIRED push notification
			server.sendSessionExpiredNotification(updated)
		}()
	}
}

// ─── GET /v1/child/activity/ws ────────────────────────────────────────────────
// WebSocket endpoint. Child connects once on app-open; server pushes
// SESSION_EXPIRED events in real-time when the timer elapses.

func (server *Server) handleSessionWS(ctx *gin.Context) {
	child := ctx.MustGet(authorizationPayloadKey).(db.Child)

	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		log.Error().Err(err).Str("child_id", child.ID).Msg("session ws: upgrade failed")
		return
	}

	server.sessionHub.Register(child.ID, conn)
	defer server.sessionHub.Unregister(child.ID)

	// Keep the connection alive: read and discard any pings from the client.
	// The connection drops when the client disconnects or an error occurs.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (server *Server) sendSessionExpiredNotification(session db.AppSession) {
	payload := sessionExpiredPayload{
		Type:            "SESSION_EXPIRED",
		SessionID:       session.ID.String(),
		ChildID:         session.ChildID,
		SocialMediaID:   session.SocialMediaID,
		ExpectedEndTime: session.ExpectedEndTime.UTC().Format(time.RFC3339),
		Action:          "close_webview_and_prompt_reflection",
	}

	// Deliver over WebSocket if the child is currently connected
	server.sessionHub.BroadcastToChild(session.ChildID, payload)
}
