package db

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ─── State constants ────────────────────────────────────────────────────────

const (
	SocialAppStateLocked                  = "LOCKED"
	SocialAppStateEligiblePendingApproval = "ELIGIBLE_PENDING_PARENT_APPROVAL"
	SocialAppStateEnabled                 = "ENABLED"
)

const (
	CourseStatusLocked    = "LOCKED"
	CourseStatusUnlocked  = "UNLOCKED"
	CourseStatusCompleted = "COMPLETED"
)

const (
	UnlockEventDPTCompleted           = "DIGITAL_PERMIT_TEST_COMPLETED"
	UnlockEventCourseCompleted        = "COURSE_COMPLETED"
	UnlockEventSocialSlotEligible     = "SOCIAL_SLOT_ELIGIBLE"
	UnlockEventParentEnabledSocialApp = "PARENT_ENABLED_SOCIAL_APP"
)

// ─── Struct models ──────────────────────────────────────────────────────────

// ChildProgressGate stores per-child DPT completion and unlock cadence config.
type ChildProgressGate struct {
	ChildID                      string             `json:"child_id"`
	DigitalPermitTestCompletedAt pgtype.Timestamptz `json:"digital_permit_test_completed_at"`
	UnlockAfterCourses           int32              `json:"unlock_after_courses"`
	CreatedAt                    time.Time          `json:"created_at"`
	UpdatedAt                    time.Time          `json:"updated_at"`
}

// ChildCourseUnlock tracks lock/unlock/complete state for one course per child.
type ChildCourseUnlock struct {
	ID          int64              `json:"id"`
	ChildID     string             `json:"child_id"`
	CourseID    string             `json:"course_id"`
	CourseOrder int32              `json:"course_order"`
	Status      string             `json:"status"`
	UnlockedAt  pgtype.Timestamptz `json:"unlocked_at"`
	CompletedAt pgtype.Timestamptz `json:"completed_at"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// SocialAppAccess tracks the gated state machine per child per social platform.
type SocialAppAccess struct {
	ID                   int64              `json:"id"`
	ChildID              string             `json:"child_id"`
	SocialMediaID        int64              `json:"social_media_id"`
	State                string             `json:"state"`
	EligibilityGrantedAt pgtype.Timestamptz `json:"eligibility_granted_at"`
	EnabledByParentID    pgtype.Text        `json:"enabled_by_parent_id"`
	EnabledAt            pgtype.Timestamptz `json:"enabled_at"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

// SocialAppAccessWithPlatform joins social_app_access with social_medias name/icon.
type SocialAppAccessWithPlatform struct {
	ID                   int64              `json:"id"`
	ChildID              string             `json:"child_id"`
	SocialMediaID        int64              `json:"social_media_id"`
	Name                 string             `json:"name"`
	IconUrl              pgtype.Text        `json:"icon_url"`
	State                string             `json:"state"`
	EligibilityGrantedAt pgtype.Timestamptz `json:"eligibility_granted_at"`
	EnabledByParentID    pgtype.Text        `json:"enabled_by_parent_id"`
	EnabledAt            pgtype.Timestamptz `json:"enabled_at"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

// UnlockAuditEvent is an immutable audit record for a progression event.
type UnlockAuditEvent struct {
	ID        int64     `json:"id"`
	ChildID   string    `json:"child_id"`
	EventType string    `json:"event_type"`
	Metadata  []byte    `json:"metadata"`
	CreatedAt time.Time `json:"created_at"`
}
