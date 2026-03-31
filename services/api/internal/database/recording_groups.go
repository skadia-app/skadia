package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// RecordingGroup represents a group of N recordings of the same event.
type RecordingGroup struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	PrimaryActivityID *string   `json:"primary_activity_id,omitempty"`
	SportType         *string   `json:"sport_type,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	Confirmed         bool      `json:"confirmed"`
}

// RecordingGroupMember is a single member row from recording_group_members.
type RecordingGroupMember struct {
	GroupID    string `json:"group_id"`
	ActivityID string `json:"activity_id"`
	IsFused    bool   `json:"is_fused"`
}

// RecordingGroupMemberDetail augments a member with activity metadata for display.
type RecordingGroupMemberDetail struct {
	ActivityID         string   `json:"activity_id"`
	IsFused            bool     `json:"is_fused"`
	IsPrimary          bool     `json:"is_primary"`
	Title              *string  `json:"title,omitempty"`
	TitleAuto          *string  `json:"title_auto,omitempty"`
	StartedAt          time.Time `json:"started_at"`
	SportType          string   `json:"sport_type"`
	SourceDevice       *string  `json:"source_device,omitempty"`
	Source             *string  `json:"source,omitempty"`
	OriginalFileFormat *string  `json:"original_file_format,omitempty"`
	DistanceM          *float64 `json:"distance_m,omitempty"`
	ElapsedTimeS       *int     `json:"elapsed_time_s,omitempty"`
	FieldRichness      int      `json:"field_richness"`
	HasGPS             bool     `json:"has_gps"`
}

// RecordingGroupWithMembers is a RecordingGroup together with its member details.
type RecordingGroupWithMembers struct {
	RecordingGroup
	Members []RecordingGroupMemberDetail `json:"members"`
}

// CreateRecordingGroup creates a new recording group with the given member activity IDs.
// Uses a transaction so the DEFERRABLE CONSTRAINT TRIGGER fires only at commit,
// allowing all member inserts to complete before the trigger evaluates.
// primaryActivityID may be empty — in that case the DB trigger picks the best primary.
func (db *DB) CreateRecordingGroup(ctx context.Context, userID string, memberActivityIDs []string, primaryActivityID string, sportType string) (*RecordingGroup, error) {
	if len(memberActivityIDs) < 2 {
		return nil, fmt.Errorf("recording group requires at least 2 members, got %d", len(memberActivityIDs))
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var g RecordingGroup
	var primaryArg any
	if primaryActivityID != "" {
		primaryArg = primaryActivityID
	}
	var sportTypeArg any
	if sportType != "" {
		sportTypeArg = sportType
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO recording_groups (user_id, primary_activity_id, sport_type)
		 VALUES ($1, $2, $3)
		 RETURNING id, user_id, primary_activity_id, sport_type, created_at, confirmed`,
		userID, primaryArg, sportTypeArg,
	).Scan(&g.ID, &g.UserID, &g.PrimaryActivityID, &g.SportType, &g.CreatedAt, &g.Confirmed)
	if err != nil {
		return nil, fmt.Errorf("insert recording group: %w", err)
	}

	batch := &pgx.Batch{}
	for _, actID := range memberActivityIDs {
		batch.Queue(
			`INSERT INTO recording_group_members (group_id, activity_id) VALUES ($1, $2)`,
			g.ID, actID,
		)
	}
	br := tx.SendBatch(ctx, batch)
	for i := range memberActivityIDs {
		if _, err := br.Exec(); err != nil {
			br.Close() //nolint:errcheck
			return nil, fmt.Errorf("insert member %d: %w", i, err)
		}
	}
	if err := br.Close(); err != nil {
		return nil, fmt.Errorf("close member batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit recording group: %w", err)
	}

	return &g, nil
}

// GetRecordingGroup returns a single recording group by ID, or nil if not found.
func (db *DB) GetRecordingGroup(ctx context.Context, groupID string) (*RecordingGroup, error) {
	var g RecordingGroup
	err := db.Pool.QueryRow(ctx,
		`SELECT id, user_id, primary_activity_id, sport_type, created_at, confirmed
		 FROM recording_groups WHERE id = $1`,
		groupID,
	).Scan(&g.ID, &g.UserID, &g.PrimaryActivityID, &g.SportType, &g.CreatedAt, &g.Confirmed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get recording group: %w", err)
	}
	return &g, nil
}

// GetRecordingGroupForActivity returns the recording group that contains the given
// activity, or nil if the activity is not part of any group.
func (db *DB) GetRecordingGroupForActivity(ctx context.Context, activityID string) (*RecordingGroup, error) {
	var g RecordingGroup
	err := db.Pool.QueryRow(ctx,
		`SELECT rg.id, rg.user_id, rg.primary_activity_id, rg.sport_type, rg.created_at, rg.confirmed
		 FROM recording_groups rg
		 JOIN recording_group_members rgm ON rgm.group_id = rg.id
		 WHERE rgm.activity_id = $1`,
		activityID,
	).Scan(&g.ID, &g.UserID, &g.PrimaryActivityID, &g.SportType, &g.CreatedAt, &g.Confirmed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get recording group for activity: %w", err)
	}
	return &g, nil
}

// GetRecordingGroupsForUser returns all recording groups for a user with member details.
// Uses a CTE to limit groups first, then JOINs members — prevents LIMIT applying to joined rows.
// When limit <= 0, all groups are returned (GDPR export). offset is ignored when limit <= 0.
func (db *DB) GetRecordingGroupsForUser(ctx context.Context, userID string, limit, offset int) ([]RecordingGroupWithMembers, error) {
	var (
		rows pgx.Rows
		err  error
	)

	if limit > 0 {
		rows, err = db.Pool.Query(ctx, `
			WITH limited_groups AS (
				SELECT id, user_id, primary_activity_id, sport_type, created_at, confirmed
				FROM recording_groups
				WHERE user_id = $1
				ORDER BY created_at DESC
				LIMIT $2 OFFSET $3
			)
			SELECT
				lg.id, lg.user_id, lg.primary_activity_id, lg.sport_type, lg.created_at, lg.confirmed,
				rgm.activity_id, rgm.is_fused,
				(lg.primary_activity_id = rgm.activity_id) AS is_primary,
				a.title, a.title_auto, a.started_at, a.sport_type,
				a.source_device, a.source, a.original_file_format, a.distance_m, a.elapsed_time_s,
				(CASE WHEN a.avg_heart_rate IS NOT NULL THEN 1 ELSE 0 END +
				 CASE WHEN a.avg_power_w IS NOT NULL THEN 1 ELSE 0 END +
				 CASE WHEN a.has_gps THEN 1 ELSE 0 END +
				 CASE WHEN a.avg_temperature_c IS NOT NULL THEN 1 ELSE 0 END +
				 CASE WHEN a.avg_cadence IS NOT NULL THEN 1 ELSE 0 END +
				 CASE WHEN a.elevation_gain_m IS NOT NULL THEN 1 ELSE 0 END) AS field_richness,
				a.has_gps
			FROM limited_groups lg
			JOIN recording_group_members rgm ON rgm.group_id = lg.id
			JOIN activities a ON a.id = rgm.activity_id
			ORDER BY lg.created_at DESC, rgm.activity_id`,
			userID, limit, offset)
	} else {
		rows, err = db.Pool.Query(ctx, `
			SELECT
				rg.id, rg.user_id, rg.primary_activity_id, rg.sport_type, rg.created_at, rg.confirmed,
				rgm.activity_id, rgm.is_fused,
				(rg.primary_activity_id = rgm.activity_id) AS is_primary,
				a.title, a.title_auto, a.started_at, a.sport_type,
				a.source_device, a.source, a.original_file_format, a.distance_m, a.elapsed_time_s,
				(CASE WHEN a.avg_heart_rate IS NOT NULL THEN 1 ELSE 0 END +
				 CASE WHEN a.avg_power_w IS NOT NULL THEN 1 ELSE 0 END +
				 CASE WHEN a.has_gps THEN 1 ELSE 0 END +
				 CASE WHEN a.avg_temperature_c IS NOT NULL THEN 1 ELSE 0 END +
				 CASE WHEN a.avg_cadence IS NOT NULL THEN 1 ELSE 0 END +
				 CASE WHEN a.elevation_gain_m IS NOT NULL THEN 1 ELSE 0 END) AS field_richness,
				a.has_gps
			FROM recording_groups rg
			JOIN recording_group_members rgm ON rgm.group_id = rg.id
			JOIN activities a ON a.id = rgm.activity_id
			WHERE rg.user_id = $1
			ORDER BY rg.created_at DESC, rgm.activity_id`,
			userID)
	}
	if err != nil {
		return nil, fmt.Errorf("query recording groups: %w", err)
	}
	defer rows.Close()

	// Collect into map keyed by group ID to merge member rows.
	groupOrder := []string{}
	groupMap := map[string]*RecordingGroupWithMembers{}

	for rows.Next() {
		var (
			g      RecordingGroup
			member RecordingGroupMemberDetail
		)
		if err := rows.Scan(
			&g.ID, &g.UserID, &g.PrimaryActivityID, &g.SportType, &g.CreatedAt, &g.Confirmed,
			&member.ActivityID, &member.IsFused, &member.IsPrimary,
			&member.Title, &member.TitleAuto, &member.StartedAt, &member.SportType,
			&member.SourceDevice, &member.Source, &member.OriginalFileFormat, &member.DistanceM, &member.ElapsedTimeS,
			&member.FieldRichness, &member.HasGPS,
		); err != nil {
			return nil, fmt.Errorf("scan recording group row: %w", err)
		}

		if _, ok := groupMap[g.ID]; !ok {
			groupOrder = append(groupOrder, g.ID)
			groupMap[g.ID] = &RecordingGroupWithMembers{
				RecordingGroup: g,
				Members:        []RecordingGroupMemberDetail{},
			}
		}
		groupMap[g.ID].Members = append(groupMap[g.ID].Members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recording groups: %w", err)
	}

	result := make([]RecordingGroupWithMembers, 0, len(groupOrder))
	for _, id := range groupOrder {
		result = append(result, *groupMap[id])
	}
	return result, nil
}

// AddRecordingGroupMember adds an activity to an existing recording group.
// The DEFERRABLE CONSTRAINT TRIGGER fires at commit of the caller's transaction,
// so this function uses its own transaction to ensure trigger execution.
func (db *DB) AddRecordingGroupMember(ctx context.Context, groupID, activityID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx,
		`INSERT INTO recording_group_members (group_id, activity_id) VALUES ($1, $2)`,
		groupID, activityID,
	)
	if err != nil {
		return fmt.Errorf("add recording group member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("add recording group member: no rows affected")
	}

	return tx.Commit(ctx)
}

// AddRecordingGroupMemberFused adds an activity to an existing recording group with is_fused = TRUE.
// Used when a unified-timeline fusion result is added to its source group.
func (db *DB) AddRecordingGroupMemberFused(ctx context.Context, groupID, activityID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx,
		`INSERT INTO recording_group_members (group_id, activity_id, is_fused) VALUES ($1, $2, TRUE)`,
		groupID, activityID,
	)
	if err != nil {
		return fmt.Errorf("add fused recording group member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("add fused recording group member: no rows affected")
	}

	return tx.Commit(ctx)
}

// RemoveRecordingGroupMember removes an activity from a recording group.
// If fewer than 2 members remain, the DB trigger dissolves the group automatically.
func (db *DB) RemoveRecordingGroupMember(ctx context.Context, groupID, activityID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Clear is_group_secondary before removal so the trigger sees the correct state.
	if _, err := tx.Exec(ctx,
		`UPDATE activities SET is_group_secondary = FALSE WHERE id = $1`,
		activityID,
	); err != nil {
		return fmt.Errorf("clear group secondary flag: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`DELETE FROM recording_group_members WHERE group_id = $1 AND activity_id = $2`,
		groupID, activityID,
	)
	if err != nil {
		return fmt.Errorf("remove recording group member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("remove recording group member: activity %s not found in group %s", activityID, groupID)
	}

	return tx.Commit(ctx)
}

// SetRecordingGroupPrimary sets the primary activity of a recording group.
// Validates that activityID is an actual member of the group before updating.
func (db *DB) SetRecordingGroupPrimary(ctx context.Context, groupID, activityID string) error {
	// Validate membership first.
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM recording_group_members WHERE group_id = $1 AND activity_id = $2)`,
		groupID, activityID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check group membership: %w", err)
	}
	if !exists {
		return fmt.Errorf("activity %s is not a member of group %s", activityID, groupID)
	}

	tag, err := db.Pool.Exec(ctx,
		`UPDATE recording_groups SET primary_activity_id = $2 WHERE id = $1`,
		groupID, activityID,
	)
	if err != nil {
		return fmt.Errorf("set recording group primary: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set recording group primary: group %s not found", groupID)
	}
	return nil
}

// DissolveRecordingGroup dissolves a recording group by first clearing the
// is_group_secondary flag on all members, then deleting the group.
// Must be done in a transaction: CASCADE delete removes members before the
// trigger can find their IDs to reset the flag.
func (db *DB) DissolveRecordingGroup(ctx context.Context, groupID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Step 1: Clear is_group_secondary for all current members.
	if _, err := tx.Exec(ctx,
		`UPDATE activities SET is_group_secondary = FALSE
		 WHERE id IN (SELECT activity_id FROM recording_group_members WHERE group_id = $1)`,
		groupID,
	); err != nil {
		return fmt.Errorf("clear group secondary flags: %w", err)
	}

	// Step 2: Delete the group (CASCADE removes members, trigger won't find IDs anymore).
	tag, err := tx.Exec(ctx,
		`DELETE FROM recording_groups WHERE id = $1`,
		groupID,
	)
	if err != nil {
		return fmt.Errorf("delete recording group: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("dissolve recording group: group %s not found", groupID)
	}

	return tx.Commit(ctx)
}

// GetRecordingGroupMembers returns the member details for a recording group.
func (db *DB) GetRecordingGroupMembers(ctx context.Context, groupID string) ([]RecordingGroupMemberDetail, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			rgm.activity_id, rgm.is_fused,
			(rg.primary_activity_id = rgm.activity_id) AS is_primary,
			a.title, a.title_auto, a.started_at, a.sport_type,
			a.source_device, a.source, a.original_file_format, a.distance_m, a.elapsed_time_s,
			(CASE WHEN a.avg_heart_rate IS NOT NULL THEN 1 ELSE 0 END +
			 CASE WHEN a.avg_power_w IS NOT NULL THEN 1 ELSE 0 END +
			 CASE WHEN a.has_gps THEN 1 ELSE 0 END +
			 CASE WHEN a.avg_temperature_c IS NOT NULL THEN 1 ELSE 0 END +
			 CASE WHEN a.avg_cadence IS NOT NULL THEN 1 ELSE 0 END +
			 CASE WHEN a.elevation_gain_m IS NOT NULL THEN 1 ELSE 0 END) AS field_richness,
			a.has_gps
		FROM recording_group_members rgm
		JOIN recording_groups rg ON rg.id = rgm.group_id
		JOIN activities a ON a.id = rgm.activity_id
		WHERE rgm.group_id = $1
		ORDER BY a.started_at`, groupID)
	if err != nil {
		return nil, fmt.Errorf("query recording group members: %w", err)
	}
	defer rows.Close()

	result := make([]RecordingGroupMemberDetail, 0)
	for rows.Next() {
		var m RecordingGroupMemberDetail
		if err := rows.Scan(
			&m.ActivityID, &m.IsFused, &m.IsPrimary,
			&m.Title, &m.TitleAuto, &m.StartedAt, &m.SportType,
			&m.SourceDevice, &m.Source, &m.OriginalFileFormat, &m.DistanceM, &m.ElapsedTimeS,
			&m.FieldRichness, &m.HasGPS,
		); err != nil {
			return nil, fmt.Errorf("scan recording group member: %w", err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recording group members: %w", err)
	}
	return result, nil
}

// SetRecordingGroupSportType updates the sport_type of a recording group.
func (db *DB) SetRecordingGroupSportType(ctx context.Context, groupID, sportType string) error {
	var sportTypeArg any
	if sportType != "" {
		sportTypeArg = sportType
	}
	tag, err := db.Pool.Exec(ctx,
		`UPDATE recording_groups SET sport_type = $2 WHERE id = $1`,
		groupID, sportTypeArg,
	)
	if err != nil {
		return fmt.Errorf("set recording group sport type: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set recording group sport type: group %s not found", groupID)
	}
	return nil
}
