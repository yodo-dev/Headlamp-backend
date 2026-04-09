package api

import (
	"database/sql"
	"errors"
	"net/http"

	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type socialMediaRequest struct {
	ID string `uri:"id" binding:"required,uuid"`
}

func (server *Server) getSocialMediaForChild(ctx *gin.Context) {
	var req socialMediaRequest
	if !bindAndValidateUri(ctx, &req) {
		return
	}

	// Calculate the start of the current week (Monday)
	now := time.Now().UTC()
	weekStart := now.Truncate(24 * time.Hour)
	if weekStart.Weekday() != time.Monday {
		daysToSubtract := (int(weekStart.Weekday()) - int(time.Monday) + 7) % 7
		weekStart = weekStart.AddDate(0, 0, -daysToSubtract)
	}

	// Check if the current week's booster is completed
	currentBooster, err := server.store.GetChildBoosterByWeek(ctx, db.GetChildBoosterByWeekParams{
		ChildID:       req.ID,
		WeekStartDate: pgtype.Date{Time: weekStart, Valid: true},
	})

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// If the booster exists and is not completed, block all social media
	if err == nil && !currentBooster.CompletedAt.Valid {
		allPlatforms, err := server.store.GetAllSocialMediaPlatforms(ctx)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}

		blockedResponse := []db.GetSocialMediaAccessStatusForChildRow{}
		for _, p := range allPlatforms {
			blockedResponse = append(blockedResponse, db.GetSocialMediaAccessStatusForChildRow{
				SocialMediaID: p.ID,
				Name:          p.Name,
				IconUrl:       p.IconUrl,
				IsAccessible:  false,
			})
		}
		ctx.JSON(http.StatusOK, blockedResponse)
		return
	}

	// If booster is completed or doesn't exist, return the parent-defined settings
	platforms, err := server.store.GetSocialMediaAccessStatusForChild(ctx, req.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, platforms)
}

type setSocialMediaAccessRequest struct {
	SocialMediaID          int64 `json:"social_media_id" binding:"required,min=1"`
	IsAccessible           bool  `json:"is_accessible"`
	SessionDurationSeconds int32 `json:"session_duration_seconds"`
}

func (server *Server) getParentSocialMediaSettings(ctx *gin.Context) {
	var req socialMediaRequest
	if !bindAndValidateUri(ctx, &req) {
		return
	}

	platforms, err := server.store.GetSocialMediaAccessSettingsForParent(ctx, req.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, platforms)
}

func (server *Server) setSocialMediaAccess(ctx *gin.Context) {
	var uriReq socialMediaRequest
	if !bindAndValidateUri(ctx, &uriReq) {
		return
	}

	var jsonReq setSocialMediaAccessRequest
	if !bindAndValidate(ctx, &jsonReq) {
		return
	}

	if jsonReq.SessionDurationSeconds <= 0 {
		jsonReq.SessionDurationSeconds = 3600 // default 1 hour
	}

	arg := db.SetSocialMediaAccessParams{
		ChildID:                uriReq.ID,
		SocialMediaID:          jsonReq.SocialMediaID,
		IsAccessible:           jsonReq.IsAccessible,
		SessionDurationSeconds: jsonReq.SessionDurationSeconds,
	}

	result, err := server.store.SetSocialMediaAccess(ctx, arg)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, result)
}
