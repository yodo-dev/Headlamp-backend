-- name: CreateReflectionVideo :one
INSERT INTO reflection_videos (
  child_id,
  booster_id,
  video_url,
  strapi_asset_id
) VALUES (
  $1, $2, $3, $4
)
RETURNING *;

-- name: GetReflectionVideo :one
SELECT * FROM reflection_videos
WHERE child_id = $1 AND booster_id = $2;
