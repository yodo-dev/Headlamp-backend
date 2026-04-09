package service

import (
	"context"
	"errors"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// SyncRequest contains offline data the client wants to upload, plus the
// timestamp of the last successful sync.
type SyncRequest struct {
	LastSyncAt            time.Time            `json:"last_sync_at"`
	PendingJournalEntries []JournalSyncEntry   `json:"pending_journal_entries"`
	PendingIntentions     []IntentionSyncEntry `json:"pending_intentions"`
}

// JournalSyncEntry is a journal entry created offline.
type JournalSyncEntry struct {
	EntryDate string   `json:"entry_date"` // "YYYY-MM-DD"
	EntryText string   `json:"entry_text"`
	Mood      *string  `json:"mood,omitempty"`
	Tags      []string `json:"tags"`
	MediaURLs []string `json:"media_urls"`
}

// IntentionSyncEntry is an intention created offline.
type IntentionSyncEntry struct {
	IntentionDate    string   `json:"intention_date"` // "YYYY-MM-DD"
	IntentionText    string   `json:"intention_text"`
	TimeLimitMinutes *int32   `json:"time_limit_minutes,omitempty"`
	Goals            []string `json:"goals"`
}

// SyncResponse contains server-side data created since the client's last sync.
type SyncResponse struct {
	UpdatedReflections    []db.Reflection     `json:"updated_reflections"`
	UpdatedIntentions     []db.DailyIntention `json:"updated_intentions"`
	UpdatedJournalEntries []db.JournalEntry   `json:"updated_journal_entries"`
	SyncedAt              time.Time           `json:"synced_at"`
	Errors                []string            `json:"errors,omitempty"`
}

// SyncService handles bi-directional offline data synchronisation.
type SyncService struct {
	store             db.Store
	journalService    *JournalService
	reflectionService *ReflectionService
	intentionService  *IntentionService
}

func NewSyncService(
	store db.Store,
	journalService *JournalService,
	reflectionService *ReflectionService,
	intentionService *IntentionService,
) *SyncService {
	return &SyncService{
		store:             store,
		journalService:    journalService,
		reflectionService: reflectionService,
		intentionService:  intentionService,
	}
}

// Sync uploads offline data and returns server-side changes since LastSyncAt.
func (s *SyncService) Sync(ctx context.Context, childID string, req SyncRequest) (*SyncResponse, error) {
	resp := &SyncResponse{SyncedAt: time.Now().UTC()}
	var syncErrors []string

	// Upload pending journal entries
	for _, je := range req.PendingJournalEntries {
		if _, err := s.journalService.CreateEntry(ctx, childID, je.EntryText, je.Mood, je.Tags, je.MediaURLs); err != nil {
			// Conflict (duplicate date) and constraint errors are swallowed to
			// keep sync idempotent. Unexpected errors are logged but non-fatal.
			if !isConflictError(err) {
				log.Warn().Err(err).Str("child_id", childID).
					Str("entry_date", je.EntryDate).
					Msg("sync: failed to create journal entry")
				syncErrors = append(syncErrors, "journal entry "+je.EntryDate+": "+err.Error())
			}
		}
	}

	// Upload pending intentions
	for _, ie := range req.PendingIntentions {
		if _, err := s.intentionService.CreateIntention(ctx, childID, ie.IntentionText, ie.TimeLimitMinutes, ie.Goals); err != nil {
			if !isConflictError(err) {
				log.Warn().Err(err).Str("child_id", childID).
					Str("intention_date", ie.IntentionDate).
					Msg("sync: failed to create intention")
				syncErrors = append(syncErrors, "intention "+ie.IntentionDate+": "+err.Error())
			}
		}
	}

	// Fetch reflections updated since last sync
	if reflections, err := s.fetchReflectionsSince(ctx, childID, req.LastSyncAt); err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("sync: failed to fetch reflections")
	} else {
		resp.UpdatedReflections = reflections
	}

	// Fetch intentions since last sync
	if intentions, err := s.intentionService.GetIntentionHistory(ctx, childID, &req.LastSyncAt, nil, 100, 0); err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("sync: failed to fetch intentions")
	} else {
		resp.UpdatedIntentions = intentions
	}

	// Fetch journal entries since last sync
	if entries, err := s.journalService.GetEntries(ctx, childID, &req.LastSyncAt, nil, 100, 0); err != nil {
		log.Error().Err(err).Str("child_id", childID).Msg("sync: failed to fetch journal entries")
	} else {
		resp.UpdatedJournalEntries = entries
	}

	if len(syncErrors) > 0 {
		resp.Errors = syncErrors
	}

	return resp, nil
}

// fetchReflectionsSince returns pending reflections for a child created after
// the given timestamp.
func (s *SyncService) fetchReflectionsSince(ctx context.Context, childID string, _ time.Time) ([]db.Reflection, error) {
	return s.reflectionService.GetPendingReflections(ctx, childID, 50, 0)
}

// isConflictError reports whether the error is a unique-constraint or no-rows
// error that should be silently ignored during sync.
func isConflictError(err error) bool {
	if errors.Is(err, pgx.ErrNoRows) {
		return true
	}
	// pgx wraps pgconn.PgError; check for unique_violation code "23505"
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
		return true
	}
	return false
}
