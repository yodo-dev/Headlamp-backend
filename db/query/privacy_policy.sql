-- name: UpsertPrivacyPolicyDocument :one
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
RETURNING id, document_key, title, version, content, created_at, updated_at;

-- name: GetPrivacyPolicyDocumentByKey :one
SELECT id, document_key, title, version, content, created_at, updated_at
FROM privacy_policy_documents
WHERE document_key = $1
LIMIT 1;
