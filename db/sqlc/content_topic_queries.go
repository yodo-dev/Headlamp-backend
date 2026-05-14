package db

import (
	"context"
	"time"
)

type ContentTopicDocument struct {
	ID        int64     `json:"id"`
	Category  string    `json:"category"`
	TopicKey  string    `json:"topic_key"`
	Title     string    `json:"title"`
	Subtitle  string    `json:"subtitle"`
	Version   string    `json:"version"`
	SortOrder int32     `json:"sort_order"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpsertContentTopicDocumentParams struct {
	Category  string `json:"category"`
	TopicKey  string `json:"topic_key"`
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle"`
	Version   string `json:"version"`
	SortOrder int32  `json:"sort_order"`
	Content   string `json:"content"`
}

const upsertContentTopicDocument = `
INSERT INTO content_topic_documents (
  category,
  topic_key,
  title,
  subtitle,
  version,
  sort_order,
  content
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (category, topic_key)
DO UPDATE SET
  title = EXCLUDED.title,
  subtitle = EXCLUDED.subtitle,
  version = EXCLUDED.version,
  sort_order = EXCLUDED.sort_order,
  content = EXCLUDED.content,
  updated_at = now()
RETURNING id, category, topic_key, title, subtitle, version, sort_order, content, created_at, updated_at
`

func (store *SQLStore) UpsertContentTopicDocument(ctx context.Context, arg UpsertContentTopicDocumentParams) (ContentTopicDocument, error) {
	row := store.connPool.QueryRow(ctx, upsertContentTopicDocument,
		arg.Category,
		arg.TopicKey,
		arg.Title,
		arg.Subtitle,
		arg.Version,
		arg.SortOrder,
		arg.Content,
	)

	var out ContentTopicDocument
	err := row.Scan(
		&out.ID,
		&out.Category,
		&out.TopicKey,
		&out.Title,
		&out.Subtitle,
		&out.Version,
		&out.SortOrder,
		&out.Content,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	return out, err
}

const listContentTopicDocumentsByCategory = `
SELECT id, category, topic_key, title, subtitle, version, sort_order, content, created_at, updated_at
FROM content_topic_documents
WHERE category = $1
ORDER BY sort_order ASC, id ASC
`

func (store *SQLStore) ListContentTopicDocumentsByCategory(ctx context.Context, category string) ([]ContentTopicDocument, error) {
	rows, err := store.connPool.Query(ctx, listContentTopicDocumentsByCategory, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ContentTopicDocument, 0)
	for rows.Next() {
		var item ContentTopicDocument
		err := rows.Scan(
			&item.ID,
			&item.Category,
			&item.TopicKey,
			&item.Title,
			&item.Subtitle,
			&item.Version,
			&item.SortOrder,
			&item.Content,
			&item.CreatedAt,
			&item.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

const getContentTopicDocumentByCategoryAndTopicKey = `
SELECT id, category, topic_key, title, subtitle, version, sort_order, content, created_at, updated_at
FROM content_topic_documents
WHERE category = $1
  AND topic_key = $2
LIMIT 1
`

func (store *SQLStore) GetContentTopicDocumentByCategoryAndTopicKey(ctx context.Context, category string, topicKey string) (ContentTopicDocument, error) {
	row := store.connPool.QueryRow(ctx, getContentTopicDocumentByCategoryAndTopicKey, category, topicKey)
	var out ContentTopicDocument
	err := row.Scan(
		&out.ID,
		&out.Category,
		&out.TopicKey,
		&out.Title,
		&out.Subtitle,
		&out.Version,
		&out.SortOrder,
		&out.Content,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	return out, err
}
