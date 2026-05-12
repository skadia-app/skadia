package auth

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/automation"
)

func (h *Handler) seedSystemArtifactsTx(ctx context.Context, tx pgx.Tx, userID string) error {
	return automation.EnsureSystemArtifactsTx(ctx, tx, userID)
}
