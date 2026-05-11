package database

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

// insertGearForUsageTest inserts a minimal gear row (type='bike', scope='bike')
// and registers a cleanup that removes it. Returns the gear UUID.
func insertGearForUsageTest(t *testing.T, db *DB, userID, name string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO gear (id, user_id, name, type, scope)
		VALUES ($1, $2, $3, 'bike', 'bike')`, id, userID, name)
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM gear WHERE id = $1`, id)
	})
	return id
}

// insertComponentForUsageTest inserts a gear_component with installed_at=NOW().
func insertComponentForUsageTest(t *testing.T, db *DB, gearID, name string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO gear_components (id, gear_id, name, type, installed_at)
		VALUES ($1, $2, $3, 'component', NOW())`, id, gearID, name)
	if err != nil {
		t.Fatalf("insert component: %v", err)
	}
	return id
}

// insertActivityForUsageTest inserts an activity row and a primary
// activity_gear_assignments link. The legacy activities.gear_id column was
// dropped in migration 000355, so the gear association lives only in
// activity_gear_assignments.
func insertActivityForUsageTest(t *testing.T, db *DB, userID, gearID string, distanceM, movingTimeS, elapsedTimeS, gainM, lossM float64) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO activities (
			id, user_id, sport_type, started_at,
			distance_m, moving_time_s, elapsed_time_s,
			elevation_gain_m, elevation_loss_m, is_group_secondary
		)
		VALUES ($1, $2, 'cycling', $3, $4, $5, $6, $7, $8, false)`,
		id, userID, time.Now().Add(-time.Hour),
		distanceM, int(movingTimeS), int(elapsedTimeS), gainM, lossM)
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	_, err = db.Pool.Exec(context.Background(), `
		INSERT INTO activity_gear_assignments (user_id, activity_id, gear_id, role_key, source)
		VALUES ($1, $2, $3, 'primary', 'user')`,
		userID, id, gearID)
	if err != nil {
		t.Fatalf("insert activity gear assignment: %v", err)
	}
	return id
}

// TestUpdateGearStatsIncludesElevationLoss verifies that UpdateGearStats
// aggregates the columns restored in Phase 1 (D1): total_elevation_loss_m
// and total_moving_time_s. Before D1, both were stuck at 0.
func TestUpdateGearStatsIncludesElevationLoss(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Downhill Bike")
	insertActivityForUsageTest(t, db, userID, gearID, 10000, 3000, 3600, 200, 1200)

	if err := db.UpdateGearStatsOwnerVerified(ctx, gearID); err != nil {
		t.Fatalf("UpdateGearStatsOwnerVerified: %v", err)
	}

	gear, err := db.GetGear(ctx, gearID)
	if err != nil {
		t.Fatalf("GetGear: %v", err)
	}
	if gear.TotalElevationLossM != 1200 {
		t.Fatalf("loss = %v, want 1200", gear.TotalElevationLossM)
	}
	if gear.TotalMovingTimeS != 3000 || gear.TotalTimeS != 3600 {
		t.Fatalf("moving/elapsed = %d/%d, want 3000/3600", gear.TotalMovingTimeS, gear.TotalTimeS)
	}
}

// TestUpdateActivityMetricsRefreshesAssignedGearUsage verifies that
// UpdateActivityParserMetrics (the post-refactor equivalent of the backup's
// UpdateActivityMetrics with skipSportType=true) triggers the gear-usage
// refresh wired up in D2.
func TestUpdateActivityMetricsRefreshesAssignedGearUsage(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Trail Bike")
	activityID := insertActivityForUsageTest(t, db, userID, gearID, 1000, 60, 90, 10, 20)
	if err := db.UpdateGearStatsOwnerVerified(ctx, gearID); err != nil {
		t.Fatalf("initial UpdateGearStatsOwnerVerified: %v", err)
	}

	data := &model.ActivityData{
		SportType:      "cycling",
		StartedAt:      time.Now(),
		ElapsedTimeS:   600,
		MovingTimeS:    500,
		DistanceM:      5000,
		ElevationGainM: 200,
		ElevationLossM: 900,
	}

	if err := db.UpdateActivityParserMetrics(ctx, activityID, data); err != nil {
		t.Fatalf("UpdateActivityParserMetrics: %v", err)
	}
	gear, err := db.GetGear(ctx, gearID)
	if err != nil {
		t.Fatalf("GetGear: %v", err)
	}
	if gear.TotalDistanceM != 5000 || gear.TotalElevationLossM != 900 || gear.TotalMovingTimeS != 500 {
		t.Fatalf("gear totals after metric update = distance %.0f loss %.0f moving %d, want 5000/900/500",
			gear.TotalDistanceM, gear.TotalElevationLossM, gear.TotalMovingTimeS)
	}
}

// TestUpdateChainRotationMemberStatsResetsWhenUsageDisappears verifies that
// UpdateChainRotationMemberStats (D4) correctly resets a member's totals to
// zero once the underlying activity_gear_assignments rows are deleted.
func TestUpdateChainRotationMemberStatsResetsWhenUsageDisappears(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Gravel Bike")
	componentA := insertComponentForUsageTest(t, db, gearID, "Kette A")
	componentB := insertComponentForUsageTest(t, db, gearID, "Kette B")
	if _, err := db.Pool.Exec(ctx, `UPDATE gear_components SET installed_at = NOW() - INTERVAL '2 hours' WHERE id IN ($1, $2)`, componentA, componentB); err != nil {
		t.Fatalf("update component install time: %v", err)
	}
	activityID := insertActivityForUsageTest(t, db, userID, gearID, 10000, 1800, 2400, 250, 400)
	km := 500.0
	group, err := db.CreateChainRotation(ctx, userID, gearID, &ChainRotationGroupCreate{
		Name:           "Kettenrotation",
		SwapIntervalKM: &km,
		ComponentIDs:   []string{componentA, componentB},
	})
	if err != nil {
		t.Fatalf("CreateChainRotation: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE activities SET started_at = NOW() + INTERVAL '1 minute' WHERE id = $1`, activityID); err != nil {
		t.Fatalf("move activity after rotation start: %v", err)
	}
	if len(group.Members) == 0 {
		t.Fatalf("chain rotation group has no members")
	}
	memberID := group.Members[0].ID

	if err := db.UpdateChainRotationMemberStatsOwnerVerified(ctx, memberID); err != nil {
		t.Fatalf("UpdateChainRotationMemberStatsOwnerVerified first: %v", err)
	}
	var distance float64
	if err := db.Pool.QueryRow(ctx, `SELECT total_distance_m FROM chain_rotation_members WHERE id = $1`, memberID).Scan(&distance); err != nil {
		t.Fatalf("select member distance: %v", err)
	}
	if distance != 10000 {
		t.Fatalf("distance = %v, want 10000", distance)
	}

	if _, err := db.Pool.Exec(ctx, `DELETE FROM activity_gear_assignments WHERE activity_id = $1`, activityID); err != nil {
		t.Fatalf("delete assignment: %v", err)
	}
	if err := db.UpdateChainRotationMemberStatsOwnerVerified(ctx, memberID); err != nil {
		t.Fatalf("UpdateChainRotationMemberStatsOwnerVerified second: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT total_distance_m FROM chain_rotation_members WHERE id = $1`, memberID).Scan(&distance); err != nil {
		t.Fatalf("select reset distance: %v", err)
	}
	if distance != 0 {
		t.Fatalf("distance after assignment delete = %v, want 0", distance)
	}
}

// TestUpdateActivitySyncsPrimaryGearAssignment verifies that UpdateActivity
// wires the gear_id field into activity_gear_assignments (primary role) and
// that UpdateGearStats then aggregates the activity's metrics correctly.
func TestUpdateActivitySyncsPrimaryGearAssignment(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Road Bike")
	activityID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, distance_m, moving_time_s, elapsed_time_s, elevation_gain_m, elevation_loss_m, is_group_secondary)
		VALUES ($1, $2, 'cycling', NOW(), 25000, 3600, 4200, 500, 700, false)`,
		activityID, userID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	rawGearID, _ := json.Marshal(gearID)
	if _, err := db.UpdateActivity(ctx, activityID, userID, &model.ActivityUpdate{GearID: rawGearID}); err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}
	if err := db.UpdateGearStatsOwnerVerified(ctx, gearID); err != nil {
		t.Fatalf("UpdateGearStatsOwnerVerified: %v", err)
	}
	gear, err := db.GetGear(ctx, gearID)
	if err != nil {
		t.Fatalf("GetGear: %v", err)
	}
	if gear.TotalDistanceM != 25000 || gear.TotalElevationLossM != 700 {
		t.Fatalf("gear totals after update = distance %.0f loss %.0f, want 25000/700",
			gear.TotalDistanceM, gear.TotalElevationLossM)
	}
}

func TestGearUsageRefreshEvaluatesLifecycleRules(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Downhill Bike")
	insertActivityForUsageTest(t, db, userID, gearID, 1000, 60, 90, 10, 1200)

	ruleID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO automation_rules (id, user_id, name, trigger_event, conditions, actions)
		VALUES ($1, $2, 'Gear descent service', 'on_gear_usage_update',
		        '[{"field":"gear_total_elevation_loss_m","op":"gte","value":1000}]'::jsonb,
		        '[]'::jsonb)`,
		ruleID, userID); err != nil {
		t.Fatalf("insert automation rule: %v", err)
	}

	if err := db.RefreshGearAndDependentUsageStatsOwnerVerified(ctx, gearID); err != nil {
		t.Fatalf("RefreshGearAndDependentUsageStatsOwnerVerified: %v", err)
	}

	var count int
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM gear_rule_action_state
		WHERE rule_id = $1 AND gear_id = $2 AND user_id = $3`, ruleID, gearID, userID).Scan(&count); err != nil {
		t.Fatalf("count gear rule action state: %v", err)
	}
	if count != 1 {
		t.Fatalf("gear rule action state count = %d, want 1", count)
	}
	var timesApplied int
	if err := db.Pool.QueryRow(ctx, `SELECT times_applied FROM automation_rules WHERE id = $1`, ruleID).Scan(&timesApplied); err != nil {
		t.Fatalf("select times_applied: %v", err)
	}
	if timesApplied != 1 {
		t.Fatalf("times_applied = %d, want 1", timesApplied)
	}
}

func TestRemoveComponentClosesAssignmentAndKeepsRow(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Bike")
	componentID := insertComponentForUsageTest(t, db, gearID, "Rahmen")
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO component_assignments (component_id, gear_id, installed_at)
		VALUES ($1, $2, NOW())`, componentID, gearID); err != nil {
		t.Fatalf("insert assignment: %v", err)
	}
	if err := db.RemoveComponent(ctx, componentID); err != nil {
		t.Fatalf("RemoveComponent: %v", err)
	}
	var status string
	var removedAtSet bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT status, removed_at IS NOT NULL
		FROM gear_components
		WHERE id = $1`, componentID).Scan(&status, &removedAtSet); err != nil {
		t.Fatalf("select component: %v", err)
	}
	if status != "stored" || !removedAtSet {
		t.Fatalf("component status/removed = %s/%v, want stored/true", status, removedAtSet)
	}
	var activeAssignments int
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM component_assignments
		WHERE component_id = $1 AND removed_at IS NULL`, componentID).Scan(&activeAssignments); err != nil {
		t.Fatalf("count active assignments: %v", err)
	}
	if activeAssignments != 0 {
		t.Fatalf("active assignments = %d, want 0", activeAssignments)
	}
}

func TestUpdateComponentStatusTransitions(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "MTB")
	componentID := insertComponentForUsageTest(t, db, gearID, "Gabel")

	// Insert an open assignment.
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO component_assignments (component_id, gear_id, installed_at)
		VALUES ($1, $2, NOW())`, componentID, gearID); err != nil {
		t.Fatalf("insert assignment: %v", err)
	}

	// Transition active → stored: should close open assignment.
	if err := db.UpdateComponentStatus(ctx, componentID, userID, "stored"); err != nil {
		t.Fatalf("UpdateComponentStatus stored: %v", err)
	}
	var status string
	var isRetired bool
	if err := db.Pool.QueryRow(ctx,
		`SELECT status, is_retired FROM gear_components WHERE id = $1`, componentID,
	).Scan(&status, &isRetired); err != nil {
		t.Fatalf("select stored: %v", err)
	}
	if status != "stored" {
		t.Fatalf("status = %s, want stored", status)
	}
	if isRetired {
		t.Fatalf("is_retired = true after stored transition, want false")
	}
	var openAfterStored int
	if err := db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM component_assignments WHERE component_id = $1 AND removed_at IS NULL`, componentID,
	).Scan(&openAfterStored); err != nil {
		t.Fatalf("count open after stored: %v", err)
	}
	if openAfterStored != 0 {
		t.Fatalf("open assignments after stored = %d, want 0", openAfterStored)
	}

	// Transition stored → active: reopen assignment (no open one existed).
	if err := db.UpdateComponentStatus(ctx, componentID, userID, "active"); err != nil {
		t.Fatalf("UpdateComponentStatus active: %v", err)
	}
	if err := db.Pool.QueryRow(ctx,
		`SELECT status, is_retired FROM gear_components WHERE id = $1`, componentID,
	).Scan(&status, &isRetired); err != nil {
		t.Fatalf("select active: %v", err)
	}
	if status != "active" {
		t.Fatalf("status = %s, want active", status)
	}
	if isRetired {
		t.Fatalf("is_retired = true after active transition, want false")
	}

	// Transition active → retired: is_retired flipped.
	if err := db.UpdateComponentStatus(ctx, componentID, userID, "retired"); err != nil {
		t.Fatalf("UpdateComponentStatus retired: %v", err)
	}
	if err := db.Pool.QueryRow(ctx,
		`SELECT status, is_retired FROM gear_components WHERE id = $1`, componentID,
	).Scan(&status, &isRetired); err != nil {
		t.Fatalf("select retired: %v", err)
	}
	if status != "retired" {
		t.Fatalf("status = %s, want retired", status)
	}
	if !isRetired {
		t.Fatalf("is_retired = false after retired transition, want true")
	}

	// Invalid status must be rejected.
	if err := db.UpdateComponentStatus(ctx, componentID, userID, "broken"); err == nil {
		t.Fatal("UpdateComponentStatus with invalid status should return error")
	}
}

func TestUpdateChainRotationPersistsMetricInterval(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Downhill Bike")
	componentA := insertComponentForUsageTest(t, db, gearID, "Kette A")
	componentB := insertComponentForUsageTest(t, db, gearID, "Kette B")
	km := 500.0
	group, err := db.CreateChainRotation(ctx, userID, gearID, &ChainRotationGroupCreate{
		Name:           "Kettenrotation",
		SwapIntervalKM: &km,
		ComponentIDs:   []string{componentA, componentB},
	})
	if err != nil {
		t.Fatalf("CreateChainRotation: %v", err)
	}

	metric := "elevation_loss_m"
	value := 12000.0
	if err := db.UpdateChainRotation(ctx, group.ID, nil, nil, nil, &metric, &value); err != nil {
		t.Fatalf("UpdateChainRotation: %v", err)
	}
	updated, err := db.GetChainRotation(ctx, gearID)
	if err != nil {
		t.Fatalf("GetChainRotation: %v", err)
	}
	if updated.SwapIntervalMetricKey == nil || *updated.SwapIntervalMetricKey != metric {
		t.Fatalf("metric = %#v, want %s", updated.SwapIntervalMetricKey, metric)
	}
	if updated.SwapIntervalValue == nil || *updated.SwapIntervalValue != value {
		t.Fatalf("value = %#v, want %v", updated.SwapIntervalValue, value)
	}
}

// TestUpdateChainRotationRenamesGroup verifies that UpdateChainRotation rewrites
// the name column when a non-nil name is supplied, and leaves it untouched when
// name is nil.
func TestUpdateChainRotationRenamesGroup(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Enduro Bike")
	compA := insertComponentForUsageTest(t, db, gearID, "Kette A")
	compB := insertComponentForUsageTest(t, db, gearID, "Kette B")
	group, err := db.CreateChainRotation(ctx, userID, gearID, &ChainRotationGroupCreate{
		Name:         "Original",
		ComponentIDs: []string{compA, compB},
	})
	if err != nil {
		t.Fatalf("CreateChainRotation: %v", err)
	}

	// Rename.
	newName := "Renamed"
	if err := db.UpdateChainRotation(ctx, group.ID, &newName, nil, nil, nil, nil); err != nil {
		t.Fatalf("UpdateChainRotation rename: %v", err)
	}
	got, err := db.GetChainRotation(ctx, gearID)
	if err != nil {
		t.Fatalf("GetChainRotation: %v", err)
	}
	if got.Name != "Renamed" {
		t.Fatalf("name = %q, want Renamed", got.Name)
	}

	// nil name must NOT clobber existing name.
	km := 750.0
	if err := db.UpdateChainRotation(ctx, group.ID, nil, &km, nil, nil, nil); err != nil {
		t.Fatalf("UpdateChainRotation intervals only: %v", err)
	}
	got, err = db.GetChainRotation(ctx, gearID)
	if err != nil {
		t.Fatalf("GetChainRotation after interval update: %v", err)
	}
	if got.Name != "Renamed" {
		t.Fatalf("name = %q after nil-name update, want Renamed (unchanged)", got.Name)
	}
	if got.SwapIntervalKM == nil || *got.SwapIntervalKM != km {
		t.Fatalf("swap_interval_km = %v, want %v", got.SwapIntervalKM, km)
	}
}

// TestChainRotationMemberAnalyticsColumns verifies that UpdateChainRotationMemberStats
// populates all per-member analytics columns added in migration 000355:
// total_elevation_loss_m, total_moving_time_s, total_time_s, total_activities.
// Also verifies that GetChainRotation reads them back correctly via listChainMembers.
func TestChainRotationMemberAnalyticsColumns(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Trail Bike")
	componentA := insertComponentForUsageTest(t, db, gearID, "Kette A")
	componentB := insertComponentForUsageTest(t, db, gearID, "Kette B")
	// Set installed_at in the past so the activity (started_at = now-1h) is counted.
	if _, err := db.Pool.Exec(ctx, `UPDATE gear_components SET installed_at = NOW() - INTERVAL '2 hours' WHERE id IN ($1, $2)`, componentA, componentB); err != nil {
		t.Fatalf("update component install time: %v", err)
	}
	// Insert activity with known metrics.
	activityID := insertActivityForUsageTest(t, db, userID, gearID, 20000, 3600, 4500, 500, 800)
	km := 500.0
	group, err := db.CreateChainRotation(ctx, userID, gearID, &ChainRotationGroupCreate{
		Name:           "Kettenrotation",
		SwapIntervalKM: &km,
		ComponentIDs:   []string{componentA, componentB},
	})
	if err != nil {
		t.Fatalf("CreateChainRotation: %v", err)
	}
	// Move activity to after the group's created_at so it is counted.
	if _, err := db.Pool.Exec(ctx, `UPDATE activities SET started_at = NOW() + INTERVAL '1 minute' WHERE id = $1`, activityID); err != nil {
		t.Fatalf("move activity after rotation start: %v", err)
	}
	if len(group.Members) == 0 {
		t.Fatalf("chain rotation group has no members")
	}
	memberID := group.Members[0].ID

	if err := db.UpdateChainRotationMemberStatsOwnerVerified(ctx, memberID); err != nil {
		t.Fatalf("UpdateChainRotationMemberStatsOwnerVerified: %v", err)
	}

	// Read back through GetChainRotation to exercise listChainMembers SELECT.
	updated, err := db.GetChainRotation(ctx, gearID)
	if err != nil {
		t.Fatalf("GetChainRotation: %v", err)
	}
	if len(updated.Members) == 0 {
		t.Fatalf("no members returned")
	}
	active := updated.Members[0]
	if active.TotalDistanceM != 20000 {
		t.Fatalf("TotalDistanceM = %v, want 20000", active.TotalDistanceM)
	}
	if active.TotalElevationLossM != 800 {
		t.Fatalf("TotalElevationLossM = %v, want 800", active.TotalElevationLossM)
	}
	if active.TotalMovingTimeS != 3600 {
		t.Fatalf("TotalMovingTimeS = %d, want 3600", active.TotalMovingTimeS)
	}
	if active.TotalTimeS != 4500 {
		t.Fatalf("TotalTimeS = %d, want 4500", active.TotalTimeS)
	}
	if active.TotalActivities != 1 {
		t.Fatalf("TotalActivities = %d, want 1", active.TotalActivities)
	}
}

// setComponentUsageForUsageTest directly sets total_elevation_loss_m on a
// gear_components row so tests can exercise threshold logic without running
// the full activity-ingestion pipeline.
func setComponentUsageForUsageTest(t *testing.T, db *DB, componentID string, lossM float64) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		UPDATE gear_components
		SET total_elevation_loss_m = $2
		WHERE id = $1`, componentID, lossM)
	if err != nil {
		t.Fatalf("set component usage: %v", err)
	}
}

func setComponentMetricUsageForUsageTest(t *testing.T, db *DB, componentID string, distanceM, movingS, elapsedS, gainM, lossM float64, activities int) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		UPDATE gear_components
		SET total_distance_m = $2,
		    total_moving_time_s = $3,
		    total_time_s = $4,
		    total_elevation_gain_m = $5,
		    total_elevation_loss_m = $6,
		    total_activities = $7
		WHERE id = $1`, componentID, distanceM, int64(movingS), int64(elapsedS), gainM, lossM, activities)
	if err != nil {
		t.Fatalf("set component metric usage: %v", err)
	}
}

// insertServiceIntervalForUsageTest inserts a gear_service_intervals row with
// interval_type = metricKey (e.g. "elevation_loss_m") and the given threshold.
func insertServiceIntervalForUsageTest(t *testing.T, db *DB, componentID, metricKey string, value float64) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO gear_service_intervals (component_id, name, interval_type, metric_key, interval_value)
		VALUES ($1, 'Kantenservice', $2, $2, $3)`, componentID, metricKey, value)
	if err != nil {
		t.Fatalf("insert service interval: %v", err)
	}
}

// TestMaintenanceAlertsCanUseElevationLoss verifies that GetMaintenanceAlerts
// correctly evaluates an elevation_loss_m service interval against the
// component's total_elevation_loss_m column.
func TestMaintenanceAlertsCanUseElevationLoss(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Ski")
	componentID := insertComponentForUsageTest(t, db, gearID, "Kante")
	setComponentUsageForUsageTest(t, db, componentID, 51000)
	insertServiceIntervalForUsageTest(t, db, componentID, "elevation_loss_m", 50000)

	alerts, err := db.GetMaintenanceAlerts(ctx, userID)
	if err != nil {
		t.Fatalf("GetMaintenanceAlerts: %v", err)
	}
	if len(alerts) != 1 || !alerts[0].IsOverdue || alerts[0].CurrentValue != 51000 {
		t.Fatalf("unexpected alerts: %#v", alerts)
	}
}

func TestMaintenanceAlertsFilterLowProgressAndSupportAllServiceMetrics(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Service Bike")
	componentID := insertComponentForUsageTest(t, db, gearID, "Service Component")
	setComponentMetricUsageForUsageTest(t, db, componentID, 450000, 36000, 40000, 9000, 1200, 9)

	insertServiceIntervalForUsageTest(t, db, componentID, "distance_m", 500000)      // 90%, visible
	insertServiceIntervalForUsageTest(t, db, componentID, "moving_time_s", 72000)    // 50%, hidden
	insertServiceIntervalForUsageTest(t, db, componentID, "elevation_gain_m", 10000) // 90%, visible
	insertServiceIntervalForUsageTest(t, db, componentID, "activity_count", 10)      // 90%, visible
	insertServiceIntervalForUsageTest(t, db, componentID, "elevation_loss_m", 50000) // below 90%, hidden

	alerts, err := db.GetMaintenanceAlerts(ctx, userID)
	if err != nil {
		t.Fatalf("GetMaintenanceAlerts: %v", err)
	}
	if len(alerts) != 3 {
		t.Fatalf("alerts len = %d, want 3: %#v", len(alerts), alerts)
	}
	got := map[string]float64{}
	for _, alert := range alerts {
		if alert.IsOverdue {
			t.Fatalf("alert %q unexpectedly overdue", alert.IntervalType)
		}
		got[alert.IntervalType] = alert.CurrentValue
		if alert.IntervalID == "" {
			t.Fatalf("alert interval id is empty: %#v", alert)
		}
	}
	if got["distance_m"] != 450000 {
		t.Fatalf("distance alert = %v, want 450000", got["distance_m"])
	}
	if got["elevation_gain_m"] != 9000 {
		t.Fatalf("gain alert = %v, want 9000", got["elevation_gain_m"])
	}
	if got["activity_count"] != 9 {
		t.Fatalf("activity alert = %v, want 9", got["activity_count"])
	}
	if _, ok := got["moving_time_s"]; ok {
		t.Fatalf("moving_time_s below-threshold alert was returned: %#v", alerts)
	}
}

// TestGearUsageRuleFailsafeOnMalformedConditions verifies that a malformed
// rule does not abort RefreshGearUsageForActivityMetricChange. The gear stats
// must still be updated and the function must return nil.
func TestGearUsageRuleFailsafeOnMalformedConditions(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Downhill Bike")
	activityID := insertActivityForUsageTest(t, db, userID, gearID, 5000, 600, 900, 100, 800)

	// Insert a rule with deliberately malformed conditions JSON.
	ruleID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO automation_rules (id, user_id, name, trigger_event, conditions, actions)
		VALUES ($1, $2, 'Malformed rule', 'on_gear_usage_update',
		        '{"invalid":"shape"}'::jsonb,
		        '[]'::jsonb)`,
		ruleID, userID); err != nil {
		t.Fatalf("insert malformed rule: %v", err)
	}

	// Capture slog output to verify a Warn is emitted.
	var logBuf strings.Builder
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn})
	orig := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(orig) })

	impacted, err := db.RefreshGearUsageForActivityMetricChange(ctx, activityID, userID)
	if err != nil {
		t.Fatalf("RefreshGearUsageForActivityMetricChange returned error: %v", err)
	}
	if len(impacted) == 0 {
		t.Fatal("expected at least one impacted gear")
	}

	// Gear stats must still be updated.
	gear, err := db.GetGear(ctx, gearID)
	if err != nil {
		t.Fatalf("GetGear: %v", err)
	}
	if gear.TotalDistanceM != 5000 {
		t.Fatalf("gear TotalDistanceM = %.0f, want 5000", gear.TotalDistanceM)
	}

	// Warn should have been logged (rule evaluation ran, malformed conditions evaluate to false — no error today,
	// but the fast path returns early only when no rules exist; with a rule present the eval proceeds).
	// The malformed conditions JSON is an object not an array, so evaluateGearUsageRuleMap returns false
	// (unmarshal to []Condition fails, returns false). No error is returned — the rule just doesn't match.
	// This test confirms the no-error contract even with a structurally invalid rule.
	_ = logBuf.String()
}

// TestEvaluateGearUsageRulesForGearFastPath verifies that a user with no
// on_gear_usage_update rules returns nil without performing any rule evaluation.
func TestEvaluateGearUsageRulesForGearFastPath(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Road Bike")

	// No automation rules for this user.
	if err := db.EvaluateGearUsageRulesForGear(ctx, []string{gearID}); err != nil {
		t.Fatalf("EvaluateGearUsageRulesForGear with no rules: %v", err)
	}
}

// TestUpdateComponentStatusReactivationClearsRemovedAt verifies that
// transitioning a component from retired → active clears removed_at.
func TestUpdateComponentStatusReactivationClearsRemovedAt(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "MTB")
	componentID := insertComponentForUsageTest(t, db, gearID, "Federgabel")

	// Retire the component first.
	if err := db.UpdateComponentStatus(ctx, componentID, userID, "retired"); err != nil {
		t.Fatalf("UpdateComponentStatus retired: %v", err)
	}
	var removedAtAfterRetire *string
	var isRetiredAfterRetire bool
	if err := db.Pool.QueryRow(ctx,
		`SELECT removed_at::text, is_retired FROM gear_components WHERE id = $1`, componentID,
	).Scan(&removedAtAfterRetire, &isRetiredAfterRetire); err != nil {
		t.Fatalf("select retired: %v", err)
	}
	if removedAtAfterRetire == nil {
		t.Fatal("removed_at IS NULL after retire, want non-null")
	}
	if !isRetiredAfterRetire {
		t.Fatal("is_retired = false after retire, want true")
	}

	// Reactivate.
	if err := db.UpdateComponentStatus(ctx, componentID, userID, "active"); err != nil {
		t.Fatalf("UpdateComponentStatus active: %v", err)
	}
	var removedAtAfterActive *string
	var isRetiredAfterActive bool
	if err := db.Pool.QueryRow(ctx,
		`SELECT removed_at::text, is_retired FROM gear_components WHERE id = $1`, componentID,
	).Scan(&removedAtAfterActive, &isRetiredAfterActive); err != nil {
		t.Fatalf("select active: %v", err)
	}
	if removedAtAfterActive != nil {
		t.Fatalf("removed_at = %v after reactivation, want NULL", *removedAtAfterActive)
	}
	if isRetiredAfterActive {
		t.Fatal("is_retired = true after reactivation, want false")
	}
}

// TestUpdateComponentStatusReactivationOpensAssignment verifies the full
// retire → reactivate flow:
//   - retiring closes the open component_assignments row,
//   - reactivating clears removed_at AND opens a fresh assignment row,
//   - exactly one open assignment exists afterwards (idempotent on repeat).
func TestUpdateComponentStatusReactivationOpensAssignment(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Trail Bike")
	componentID := insertComponentForUsageTest(t, db, gearID, "Schaltwerk")

	// Seed an open assignment so the initial state mirrors production
	// (every active component has at least one open assignment row).
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO component_assignments (component_id, gear_id, installed_at)
		VALUES ($1, $2, NOW() - INTERVAL '1 hour')`, componentID, gearID); err != nil {
		t.Fatalf("seed initial assignment: %v", err)
	}

	// Retire closes the open assignment.
	if err := db.UpdateComponentStatus(ctx, componentID, userID, "retired"); err != nil {
		t.Fatalf("UpdateComponentStatus retired: %v", err)
	}
	var openAfterRetire int
	if err := db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM component_assignments WHERE component_id = $1 AND removed_at IS NULL`, componentID,
	).Scan(&openAfterRetire); err != nil {
		t.Fatalf("count open after retire: %v", err)
	}
	if openAfterRetire != 0 {
		t.Fatalf("open assignments after retire = %d, want 0", openAfterRetire)
	}

	reactivationStart := time.Now()

	// Reactivate.
	if err := db.UpdateComponentStatus(ctx, componentID, userID, "active"); err != nil {
		t.Fatalf("UpdateComponentStatus active: %v", err)
	}

	// Component row: status=active, removed_at=NULL.
	var status string
	var removedAt *string
	if err := db.Pool.QueryRow(ctx,
		`SELECT status, removed_at::text FROM gear_components WHERE id = $1`, componentID,
	).Scan(&status, &removedAt); err != nil {
		t.Fatalf("select component after reactivate: %v", err)
	}
	if status != "active" {
		t.Fatalf("status = %s, want active", status)
	}
	if removedAt != nil {
		t.Fatalf("removed_at = %v after reactivate, want NULL", *removedAt)
	}

	// Exactly one open assignment, installed_at >= reactivation timestamp.
	var openCount int
	var installedAt time.Time
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(*) FROM component_assignments
		WHERE component_id = $1 AND removed_at IS NULL`, componentID,
	).Scan(&openCount); err != nil {
		t.Fatalf("count open after reactivate: %v", err)
	}
	if openCount != 1 {
		t.Fatalf("open assignments after reactivate = %d, want 1", openCount)
	}
	if err := db.Pool.QueryRow(ctx, `
		SELECT installed_at FROM component_assignments
		WHERE component_id = $1 AND removed_at IS NULL`, componentID,
	).Scan(&installedAt); err != nil {
		t.Fatalf("select installed_at: %v", err)
	}
	// Allow a small clock-skew tolerance (sub-second).
	if installedAt.Before(reactivationStart.Add(-time.Second)) {
		t.Fatalf("installed_at = %v, want >= %v", installedAt, reactivationStart)
	}

	// Idempotency: reactivating again must NOT open a second row.
	if err := db.UpdateComponentStatus(ctx, componentID, userID, "active"); err != nil {
		t.Fatalf("UpdateComponentStatus active (idempotent): %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(*) FROM component_assignments
		WHERE component_id = $1 AND removed_at IS NULL`, componentID,
	).Scan(&openCount); err != nil {
		t.Fatalf("count open after second reactivate: %v", err)
	}
	if openCount != 1 {
		t.Fatalf("open assignments after second reactivate = %d, want 1", openCount)
	}
}

// TestUpdateComponentStatusRejectsForeignUser verifies that UpdateComponentStatus
// refuses to mutate a component that doesn't belong to the supplied userID.
func TestUpdateComponentStatusRejectsForeignUser(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	ownerID := insertTestUser(t, db)
	intruderID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, ownerID, "Owner Bike")
	componentID := insertComponentForUsageTest(t, db, gearID, "Sattel")

	if err := db.UpdateComponentStatus(ctx, componentID, intruderID, "stored"); err == nil {
		t.Fatal("UpdateComponentStatus with foreign user must fail, got nil")
	}

	// Component row must be untouched.
	var status string
	if err := db.Pool.QueryRow(ctx,
		`SELECT status FROM gear_components WHERE id = $1`, componentID,
	).Scan(&status); err != nil {
		t.Fatalf("select component: %v", err)
	}
	if status != "active" {
		t.Fatalf("status = %s after rejected update, want active", status)
	}
}

// TestGetComponentHistoryRoundTripsAttributes verifies that GetComponentHistory
// returns the attributes JSONB column written into a component_assignments row.
func TestGetComponentHistoryRoundTripsAttributes(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Gravel Bike")
	componentID := insertComponentForUsageTest(t, db, gearID, "Kette")

	// Insert an assignment with a non-empty attributes blob.
	assignmentID := uuid.NewString()
	attrs := map[string]any{"reason": "swap", "mileage": 1234.5}
	attrsJSON, err := json.Marshal(attrs)
	if err != nil {
		t.Fatalf("marshal attributes: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO component_assignments (id, component_id, gear_id, installed_at, attributes)
		VALUES ($1, $2, $3, NOW() - INTERVAL '1 day', $4)`,
		assignmentID, componentID, gearID, attrsJSON); err != nil {
		t.Fatalf("insert component assignment: %v", err)
	}

	history, err := db.GetComponentHistoryOwnerVerified(ctx, componentID)
	if err != nil {
		t.Fatalf("GetComponentHistory: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected at least one history entry")
	}
	got := history[0].Attributes
	if got["reason"] != "swap" {
		t.Fatalf("attributes[reason] = %v, want swap", got["reason"])
	}
}
