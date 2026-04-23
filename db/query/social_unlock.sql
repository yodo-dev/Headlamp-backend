-- ============================================================
-- Social Unlock Flow Queries
-- Used by the gated progression system (migration 000017)
-- ============================================================

-- name: UpsertChildProgressGate :one
INSERT INTO child_progress_gate (child_id, digital_permit_test_completed_at, unlock_after_courses)
VALUES ($1, $2, $3)
ON CONFLICT (child_id) DO UPDATE SET
  digital_permit_test_completed_at = COALESCE(
    EXCLUDED.digital_permit_test_completed_at,
    child_progress_gate.digital_permit_test_completed_at
  ),
  updated_at = NOW()
RETURNING *;

-- name: GetChildProgressGate :one
SELECT * FROM child_progress_gate WHERE child_id = $1;

-- name: GetChildCourseUnlocks :many
SELECT * FROM child_course_unlock
WHERE child_id = $1
ORDER BY course_order ASC;

-- name: GetChildCourseUnlockByCourse :one
SELECT * FROM child_course_unlock
WHERE child_id = $1 AND course_id = $2;

-- name: UpsertChildCourseUnlock :exec
INSERT INTO child_course_unlock (child_id, course_id, course_order, status)
VALUES ($1, $2, $3, $4)
ON CONFLICT (child_id, course_id) DO NOTHING;

-- name: UpdateChildCourseUnlockStatus :one
UPDATE child_course_unlock SET
  status       = $3,
  unlocked_at  = CASE WHEN $3::varchar = 'UNLOCKED'   AND unlocked_at  IS NULL THEN NOW() ELSE unlocked_at  END,
  completed_at = CASE WHEN $3::varchar = 'COMPLETED'  AND completed_at IS NULL THEN NOW() ELSE completed_at END,
  updated_at   = NOW()
WHERE child_id = $1 AND course_id = $2
RETURNING *;

-- name: CountCompletedCoursesForChild :one
SELECT COUNT(*) FROM child_course_unlock
WHERE child_id = $1 AND status = 'COMPLETED';

-- name: GetFirstLockedCourseForChild :one
SELECT * FROM child_course_unlock
WHERE child_id = $1 AND status = 'LOCKED'
ORDER BY course_order ASC
LIMIT 1;

-- name: GetSocialAppAccessForChild :many
SELECT
  saa.id,
  saa.child_id,
  saa.social_media_id,
  sm.name,
  sm.icon_url,
  saa.state,
  saa.eligibility_granted_at,
  saa.enabled_by_parent_id,
  saa.enabled_at,
  saa.created_at,
  saa.updated_at
FROM social_app_access saa
JOIN social_medias sm ON saa.social_media_id = sm.id
WHERE saa.child_id = $1
ORDER BY sm.name;

-- name: GetSocialAppAccessByChildAndSocialMedia :one
SELECT * FROM social_app_access
WHERE child_id = $1 AND social_media_id = $2;

-- name: UpsertSocialAppAccess :exec
INSERT INTO social_app_access (child_id, social_media_id, state)
VALUES ($1, $2, 'LOCKED')
ON CONFLICT (child_id, social_media_id) DO NOTHING;

-- name: MakeSocialAppEligible :one
UPDATE social_app_access SET
  state                  = 'ELIGIBLE_PENDING_PARENT_APPROVAL',
  eligibility_granted_at = COALESCE(eligibility_granted_at, NOW()),
  updated_at             = NOW()
WHERE child_id = $1 AND social_media_id = $2 AND state = 'LOCKED'
RETURNING *;

-- name: EnableSocialApp :one
UPDATE social_app_access SET
  state                = 'ENABLED',
  enabled_by_parent_id = $3,
  enabled_at           = COALESCE(enabled_at, NOW()),
  updated_at           = NOW()
WHERE child_id = $1 AND social_media_id = $2
RETURNING *;

-- name: CountNonLockedSocialAppsForChild :one
SELECT COUNT(*) FROM social_app_access
WHERE child_id = $1 AND state != 'LOCKED';

-- name: GetFirstLockedSocialAppForChild :one
SELECT * FROM social_app_access
WHERE child_id = $1 AND state = 'LOCKED'
ORDER BY social_media_id ASC
LIMIT 1;

-- name: InsertUnlockAuditEvent :exec
INSERT INTO unlock_audit_events (child_id, event_type, metadata)
VALUES ($1, $2, $3);
