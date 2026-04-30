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

	if resolution != nil {
		var sportType any
		if resolution.SportType != "" {
			sportType = resolution.SportType
		}
		var subType any
		if resolution.SubType != "" {
			subType = resolution.SubType
		}
		query = `
			UPDATE activities
			SET sport_type = COALESCE($1, sport_type),
			    sub_type = CASE WHEN $1 IS NULL THEN sub_type ELSE $2 END,
			    is_electric = COALESCE($3, is_electric),
			    is_indoor = COALESCE($4, is_indoor),
			    is_virtual = COALESCE($5, is_virtual),
			    exclude_from_heatmap = CASE WHEN $5 IS TRUE THEN TRUE WHEN $5 IS FALSE THEN FALSE ELSE exclude_from_heatmap END,
			    sport_type_is_manual = true,
			    has_edits = true,
			    sport_conflict_pending = false
			WHERE id = $6 AND user_id = $7 AND sport_conflict_pending = true`
		args = []any{
			sportType, subType,
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
