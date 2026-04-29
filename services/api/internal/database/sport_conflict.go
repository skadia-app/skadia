package database

import "context"

type SportConflictResolution struct {
	SportType  string
	SubType    string
	IsElectric *bool
	IsIndoor   *bool
	IsVirtual  *bool
}

// ResolveSportConflict atomically clears the sport_conflict_pending flag.
// Uses WHERE sport_conflict_pending = true as TOCTOU guard.
// Returns true if a row was updated (conflict was pending), false if already resolved.
func (db *DB) ResolveSportConflict(ctx context.Context, activityID, userID string, resolution *SportConflictResolution) (bool, error) {
	var query string
	var args []any

	if resolution != nil && resolution.SportType != "" {
		var subType any
		if resolution.SubType != "" {
			subType = resolution.SubType
		}
		query = `
			UPDATE activities
			SET sport_type = $1,
			    sub_type = $2,
			    is_electric = COALESCE($3, is_electric),
			    is_indoor = COALESCE($4, is_indoor),
			    is_virtual = COALESCE($5, is_virtual),
			    sport_type_is_manual = true,
			    sport_conflict_pending = false
			WHERE id = $6 AND user_id = $7 AND sport_conflict_pending = true`
		args = []any{
			resolution.SportType, subType,
			resolution.IsElectric, resolution.IsIndoor, resolution.IsVirtual,
			activityID, userID,
		}
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
