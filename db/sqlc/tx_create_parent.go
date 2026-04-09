package db

import (
	"context"

	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreateParentTxParams contains the input parameters for creating a parent within a transaction.
// It includes details for creating both a family and the parent user.
type CreateParentTxParams struct {
	Firstname string `json:"firstname"`
	Surname   string `json:"surname"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

// CreateParentTxResult is the result of the create parent transaction.
// It contains the newly created family and parent records.
type CreateParentTxResult struct {
	Family Family `json:"family"`
	Parent Parent `json:"parent"`
}

// CreateParentTx performs the creation of a family and a parent user within a single database transaction.
func (store *SQLStore) CreateParentTx(ctx context.Context, arg CreateParentTxParams, publicKey []byte, privateKey []byte) (CreateParentTxResult, error) {
	var result CreateParentTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		result.Family, err = q.CreateFamily(ctx, CreateFamilyParams{
			ID:         uuid.New().String(),
			PublicKey:  publicKey,
			PrivateKey: privateKey,
		})
		if err != nil {
			return err
		}

		hashedPassword, err := util.HashPassword(arg.Password)
		if err != nil {
			return err
		}

		result.Parent, err = q.CreateParent(ctx, CreateParentParams{
			ParentID:       uuid.New().String(),
			FamilyID:       result.Family.ID,
			Firstname:      arg.Firstname,
			Surname:        arg.Surname,
			Email:          arg.Email,
			HashedPassword: pgtype.Text{String: string(hashedPassword), Valid: true},
		})

		return err
	})

	return result, err
}
