package service

import (
	"context"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// JournalService manages journal entries for children.
type JournalService struct {
	store db.Store
}

func NewJournalService(store db.Store) *JournalService {
	return &JournalService{store: store}
}

// CreateEntry creates a new journal entry for today.
func (s *JournalService) CreateEntry(ctx context.Context, childID, text string, mood *string, tags, mediaURLs []string) (*db.JournalEntry, error) {
	if tags == nil {
		tags = []string{}
	}
	if mediaURLs == nil {
		mediaURLs = []string{}
	}

	moodParam := pgtype.Text{}
	if mood != nil {
		moodParam = pgtype.Text{String: *mood, Valid: true}
	}

	entry, err := s.store.CreateJournalEntry(ctx, db.CreateJournalEntryParams{
		ChildID:   childID,
		EntryDate: pgtype.Date{Time: time.Now().UTC(), Valid: true},
		EntryText: text,
		Mood:      moodParam,
		Tags:      tags,
		MediaUrls: mediaURLs,
	})
	if err != nil {
		return nil, err
	}

	return &entry, nil
}

// GetEntry retrieves a single journal entry by ID.
func (s *JournalService) GetEntry(ctx context.Context, id uuid.UUID) (*db.JournalEntry, error) {
	entry, err := s.store.GetJournalEntry(ctx, id)
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// UpdateEntry updates the text, mood, and tags of an existing journal entry.
func (s *JournalService) UpdateEntry(ctx context.Context, id uuid.UUID, text string, mood *string, tags []string) (*db.JournalEntry, error) {
	if tags == nil {
		tags = []string{}
	}

	moodParam := pgtype.Text{}
	if mood != nil {
		moodParam = pgtype.Text{String: *mood, Valid: true}
	}

	entry, err := s.store.UpdateJournalEntry(ctx, db.UpdateJournalEntryParams{
		ID:        id,
		EntryText: text,
		Mood:      moodParam,
		Tags:      tags,
	})
	if err != nil {
		return nil, err
	}

	return &entry, nil
}

// DeleteEntry permanently deletes a journal entry.
func (s *JournalService) DeleteEntry(ctx context.Context, id uuid.UUID) error {
	return s.store.DeleteJournalEntry(ctx, id)
}

// GetEntries returns paginated journal entries, optionally filtered by date range.
func (s *JournalService) GetEntries(ctx context.Context, childID string, from, to *time.Time, limit, offset int32) ([]db.JournalEntry, error) {
	fromDate := pgtype.Date{}
	if from != nil {
		fromDate = pgtype.Date{Time: *from, Valid: true}
	}

	toDate := pgtype.Date{}
	if to != nil {
		toDate = pgtype.Date{Time: *to, Valid: true}
	}

	return s.store.GetJournalEntriesForChild(ctx, db.GetJournalEntriesForChildParams{
		ChildID: childID,
		Column2: fromDate,
		Column3: toDate,
		Limit:   limit,
		Offset:  offset,
	})
}

// GetStats returns journal aggregate statistics for a child.
func (s *JournalService) GetStats(ctx context.Context, childID string) (db.GetJournalStatsRow, error) {
	return s.store.GetJournalStats(ctx, childID)
}
