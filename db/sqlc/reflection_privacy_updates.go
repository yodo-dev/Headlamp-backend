package db

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type UpdateReflectionWithPrivacyParams struct {
	ID                 uuid.UUID
	ContentHash        string
	ResponseSummary    pgtype.Text
	RetentionExpiresAt pgtype.Timestamptz
}

const updateReflectionWithPrivacy = `
UPDATE reflections
SET
  content_hash = $2,
  response_summary = $3,
  retention_expires_at = $4,
  updated_at = now()
WHERE id = $1
`

func (store *SQLStore) UpdateReflectionWithPrivacy(ctx context.Context, arg UpdateReflectionWithPrivacyParams) error {
	_, err := store.connPool.Exec(ctx, updateReflectionWithPrivacy,
		arg.ID,
		arg.ContentHash,
		arg.ResponseSummary,
		arg.RetentionExpiresAt,
	)
	return err
}

const purgeExpiredReflectionsByRetention = `
DELETE FROM reflection_content
WHERE content_hash IN (
  SELECT r.content_hash
  FROM reflections r
  WHERE r.retention_expires_at IS NOT NULL
    AND r.retention_expires_at < $1
    AND r.content_hash IS NOT NULL
)
`

func (store *SQLStore) PurgeReflectionContentByRetention(ctx context.Context, olderThan time.Time) (int64, error) {
	result, err := store.connPool.Exec(ctx, purgeExpiredReflectionsByRetention, olderThan)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

const getReflectionWithContent = `
SELECT
  r.id,
  r.child_id,
  r.trigger_type,
  r.trigger_event_id,
  r.prompt_content,
  r.response_text,
  r.response_media_url,
  r.response_type,
  r.responded_at,
  r.is_acknowledged,
  r.acknowledgment_feedback,
  r.delivered_at,
  r.metadata,
  r.content_hash,
  r.response_summary,
  r.retention_expires_at,
  r.created_at,
  r.updated_at
FROM reflections r
WHERE r.id = $1
LIMIT 1
`

type ReflectionWithPrivacy struct {
	ID                 uuid.UUID
	ChildID            string
	TriggerType        ReflectionTriggerType
	TriggerEventID     pgtype.UUID
	PromptContent      []byte
	ResponseText       pgtype.Text
	ResponseMediaUrl   pgtype.Text
	ResponseType       pgtype.Text
	RespondedAt        pgtype.Timestamptz
	IsAcknowledged     bool
	AcknowledgmentFeedback pgtype.Text
	DeliveredAt        time.Time
	Metadata           []byte
	ContentHash        pgtype.Text
	ResponseSummary    pgtype.Text
	RetentionExpiresAt pgtype.Timestamptz
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (store *SQLStore) GetReflectionWithContent(ctx context.Context, id uuid.UUID) (ReflectionWithPrivacy, error) {
	row := store.connPool.QueryRow(ctx, getReflectionWithContent, id)
	var r ReflectionWithPrivacy
	err := row.Scan(
		&r.ID,
		&r.ChildID,
		&r.TriggerType,
		&r.TriggerEventID,
		&r.PromptContent,
		&r.ResponseText,
		&r.ResponseMediaUrl,
		&r.ResponseType,
		&r.RespondedAt,
		&r.IsAcknowledged,
		&r.AcknowledgmentFeedback,
		&r.DeliveredAt,
		&r.Metadata,
		&r.ContentHash,
		&r.ResponseSummary,
		&r.RetentionExpiresAt,
		&r.CreatedAt,
		&r.UpdatedAt,
	)
	return r, err
}
