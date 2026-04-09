-- name: GetCurrentBoosterForChild :one
SELECT * FROM child_weekly_modules
WHERE child_id = $1 AND week_start_date = date_trunc('week', now() at time zone 'utc');

-- name: GetNextBoosterForChild :one
SELECT * FROM child_weekly_modules
WHERE child_id = $1 AND week_start_date = date_trunc('week', now() at time zone 'utc' + interval '1 week');

-- name: GetBoostersForChildInMonth :many
SELECT
    booster_id,
    completed_at
FROM
    child_weekly_modules
WHERE
    child_id = $1
    AND completed_at >= date_trunc('month', CURRENT_DATE)
    AND completed_at < date_trunc('month', CURRENT_DATE) + interval '1 month';

-- name: GetBoosterByID :one
SELECT * FROM child_weekly_modules
WHERE booster_id = $1;

-- name: AssignBoosterToChild :one
INSERT INTO child_weekly_modules (
  child_id,
  external_module_id,
  week_start_date,
  booster_id
) VALUES (
  $1, $2, $3, $4
)
RETURNING *;

-- name: GetAssignedBoosterModuleIDsForChild :many
SELECT external_module_id FROM child_weekly_modules
WHERE child_id = $1;

-- name: CompleteBooster :one
UPDATE child_weekly_modules
SET completed_at = now()
WHERE booster_id = $1
RETURNING *;


-- name: GetChildBoosterByWeek :one
SELECT * FROM child_weekly_modules
WHERE child_id = $1 AND week_start_date = $2;


-- name: GetBoostersForChildByParent :many
SELECT
    cwm.*,
    rv.video_url AS reflection_video_url,
    rv.created_at AS reflection_submitted_at
FROM
    child_weekly_modules cwm
LEFT JOIN
    reflection_videos rv ON cwm.booster_id = rv.booster_id
WHERE
    cwm.child_id = $1
    AND cwm.week_start_date <= date_trunc('week', now() at time zone 'utc')
ORDER BY
    cwm.week_start_date DESC;


-- name: GetReflectionVideoForBooster :one
SELECT * FROM reflection_videos
WHERE booster_id = $1
LIMIT 1;


-- name: GetReflectionVideosForChild :many
SELECT
    rv.id,
    rv.child_id,
    rv.booster_id,
    rv.video_url,
    rv.strapi_asset_id,
    rv.created_at,
    cwm.external_module_id
FROM
    reflection_videos rv
JOIN
    child_weekly_modules cwm ON rv.booster_id = cwm.booster_id
WHERE
    rv.child_id = $1
ORDER BY
    rv.created_at DESC;
