package db

import (
	"context"

	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreateParentSocialTxParams contains the input parameters for the create parent with social provider transaction.
type CreateParentSocialTxParams struct {
	Firstname       string         `json:"firstname"`
	Surname         string         `json:"surname"`
	Email           string         `json:"email"`
	AuthProvider    NullAuthProvider `json:"auth_provider"`
	ProviderSubject string         `json:"provider_subject"`
	EmailVerified   bool           `json:"email_verified"`
}

// CreateParentSocialTxResult contains the result of the create parent with social provider transaction.
type CreateParentSocialTxResult struct {
	Parent Parent `json:"parent"`
}

// CreateParentSocialTx performs the creation of a family and a parent (from a social provider) within a single database transaction.
func (store *SQLStore) CreateParentSocialTx(ctx context.Context, arg CreateParentSocialTxParams) (CreateParentSocialTxResult, error) {
	var result CreateParentSocialTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		publicKey, privateKey, err := util.GenerateKeyPair()
		if err != nil {
			return err
		}

		family, err := q.CreateFamily(ctx, CreateFamilyParams{
			ID:         uuid.New().String(),
			PrivateKey: privateKey,
			PublicKey:  publicKey,
		})
		if err != nil {
			return err
		}

		result.Parent, err = q.CreateParent(ctx, CreateParentParams{
			ParentID:        uuid.New().String(),
			FamilyID:        family.ID,
			Firstname:       arg.Firstname,
			Surname:         arg.Surname,
			Email:           arg.Email,
			AuthProvider:    arg.AuthProvider,
			ProviderSubject: pgtype.Text{String: arg.ProviderSubject, Valid: arg.ProviderSubject != ""},
			EmailVerified:   arg.EmailVerified,
			HashedPassword:  pgtype.Text{Valid: false},
		})

		return err
	})

	return result, err
}
