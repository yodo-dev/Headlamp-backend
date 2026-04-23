package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/golang/mock/mockgen/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store defines all functions to execute db queries and transactions
type Store interface {
	Querier
	ExecTx(ctx context.Context, fn func(*Queries) error) error

	CreateParentSocialTx(ctx context.Context, arg CreateParentSocialTxParams) (CreateParentSocialTxResult, error)
	CreateParentTx(ctx context.Context, arg CreateParentTxParams, publicKey []byte, privateKey []byte) (CreateParentTxResult, error)
	ReplaceDeviceTx(ctx context.Context, arg ReplaceDeviceTxParams) error
	CreateChildSetupTx(ctx context.Context, arg CreateChildSetupTxParams) (CreateChildSetupTxResult, error)
	CreateChildTx(ctx context.Context, arg CreateChildTxParams) (CreateChildTxResult, error)
	DeleteChildTx(ctx context.Context, childID string) error
	CreateQuizAttemptTx(ctx context.Context, arg CreateQuizAttemptTxParams) (CreateQuizAttemptTxResult, error)
	SubmitQuizAnswersTx(ctx context.Context, arg SubmitQuizAnswersTxParams) (SubmitQuizAnswersTxResult, error)

	// ── AI Insights ──────────────────────────────────────────────────────────
	UpsertInsightsSnapshot(ctx context.Context, arg UpsertInsightsSnapshotParams) (AiInsightsSnapshot, error)
	GetInsightsSnapshot(ctx context.Context, arg GetInsightsSnapshotParams) (AiInsightsSnapshot, error)
	MarkInsightSnapshotStale(ctx context.Context, childID string) error

	// ── Content Monitoring ───────────────────────────────────────────────────
	CreateContentMonitoringEvent(ctx context.Context, arg CreateContentMonitoringEventParams) (ContentMonitoringEvent, error)
	GetContentMonitoringEventsForChild(ctx context.Context, arg GetInsightAggregateParams) ([]ContentMonitoringEvent, error)
	GetLatestContentMonitoringAlert(ctx context.Context, childID string) (ContentMonitoringEvent, error)
	GetContentMonitoringCounts(ctx context.Context, arg GetInsightAggregateParams) ([]ContentCountRow, error)
	GetTopRiskyPlatforms(ctx context.Context, arg GetInsightAggregateParams) ([]PlatformCountRow, error)

	// ── Aggregation queries for insight computation ───────────────────────────
	GetAppSessionAggregateForChild(ctx context.Context, arg GetInsightAggregateParams) ([]AppSessionAggregate, error)
	GetOverLimitSessionCount(ctx context.Context, arg GetInsightAggregateParams) (int64, error)
	GetNightSessionCount(ctx context.Context, arg GetInsightAggregateParams) (int64, error)
	GetReflectionAggregateForChild(ctx context.Context, arg GetInsightAggregateParams) (ReflectionAggregate, error)
	GetRecentReflectionResponsesForChild(ctx context.Context, arg GetInsightAggregateParams) ([]ReflectionResponseRow, error)
	GetQuizAggregateForChild(ctx context.Context, arg GetInsightAggregateParams) (QuizAggregate, error)

	// ── Password Reset OTP ────────────────────────────────────────────────────
	CreatePasswordResetOTP(ctx context.Context, arg CreatePasswordResetOTPParams) (PasswordResetOtp, error)
	GetLatestValidOTPByEmail(ctx context.Context, email string) (PasswordResetOtp, error)
	GetOTPByResetToken(ctx context.Context, resetToken uuid.UUID) (PasswordResetOtp, error)
	MarkOTPVerified(ctx context.Context, id uuid.UUID) (PasswordResetOtp, error)
	MarkOTPUsed(ctx context.Context, id uuid.UUID) error
	InvalidateOTPsByEmail(ctx context.Context, email string) error
	UpdateParentPassword(ctx context.Context, hashedPassword, parentID string) error
}

// QuizAnswer defines the structure for a single answer submission.
type QuizAnswer struct {
	QuestionID        string
	SelectedOptionIDs []string
	IsCorrect         bool
}

// CreateQuizAttemptTxParams contains the input parameters for creating a quiz attempt with its answers.
type CreateQuizAttemptTxParams struct {
	ChildID        string
	ExternalQuizID string
	Context        string
	ContextRef     sql.NullString
	AttemptNumber  int32
	Score          int32
	Passed         bool
	Answers        []QuizAnswer
}

// CreateQuizAttemptTxResult contains the result of the create quiz attempt transaction.
type CreateQuizAttemptTxResult struct {
	Attempt ChildQuizAttempt
	Answers []ChildQuizAnswer
}

// SQLStore provides all functions to execute SQL queries and transactions
type SQLStore struct {
	connPool *pgxpool.Pool
	*Queries
}

// NewStore creates a new store
func NewStore(connPool *pgxpool.Pool) Store {
	return &SQLStore{
		connPool: connPool,
		Queries:  New(connPool),
	}
}

// ExecTx executes a function within a database transaction
func (store *SQLStore) ExecTx(ctx context.Context, fn func(*Queries) error) error {
	tx, err := store.connPool.Begin(ctx)
	if err != nil {
		return err
	}

	q := New(tx)
	err = fn(q)
	if err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx err: %v, rb err: %v", err, rbErr)
		}
		return err
	}

	return tx.Commit(ctx)
}
