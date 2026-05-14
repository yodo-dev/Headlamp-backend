package db

import (
	"context"
	"time"
)

type PrivacyPolicyDocument struct {
	ID          int64     `json:"id"`
	DocumentKey string    `json:"document_key"`
	Title       string    `json:"title"`
	Version     string    `json:"version"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UpsertPrivacyPolicyDocumentParams struct {
	DocumentKey string `json:"document_key"`
	Title       string `json:"title"`
	Version     string `json:"version"`
	Content     string `json:"content"`
}

const upsertPrivacyPolicyDocument = `
INSERT INTO privacy_policy_documents (
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

func (store *SQLStore) UpsertPrivacyPolicyDocument(ctx context.Context, arg UpsertPrivacyPolicyDocumentParams) (PrivacyPolicyDocument, error) {
	row := store.connPool.QueryRow(ctx, upsertPrivacyPolicyDocument, arg.DocumentKey, arg.Title, arg.Version, arg.Content)
	var out PrivacyPolicyDocument
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

const getPrivacyPolicyDocumentByKey = `
SELECT id, document_key, title, version, content, created_at, updated_at
FROM privacy_policy_documents
WHERE document_key = $1
LIMIT 1
`

func (store *SQLStore) GetPrivacyPolicyDocumentByKey(ctx context.Context, documentKey string) (PrivacyPolicyDocument, error) {
	row := store.connPool.QueryRow(ctx, getPrivacyPolicyDocumentByKey, documentKey)
	var out PrivacyPolicyDocument
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
