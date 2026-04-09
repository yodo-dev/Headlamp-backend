package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreateChildTxParams contains the input parameters for creating a child within a transaction.
type CreateChildTxParams struct {
	FamilyID        string
	FirstName       string
	Surname         string
	Age             sql.NullInt32
	Gender          sql.NullString
	ProfileImageUrl sql.NullString
}

// CreateChildTxResult is the result of the create child transaction.
// It contains the newly created child record.
type CreateChildTxResult struct {
	Child         Child         `json:"child"`
	DeepLinkCode  DeepLinkCode  `json:"deepLinkCode"`
}

// CreateChildTx performs the creation of a child and their initial onboarding progress within a single database transaction.
func (store *SQLStore) CreateChildTx(ctx context.Context, arg CreateChildTxParams) (CreateChildTxResult, error) {
	var result CreateChildTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		// Step 1: Create the child record.
		childID := uuid.New().String()

		// Convert from sql.Null* to pgtype for the query
		pgAge := pgtype.Int4{Int32: arg.Age.Int32, Valid: arg.Age.Valid}
		pgGender := pgtype.Text{String: arg.Gender.String, Valid: arg.Gender.Valid}
		pgProfileImageURL := pgtype.Text{String: arg.ProfileImageUrl.String, Valid: arg.ProfileImageUrl.Valid}

		result.Child, err = q.CreateChild(ctx, CreateChildParams{
			ID:              childID,
			FamilyID:        arg.FamilyID,
			FirstName:       arg.FirstName,
			Surname:         arg.Surname,
			Age:             pgAge,
			Gender:          pgGender,
			ProfileImageUrl: pgProfileImageURL,
		})
		if err != nil {
			return err
		}

		// Step 2: Create deep link code for the child.
		result.DeepLinkCode, err = q.CreateDeepLinkCode(ctx, CreateDeepLinkCodeParams{
			FamilyID:  arg.FamilyID,
			ChildID:   result.Child.ID,
			Code:      util.RandomString(6),
			ExpiresAt: time.Now().Add(time.Hour * 24 * 7),
		})
		if err != nil {
			return err
		}

		// Step 3: Create onboarding progress for all active steps.
		err = q.CreateChildOnboardingProgress(ctx, result.Child.ID)
		if err != nil {
			return err
		}

		return nil
	})

	return result, err
}
