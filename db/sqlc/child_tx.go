package db

import "context"

// DeleteChildTx is a transaction that deletes a child and, if it's the last child in the family, deletes the family as well.
func (store *SQLStore) DeleteChildTx(ctx context.Context, childID string) error {
	return store.execTx(ctx, func(q *Queries) error {
		return q.DeleteChild(ctx, childID)
	})
}
