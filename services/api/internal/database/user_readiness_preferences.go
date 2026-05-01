package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// UserReadinessPreference holds the per-user override for the readiness source.
// PreferredSource == nil means automatic selection (no manual override).
type UserReadinessPreference struct {
	UserID          string
	PreferredSource *string
	UpdatedAt       time.Time
}

// validReadinessSources lists the values accepted by the typed Upsert.
// Must stay in sync with the CHECK constraint in migration 000343.
var validReadinessSources = map[string]struct{}{
	"manufacturer": {},
	"skadia":      {},
}

// GetUserReadinessPreference returns the preference row for a user.
// Returns (nil, nil) if no row exists (no preference == automatic).
func (db *DB) GetUserReadinessPreference(ctx context.Context, userID string) (*UserReadinessPreference, error) {
	var (
		source    *string
		updatedAt time.Time
	)
	err := db.Pool.QueryRow(ctx,
		`SELECT preferred_source, updated_at
		 FROM user_readiness_preferences
		 WHERE user_id = $1`,
		userID,
	).Scan(&source, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user readiness preference %s: %w", userID, err)
	}
	return &UserReadinessPreference{
		UserID:          userID,
		PreferredSource: source,
		UpdatedAt:       updatedAt,
	}, nil
}

// UpsertUserReadinessPreference sets a manual override for the readiness source.
// source must be one of {"manufacturer","skadia"}; other values are rejected.
//
// updated_at is set explicitly to NOW() in the ON CONFLICT branch because the
// column DEFAULT only fires on INSERT — the explicit NOW() is what later
// pref-race checks rely on.
func (db *DB) UpsertUserReadinessPreference(ctx context.Context, userID, source string) error {
	if _, ok := validReadinessSources[source]; !ok {
		return fmt.Errorf("invalid readiness source %q (expected 'manufacturer' or 'skadia')", source)
	}
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO user_readiness_preferences (user_id, preferred_source, updated_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (user_id) DO UPDATE
		 SET preferred_source = EXCLUDED.preferred_source,
		     updated_at       = NOW()`,
		userID, source,
	)
	if err != nil {
		return fmt.Errorf("upsert user readiness preference %s: %w", userID, err)
	}
	return nil
}

// UpsertUserReadinessPreferenceNull clears the manual override (preferred_source
// becomes NULL → automatic selection).
func (db *DB) UpsertUserReadinessPreferenceNull(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO user_readiness_preferences (user_id, preferred_source, updated_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (user_id) DO UPDATE
		 SET preferred_source = EXCLUDED.preferred_source,
		     updated_at       = NOW()`,
		userID, nil,
	)
	if err != nil {
		return fmt.Errorf("upsert user readiness preference (null) %s: %w", userID, err)
	}
	return nil
}
