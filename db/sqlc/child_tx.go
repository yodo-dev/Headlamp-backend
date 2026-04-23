package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// DeleteChildTx soft-deletes a child and cascades the side-effects within a
// single transaction:
//  1. Marks the children row as deleted (sets deleted_at = NOW()).
//  2. Deactivates all devices registered for this child so the device can
//     no longer authenticate.
//  3. Closes any open app sessions for the child.
//
// Hard-delete FK cascades (reflections, insights, quiz attempts, etc.) are NOT
// triggered because we only soft-delete; the related data is preserved for
// audit / reporting purposes and will be invisible to normal queries because
// those queries join through children WHERE deleted_at IS NULL.
func (store *SQLStore) DeleteChildTx(ctx context.Context, childID string) error {
	return store.execTx(ctx, func(q *Queries) error {
		// 1. Fetch the child so we have the family_id for the scoped soft-delete.
		child, err := q.GetChild(ctx, childID)
		if err != nil {
			return fmt.Errorf("DeleteChildTx: fetch child: %w", err)
		}

		// 2. Soft-delete the child row.
		if err := q.SoftDeleteChild(ctx, SoftDeleteChildParams{
			ID:       childID,
			FamilyID: child.FamilyID,
		}); err != nil {
			return fmt.Errorf("DeleteChildTx: soft-delete child: %w", err)
		}

		// 3. Deactivate all devices for this child (devices use UUID user_id).
		childUUID, err := uuid.Parse(childID)
		if err != nil {
			return fmt.Errorf("DeleteChildTx: parse child UUID: %w", err)
		}
		if err := q.DeactivateUserDevices(ctx, childUUID); err != nil {
			return fmt.Errorf("DeleteChildTx: deactivate devices: %w", err)
		}

		// 4. Close any open app sessions.
		if _, err := q.CloseSessionsForChild(ctx, childID); err != nil {
			return fmt.Errorf("DeleteChildTx: close sessions: %w", err)
		}

		return nil
	})
}
