package service

import (
	"context"
	"time"

	firebaseMessaging "firebase.google.com/go/v4/messaging"
	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

// NotificationService handles push notifications and in-app notification records.
type NotificationService struct {
	store     db.Store
	messaging *firebaseMessaging.Client
}

// NewNotificationService creates a NotificationService. messagingClient may be nil
// in dev environments where Firebase is not configured — all methods are safe to
// call and become no-ops in that case.
func NewNotificationService(store db.Store, messagingClient *firebaseMessaging.Client) *NotificationService {
	return &NotificationService{
		store:     store,
		messaging: messagingClient,
	}
}

// CreateAndSend persists a notification record to the database and sends a push
// notification to all registered devices for the recipient.
func (s *NotificationService) CreateAndSend(
	ctx context.Context,
	recipientID uuid.UUID,
	recipientType db.NotificationRecipientType,
	title, body string,
) error {
	now := time.Now()
	if _, err := s.store.CreateNotification(ctx, db.CreateNotificationParams{
		RecipientID:   recipientID,
		RecipientType: recipientType,
		Title:         title,
		Message:       body,
		SentAt:        pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		log.Error().Err(err).
			Str("recipient_id", recipientID.String()).
			Msg("notification: failed to persist record")
		// continue — still attempt push delivery
	}

	return s.SendPush(ctx, recipientID, title, body)
}

// SendPush sends a push notification to all active devices for recipientID without
// writing a notification record to the database.
func (s *NotificationService) SendPush(ctx context.Context, recipientID uuid.UUID, title, body string) error {
	if s.messaging == nil {
		return nil // Firebase not configured (dev / test mode)
	}

	rawTokens, err := s.store.ListPushTokensForUser(ctx, recipientID)
	if err != nil {
		return err
	}

	var tokens []string
	for _, t := range rawTokens {
		if t.Valid && t.String != "" {
			tokens = append(tokens, t.String)
		}
	}

	if len(tokens) == 0 {
		return nil
	}

	return s.sendToTokens(ctx, tokens, title, body)
}

// sendToTokens delivers an FCM multicast message to the given tokens and logs
// failures. Invalid tokens are reported but not cleaned up here — token lifecycle
// management should be handled separately via the device registration flow.
func (s *NotificationService) sendToTokens(ctx context.Context, tokens []string, title, body string) error {
	msg := &firebaseMessaging.MulticastMessage{
		Tokens: tokens,
		Notification: &firebaseMessaging.Notification{
			Title: title,
			Body:  body,
		},
	}

	resp, err := s.messaging.SendEachForMulticast(ctx, msg)
	if err != nil {
		return err
	}

	if resp.FailureCount > 0 {
		for i, r := range resp.Responses {
			if !r.Success {
				log.Warn().
					Str("token_prefix", safeTokenPrefix(tokens[i])).
					Str("error", r.Error.Error()).
					Msg("notification: FCM delivery failed for token")
			}
		}
	}

	log.Debug().
		Int("sent", resp.SuccessCount).
		Int("failed", resp.FailureCount).
		Msg("notification: FCM multicast complete")

	return nil
}

// safeTokenPrefix returns the first 8 characters of a token for safe logging.
func safeTokenPrefix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8] + "..."
}
