package database

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

// TestGearFilterSQL_AgaOnly checks that buildFilterSQL for GearID produces an
// EXISTS subquery rooted at activity_gear_assignments. The legacy
// activities.gear_id column was dropped in 000355's final block; any remaining
// reference in the generated SQL would crash at runtime.
func TestGearFilterSQL_AgaOnly(t *testing.T) {
	clause, _ := buildFilterSQL(model.ActivityFilters{GearID: uuid.NewString()}, 1)

	if strings.Contains(clause, "activities.gear_id") {
		t.Errorf("buildFilterSQL GearID clause still references the dropped activities.gear_id column:\n%s", clause)
	}
	if !strings.Contains(clause, "EXISTS") {
		t.Errorf("buildFilterSQL GearID clause missing EXISTS subquery:\n%s", clause)
	}
	if !strings.Contains(clause, "activity_gear_assignments") {
		t.Errorf("buildFilterSQL GearID clause missing activity_gear_assignments reference:\n%s", clause)
	}
}

// seedCrossUserGear creates two independent users each with their own gear and
// an activity linked to it via activity_gear_assignments.  Returns both (db,
// userAID, gearAID, actAID) and (userBID, gearBID).
func seedCrossUserGear(t *testing.T) (db *DB, userAID, gearAID, actAID, userBID, gearBID string) {
	t.Helper()
	db, userAID = gearTestDB(t)
	ctx := context.Background()

	// Create user B independently (gearTestDB only creates one user).
	userBID = uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userBID, "xuser-"+userBID[:8], "xuser-"+userBID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user B: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userBID)
	})

	// Gear for user A.
	gA, err := db.CreateGear(ctx, userAID, &model.GearCreate{Name: "UserA-Bike", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear A: %v", err)
	}
	gearAID = gA.ID

	// Gear for user B.
	gB, err := db.CreateGear(ctx, userBID, &model.GearCreate{Name: "UserB-Bike", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear B: %v", err)
	}
	gearBID = gB.ID

	// Activity for user A, linked to gearA via aga.
	actAID = uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities
			(id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status, moving_time_s)
		VALUES
			($1, $2, 'cycling', 'xuser-act', $3, $4, 'private', 'complete', 600)`,
		actAID, userAID, time.Now().UTC(), "hash-xu-"+actAID[:8],
	); err != nil {
		t.Fatalf("insert activity A: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_gear_assignments (user_id, activity_id, gear_id, role_key, source)
		VALUES ($1, $2, $3, 'primary', 'test')`,
		userAID, actAID, gearAID,
	); err != nil {
		t.Fatalf("insert aga for user A: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM activity_gear_assignments WHERE activity_id = $1`, actAID)
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM activities WHERE id = $1`, actAID)
	})

	return db, userAID, gearAID, actAID, userBID, gearBID
}

// TestListActivities_GearFilter_CrossUserIsolation verifies that user B cannot
// retrieve user A's activity by filtering with user A's gear ID.  The gear JOIN
// and aga user_id constraint must prevent cross-tenant data leaks.
func TestListActivities_GearFilter_CrossUserIsolation(t *testing.T) {
	db, _, gearAID, actAID, userBID, _ := seedCrossUserGear(t)
	ctx := context.Background()

	// Query as user B using user A's gear ID — must return 0 results.
	list, err := db.ListActivities(ctx, userBID, "", 50, "started_at", "desc",
		model.ActivityFilters{GearID: gearAID})
	if err != nil {
		t.Fatalf("ListActivities as user B: %v", err)
	}
	for _, a := range list.Activities {
		if a.ID == actAID {
			t.Errorf("cross-user contamination: activity %s (user A) returned for user B query with gearID %s", actAID, gearAID)
		}
	}
}

// TestGetDistribution_GearCrossUserIsolation verifies that GetDistribution("gear")
// for user B never groups in user A's gear even when user A's gear ID happens to
// match a fabricated lookup by user B.
func TestGetDistribution_GearCrossUserIsolation(t *testing.T) {
	db, _, _, _, userBID, _ := seedCrossUserGear(t)
	ctx := context.Background()

	rows, err := db.GetDistribution(ctx, userBID, "gear", "count", "", nil)
	if err != nil {
		t.Fatalf("GetDistribution as user B: %v", err)
	}
	for _, row := range rows {
		if row.Label == "UserA-Bike" {
			t.Errorf("cross-user contamination: UserA-Bike appeared in user B's gear distribution")
		}
	}
}

// TestGetSegmentPerformances_GearCrossUserIsolation verifies that segment
// performances returned for user A's segment effort carry gear info only from
// user A's own aga rows — not from a fabricated cross-user aga row.
func TestGetSegmentPerformances_GearCrossUserIsolation(t *testing.T) {
	db, userAID, gearAID, actAID, userBID, gearBID := seedCrossUserGear(t)
	ctx := context.Background()

	// Insert a segment owned by user A.
	segID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type)
		VALUES ($1, $2, 'xuser-seg', 'bike', 'standard')`,
		segID, userAID,
	); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	segVersionID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE, $3)`,
		segVersionID, segID, userAID,
	); err != nil {
		t.Fatalf("insert segment_version: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type, in_leaderboard)
		VALUES
			($1, $2, $3, $4, $5, 120, 'cycling', 'generic', true)`,
		segID, segVersionID, actAID, userAID, time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert segment_effort: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segment_efforts WHERE activity_id = $1`, actAID)
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segment_versions WHERE id = $1`, segVersionID)
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = $1`, segID)
	})

	// Attempt to inject a cross-user aga row (user B's gear on user A's activity).
	// The activity_gear_assignments_user_match trigger should block it; if not,
	// the JOIN with AND aga.user_id = a.user_id in GetSegmentPerformances must
	// exclude it anyway.
	_, _ = db.Pool.Exec(ctx, `
		INSERT INTO activity_gear_assignments (user_id, activity_id, gear_id, role_key, source)
		VALUES ($1, $2, $3, 'primary', 'test')
		ON CONFLICT DO NOTHING`,
		userBID, actAID, gearBID,
	)

	perfs, err := db.GetSegmentPerformances(ctx, segID, userAID, nil)
	if err != nil {
		t.Fatalf("GetSegmentPerformances: %v", err)
	}
	if len(perfs) == 0 {
		t.Fatal("GetSegmentPerformances returned empty — expected one effort")
	}

	p := perfs[0]
	// Gear must be user A's gear, not user B's.
	if p.GearID != nil && *p.GearID == gearBID {
		t.Errorf("cross-user gear leaked into segment performance: got gear %s (user B), want gear %s (user A)", gearBID, gearAID)
	}
	if p.GearID == nil {
		t.Errorf("GearID is nil for user A's performance; expected gear %s", gearAID)
	} else if *p.GearID != gearAID {
		t.Errorf("GearID = %q, want %q", *p.GearID, gearAID)
	}
	_ = gearBID // suppress unused warning
}
