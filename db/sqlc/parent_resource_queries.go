package db

import (
	"context"
	"time"
)

type ParentResourceDocument struct {
	ID          int64     `json:"id"`
	DocumentKey string    `json:"document_key"`
	Title       string    `json:"title"`
	Version     string    `json:"version"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UpsertParentResourceDocumentParams struct {
	DocumentKey string `json:"document_key"`
	Title       string `json:"title"`
	Version     string `json:"version"`
	Content     string `json:"content"`
}

const upsertParentResourceDocument = `
INSERT INTO parent_resource_documents (
  document_key,
  title,
  version,
  content
) VALUES (
  $1, $2, $3, $4
)
ON CONFLICT (document_key)
DO UPDATE SET
  title = EXCLUDED.title,
  version = EXCLUDED.version,
  content = EXCLUDED.content,
  updated_at = now()
RETURNING id, document_key, title, version, content, created_at, updated_at
`

func (store *SQLStore) UpsertParentResourceDocument(ctx context.Context, arg UpsertParentResourceDocumentParams) (ParentResourceDocument, error) {
	row := store.connPool.QueryRow(ctx, upsertParentResourceDocument, arg.DocumentKey, arg.Title, arg.Version, arg.Content)
	var out ParentResourceDocument
	err := row.Scan(
		&out.ID,
		&out.DocumentKey,
		&out.Title,
		&out.Version,
		&out.Content,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	return out, err
}

const getParentResourceDocumentByKey = `
SELECT id, document_key, title, version, content, created_at, updated_at
FROM parent_resource_documents
WHERE document_key = $1
LIMIT 1
`

func (store *SQLStore) GetParentResourceDocumentByKey(ctx context.Context, documentKey string) (ParentResourceDocument, error) {
	row := store.connPool.QueryRow(ctx, getParentResourceDocumentByKey, documentKey)
	var out ParentResourceDocument
	err := row.Scan(
		&out.ID,
		&out.DocumentKey,
		&out.Title,
		&out.Version,
		&out.Content,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	return out, err
}
