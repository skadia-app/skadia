package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// SetActivityKomootTourID sets the komoot_tour_id on an existing activity.
func (db *DB) SetActivityKomootTourID(ctx context.Context, activityID string, tourID int64) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activities SET komoot_tour_id = $2 WHERE id = $1`,
		activityID, tourID,
	)
	return err
}

// ActivityExistsByKomootTourID returns true if the user already has an activity with this Komoot tour ID.
func (db *DB) ActivityExistsByKomootTourID(ctx context.Context, userID string, tourID int64) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM activities WHERE user_id = $1 AND komoot_tour_id = $2)`,
		userID, tourID,
	).Scan(&exists)
	return exists, err
}

// GetActivityIDByKomootTourID returns the activity ID for a given Komoot tour ID.
func (db *DB) GetActivityIDByKomootTourID(ctx context.Context, userID string, tourID int64) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM activities WHERE user_id = $1 AND komoot_tour_id = $2`,
		userID, tourID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}
