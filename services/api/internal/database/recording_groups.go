package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// fieldRichnessExpr is the SQL CASE expression that counts non-nil data-rich
// fields on an activity row. Used in all recording-group queries that return
// RecordingGroupMemberDetail.field_richness.
const fieldRichnessExpr = `(CASE WHEN a.avg_heart_rate IS NOT NULL THEN 1 ELSE 0 END +
				 CASE WHEN a.avg_power_w IS NOT NULL THEN 1 ELSE 0 END +
				 CASE WHEN a.has_gps THEN 1 ELSE 0 END +
				 CASE WHEN a.avg_temperature_c IS NOT NULL THEN 1 ELSE 0 END +
				 CASE WHEN a.avg_cadence IS NOT NULL THEN 1 ELSE 0 END +
				 CASE WHEN a.elevation_gain_m IS NOT NULL THEN 1 ELSE 0 END)`

const (
	deviceNameSelect = "COALESCE(rd.custom_name, rd.product_name) AS recording_device_name"
	deviceNameJoin   = "LEFT JOIN devices rd ON rd.id = a.recording_device_id"
)

// RecordingGroup represents a group of N recordings of the same event.
type RecordingGroup struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	PrimaryActivityID *string   `json:"primary_activity_id,omitempty"`
	SportType         *string   `json:"sport_type,omitempty"`
	SubType           *string   `json:"sub_type,omitempty"`
	IsElectric        bool      `json:"is_electric"`
	IsIndoor          bool      `json:"is_indoor"`
	IsVirtual         bool      `json:"is_virtual"`
	CreatedAt         time.Time `json:"created_at"`
	Confirmed         bool      `json:"confirmed"`
}

type RecordingGroupSportTuple struct {
	SportType  string
	SubType    *string
	IsElectric *bool
	IsIndoor   *bool
	IsVirtual  *bool
}

// RecordingGroupMember is a single member row from recording_group_members.
type RecordingGroupMember struct {
	GroupID    string `json:"group_id"`
	ActivityID string `json:"activity_id"`
	IsFused    bool   `json:"is_fused"`
}

// RecordingGroupMemberDetail augments a member with activity metadata for display.
type RecordingGroupMemberDetail struct {
	ActivityID          string    `json:"activity_id"`
	IsFused             bool      `json:"is_fused"`
	FusionSourceCount   int       `json:"fusion_source_count"`
	IsPrimary           bool      `json:"is_primary"`
	Title               *string   `json:"title,omitempty"`
	TitleAuto           *string   `json:"title_auto,omitempty"`
	StartedAt           time.Time `json:"started_at"`
	SportType           string    `json:"sport_type"`
	SubType             *string   `json:"sub_type,omitempty"`
	IsElectric          bool      `json:"is_electric"`
	IsIndoor            bool      `json:"is_indoor"`
	IsVirtual           bool      `json:"is_virtual"`
	SourceDevice        *string   `json:"source_device,omitempty"`
	Source              *string   `json:"source,omitempty"`
	RecordingDeviceName *string   `json:"recording_device_name,omitempty"`
	OriginalFileFormat  *string   `json:"original_file_format,omitempty"`
	DistanceM           *float64  `json:"distance_m,omitempty"`
	ElapsedTimeS        *int      `json:"elapsed_time_s,omitempty"`
	FieldRichness       int       `json:"field_richness"`
	HasGPS              bool      `json:"has_gps"`
	ElevationGainM      *float64  `json:"elevation_gain_m,omitempty"`
	ElevationLossM      *float64  `json:"elevation_loss_m,omitempty"`
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
		 RETURNING id, user_id, primary_activity_id, sport_type, sub_type, is_electric, is_indoor, is_virtual, created_at, confirmed`,
		userID, primaryArg, sportTypeArg,
	).Scan(&g.ID, &g.UserID, &g.PrimaryActivityID, &g.SportType, &g.SubType, &g.IsElectric, &g.IsIndoor, &g.IsVirtual, &g.CreatedAt, &g.Confirmed)
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
		`SELECT id, user_id, primary_activity_id, sport_type, sub_type, is_electric, is_indoor, is_virtual, created_at, confirmed
		 FROM recording_groups WHERE id = $1`,
		groupID,
	).Scan(&g.ID, &g.UserID, &g.PrimaryActivityID, &g.SportType, &g.SubType, &g.IsElectric, &g.IsIndoor, &g.IsVirtual, &g.CreatedAt, &g.Confirmed)
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
		`SELECT rg.id, rg.user_id, rg.primary_activity_id, rg.sport_type, rg.sub_type, rg.is_electric, rg.is_indoor, rg.is_virtual, rg.created_at, rg.confirmed
		 FROM recording_groups rg
		 JOIN recording_group_members rgm ON rgm.group_id = rg.id
		 WHERE rgm.activity_id = $1`,
		activityID,
	).Scan(&g.ID, &g.UserID, &g.PrimaryActivityID, &g.SportType, &g.SubType, &g.IsElectric, &g.IsIndoor, &g.IsVirtual, &g.CreatedAt, &g.Confirmed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get recording group for activity: %w", err)
	}
	return &g, nil
}

// GetRecordingGroupsForActivities returns a map of activityID → *RecordingGroup for all
// activity IDs in the given slice that belong to a recording group.
// Activities not in any group are absent from the returned map.
// A single WHERE activity_id = ANY($1) query replaces N individual lookups.
func (db *DB) GetRecordingGroupsForActivities(ctx context.Context, activityIDs []string) (map[string]*RecordingGroup, error) {
	if len(activityIDs) == 0 {
		return map[string]*RecordingGroup{}, nil
	}
	rows, err := db.Pool.Query(ctx,
		`SELECT rgm.activity_id, rg.id, rg.user_id, rg.primary_activity_id, rg.sport_type, rg.sub_type, rg.is_electric, rg.is_indoor, rg.is_virtual, rg.created_at, rg.confirmed
		 FROM recording_group_members rgm
		 JOIN recording_groups rg ON rg.id = rgm.group_id
		 WHERE rgm.activity_id = ANY($1)`,
		activityIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("get recording groups for activities: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*RecordingGroup, len(activityIDs))
	for rows.Next() {
		var actID string
		var g RecordingGroup
		if err := rows.Scan(&actID, &g.ID, &g.UserID, &g.PrimaryActivityID, &g.SportType, &g.SubType, &g.IsElectric, &g.IsIndoor, &g.IsVirtual, &g.CreatedAt, &g.Confirmed); err != nil {
			return nil, fmt.Errorf("scan recording group for activity: %w", err)
		}
		result[actID] = &g
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recording groups for activities: %w", err)
	}
	return result, nil
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
				SELECT id, user_id, primary_activity_id, sport_type, sub_type, is_electric, is_indoor, is_virtual, created_at, confirmed
				FROM recording_groups
				WHERE user_id = $1
				ORDER BY created_at DESC
				LIMIT $2 OFFSET $3
			)
			SELECT
				lg.id, lg.user_id, lg.primary_activity_id, lg.sport_type, lg.sub_type, lg.is_electric, lg.is_indoor, lg.is_virtual, lg.created_at, lg.confirmed,
				rgm.activity_id, rgm.is_fused,
				(SELECT COUNT(DISTINCT merged_id) FROM activity_fusion_memberships WHERE source_id = a.id) AS fusion_source_count,
				(lg.primary_activity_id = rgm.activity_id) AS is_primary,
					a.title, a.title_auto, a.started_at, a.sport_type, a.sub_type,
					a.is_electric, a.is_indoor, a.is_virtual,
				a.source_device, a.source, `+deviceNameSelect+`,
				a.original_file_format, a.distance_m, a.elapsed_time_s,
				`+fieldRichnessExpr+` AS field_richness,
				a.has_gps, a.elevation_gain_m, a.elevation_loss_m
			FROM limited_groups lg
			JOIN recording_group_members rgm ON rgm.group_id = lg.id
			JOIN activities a ON a.id = rgm.activity_id
			`+deviceNameJoin+`
			ORDER BY lg.created_at DESC, rgm.activity_id`,
			userID, limit, offset)
	} else {
		rows, err = db.Pool.Query(ctx, `
			SELECT
				rg.id, rg.user_id, rg.primary_activity_id, rg.sport_type, rg.sub_type, rg.is_electric, rg.is_indoor, rg.is_virtual, rg.created_at, rg.confirmed,
				rgm.activity_id, rgm.is_fused, (SELECT COUNT(DISTINCT merged_id) FROM activity_fusion_memberships WHERE source_id = a.id) AS fusion_source_count,
				(rg.primary_activity_id = rgm.activity_id) AS is_primary,
					a.title, a.title_auto, a.started_at, a.sport_type, a.sub_type,
					a.is_electric, a.is_indoor, a.is_virtual,
				a.source_device, a.source, `+deviceNameSelect+`,
				a.original_file_format, a.distance_m, a.elapsed_time_s,
				`+fieldRichnessExpr+` AS field_richness,
				a.has_gps, a.elevation_gain_m, a.elevation_loss_m
			FROM recording_groups rg
			JOIN recording_group_members rgm ON rgm.group_id = rg.id
			JOIN activities a ON a.id = rgm.activity_id
			`+deviceNameJoin+`
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
			&g.ID, &g.UserID, &g.PrimaryActivityID, &g.SportType, &g.SubType, &g.IsElectric, &g.IsIndoor, &g.IsVirtual, &g.CreatedAt, &g.Confirmed,
			&member.ActivityID, &member.IsFused, &member.FusionSourceCount, &member.IsPrimary,
			&member.Title, &member.TitleAuto, &member.StartedAt, &member.SportType, &member.SubType,
			&member.IsElectric, &member.IsIndoor, &member.IsVirtual,
			&member.SourceDevice, &member.Source, &member.RecordingDeviceName, &member.OriginalFileFormat, &member.DistanceM, &member.ElapsedTimeS,
			&member.FieldRichness, &member.HasGPS, &member.ElevationGainM, &member.ElevationLossM,
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

// addMember inserts an activity into a recording group, optionally marking it as fused.
// Runs in its own transaction so the DEFERRABLE CONSTRAINT TRIGGER fires at commit.
func (db *DB) addMember(ctx context.Context, groupID, activityID string, isFused bool) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var query string
	if isFused {
		query = `INSERT INTO recording_group_members (group_id, activity_id, is_fused) VALUES ($1, $2, TRUE)`
	} else {
		query = `INSERT INTO recording_group_members (group_id, activity_id) VALUES ($1, $2)`
	}
	tag, err := tx.Exec(ctx, query, groupID, activityID)
	if err != nil {
		return fmt.Errorf("add recording group member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("add recording group member: no rows affected")
	}
	return tx.Commit(ctx)
}

// AddRecordingGroupMember adds an activity to an existing recording group.
// The DEFERRABLE CONSTRAINT TRIGGER fires at commit of the caller's transaction,
// so this function uses its own transaction to ensure trigger execution.
func (db *DB) AddRecordingGroupMember(ctx context.Context, groupID, activityID string) error {
	return db.addMember(ctx, groupID, activityID, false)
}

// AddRecordingGroupMemberFused adds an activity to an existing recording group with is_fused = TRUE.
// Used when a unified-timeline fusion result is added to its source group.
// Delegates to addMemberTx after opening its own transaction (preserves behaviour
// for callers that do not already hold a TX).
func (db *DB) AddRecordingGroupMemberFused(ctx context.Context, groupID, activityID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := db.addMemberTx(ctx, tx, groupID, activityID, true); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AddRecordingGroupMemberTx inserts a non-fused member inside an existing TX.
// The caller owns the TX and is responsible for Commit/Rollback.
func (db *DB) AddRecordingGroupMemberTx(ctx context.Context, tx pgx.Tx, groupID, activityID string) error {
	return db.addMemberTx(ctx, tx, groupID, activityID, false)
}

// addMemberTx is the transaction-accepting variant of addMember.
// The caller owns the TX and is responsible for Commit/Rollback.
func (db *DB) addMemberTx(ctx context.Context, tx pgx.Tx, groupID, activityID string, isFused bool) error {
	query := `INSERT INTO recording_group_members (group_id, activity_id, is_fused) VALUES ($1, $2, $3)`
	tag, err := tx.Exec(ctx, query, groupID, activityID, isFused)
	if err != nil {
		return fmt.Errorf("add recording group member (tx): %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("add recording group member (tx): no rows affected")
	}
	return nil
}

// AddRecordingGroupMemberFusedTx inserts a fused member inside an existing TX.
// The caller owns the TX and is responsible for Commit/Rollback.
func (db *DB) AddRecordingGroupMemberFusedTx(ctx context.Context, tx pgx.Tx, groupID, activityID string) error {
	return db.addMemberTx(ctx, tx, groupID, activityID, true)
}

// SetRecordingGroupPrimaryTx validates membership and updates primary_activity_id
// inside an existing TX. The caller owns the TX and is responsible for Commit/Rollback.
func (db *DB) SetRecordingGroupPrimaryTx(ctx context.Context, tx pgx.Tx, groupID, activityID string) error {
	var exists bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM recording_group_members WHERE group_id = $1 AND activity_id = $2)`,
		groupID, activityID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check group membership (tx): %w", err)
	}
	if !exists {
		return fmt.Errorf("activity %s is not a member of group %s", activityID, groupID)
	}

	tag, err := tx.Exec(ctx,
		`UPDATE recording_groups SET primary_activity_id = $2 WHERE id = $1`,
		groupID, activityID,
	)
	if err != nil {
		return fmt.Errorf("set recording group primary (tx): %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set recording group primary (tx): group %s not found", groupID)
	}
	return nil
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
			(SELECT COUNT(DISTINCT merged_id) FROM activity_fusion_memberships WHERE source_id = a.id) AS fusion_source_count,
			(rg.primary_activity_id = rgm.activity_id) AS is_primary,
			a.title, a.title_auto, a.started_at, a.sport_type, a.sub_type,
			a.is_electric, a.is_indoor, a.is_virtual,
			a.source_device, a.source, `+deviceNameSelect+`,
			a.original_file_format, a.distance_m, a.elapsed_time_s,
			`+fieldRichnessExpr+` AS field_richness,
			a.has_gps, a.elevation_gain_m, a.elevation_loss_m
		FROM recording_group_members rgm
		JOIN recording_groups rg ON rg.id = rgm.group_id
		JOIN activities a ON a.id = rgm.activity_id
		`+deviceNameJoin+`
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
			&m.ActivityID, &m.IsFused, &m.FusionSourceCount, &m.IsPrimary,
			&m.Title, &m.TitleAuto, &m.StartedAt, &m.SportType, &m.SubType,
			&m.IsElectric, &m.IsIndoor, &m.IsVirtual,
			&m.SourceDevice, &m.Source, &m.RecordingDeviceName, &m.OriginalFileFormat, &m.DistanceM, &m.ElapsedTimeS,
			&m.FieldRichness, &m.HasGPS, &m.ElevationGainM, &m.ElevationLossM,
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

// SetRecordingGroupSportType updates the persisted sport tuple of a recording group.
func (db *DB) SetRecordingGroupSportType(ctx context.Context, groupID string, tuple RecordingGroupSportTuple) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := db.SetRecordingGroupSportTypeTx(ctx, tx, groupID, tuple); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetRecordingGroupSportTypeTx updates a group's sport tuple and propagates the
// explicit user decision to its member activities in the same transaction.
func (db *DB) SetRecordingGroupSportTypeTx(ctx context.Context, tx pgx.Tx, groupID string, tuple RecordingGroupSportTuple) error {
	var sportTypeArg any
	if tuple.SportType != "" {
		sportTypeArg = tuple.SportType
	}
	tag, err := tx.Exec(ctx,
		`UPDATE recording_groups
		 SET sport_type = COALESCE($2, sport_type),
		     sub_type = CASE WHEN $2 IS NULL THEN sub_type ELSE $3 END,
		     is_electric = COALESCE($4, is_electric),
		     is_indoor = COALESCE($5, is_indoor),
		     is_virtual = COALESCE($6, is_virtual)
		 WHERE id = $1`,
		groupID, sportTypeArg, tuple.SubType, tuple.IsElectric, tuple.IsIndoor, tuple.IsVirtual,
	)
	if err != nil {
		return fmt.Errorf("set recording group sport type: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set recording group sport type: group %s not found", groupID)
	}

	_, err = tx.Exec(ctx,
		`UPDATE activities a
		 SET sport_type = COALESCE($2, a.sport_type),
		     sub_type = CASE WHEN $2 IS NULL THEN a.sub_type ELSE $3 END,
		     is_electric = COALESCE($4, a.is_electric),
		     is_indoor = COALESCE($5, a.is_indoor),
		     is_virtual = COALESCE($6, a.is_virtual),
		     exclude_from_heatmap = CASE
		       WHEN $6 IS TRUE THEN TRUE
		       WHEN $6 IS FALSE THEN FALSE
		       ELSE a.exclude_from_heatmap
		     END,
		     sport_type_is_manual = CASE
		       WHEN $2 IS NULL AND $4 IS NULL AND $5 IS NULL AND $6 IS NULL THEN a.sport_type_is_manual
		       ELSE TRUE
		     END,
		     has_edits = CASE
		       WHEN $2 IS NULL AND $4 IS NULL AND $5 IS NULL AND $6 IS NULL THEN a.has_edits
		       ELSE TRUE
		     END
		 FROM recording_group_members rgm
		 JOIN recording_groups rg ON rg.id = rgm.group_id
		 WHERE rgm.group_id = $1
		   AND rgm.activity_id = a.id
		   AND rg.user_id = a.user_id`,
		groupID, sportTypeArg, tuple.SubType, tuple.IsElectric, tuple.IsIndoor, tuple.IsVirtual,
	)
	if err != nil {
		return fmt.Errorf("set recording group member sport type: %w", err)
	}
	return nil
}
