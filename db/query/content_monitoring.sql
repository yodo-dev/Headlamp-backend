-- name: CreateContentMonitoringEvent :one
INSERT INTO content_monitoring_events (child_id, platform, category, severity, event_timestamp, metadata)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, child_id, platform, category, severity, event_timestamp, metadata, created_at;

-- name: GetContentMonitoringEventsForChild :many
SELECT id, child_id, platform, category, severity, event_timestamp, metadata, created_at
FROM content_monitoring_events
WHERE child_id = $1
  AND event_timestamp >= $2
ORDER BY event_timestamp DESC;

-- name: GetLatestContentMonitoringAlert :one
SELECT id, child_id, platform, category, severity, event_timestamp, metadata, created_at
FROM content_monitoring_events
WHERE child_id = $1
ORDER BY event_timestamp DESC
LIMIT 1;

-- name: GetContentMonitoringCountsByCategoryAndSeverity :many
SELECT
  category,
  severity,
  COUNT(*)::bigint AS event_count
FROM content_monitoring_events
WHERE child_id = $1
  AND event_timestamp >= $2
GROUP BY category, severity
ORDER BY event_count DESC;

-- name: GetTopRiskyPlatforms :many
SELECT
  platform,
  COUNT(*)::bigint AS event_count
FROM content_monitoring_events
WHERE child_id = $1
  AND event_timestamp >= $2
  AND severity IN ('medium', 'high')
GROUP BY platform
ORDER BY event_count DESC
LIMIT 5;
