package enrichment

import (
	"context"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// withCachedEnrichmentSettings runs fn with EnrichmentSettings loaded once for
// userID. Use in bulk re-enrichment loops to avoid one SELECT per activity.
// The loaded settings are immutable for the duration of the call; fn must not
// modify the pointer.
func withCachedEnrichmentSettings(
	ctx context.Context,
	db *database.DB,
	userID string,
	fn func(*model.EnrichmentSettings) error,
) error {
	settings, err := db.GetEnrichmentSettings(ctx, userID)
	if err != nil {
		return fmt.Errorf("withCachedEnrichmentSettings: load settings: %w", err)
	}
	return fn(settings)
}
