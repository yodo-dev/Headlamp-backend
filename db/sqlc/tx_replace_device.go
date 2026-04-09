package db

import (
	"context"

	"github.com/google/uuid"
)

// ReplaceDeviceTxParams contains the input parameters for the replace device transaction
type ReplaceDeviceTxParams struct {
	Code        string    `json:"code"`
	OldDeviceID uuid.UUID `json:"old_device_id"`
	NewDeviceID uuid.UUID `json:"new_device_id"`
	ChildID     uuid.UUID `json:"child_id"`
}

// ReplaceDeviceTx performs a transaction to replace a device for a child.
func (store *SQLStore) ReplaceDeviceTx(ctx context.Context, arg ReplaceDeviceTxParams) error {
	return store.execTx(ctx, func(q *Queries) error {
		var err error

		_, err = q.UseDeepLinkCode(ctx, arg.Code)
		if err != nil {
			return err
		}

		err = q.DeleteDeviceByID(ctx, arg.OldDeviceID.String())
		if err != nil {
			return err
		}

		_, err = q.CreateDevice(ctx, CreateDeviceParams{
			UserID:   arg.ChildID,
			UserType: "child",
			DeviceID: arg.NewDeviceID.String(),
		})
		if err != nil {
			return err
		}

		return nil
	})
}
