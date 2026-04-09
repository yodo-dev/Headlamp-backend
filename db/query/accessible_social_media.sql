-- name: SetSocialMediaAccess :one
INSERT INTO accessible_social_media (
  child_id,
  social_media_id,
  is_accessible,
  session_duration_seconds
) VALUES (
  $1, $2, $3, $4
) ON CONFLICT (child_id, social_media_id) DO UPDATE SET
  is_accessible = EXCLUDED.is_accessible,
  session_duration_seconds = EXCLUDED.session_duration_seconds,
  access_revoked_at = CASE WHEN EXCLUDED.is_accessible = false THEN now() ELSE NULL END
RETURNING *;

-- name: GetSocialMediaAccessRule :one
SELECT * FROM accessible_social_media
WHERE child_id = $1 AND social_media_id = $2
LIMIT 1;

-- name: GetAllSocialMediaPlatforms :many
SELECT id, name, icon_url FROM social_medias;

-- name: GetSocialMediaAccessStatusForChild :many
SELECT
  sm.id AS social_media_id,
  sm.name,
  sm.icon_url,
  COALESCE(asm.is_accessible, false) AS is_accessible
FROM social_medias sm
LEFT JOIN accessible_social_media asm
  ON sm.id = asm.social_media_id AND asm.child_id = $1;

-- name: GetSocialMediaAccessSettingsForParent :many
SELECT
  sm.id AS social_media_id,
  sm.name,
  sm.icon_url,
  COALESCE(asm.is_accessible, false) AS is_accessible,
  COALESCE(asm.session_duration_seconds, 3600) AS session_duration_seconds
FROM social_medias sm
LEFT JOIN accessible_social_media asm
  ON sm.id = asm.social_media_id AND asm.child_id = $1
ORDER BY sm.name;
