package database

import "context"

// ResolveSportConflict atomically clears the sport_conflict_pending flag.
// Uses WHERE sport_conflict_pending = true as TOCTOU guard.
// Returns true if a row was updated (conflict was pending), false if already resolved.
func (db *DB) ResolveSportConflict(ctx context.Context, activityID, userID string, newSportType *string) (bool, error) {
	var query string
	var args []any

	if newSportType != nil && *newSportType != "" {
		query = `
			UPDATE activities
			SET sport_type = $1,
			    sport_type_is_manual = true,
			    sport_conflict_pending = false
			WHERE id = $2 AND user_id = $3 AND sport_conflict_pending = true`
		args = []any{*newSportType, activityID, userID}
	} else {
		query = `
			UPDATE activities
			SET sport_conflict_pending = false
			WHERE id = $1 AND user_id = $2 AND sport_conflict_pending = true`
		args = []any{activityID, userID}
	}

	tag, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
