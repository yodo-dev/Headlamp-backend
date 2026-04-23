// Hand-written query implementations for the social unlock flow (migration 000017).
// These follow the exact same pattern as sqlc-generated files.
// Run `sqlc generate` after running the migration to replace this file with
// a fully generated version.

package db

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
)

// ─── child_progress_gate ────────────────────────────────────────────────────

const upsertChildProgressGate = `
INSERT INTO child_progress_gate (child_id, digital_permit_test_completed_at, unlock_after_courses)
VALUES ($1, $2, $3)
ON CONFLICT (child_id) DO UPDATE SET
  digital_permit_test_completed_at = COALESCE(
    EXCLUDED.digital_permit_test_completed_at,
    child_progress_gate.digital_permit_test_completed_at
  ),
  updated_at = NOW()
RETURNING child_id, digital_permit_test_completed_at, unlock_after_courses, created_at, updated_at
`

type UpsertChildProgressGateParams struct {
	ChildID                      string             `json:"child_id"`
	DigitalPermitTestCompletedAt pgtype.Timestamptz `json:"digital_permit_test_completed_at"`
	UnlockAfterCourses           int32              `json:"unlock_after_courses"`
}

func (q *Queries) UpsertChildProgressGate(ctx context.Context, arg UpsertChildProgressGateParams) (ChildProgressGate, error) {
	row := q.db.QueryRow(ctx, upsertChildProgressGate,
		arg.ChildID,
		arg.DigitalPermitTestCompletedAt,
		arg.UnlockAfterCourses,
	)
	var i ChildProgressGate
	err := row.Scan(
		&i.ChildID,
		&i.DigitalPermitTestCompletedAt,
		&i.UnlockAfterCourses,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const getChildProgressGate = `
SELECT child_id, digital_permit_test_completed_at, unlock_after_courses, created_at, updated_at
FROM child_progress_gate WHERE child_id = $1
`

func (q *Queries) GetChildProgressGate(ctx context.Context, childID string) (ChildProgressGate, error) {
	row := q.db.QueryRow(ctx, getChildProgressGate, childID)
	var i ChildProgressGate
	err := row.Scan(
		&i.ChildID,
		&i.DigitalPermitTestCompletedAt,
		&i.UnlockAfterCourses,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

// ─── child_course_unlock ────────────────────────────────────────────────────

const getChildCourseUnlocks = `
SELECT id, child_id, course_id, course_order, status, unlocked_at, completed_at, created_at, updated_at
FROM child_course_unlock
WHERE child_id = $1
ORDER BY course_order ASC
`

func (q *Queries) GetChildCourseUnlocks(ctx context.Context, childID string) ([]ChildCourseUnlock, error) {
	rows, err := q.db.Query(ctx, getChildCourseUnlocks, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ChildCourseUnlock
	for rows.Next() {
		var i ChildCourseUnlock
		if err := rows.Scan(
			&i.ID, &i.ChildID, &i.CourseID, &i.CourseOrder,
			&i.Status, &i.UnlockedAt, &i.CompletedAt,
			&i.CreatedAt, &i.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const getChildCourseUnlockByCourse = `
SELECT id, child_id, course_id, course_order, status, unlocked_at, completed_at, created_at, updated_at
FROM child_course_unlock
WHERE child_id = $1 AND course_id = $2
`

type GetChildCourseUnlockByCourseParams struct {
	ChildID  string `json:"child_id"`
	CourseID string `json:"course_id"`
}

func (q *Queries) GetChildCourseUnlockByCourse(ctx context.Context, arg GetChildCourseUnlockByCourseParams) (ChildCourseUnlock, error) {
	row := q.db.QueryRow(ctx, getChildCourseUnlockByCourse, arg.ChildID, arg.CourseID)
	var i ChildCourseUnlock
	err := row.Scan(
		&i.ID, &i.ChildID, &i.CourseID, &i.CourseOrder,
		&i.Status, &i.UnlockedAt, &i.CompletedAt,
		&i.CreatedAt, &i.UpdatedAt,
	)
	return i, err
}

const upsertChildCourseUnlock = `
INSERT INTO child_course_unlock (child_id, course_id, course_order, status)
VALUES ($1, $2, $3, $4)
ON CONFLICT (child_id, course_id) DO NOTHING
`

type UpsertChildCourseUnlockParams struct {
	ChildID     string `json:"child_id"`
	CourseID    string `json:"course_id"`
	CourseOrder int32  `json:"course_order"`
	Status      string `json:"status"`
}

func (q *Queries) UpsertChildCourseUnlock(ctx context.Context, arg UpsertChildCourseUnlockParams) error {
	_, err := q.db.Exec(ctx, upsertChildCourseUnlock,
		arg.ChildID, arg.CourseID, arg.CourseOrder, arg.Status,
	)
	return err
}

const updateChildCourseUnlockStatus = `
UPDATE child_course_unlock SET
  status       = $3,
  unlocked_at  = CASE WHEN $3 = 'UNLOCKED'  AND unlocked_at  IS NULL THEN NOW() ELSE unlocked_at  END,
  completed_at = CASE WHEN $3 = 'COMPLETED' AND completed_at IS NULL THEN NOW() ELSE completed_at END,
  updated_at   = NOW()
WHERE child_id = $1 AND course_id = $2
RETURNING id, child_id, course_id, course_order, status, unlocked_at, completed_at, created_at, updated_at
`

type UpdateChildCourseUnlockStatusParams struct {
	ChildID  string `json:"child_id"`
	CourseID string `json:"course_id"`
	Status   string `json:"status"`
}

func (q *Queries) UpdateChildCourseUnlockStatus(ctx context.Context, arg UpdateChildCourseUnlockStatusParams) (ChildCourseUnlock, error) {
	row := q.db.QueryRow(ctx, updateChildCourseUnlockStatus, arg.ChildID, arg.CourseID, arg.Status)
	var i ChildCourseUnlock
	err := row.Scan(
		&i.ID, &i.ChildID, &i.CourseID, &i.CourseOrder,
		&i.Status, &i.UnlockedAt, &i.CompletedAt,
		&i.CreatedAt, &i.UpdatedAt,
	)
	return i, err
}

const countCompletedCoursesForChild = `
SELECT COUNT(*) FROM child_course_unlock
WHERE child_id = $1 AND status = 'COMPLETED'
`

func (q *Queries) CountCompletedCoursesForChild(ctx context.Context, childID string) (int64, error) {
	row := q.db.QueryRow(ctx, countCompletedCoursesForChild, childID)
	var count int64
	return count, row.Scan(&count)
}

const getFirstLockedCourseForChild = `
SELECT id, child_id, course_id, course_order, status, unlocked_at, completed_at, created_at, updated_at
FROM child_course_unlock
WHERE child_id = $1 AND status = 'LOCKED'
ORDER BY course_order ASC
LIMIT 1
`

func (q *Queries) GetFirstLockedCourseForChild(ctx context.Context, childID string) (ChildCourseUnlock, error) {
	row := q.db.QueryRow(ctx, getFirstLockedCourseForChild, childID)
	var i ChildCourseUnlock
	err := row.Scan(
		&i.ID, &i.ChildID, &i.CourseID, &i.CourseOrder,
		&i.Status, &i.UnlockedAt, &i.CompletedAt,
		&i.CreatedAt, &i.UpdatedAt,
	)
	return i, err
}

// ─── social_app_access ──────────────────────────────────────────────────────

const getSocialAppAccessForChild = `
SELECT
  saa.id, saa.child_id, saa.social_media_id,
  sm.name, sm.icon_url,
  saa.state, saa.eligibility_granted_at, saa.enabled_by_parent_id, saa.enabled_at,
  saa.created_at, saa.updated_at
FROM social_app_access saa
JOIN social_medias sm ON saa.social_media_id = sm.id
WHERE saa.child_id = $1
ORDER BY sm.name
`

func (q *Queries) GetSocialAppAccessForChild(ctx context.Context, childID string) ([]SocialAppAccessWithPlatform, error) {
	rows, err := q.db.Query(ctx, getSocialAppAccessForChild, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SocialAppAccessWithPlatform
	for rows.Next() {
		var i SocialAppAccessWithPlatform
		if err := rows.Scan(
			&i.ID, &i.ChildID, &i.SocialMediaID,
			&i.Name, &i.IconUrl,
			&i.State, &i.EligibilityGrantedAt, &i.EnabledByParentID, &i.EnabledAt,
			&i.CreatedAt, &i.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const getSocialAppAccessByChildAndSocialMedia = `
SELECT id, child_id, social_media_id, state, eligibility_granted_at, enabled_by_parent_id, enabled_at, created_at, updated_at
FROM social_app_access
WHERE child_id = $1 AND social_media_id = $2
`

type GetSocialAppAccessByChildAndSocialMediaParams struct {
	ChildID       string `json:"child_id"`
	SocialMediaID int64  `json:"social_media_id"`
}

func (q *Queries) GetSocialAppAccessByChildAndSocialMedia(ctx context.Context, arg GetSocialAppAccessByChildAndSocialMediaParams) (SocialAppAccess, error) {
	row := q.db.QueryRow(ctx, getSocialAppAccessByChildAndSocialMedia, arg.ChildID, arg.SocialMediaID)
	var i SocialAppAccess
	err := row.Scan(
		&i.ID, &i.ChildID, &i.SocialMediaID,
		&i.State, &i.EligibilityGrantedAt, &i.EnabledByParentID, &i.EnabledAt,
		&i.CreatedAt, &i.UpdatedAt,
	)
	return i, err
}

const upsertSocialAppAccess = `
INSERT INTO social_app_access (child_id, social_media_id, state)
VALUES ($1, $2, 'LOCKED')
ON CONFLICT (child_id, social_media_id) DO NOTHING
`

type UpsertSocialAppAccessParams struct {
	ChildID       string `json:"child_id"`
	SocialMediaID int64  `json:"social_media_id"`
}

func (q *Queries) UpsertSocialAppAccess(ctx context.Context, arg UpsertSocialAppAccessParams) error {
	_, err := q.db.Exec(ctx, upsertSocialAppAccess, arg.ChildID, arg.SocialMediaID)
	return err
}

const makeSocialAppEligible = `
UPDATE social_app_access SET
  state                  = 'ELIGIBLE_PENDING_PARENT_APPROVAL',
  eligibility_granted_at = COALESCE(eligibility_granted_at, NOW()),
  updated_at             = NOW()
WHERE child_id = $1 AND social_media_id = $2 AND state = 'LOCKED'
RETURNING id, child_id, social_media_id, state, eligibility_granted_at, enabled_by_parent_id, enabled_at, created_at, updated_at
`

type MakeSocialAppEligibleParams struct {
	ChildID       string `json:"child_id"`
	SocialMediaID int64  `json:"social_media_id"`
}

func (q *Queries) MakeSocialAppEligible(ctx context.Context, arg MakeSocialAppEligibleParams) (SocialAppAccess, error) {
	row := q.db.QueryRow(ctx, makeSocialAppEligible, arg.ChildID, arg.SocialMediaID)
	var i SocialAppAccess
	err := row.Scan(
		&i.ID, &i.ChildID, &i.SocialMediaID,
		&i.State, &i.EligibilityGrantedAt, &i.EnabledByParentID, &i.EnabledAt,
		&i.CreatedAt, &i.UpdatedAt,
	)
	return i, err
}

const enableSocialApp = `
UPDATE social_app_access SET
  state                = 'ENABLED',
  enabled_by_parent_id = $3,
  enabled_at           = COALESCE(enabled_at, NOW()),
  updated_at           = NOW()
WHERE child_id = $1 AND social_media_id = $2
RETURNING id, child_id, social_media_id, state, eligibility_granted_at, enabled_by_parent_id, enabled_at, created_at, updated_at
`

type EnableSocialAppParams struct {
	ChildID           string `json:"child_id"`
	SocialMediaID     int64  `json:"social_media_id"`
	EnabledByParentID string `json:"enabled_by_parent_id"`
}

func (q *Queries) EnableSocialApp(ctx context.Context, arg EnableSocialAppParams) (SocialAppAccess, error) {
	row := q.db.QueryRow(ctx, enableSocialApp, arg.ChildID, arg.SocialMediaID, arg.EnabledByParentID)
	var i SocialAppAccess
	err := row.Scan(
		&i.ID, &i.ChildID, &i.SocialMediaID,
		&i.State, &i.EligibilityGrantedAt, &i.EnabledByParentID, &i.EnabledAt,
		&i.CreatedAt, &i.UpdatedAt,
	)
	return i, err
}

const countNonLockedSocialAppsForChild = `
SELECT COUNT(*) FROM social_app_access
WHERE child_id = $1 AND state != 'LOCKED'
`

func (q *Queries) CountNonLockedSocialAppsForChild(ctx context.Context, childID string) (int64, error) {
	row := q.db.QueryRow(ctx, countNonLockedSocialAppsForChild, childID)
	var count int64
	return count, row.Scan(&count)
}

const getFirstLockedSocialAppForChild = `
SELECT id, child_id, social_media_id, state, eligibility_granted_at, enabled_by_parent_id, enabled_at, created_at, updated_at
FROM social_app_access
WHERE child_id = $1 AND state = 'LOCKED'
ORDER BY social_media_id ASC
LIMIT 1
`

func (q *Queries) GetFirstLockedSocialAppForChild(ctx context.Context, childID string) (SocialAppAccess, error) {
	row := q.db.QueryRow(ctx, getFirstLockedSocialAppForChild, childID)
	var i SocialAppAccess
	err := row.Scan(
		&i.ID, &i.ChildID, &i.SocialMediaID,
		&i.State, &i.EligibilityGrantedAt, &i.EnabledByParentID, &i.EnabledAt,
		&i.CreatedAt, &i.UpdatedAt,
	)
	return i, err
}

// ─── unlock_audit_events ────────────────────────────────────────────────────

const insertUnlockAuditEvent = `
INSERT INTO unlock_audit_events (child_id, event_type, metadata)
VALUES ($1, $2, $3)
`

type InsertUnlockAuditEventParams struct {
	ChildID   string          `json:"child_id"`
	EventType string          `json:"event_type"`
	Metadata  json.RawMessage `json:"metadata"`
}

func (q *Queries) InsertUnlockAuditEvent(ctx context.Context, arg InsertUnlockAuditEventParams) error {
	_, err := q.db.Exec(ctx, insertUnlockAuditEvent, arg.ChildID, arg.EventType, arg.Metadata)
	return err
}
