package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const createAnalyticsEvent = `
INSERT INTO analytics_events (
  source_event_id,
  event_type,
  event_name,
  person_id,
  user_id,
  role,
  session_id,
  child_id,
  event_time,
  payload
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING
  id,
  COALESCE(source_event_id, ''),
  event_type,
  event_name,
  person_id,
  user_id,
  role,
  COALESCE(session_id, ''),
  COALESCE(child_id, ''),
  event_time,
  payload,
  sync_status,
  attempt_count,
  next_attempt_at,
  COALESCE(last_error, ''),
  COALESCE(synced_at, '0001-01-01 00:00:00+00'::timestamptz),
  created_at,
  updated_at
`

func nullIfEmpty(value string) sql.NullString {
	trimmed := value
	if trimmed == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: trimmed, Valid: true}
}

func scanAnalyticsEvent(row pgx.Row, out *AnalyticsEventRecord) error {
	return row.Scan(
		&out.ID,
		&out.SourceEventID,
		&out.EventType,
		&out.EventName,
		&out.PersonID,
		&out.UserID,
		&out.Role,
		&out.SessionID,
		&out.ChildID,
		&out.EventTime,
		&out.Payload,
		&out.SyncStatus,
		&out.AttemptCount,
		&out.NextAttemptAt,
		&out.LastError,
		&out.SyncedAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
}

func (store *SQLStore) CreateAnalyticsEvent(ctx context.Context, arg CreateAnalyticsEventParams) (AnalyticsEventRecord, error) {
	row := store.connPool.QueryRow(
		ctx,
		createAnalyticsEvent,
		nullIfEmpty(arg.SourceEventID),
		arg.EventType,
		arg.EventName,
		arg.PersonID,
		arg.UserID,
		arg.Role,
		nullIfEmpty(arg.SessionID),
		nullIfEmpty(arg.ChildID),
		arg.EventTime,
		arg.Payload,
	)
	var out AnalyticsEventRecord
	err := scanAnalyticsEvent(row, &out)
	return out, err
}

const getAnalyticsEventBySourceEventID = `
SELECT
  id,
  COALESCE(source_event_id, ''),
  event_type,
  event_name,
  person_id,
  user_id,
  role,
  COALESCE(session_id, ''),
  COALESCE(child_id, ''),
  event_time,
  payload,
  sync_status,
  attempt_count,
  next_attempt_at,
  COALESCE(last_error, ''),
  COALESCE(synced_at, '0001-01-01 00:00:00+00'::timestamptz),
  created_at,
  updated_at
FROM analytics_events
WHERE source_event_id = $1
LIMIT 1
`

func (store *SQLStore) GetAnalyticsEventBySourceEventID(ctx context.Context, sourceEventID string) (AnalyticsEventRecord, error) {
	row := store.connPool.QueryRow(ctx, getAnalyticsEventBySourceEventID, sourceEventID)
	var out AnalyticsEventRecord
	err := scanAnalyticsEvent(row, &out)
	return out, err
}

const listPendingAnalyticsEvents = `
SELECT
  id,
  COALESCE(source_event_id, ''),
  event_type,
  event_name,
  person_id,
  user_id,
  role,
  COALESCE(session_id, ''),
  COALESCE(child_id, ''),
  event_time,
  payload,
  sync_status,
  attempt_count,
  next_attempt_at,
  COALESCE(last_error, ''),
  COALESCE(synced_at, '0001-01-01 00:00:00+00'::timestamptz),
  created_at,
  updated_at
FROM analytics_events
WHERE sync_status = 'pending' AND next_attempt_at <= now()
ORDER BY created_at ASC
LIMIT $1
`

func (store *SQLStore) ListPendingAnalyticsEvents(ctx context.Context, limit int32) ([]AnalyticsEventRecord, error) {
	rows, err := store.connPool.Query(ctx, listPendingAnalyticsEvents, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AnalyticsEventRecord, 0)
	for rows.Next() {
		var item AnalyticsEventRecord
		if err := rows.Scan(
			&item.ID,
			&item.SourceEventID,
			&item.EventType,
			&item.EventName,
			&item.PersonID,
			&item.UserID,
			&item.Role,
			&item.SessionID,
			&item.ChildID,
			&item.EventTime,
			&item.Payload,
			&item.SyncStatus,
			&item.AttemptCount,
			&item.NextAttemptAt,
			&item.LastError,
			&item.SyncedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

const markAnalyticsEventSynced = `
UPDATE analytics_events
SET sync_status = 'synced', synced_at = now(), last_error = NULL, updated_at = now()
WHERE id = $1
`

func (store *SQLStore) MarkAnalyticsEventSynced(ctx context.Context, id uuid.UUID) error {
	_, err := store.connPool.Exec(ctx, markAnalyticsEventSynced, id)
	return err
}

const markAnalyticsEventFailed = `
UPDATE analytics_events
SET
  attempt_count = attempt_count + 1,
  last_error = $2,
  next_attempt_at = $3,
  sync_status = CASE WHEN attempt_count + 1 >= $4 THEN 'dead' ELSE 'pending' END,
  updated_at = now()
WHERE id = $1
`

func (store *SQLStore) MarkAnalyticsEventFailed(ctx context.Context, arg MarkAnalyticsEventFailedParams) error {
	_, err := store.connPool.Exec(ctx, markAnalyticsEventFailed, arg.ID, arg.LastError, arg.NextAttemptAt, arg.MaxAttempts)
	return err
}

const createCustomerIOWebhookEvent = `
INSERT INTO customerio_webhook_events (event_type, signature, payload)
VALUES ($1, $2, $3)
RETURNING id, event_type, COALESCE(signature, ''), payload, received_at
`

func (store *SQLStore) CreateCustomerIOWebhookEvent(ctx context.Context, arg CreateCustomerIOWebhookEventParams) (CustomerIOWebhookEventRecord, error) {
	row := store.connPool.QueryRow(ctx, createCustomerIOWebhookEvent, arg.EventType, nullIfEmpty(arg.Signature), arg.Payload)
	var out CustomerIOWebhookEventRecord
	err := row.Scan(&out.ID, &out.EventType, &out.Signature, &out.Payload, &out.ReceivedAt)
	return out, err
}

const upsertUserSegment = `
INSERT INTO user_segments (
  person_id,
  user_id,
  role,
  segment_name,
  metadata,
  source,
  assigned_at,
  expires_at,
  updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, now(), NULL, now())
ON CONFLICT (person_id, segment_name)
DO UPDATE SET
  user_id = EXCLUDED.user_id,
  role = EXCLUDED.role,
  metadata = EXCLUDED.metadata,
  source = EXCLUDED.source,
  assigned_at = now(),
  expires_at = NULL,
  updated_at = now()
RETURNING
  id,
  person_id,
  user_id,
  role,
  segment_name,
  metadata,
  source,
  assigned_at,
  COALESCE(expires_at, '0001-01-01 00:00:00+00'::timestamptz),
  created_at,
  updated_at
`

func (store *SQLStore) UpsertUserSegment(ctx context.Context, arg UpsertUserSegmentParams) (UserSegmentRecord, error) {
	if len(arg.Metadata) == 0 {
		arg.Metadata = []byte(`{}`)
	}
	row := store.connPool.QueryRow(
		ctx,
		upsertUserSegment,
		arg.PersonID,
		arg.UserID,
		arg.Role,
		arg.SegmentName,
		arg.Metadata,
		defaultIfEmpty(arg.Source, "system"),
	)
	var out UserSegmentRecord
	err := row.Scan(
		&out.ID,
		&out.PersonID,
		&out.UserID,
		&out.Role,
		&out.SegmentName,
		&out.Metadata,
		&out.Source,
		&out.AssignedAt,
		&out.ExpiresAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	return out, err
}

const listActiveUserSegments = `
SELECT
  id,
  person_id,
  user_id,
  role,
  segment_name,
  metadata,
  source,
  assigned_at,
  COALESCE(expires_at, '0001-01-01 00:00:00+00'::timestamptz),
  created_at,
  updated_at
FROM user_segments
WHERE person_id = $1
  AND (expires_at IS NULL OR expires_at > now())
ORDER BY assigned_at DESC
`

func (store *SQLStore) ListActiveUserSegments(ctx context.Context, personID string) ([]UserSegmentRecord, error) {
	rows, err := store.connPool.Query(ctx, listActiveUserSegments, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]UserSegmentRecord, 0)
	for rows.Next() {
		var item UserSegmentRecord
		if err := rows.Scan(
			&item.ID,
			&item.PersonID,
			&item.UserID,
			&item.Role,
			&item.SegmentName,
			&item.Metadata,
			&item.Source,
			&item.AssignedAt,
			&item.ExpiresAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

const expireUserSegment = `
UPDATE user_segments
SET expires_at = now(), updated_at = now()
WHERE person_id = $1
  AND segment_name = $2
  AND (expires_at IS NULL OR expires_at > now())
`

func (store *SQLStore) ExpireUserSegment(ctx context.Context, personID, segmentName string) error {
	_, err := store.connPool.Exec(ctx, expireUserSegment, personID, segmentName)
	return err
}

const createCustomerIOAttribution = `
INSERT INTO customerio_event_attributions (
  webhook_event_id,
  event_type,
  person_id,
  campaign_id,
  message_id,
  delivery_id,
  link_id,
  action,
  occurred_at,
  payload
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING
  id,
  COALESCE(webhook_event_id, '00000000-0000-0000-0000-000000000000'::uuid),
  event_type,
  COALESCE(person_id, ''),
  COALESCE(campaign_id, ''),
  COALESCE(message_id, ''),
  COALESCE(delivery_id, ''),
  COALESCE(link_id, ''),
  COALESCE(action, ''),
  occurred_at,
  payload,
  created_at
`

func (store *SQLStore) CreateCustomerIOAttribution(ctx context.Context, arg CreateCustomerIOAttributionParams) (CustomerIOAttributionRecord, error) {
	if arg.OccurredAt.IsZero() {
		arg.OccurredAt = time.Now().UTC()
	}
	if len(arg.Payload) == 0 {
		arg.Payload = []byte(`{}`)
	}
	queryWebhook := any(nil)
	if arg.WebhookEventID != uuid.Nil {
		queryWebhook = arg.WebhookEventID
	}

	row := store.connPool.QueryRow(
		ctx,
		createCustomerIOAttribution,
		queryWebhook,
		arg.EventType,
		nullIfEmpty(arg.PersonID),
		nullIfEmpty(arg.CampaignID),
		nullIfEmpty(arg.MessageID),
		nullIfEmpty(arg.DeliveryID),
		nullIfEmpty(arg.LinkID),
		nullIfEmpty(arg.Action),
		arg.OccurredAt,
		arg.Payload,
	)

	var out CustomerIOAttributionRecord
	err := row.Scan(
		&out.ID,
		&out.WebhookEventID,
		&out.EventType,
		&out.PersonID,
		&out.CampaignID,
		&out.MessageID,
		&out.DeliveryID,
		&out.LinkID,
		&out.Action,
		&out.OccurredAt,
		&out.Payload,
		&out.CreatedAt,
	)
	return out, err
}

func defaultIfEmpty(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
