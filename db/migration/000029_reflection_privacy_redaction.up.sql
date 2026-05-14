-- Redact existing raw reflection payloads into non-identifying placeholders.
UPDATE reflections
SET response_text = 'summary: redacted_legacy_response',
    updated_at = now()
WHERE response_text IS NOT NULL
  AND btrim(response_text) <> ''
  AND response_text NOT LIKE 'summary:%';

UPDATE reflections
SET response_media_url = 'redacted_media_response',
    updated_at = now()
WHERE response_media_url IS NOT NULL
  AND btrim(response_media_url) <> ''
  AND response_media_url <> 'redacted_media_response';
