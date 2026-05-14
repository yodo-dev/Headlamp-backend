CREATE TABLE IF NOT EXISTS content_topic_documents (
    id BIGSERIAL PRIMARY KEY,
    category VARCHAR(64) NOT NULL,
    topic_key VARCHAR(128) NOT NULL,
    title VARCHAR(255) NOT NULL,
    subtitle VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (category, topic_key)
);

CREATE INDEX IF NOT EXISTS idx_content_topic_documents_category_sort
    ON content_topic_documents (category, sort_order, id);