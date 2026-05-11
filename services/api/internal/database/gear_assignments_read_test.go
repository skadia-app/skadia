package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

// TestReadPathReturnsNullWhenActivityHasNoGear verifies that ListActivities with a
// GearID filter does not return activities that have no gear link at all.
func TestReadPathReturnsNullWhenActivityHasNoGear(t *testing.T) {
	db, userID := gearTestDB(t)
	ctx := context.Background()

	g, err := db.CreateGear(ctx, userID, &model.GearCreate{
		Name: "NullGear-" + userID[:8],
		Type: "bike",
	})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}

	// Activity with no gear link at all.
	actID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities
			(id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status, moving_time_s)
		VALUES
			($1, $2, 'cycling', 'no-gear-act', $3, $4, 'private', 'complete', 300)`,
		actID, userID, time.Now().UTC(), "hash-ng-"+actID[:8],
	); err != nil {
		t.Fatalf("insert activity without gear: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM activities WHERE id = $1`, actID)
	})

	list, err := db.ListActivities(ctx, userID, "", 50, "started_at", "desc",
		model.ActivityFilters{GearID: g.ID})
	if err != nil {
		t.Fatalf("ListActivities: %v", err)
	}

	for _, a := range list.Activities {
		if a.ID == actID {
			t.Errorf("activity with no gear link was incorrectly returned by GearID filter %s", g.ID)
		}
	}
}

// seedGearAndActivity inserts a user, a gear item, and a minimal activity, then
// links them via activity_gear_assignments (role_key='primary'). The
// activities.gear_id column was dropped in 000355's final block; aga is the
// sole authoritative source of activity-gear association.
func seedGearAndActivity(t *testing.T) (db *DB, userID, gearID, actID string) {
	t.Helper()
	db, userID = gearTestDB(t)
	ctx := context.Background()

	g, err := db.CreateGear(ctx, userID, &model.GearCreate{
		Name: "TestBike-" + userID[:8],
		Type: "bike",
	})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}
	gearID = g.ID

	// moving_time_s set so segment scans (which scan moving_time_s as int) don't fail.
	actID = uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities
			(id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status, moving_time_s)
		VALUES
			($1, $2, 'cycling', 'test-act', $3, $4, 'private', 'complete', 600)`,
		actID, userID, time.Now().UTC(), "hash-"+actID[:8],
	); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	// Link via assignment table only.
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_gear_assignments
			(user_id, activity_id, gear_id, role_key, source)
		VALUES
			($1, $2, $3, 'primary', 'test')`,
		userID, actID, gearID,
	); err != nil {
		t.Fatalf("insert activity_gear_assignment: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM activity_gear_assignments WHERE activity_id = $1`, actID)
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM activities WHERE id = $1`, actID)
	})

	return db, userID, gearID, actID
}

// TestUpdateGearStats_UsesAssignmentTable verifies that UpdateGearStats counts
// activities linked via activity_gear_assignments when activities.gear_id IS NULL.
func TestUpdateGearStats_UsesAssignmentTable(t *testing.T) {
	db, _, gearID, _ := seedGearAndActivity(t)
	ctx := context.Background()

	if err := db.UpdateGearStatsOwnerVerified(ctx, gearID); err != nil {
		t.Fatalf("UpdateGearStatsOwnerVerified: %v", err)
	}

	var totalActivities int
	if err := db.Pool.QueryRow(ctx,
		`SELECT total_activities FROM gear WHERE id = $1`, gearID,
	).Scan(&totalActivities); err != nil {
		t.Fatalf("scan total_activities: %v", err)
	}
	if totalActivities != 1 {
		t.Errorf("total_activities = %d, want 1 (activity linked via assignment, legacy column NULL)", totalActivities)
	}
}

// TestGetActivityAsMap_GearIDViaAssignment verifies that GetActivityAsMap
// returns the gear_id from activity_gear_assignments when activities.gear_id IS NULL.
func TestGetActivityAsMap_GearIDViaAssignment(t *testing.T) {
	db, _, gearID, actID := seedGearAndActivity(t)
	ctx := context.Background()

	m, err := db.GetActivityAsMap(ctx, actID)
	if err != nil {
		t.Fatalf("GetActivityAsMap: %v", err)
	}
	if m == nil {
		t.Fatal("GetActivityAsMap returned nil")
	}

	got, ok := m["gear_id"]
	if !ok {
		t.Fatalf("gear_id key missing from activity map (activities.gear_id is NULL, assignment exists for %s)", gearID)
	}
	if got != gearID {
		t.Errorf("gear_id = %v, want %s", got, gearID)
	}
}

// TestGetDistribution_GearViaAssignment verifies that GetDistribution("gear")
// groups activities by the gear from activity_gear_assignments.
func TestGetDistribution_GearViaAssignment(t *testing.T) {
	db, userID, _, _ := seedGearAndActivity(t)
	ctx := context.Background()

	rows, err := db.GetDistribution(ctx, userID, "gear", "count", "", nil)
	if err != nil {
		t.Fatalf("GetDistribution: %v", err)
	}

	var found bool
	for _, row := range rows {
		if row.Label == "TestBike-"+userID[:8] {
			found = true
			if row.Count < 1 {
				t.Errorf("gear distribution count = %d, want >= 1", row.Count)
			}
		}
	}
	if !found {
		t.Errorf("gear %q not found in distribution rows %v (activities.gear_id is NULL, linked via assignment)", "TestBike-"+userID[:8], rows)
	}
}

// TestGetActivity_GearIDViaAssignment verifies that GetActivity returns the
// gear_id from activity_gear_assignments when activities.gear_id IS NULL.
func TestGetActivity_GearIDViaAssignment(t *testing.T) {
	db, _, gearID, actID := seedGearAndActivity(t)
	ctx := context.Background()

	act, err := db.GetActivity(ctx, actID)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if act == nil {
		t.Fatal("GetActivity returned nil")
	}

	if act.GearID == nil {
		t.Fatalf("GearID is nil (activities.gear_id is NULL, assignment exists for %s)", gearID)
	}
	if *act.GearID != gearID {
		t.Errorf("GearID = %q, want %q", *act.GearID, gearID)
	}
}

// TestGetSegmentPerformances_GearViaAssignment verifies that GetSegmentPerformances
// returns gear info via activity_gear_assignments when activities.gear_id IS NULL.
func TestGetSegmentPerformances_GearViaAssignment(t *testing.T) {
	db, userID, gearID, actID := seedGearAndActivity(t)
	ctx := context.Background()

	// Insert a minimal segment and segment effort using the real schema
	// (segments.created_by, not user_id; segment_versions.version_number required).
	segID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type)
		VALUES ($1, $2, 'test-seg', 'bike', 'standard')`,
		segID, userID,
	); err != nil {
		t.Fatalf("insert segment: %v", err)
	}

	segVersionID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE, $3)`,
		segVersionID, segID, userID,
	); err != nil {
		t.Fatalf("insert segment_version: %v", err)
	}

	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type, in_leaderboard)
		VALUES
			($1, $2, $3, $4, $5, 120, 'cycling', 'generic', true)`,
		segID, segVersionID, actID, userID, time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert segment_effort: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segment_efforts WHERE activity_id = $1`, actID)
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segment_versions WHERE id = $1`, segVersionID)
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = $1`, segID)
	})

	perfs, err := db.GetSegmentPerformances(ctx, segID, userID, nil)
	if err != nil {
		t.Fatalf("GetSegmentPerformances: %v", err)
	}
	if len(perfs) == 0 {
		t.Fatal("GetSegmentPerformances returned empty slice")
	}

	p := perfs[0]
	if p.GearID == nil {
		t.Fatalf("GearID is nil (activities.gear_id is NULL, assignment exists for %s)", gearID)
	}
	if *p.GearID != gearID {
		t.Errorf("GearID = %q, want %q", *p.GearID, gearID)
	}
}

// TestListActivities_GearIDViaAssignment verifies that listing activities with a
// GearID filter includes activities linked via activity_gear_assignments when
// activities.gear_id IS NULL.
func TestListActivities_GearIDViaAssignment(t *testing.T) {
	db, userID, gearID, actID := seedGearAndActivity(t)
	ctx := context.Background()

	list, err := db.ListActivities(ctx, userID, "", 50, "started_at", "desc",
		model.ActivityFilters{GearID: gearID})
	if err != nil {
		t.Fatalf("ListActivities with gear filter: %v", err)
	}

	var found bool
	for _, a := range list.Activities {
		if a.ID == actID {
			found = true
		}
	}
	if !found {
		t.Errorf("activity %s not found by GearID filter %s (activities.gear_id is NULL, linked via assignment); total=%d",
			actID, gearID, list.Total)
	}
}
