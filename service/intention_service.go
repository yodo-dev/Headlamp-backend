package service

import (
	"context"
	"encoding/json"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// IntentionService manages daily intentions for children.
type IntentionService struct {
	store db.Store
}

func NewIntentionService(store db.Store) *IntentionService {
	return &IntentionService{store: store}
}

// CreateIntention deactivates any existing today intention and creates a new one.
func (s *IntentionService) CreateIntention(ctx context.Context, childID, intentionText string, timeLimitMin *int32, goals []string) (*db.DailyIntention, error) {
	// Deactivate any existing today intention first
	if err := s.store.DeactivateTodayIntentionsForChild(ctx, childID); err != nil {
		return nil, err
	}

	goalsJSON, err := json.Marshal(goals)
	if err != nil {
		return nil, err
	}

	timeLimitParam := pgtype.Int4{}
	if timeLimitMin != nil {
		timeLimitParam = pgtype.Int4{Int32: *timeLimitMin, Valid: true}
	}

	today := time.Now().UTC()
	intention, err := s.store.CreateDailyIntention(ctx, db.CreateDailyIntentionParams{
		ChildID:          childID,
		IntentionText:    intentionText,
		IntentionDate:    pgtype.Date{Time: today, Valid: true},
		TimeLimitMinutes: timeLimitParam,
		SpecificGoals:    goalsJSON,
	})
	if err != nil {
		return nil, err
	}

	return &intention, nil
}

// GetTodayIntention retrieves the active intention for today.
func (s *IntentionService) GetTodayIntention(ctx context.Context, childID string) (*db.DailyIntention, error) {
	intention, err := s.store.GetTodayIntention(ctx, childID)
	if err != nil {
		return nil, err
	}
	return &intention, nil
}

// GetIntentionByID retrieves a specific intention by ID.
func (s *IntentionService) GetIntentionByID(ctx context.Context, id uuid.UUID) (*db.DailyIntention, error) {
	intention, err := s.store.GetIntentionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &intention, nil
}

// DeactivateIntention marks a specific intention as inactive.
func (s *IntentionService) DeactivateIntention(ctx context.Context, id uuid.UUID) error {
	return s.store.DeactivateIntention(ctx, id)
}

// GetIntentionHistory returns paginated intentions optionally filtered by date range.
func (s *IntentionService) GetIntentionHistory(ctx context.Context, childID string, from, to *time.Time, limit, offset int32) ([]db.DailyIntention, error) {
	fromDate := pgtype.Date{}
	if from != nil {
		fromDate = pgtype.Date{Time: *from, Valid: true}
	}

	toDate := pgtype.Date{}
	if to != nil {
		toDate = pgtype.Date{Time: *to, Valid: true}
	}

	return s.store.GetIntentionHistory(ctx, db.GetIntentionHistoryParams{
		ChildID: childID,
		Column2: fromDate,
		Column3: toDate,
		Limit:   limit,
		Offset:  offset,
	})
}
