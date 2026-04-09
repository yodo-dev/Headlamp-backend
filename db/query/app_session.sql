-- name: CreateAppSession :one
INSERT INTO app_sessions (
    child_id,
    social_media_id,
    start_time,
    expected_end_time
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: GetActiveSessionByChildAndApp :one
SELECT * FROM app_sessions
WHERE child_id = $1 AND social_media_id = $2 AND status = 'active'
ORDER BY start_time DESC
LIMIT 1;

-- name: GetExpiredActiveSessions :many
SELECT * FROM app_sessions
WHERE status = 'active' AND expected_end_time <= NOW();

-- name: GetTodayExpiredSessionByChildAndApp :one
-- Returns the most recent expired/ended session for a child+app that started
-- today (UTC). Used to enforce the one-session-per-day rule.
SELECT * FROM app_sessions
WHERE child_id = $1
  AND social_media_id = $2
  AND status IN ('expired', 'ended')
  AND start_time >= DATE_TRUNC('day', NOW() AT TIME ZONE 'UTC')
  AND start_time <  DATE_TRUNC('day', NOW() AT TIME ZONE 'UTC') + INTERVAL '1 day'
ORDER BY start_time DESC
LIMIT 1;

-- name: MarkSessionExpired :one
UPDATE app_sessions
SET status = 'expired', end_time = expected_end_time, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: MarkSessionEnded :one
UPDATE app_sessions
SET status = 'ended', end_time = NOW(), updated_at = NOW()
WHERE child_id = $1 AND social_media_id = $2 AND status = 'active'
RETURNING *;

-- name: EndAppSession :one
UPDATE app_sessions
SET
    end_time = $2,
    status = 'closed',
    updated_at = NOW()
WHERE
    id = $1 AND child_id = $3
RETURNING *;

-- name: GetAppSessionByID :one
SELECT * FROM app_sessions
WHERE id = $1 LIMIT 1;

-- name: TimeoutStaleSessions :many
UPDATE app_sessions
SET
    end_time = expected_end_time,
    status = 'timed_out',
    updated_at = NOW()
WHERE
    status = 'active' AND expected_end_time <= NOW()
RETURNING *;

-- name: GetUsageSummaryForDate :many
SELECT
    sm.id as social_media_id,
    sm.name as social_media_name,
    sm.icon_url as social_media_logo,
    CAST(SUM(
        EXTRACT(EPOCH FROM (LEAST(COALESCE(s.end_time, NOW()), $3) - s.start_time))
    ) / 60.0 AS float) AS total_minutes
FROM
    app_sessions s
JOIN
    social_medias sm ON s.social_media_id = sm.id
WHERE
    s.child_id = $1
    AND s.start_time >= $2
    AND s.start_time < $3
GROUP BY
    sm.id, sm.name, sm.icon_url
ORDER BY
    total_minutes DESC;

-- name: GetDailyUsageForWeek :many
SELECT
    DATE_TRUNC('day', start_time)::date as usage_date,
    CAST(SUM(
        EXTRACT(EPOCH FROM (LEAST(COALESCE(end_time, last_ping_time, start_time), start_time + INTERVAL '1 day') - start_time))
    ) / 60.0 AS float) AS total_minutes
FROM
    app_sessions
WHERE
    child_id = $1
    AND social_media_id = $2
    AND start_time >= $3
    AND start_time < $4
GROUP BY
    usage_date
ORDER BY
    usage_date;

-- name: GetActiveAppSession :one
SELECT * FROM app_sessions
WHERE child_id = $1 AND social_media_id = $2 AND end_time IS NULL
ORDER BY start_time DESC
LIMIT 1;

-- name: UpdateSessionPing :one
UPDATE app_sessions
SET last_ping_time = NOW()
WHERE id = $1
RETURNING *;

-- name: GetActiveSessionForChild :one
SELECT * FROM app_sessions
WHERE child_id = $1 AND end_time IS NULL
ORDER BY start_time DESC
LIMIT 1;

-- name: CloseSessionsForChild :many
UPDATE app_sessions
SET end_time = last_ping_time
WHERE child_id = $1 AND end_time IS NULL
RETURNING *;

-- name: CloseStaleSessions :many
UPDATE app_sessions
SET end_time = last_ping_time
WHERE end_time IS NULL AND last_ping_time < NOW() - INTERVAL '5 minutes'
RETURNING *;
