package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/gpt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ParentInsightService handles generating, storing, and fetching daily parent
// insight digests powered by GPT.
type ParentInsightService struct {
	store     db.Store
	gptClient gpt.GptClient
}

func NewParentInsightService(store db.Store, gptClient gpt.GptClient) *ParentInsightService {
	return &ParentInsightService{store: store, gptClient: gptClient}
}

// GenerateDailyInsight generates (or returns the cached) daily insight for the
// given parent-child pair. It is idempotent — safe to call multiple times per day.
func (s *ParentInsightService) GenerateDailyInsight(ctx context.Context, parentID, childID string) (*db.ParentDailyInsight, error) {
	// Idempotency: return today's insight if it already exists
	existing, err := s.store.GetTodayParentInsightForChild(ctx, db.GetTodayParentInsightForChildParams{
		ParentID: parentID,
		ChildID:  childID,
	})
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// Build context from DB
	insightCtx, err := s.buildInsightContext(ctx, childID)
	if err != nil {
		return nil, err
	}

	gptResp, err := s.gptClient.GenerateParentInsight(ctx, *insightCtx)
	if err != nil {
		return nil, err
	}

	contentJSON, err := json.Marshal(gptResp)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var pgDate pgtype.Date
	if err := pgDate.Scan(now); err != nil {
		return nil, err
	}

	insight, err := s.store.CreateParentDailyInsight(ctx, db.CreateParentDailyInsightParams{
		ParentID:       parentID,
		ChildID:        childID,
		Date:           pgDate,
		InsightContent: contentJSON,
		OverallTone:    gptResp.OverallTone,
	})
	if err != nil {
		return nil, err
	}

	return &insight, nil
}

// GetInsightHistory returns paginated insight history for a parent-child pair.
func (s *ParentInsightService) GetInsightHistory(ctx context.Context, parentID, childID string, limit, offset int32) ([]db.ParentDailyInsight, error) {
	return s.store.GetParentInsightHistory(ctx, db.GetParentInsightHistoryParams{
		ParentID: parentID,
		ChildID:  childID,
		Limit:    limit,
		Offset:   offset,
	})
}

// MarkInsightRead marks a specific insight as read. Verifies ownership.
func (s *ParentInsightService) MarkInsightRead(ctx context.Context, insightID uuid.UUID, parentID string) (*db.ParentDailyInsight, error) {
	updated, err := s.store.MarkParentInsightRead(ctx, db.MarkParentInsightReadParams{
		ID:       insightID,
		ParentID: parentID,
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

// buildInsightContext assembles all the data needed for the GPT prompt.
func (s *ParentInsightService) buildInsightContext(ctx context.Context, childID string) (*gpt.ParentInsightContext, error) {
	child, err := s.store.GetChild(ctx, childID)
	if err != nil {
		return nil, err
	}

	age := 0
	if child.Age.Valid {
		age = int(child.Age.Int32)
	}

	insightCtx := &gpt.ParentInsightContext{
		ChildID:        childID,
		ChildFirstName: child.FirstName,
		ChildAge:       age,
	}

	// Social media — last 24 hours
	since := time.Now().UTC().Add(-24 * time.Hour)
	smRows, err := s.store.GetSocialMediaUsageForChild(ctx, db.GetSocialMediaUsageForChildParams{
		ChildID:   childID,
		StartTime: since,
	})
	if err == nil {
		var totalMinutes int
		for _, row := range smRows {
			minutes := int(row.TotalDuration / 60)
			totalMinutes += minutes
			if row.Platform != "" {
				insightCtx.AppsUsedToday = append(insightCtx.AppsUsedToday, gpt.AppUsageToday{
					AppName: row.Platform,
					Minutes: minutes,
				})
			}
		}
		insightCtx.TotalSessionsToday = len(smRows)
		insightCtx.TotalMinutesToday = totalMinutes
	}

	// Aggregate context (streak, digital permit, weekly avg)
	if cached, err := s.store.GetChildReflectionContext(ctx, childID); err == nil {
		insightCtx.ReflectionStreak = int(cached.ReflectionStreak)

		if cached.DigitalPermitStatus.Valid {
			insightCtx.DigitalPermitStatus = cached.DigitalPermitStatus.String
		} else {
			insightCtx.DigitalPermitStatus = "not_started"
		}
		if cached.DigitalPermitScore.Valid {
			f, _ := cached.DigitalPermitScore.Float64Value()
			insightCtx.DigitalPermitScore = f.Float64
		}
		if cached.AvgDailySmMinutes.Valid {
			f, _ := cached.AvgDailySmMinutes.Float64Value()
			insightCtx.WeeklyAvgMinutes = f.Float64
		}
	} else {
		insightCtx.DigitalPermitStatus = "not_started"
	}

	// Today's reflection
	todayReflection, err := s.store.GetTodayDailyReflectionForChild(ctx, childID)
	if err == nil {
		insightCtx.RespondedToReflectionToday = todayReflection.RespondedAt.Valid
		if todayReflection.ResponseType.Valid {
			insightCtx.ReflectionResponseType = string(todayReflection.ResponseType.ReflectionResponseType)
		}
	}

	return insightCtx, nil
}
