package db

import (
	"context"
	"time"
)

const redactLegacyReflectionText = `
UPDATE reflections
SET response_text = 'summary: redacted_legacy_response', updated_at = now()
WHERE response_text IS NOT NULL
  AND btrim(response_text) <> ''
  AND response_text NOT LIKE 'summary:%'
`

const redactLegacyReflectionMedia = `
UPDATE reflections
SET response_media_url = 'redacted_media_response', updated_at = now()
WHERE response_media_url IS NOT NULL
  AND btrim(response_media_url) <> ''
  AND response_media_url <> 'redacted_media_response'
`

const purgeExpiredReflectionText = `
UPDATE reflections
SET response_text = NULL, updated_at = now()
WHERE response_text IS NOT NULL
  AND responded_at IS NOT NULL
  AND responded_at < $1
`

const purgeExpiredReflectionMedia = `
UPDATE reflections
SET response_media_url = NULL, updated_at = now()
WHERE response_media_url IS NOT NULL
  AND responded_at IS NOT NULL
  AND responded_at < $1
`

// RedactLegacyReflectionResponses converts historical raw reflection payloads to
// non-identifying placeholders.
func (store *SQLStore) RedactLegacyReflectionResponses(ctx context.Context) (textRows int64, mediaRows int64, err error) {
	textTag, err := store.connPool.Exec(ctx, redactLegacyReflectionText)
	if err != nil {
		return 0, 0, err
	}

	mediaTag, err := store.connPool.Exec(ctx, redactLegacyReflectionMedia)
	if err != nil {
		return textTag.RowsAffected(), 0, err
	}

	return textTag.RowsAffected(), mediaTag.RowsAffected(), nil
}

// PurgeReflectionRawContent removes response payload fields after the retention
// window to minimize long-term content exposure.
func (store *SQLStore) PurgeReflectionRawContent(ctx context.Context, olderThan time.Time) (textRows int64, mediaRows int64, err error) {
	textTag, err := store.connPool.Exec(ctx, purgeExpiredReflectionText, olderThan)
	if err != nil {
		return 0, 0, err
	}

	mediaTag, err := store.connPool.Exec(ctx, purgeExpiredReflectionMedia, olderThan)
	if err != nil {
		return textTag.RowsAffected(), 0, err
	}

	return textTag.RowsAffected(), mediaTag.RowsAffected(), nil
}
