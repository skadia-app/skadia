package handler

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// TxFactory is a function that begins a database transaction.
// It is used by beginTx to allow tests to inject a mock pgx.Tx.
type TxFactory func(ctx context.Context) (pgx.Tx, error)

// beginTx begins a database transaction using the handler's TxFactory if set,
// or falls back to h.DB.Pool.Begin for production use.
// Tests set h.TxFactory to inject mock transactions (e.g. to simulate mid-TX errors).
func (h *Handler) beginTx(ctx context.Context) (pgx.Tx, error) {
	if h.TxFactory != nil {
		return h.TxFactory(ctx)
	}
	return h.DB.Pool.Begin(ctx)
}
