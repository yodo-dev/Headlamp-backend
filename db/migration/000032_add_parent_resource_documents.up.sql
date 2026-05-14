CREATE TABLE IF NOT EXISTS parent_resource_documents (
  id bigserial PRIMARY KEY,
  document_key varchar NOT NULL UNIQUE,
  title varchar NOT NULL,
  version varchar NOT NULL,
  content text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_parent_resource_documents_key ON parent_resource_documents (document_key);