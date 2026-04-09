-- name: GetWeeklyUsageSummary :many
SELECT
    sm.id AS social_media_id,
    sm.name AS social_media_name,
    sm.icon_url AS social_media_logo,
    CAST(DATE_TRUNC('day', s.start_time AT TIME ZONE 'UTC') AS TIMESTAMPTZ) AS usage_day,
    CAST(SUM(EXTRACT(EPOCH FROM (LEAST(s.end_time, NOW()) - s.start_time))) / 60 AS float8) AS total_minutes
FROM
    app_sessions s
JOIN
    social_medias sm ON s.social_media_id = sm.id
WHERE
    s.child_id = $1
    AND s.start_time >= (NOW() - INTERVAL '7 days')
GROUP BY
    sm.id, sm.name, sm.icon_url, usage_day
ORDER BY
    sm.id, usage_day;
