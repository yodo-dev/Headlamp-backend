package db

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// PasswordResetOtp represents a row in the password_reset_otps table.
type PasswordResetOtp struct {
	ID         uuid.UUID          `json:"id"`
	Email      string             `json:"email"`
	OtpHash    string             `json:"-"`
	ResetToken pgtype.UUID        `json:"reset_token"`
	ExpiresAt  time.Time          `json:"expires_at"`
	VerifiedAt pgtype.Timestamptz `json:"verified_at"`
	UsedAt     pgtype.Timestamptz `json:"used_at"`
	CreatedAt  time.Time          `json:"created_at"`
}

// ── SQL statements ────────────────────────────────────────────────────────────

const createPasswordResetOTP = `
INSERT INTO password_reset_otps (email, otp_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id, email, otp_hash, reset_token, expires_at, verified_at, used_at, created_at
`

const getLatestValidOTPByEmail = `
SELECT id, email, otp_hash, reset_token, expires_at, verified_at, used_at, created_at
FROM password_reset_otps
WHERE email = $1
  AND expires_at > NOW()
  AND verified_at IS NULL
  AND used_at IS NULL
ORDER BY created_at DESC
LIMIT 1
`

const getOTPByResetToken = `
SELECT id, email, otp_hash, reset_token, expires_at, verified_at, used_at, created_at
FROM password_reset_otps
WHERE reset_token = $1
  AND expires_at > NOW()
  AND verified_at IS NOT NULL
  AND used_at IS NULL
`

const markOTPVerified = `
UPDATE password_reset_otps
SET verified_at = NOW(), reset_token = gen_random_uuid()
WHERE id = $1
RETURNING id, email, otp_hash, reset_token, expires_at, verified_at, used_at, created_at
`

const markOTPUsed = `
UPDATE password_reset_otps
SET used_at = NOW()
WHERE id = $1
`

const invalidateOTPsByEmail = `
UPDATE password_reset_otps
SET used_at = NOW()
WHERE email = $1 AND used_at IS NULL
`

const updateParentPassword = `
UPDATE parents
SET hashed_password = $1, updated_at = NOW()
WHERE parent_id = $2
`

// ── scanOTP helper ────────────────────────────────────────────────────────────

func scanOTP(row interface {
	Scan(...any) error
}) (PasswordResetOtp, error) {
	var o PasswordResetOtp
	err := row.Scan(
		&o.ID,
		&o.Email,
		&o.OtpHash,
		&o.ResetToken,
		&o.ExpiresAt,
		&o.VerifiedAt,
		&o.UsedAt,
		&o.CreatedAt,
	)
	return o, err
}

// ── CreatePasswordResetOTP ────────────────────────────────────────────────────

type CreatePasswordResetOTPParams struct {
	Email     string    `json:"email"`
	OtpHash   string    `json:"otp_hash"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (q *Queries) CreatePasswordResetOTP(ctx context.Context, arg CreatePasswordResetOTPParams) (PasswordResetOtp, error) {
	row := q.db.QueryRow(ctx, createPasswordResetOTP, arg.Email, arg.OtpHash, arg.ExpiresAt)
	return scanOTP(row)
}

// ── GetLatestValidOTPByEmail ──────────────────────────────────────────────────

func (q *Queries) GetLatestValidOTPByEmail(ctx context.Context, email string) (PasswordResetOtp, error) {
	row := q.db.QueryRow(ctx, getLatestValidOTPByEmail, email)
	return scanOTP(row)
}

// ── GetOTPByResetToken ────────────────────────────────────────────────────────

func (q *Queries) GetOTPByResetToken(ctx context.Context, resetToken uuid.UUID) (PasswordResetOtp, error) {
	pgUUID := pgtype.UUID{Bytes: resetToken, Valid: true}
	row := q.db.QueryRow(ctx, getOTPByResetToken, pgUUID)
	return scanOTP(row)
}

// ── MarkOTPVerified ───────────────────────────────────────────────────────────

func (q *Queries) MarkOTPVerified(ctx context.Context, id uuid.UUID) (PasswordResetOtp, error) {
	row := q.db.QueryRow(ctx, markOTPVerified, id)
	return scanOTP(row)
}

// ── MarkOTPUsed ───────────────────────────────────────────────────────────────

func (q *Queries) MarkOTPUsed(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.Exec(ctx, markOTPUsed, id)
	return err
}

// ── InvalidateOTPsByEmail ─────────────────────────────────────────────────────

func (q *Queries) InvalidateOTPsByEmail(ctx context.Context, email string) error {
	_, err := q.db.Exec(ctx, invalidateOTPsByEmail, email)
	return err
}

// ── UpdateParentPassword ──────────────────────────────────────────────────────

func (q *Queries) UpdateParentPassword(ctx context.Context, hashedPassword, parentID string) error {
	_, err := q.db.Exec(ctx, updateParentPassword, hashedPassword, parentID)
	return err
}
