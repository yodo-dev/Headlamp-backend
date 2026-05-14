package db

import (
	"context"
	"time"
)

// ReflectionContent represents the isolated content storage for reflections
type ReflectionContent struct {
	ContentHash     string    `json:"content_hash"`
	PromptContent   string    `json:"prompt_content"`
	ResponseText    *string   `json:"response_text"`
	ResponseMediaUrl *string  `json:"response_media_url"`
	CreatedAt       time.Time `json:"created_at"`
}

type StoreReflectionContentParams struct {
	ContentHash      string
	PromptContent    string
	ResponseText     *string
	ResponseMediaUrl *string
}

const storeReflectionContent = `
INSERT INTO reflection_content (
  content_hash,
  prompt_content,
  response_text,
  response_media_url
) VALUES (
  $1, $2, $3, $4
)
ON CONFLICT (content_hash) DO NOTHING
`

func (store *SQLStore) StoreReflectionContent(ctx context.Context, arg StoreReflectionContentParams) error {
	_, err := store.connPool.Exec(ctx, storeReflectionContent,
		arg.ContentHash,
		arg.PromptContent,
		arg.ResponseText,
		arg.ResponseMediaUrl,
	)
	return err
}

const getReflectionContentByHash = `
SELECT content_hash, prompt_content, response_text, response_media_url, created_at
FROM reflection_content
WHERE content_hash = $1
LIMIT 1
`

func (store *SQLStore) GetReflectionContentByHash(ctx context.Context, contentHash string) (ReflectionContent, error) {
	row := store.connPool.QueryRow(ctx, getReflectionContentByHash, contentHash)
	var out ReflectionContent
	err := row.Scan(
		&out.ContentHash,
		&out.PromptContent,
		&out.ResponseText,
		&out.ResponseMediaUrl,
		&out.CreatedAt,
	)
	return out, err
}

const purgeExpiredReflectionContent = `
DELETE FROM reflection_content
WHERE content_hash IN (
  SELECT r.content_hash
  FROM reflections r
  WHERE r.retention_expires_at IS NOT NULL
    AND r.retention_expires_at < now()
)
`

func (store *SQLStore) PurgeExpiredReflectionContent(ctx context.Context) (int64, error) {
	result, err := store.connPool.Exec(ctx, purgeExpiredReflectionContent)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}
