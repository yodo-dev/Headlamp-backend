package service

import (
	"context"
	"errors"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

// SessionTracker manages social media session lifecycle and triggers
// post-session reflections when sessions of sufficient duration end.
type SessionTracker struct {
	store             db.Store
	reflectionService *ReflectionService
}

func NewSessionTracker(store db.Store, reflectionService *ReflectionService) *SessionTracker {
	return &SessionTracker{store: store, reflectionService: reflectionService}
}

// StartSession creates a new social media session for the child.
// Returns an error if an active session for the same app already exists.
func (s *SessionTracker) StartSession(ctx context.Context, childID string, socialMediaID int64, intentionID *uuid.UUID, categories []string) (*db.SocialMediaSession, error) {
	// Check for an already-active session for this app
	_, err := s.store.GetActiveSocialMediaSession(ctx, db.GetActiveSocialMediaSessionParams{
		ChildID:       childID,
		SocialMediaID: socialMediaID,
	})
	if err == nil {
		return nil, errors.New("an active session for this app already exists")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	intentionIDParam := pgtype.UUID{}
	if intentionID != nil {
		intentionIDParam = pgtype.UUID{Bytes: *intentionID, Valid: true}
	}

	if categories == nil {
		categories = []string{}
	}

	session, err := s.store.CreateSocialMediaSession(ctx, db.CreateSocialMediaSessionParams{
		ChildID:           childID,
		SocialMediaID:     socialMediaID,
		IntentionID:       intentionIDParam,
		SessionStart:      time.Now().UTC(),
		ContentCategories: categories,
		InteractionCount:  0,
	})
	if err != nil {
		return nil, err
	}

	return &session, nil
}

// EndSession closes a social media session. If the session lasted ≥10 minutes,
// a post-session reflection is triggered asynchronously.
func (s *SessionTracker) EndSession(ctx context.Context, sessionID uuid.UUID, categories []string, interactionCount int32) (*db.SocialMediaSession, error) {
	if categories == nil {
		categories = []string{}
	}

	session, err := s.store.EndSocialMediaSession(ctx, db.EndSocialMediaSessionParams{
		ID:      sessionID,
		Column2: categories,
		Column3: interactionCount,
	})
	if err != nil {
		return nil, err
	}

	// Trigger reflection for sessions >= 10 minutes
	if session.DurationMinutes.Valid && session.DurationMinutes.Int32 >= 10 {
		childID := session.ChildID
		go func() {
			bgCtx := context.Background()
			if _, err := s.reflectionService.GeneratePostSessionReflection(bgCtx, childID, sessionID); err != nil {
				log.Error().Err(err).
					Str("child_id", childID).
					Str("session_id", sessionID.String()).
					Msg("failed to generate post-session reflection")
			}
		}()
	}

	return &session, nil
}

// GetSession retrieves a social media session by ID.
func (s *SessionTracker) GetSession(ctx context.Context, sessionID uuid.UUID) (*db.SocialMediaSession, error) {
	session, err := s.store.GetSocialMediaSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetSessions returns paginated sessions for a child.
func (s *SessionTracker) GetSessions(ctx context.Context, childID string, limit, offset int32) ([]db.SocialMediaSession, error) {
	return s.store.GetSessionsByChild(ctx, db.GetSessionsByChildParams{
		ChildID: childID,
		Limit:   limit,
		Offset:  offset,
	})
}
