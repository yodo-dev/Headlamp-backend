-- name: CreateJournalEntry :one
INSERT INTO journal_entries (
  child_id,
  entry_date,
  entry_text,
  mood,
  tags,
  media_urls
) VALUES (
  $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetJournalEntry :one
SELECT * FROM journal_entries WHERE id = $1 LIMIT 1;

-- name: GetJournalEntryByDate :one
SELECT * FROM journal_entries
WHERE child_id = $1 AND entry_date = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateJournalEntry :one
UPDATE journal_entries
SET
  entry_text = $2,
  mood       = $3,
  tags       = $4,
  updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteJournalEntry :exec
DELETE FROM journal_entries WHERE id = $1;

-- name: GetJournalEntriesForChild :many
SELECT * FROM journal_entries
WHERE child_id = $1
  AND ($2::date IS NULL OR entry_date >= $2)
  AND ($3::date IS NULL OR entry_date <= $3)
ORDER BY entry_date DESC, created_at DESC
LIMIT $4 OFFSET $5;

-- name: GetJournalStats :one
SELECT
  COUNT(*)                            AS total_entries,
  COUNT(DISTINCT entry_date)          AS active_days,
  COUNT(CASE WHEN mood IS NOT NULL THEN 1 END) AS entries_with_mood,
  MIN(entry_date)                     AS first_entry_date,
  MAX(entry_date)                     AS last_entry_date
FROM journal_entries
WHERE child_id = $1;
