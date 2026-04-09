package db

import (
	"context"

	"github.com/google/uuid"
)

// CreateChildSetupTxParams contains the input parameters for the child setup transaction.
type CreateChildSetupTxParams struct {
	DeviceID string `json:"device_id"`
	Code     string `json:"code"`
}

// CreateChildSetupTxResult is the result of the child setup transaction.
type CreateChildSetupTxResult struct {
	Child  Child
	Device Device
}

// CreateChildSetupTx performs a transaction to find a child by ID, create a device for them,
// and mark the deep link code as used.
func (store *SQLStore) CreateChildSetupTx(ctx context.Context, arg CreateChildSetupTxParams) (CreateChildSetupTxResult, error) {
	var result CreateChildSetupTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		// Step 1: Get the deep link code by the code provided.
		deepLink, err := q.GetDeepLinkCode(ctx, arg.Code)
		if err != nil {
			return err
		}

		// Step 2: Get the child record using the ChildID from the deep link.
		child, err := q.GetChild(ctx, deepLink.ChildID)
		if err != nil {
			return err
		}
		result.Child = child

		// Step 3: Create the new device.
		childUUID, err := uuid.Parse(child.ID)
		if err != nil {
			return err
		}
		device, err := q.CreateDevice(ctx, CreateDeviceParams{
			UserID:   childUUID,
			UserType: "child",
			DeviceID: arg.DeviceID,
		})
		if err != nil {
			return err
		}
		result.Device = device

		// Step 4: Mark the deep link code as used.
		_, err = q.UseDeepLinkCode(ctx, arg.Code)
				return err
	})

	return result, err
}
