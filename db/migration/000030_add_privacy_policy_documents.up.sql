CREATE TABLE IF NOT EXISTS privacy_policy_documents (
  id bigserial PRIMARY KEY,
  document_key varchar NOT NULL UNIQUE,
  title varchar NOT NULL,
  version varchar NOT NULL,
  content text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_privacy_policy_documents_key ON privacy_policy_documents (document_key);
