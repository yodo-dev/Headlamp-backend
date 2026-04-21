package service

import (
	"context"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

// ParentInsightScheduler runs a nightly cron job to pre-generate parent daily
// insight digests for all parent-child pairs that have not yet received one today.
type ParentInsightScheduler struct {
	store          db.Store
	insightService *ParentInsightService
	cron           *cron.Cron
}

// NewParentInsightScheduler creates the scheduler. Call Start to activate it.
func NewParentInsightScheduler(store db.Store, insightService *ParentInsightService) *ParentInsightScheduler {
	return &ParentInsightScheduler{
		store:          store,
		insightService: insightService,
		cron:           cron.New(),
	}
}

// Start registers the nightly job and starts the cron runner.
// schedule is a standard 5-field cron expression (e.g. "0 20 * * *" for 20:00 UTC).
func (s *ParentInsightScheduler) Start(schedule string) error {
	_, err := s.cron.AddFunc(schedule, s.runParentInsights)
	if err != nil {
		return err
	}
	s.cron.Start()
	log.Info().Str("schedule", schedule).Msg("parent insight scheduler started")
	return nil
}

// Stop gracefully shuts down the cron runner.
func (s *ParentInsightScheduler) Stop() {
	s.cron.Stop()
	log.Info().Msg("parent insight scheduler stopped")
}

// runParentInsights fetches all parent-child pairs that need an insight today
// and generates one per pair concurrently.
func (s *ParentInsightScheduler) runParentInsights() {
	ctx := context.Background()

	pairs, err := s.store.GetAllChildrenForParentInsightScheduler(ctx)
	if err != nil {
		log.Error().Err(err).Msg("parent insight scheduler: failed to fetch parent-child pairs")
		return
	}

	log.Info().Int("count", len(pairs)).Msg("parent insight scheduler: generating daily insights")

	for _, pair := range pairs {
		parentID := pair.ParentID
		childID := pair.ChildID
		go func() {
			if _, err := s.insightService.GenerateDailyInsight(context.Background(), parentID, childID); err != nil {
				log.Error().Err(err).
					Str("parent_id", parentID).
					Str("child_id", childID).
					Msg("parent insight scheduler: failed to generate insight")
			}
		}()
	}
}
