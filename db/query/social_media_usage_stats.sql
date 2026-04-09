-- name: LogSocialMediaUsage :one
INSERT INTO social_media_usage_stats (
  child_id,
  social_media_id,
  start_time,
  end_time,
  duration_seconds
) VALUES (
  $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetSocialMediaUsageForChild :many
SELECT
  sm.name AS platform,
  SUM(s.duration_seconds) AS total_duration
FROM social_media_usage_stats s
JOIN social_medias sm ON s.social_media_id = sm.id
WHERE s.child_id = $1 AND s.start_time >= $2
GROUP BY sm.name;
