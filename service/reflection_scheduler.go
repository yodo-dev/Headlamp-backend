package service

import (
	"context"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

// ReflectionScheduler runs a scheduled job (via cron) to deliver daily
// reflections to all eligible children at the configured time.
type ReflectionScheduler struct {
	store             db.Store
	reflectionService *ReflectionService
	notificationSvc   *NotificationService
	cron              *cron.Cron
	testMode          bool // when true, bypasses idempotency so every tick generates a new reflection
}

// NewReflectionScheduler creates a new scheduler. Call Start to begin scheduling.
// Set testMode=true during development to get a new reflection on every cron tick.
func NewReflectionScheduler(store db.Store, reflectionService *ReflectionService, notificationSvc *NotificationService, testMode bool) *ReflectionScheduler {
	return &ReflectionScheduler{
		store:             store,
		reflectionService: reflectionService,
		notificationSvc:   notificationSvc,
		cron:              cron.New(),
		testMode:          testMode,
	}
}

// Start registers the daily reflection job and starts the cron runner.
// schedule is a standard 5-field cron expression (e.g. "0 8 * * *" for 08:00).
func (s *ReflectionScheduler) Start(schedule string) error {
	_, err := s.cron.AddFunc(schedule, s.runDailyReflections)
	if err != nil {
		return err
	}
	s.cron.Start()
	log.Info().Str("schedule", schedule).Msg("reflection scheduler started")
	return nil
}

// Stop gracefully shuts down the cron runner.
func (s *ReflectionScheduler) Stop() {
	s.cron.Stop()
	log.Info().Msg("reflection scheduler stopped")
}

// runDailyReflections is invoked by the cron job. It fetches all children
// needing a daily reflection and generates one per child concurrently.
func (s *ReflectionScheduler) runDailyReflections() {
	ctx := context.Background()

	if s.testMode {
		// TEST MODE: fetch ALL eligible children (age 13+) regardless of whether
		// they already have a reflection today, and force-generate a new one.
		children, err := s.store.GetAllEligibleChildrenForReflection(ctx)
		if err != nil {
			log.Error().Err(err).Msg("scheduler[test]: failed to fetch eligible children")
			return
		}
		log.Info().Int("count", len(children)).Msg("scheduler[test]: forcing new reflection for all eligible children")
		for _, row := range children {
			childID := row.ChildID
			go func() {
				if _, err := s.reflectionService.GenerateDailyReflectionForced(ctx, childID); err != nil {
					log.Error().Err(err).Str("child_id", childID).Msg("scheduler[test]: failed to generate reflection")
					return
				}
				s.notifyChild(ctx, childID)
			}()
		}
		return
	}

	children, err := s.store.GetChildrenNeedingDailyReflection(ctx)
	if err != nil {
		log.Error().Err(err).Msg("scheduler: failed to fetch children needing daily reflection")
		return
	}

	log.Info().Int("count", len(children)).Msg("scheduler: running daily reflections")

	for _, row := range children {
		childID := row.ChildID
		go func() {
			if _, err := s.reflectionService.GenerateDailyReflection(ctx, childID); err != nil {
				log.Error().Err(err).Str("child_id", childID).Msg("scheduler: failed to generate daily reflection")
				return
			}
			s.notifyChild(ctx, childID)
		}()
	}
}

// notifyChild sends a push notification to a child after their daily reflection is ready.
func (s *ReflectionScheduler) notifyChild(ctx context.Context, childID string) {
	if s.notificationSvc == nil {
		return
	}
	recipientID, err := uuid.Parse(childID)
	if err != nil {
		log.Warn().Str("child_id", childID).Msg("scheduler: invalid child UUID for notification")
		return
	}
	if err := s.notificationSvc.CreateAndSend(
		ctx,
		recipientID,
		db.NotificationRecipientTypeChild,
		"Your daily reflection is ready 🌟",
		"Take a moment to reflect on your day.",
	); err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("scheduler: failed to send reflection notification")
	}
}
