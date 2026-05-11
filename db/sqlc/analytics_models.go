package db

import (
	"time"

	"github.com/google/uuid"
)

type AnalyticsEventRecord struct {
	ID            uuid.UUID `json:"id"`
	SourceEventID string    `json:"source_event_id"`
	EventType     string    `json:"event_type"`
	EventName     string    `json:"event_name"`
	PersonID      string    `json:"person_id"`
	UserID        string    `json:"user_id"`
	Role          string    `json:"role"`
	SessionID     string    `json:"session_id"`
	ChildID       string    `json:"child_id"`
	EventTime     time.Time `json:"event_time"`
	Payload       []byte    `json:"payload"`
	SyncStatus    string    `json:"sync_status"`
	AttemptCount  int32     `json:"attempt_count"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	LastError     string    `json:"last_error"`
	SyncedAt      time.Time `json:"synced_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateAnalyticsEventParams struct {
	SourceEventID string    `json:"source_event_id"`
	EventType     string    `json:"event_type"`
	EventName     string    `json:"event_name"`
	PersonID      string    `json:"person_id"`
	UserID        string    `json:"user_id"`
	Role          string    `json:"role"`
	SessionID     string    `json:"session_id"`
	ChildID       string    `json:"child_id"`
	EventTime     time.Time `json:"event_time"`
	Payload       []byte    `json:"payload"`
}

type MarkAnalyticsEventFailedParams struct {
	ID            uuid.UUID `json:"id"`
	LastError     string    `json:"last_error"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	MaxAttempts   int32     `json:"max_attempts"`
}

type CustomerIOWebhookEventRecord struct {
	ID         uuid.UUID `json:"id"`
	EventType  string    `json:"event_type"`
	Signature  string    `json:"signature"`
	Payload    []byte    `json:"payload"`
	ReceivedAt time.Time `json:"received_at"`
}

type CreateCustomerIOWebhookEventParams struct {
	EventType string `json:"event_type"`
	Signature string `json:"signature"`
	Payload   []byte `json:"payload"`
}

type UserSegmentRecord struct {
	ID          uuid.UUID `json:"id"`
	PersonID    string    `json:"person_id"`
	UserID      string    `json:"user_id"`
	Role        string    `json:"role"`
	SegmentName string    `json:"segment_name"`
	Metadata    []byte    `json:"metadata"`
	Source      string    `json:"source"`
	AssignedAt  time.Time `json:"assigned_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UpsertUserSegmentParams struct {
	PersonID    string `json:"person_id"`
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	SegmentName string `json:"segment_name"`
	Metadata    []byte `json:"metadata"`
	Source      string `json:"source"`
}

type CustomerIOAttributionRecord struct {
	ID             uuid.UUID `json:"id"`
	WebhookEventID uuid.UUID `json:"webhook_event_id"`
	EventType      string    `json:"event_type"`
	PersonID       string    `json:"person_id"`
	CampaignID     string    `json:"campaign_id"`
	MessageID      string    `json:"message_id"`
	DeliveryID     string    `json:"delivery_id"`
	LinkID         string    `json:"link_id"`
	Action         string    `json:"action"`
	OccurredAt     time.Time `json:"occurred_at"`
	Payload        []byte    `json:"payload"`
	CreatedAt      time.Time `json:"created_at"`
}

type CreateCustomerIOAttributionParams struct {
	WebhookEventID uuid.UUID `json:"webhook_event_id"`
	EventType      string    `json:"event_type"`
	PersonID       string    `json:"person_id"`
	CampaignID     string    `json:"campaign_id"`
	MessageID      string    `json:"message_id"`
	DeliveryID     string    `json:"delivery_id"`
	LinkID         string    `json:"link_id"`
	Action         string    `json:"action"`
	OccurredAt     time.Time `json:"occurred_at"`
	Payload        []byte    `json:"payload"`
}
