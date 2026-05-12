package auth

import (
	"context"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
)

func lockGDPRTransferAdmission(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, database.LockGDPRTransferAdmission)
	return err
}
