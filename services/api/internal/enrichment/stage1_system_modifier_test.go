package enrichment

// Tests for the atomic system-modifier-tag block introduced for the
// system-modifier-tag-sync hardening follow-up. Run against the per-worktree
// test database via TEST_DATABASE_URL; skipped automatically when the env
// var is unset (testdb.RequireSafeDSN handles that gate).
//
//   - SystemModifierTagBlockSyncsCommuteIndoorAndEBike: pins the happy path.
//     A cycling activity with the FIT 'commuting' sub_type plus IsIndoor=true
//     plus IsElectric=true must end up with is_commute, is_indoor, is_electric
//     all true; activities.tags must contain Pendeln, Indoor, E-Bike;
//     activity_tags must have one junction per system tag; tags.activity_count
//     must equal 1 for each of those three system tags.
//
//   - SystemModifierTagBlockRollsBackOnInjectedFault: pins the atomicity
//     contract. With stage1ModifierTagFaultHook returning a non-nil error,
//     none of the modifier flags / tags array / junction rows / activity_count
//     fields may be observable as "applied" after Stage1 returns — the entire
//     helper transaction rolls back, leaving the pre-Stage1 state intact.
//
// The helpers in this file are self-contained — they do NOT reuse the
// integration-tagged helpers from stage1_test.go because the verification
// command for this task runs the targeted tests under the default build
// (no -tags=integration). Each helper skips the test if the per-worktree
// test database is unavailable via TEST_DATABASE_URL.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// stmtbPool opens the per-worktree test pool. Skips when TEST_DATABASE_URL
// is unset.
func stmtbPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Open(t)
}

// stmtbUser inserts a minimal user row + the four reserved system tags
// and registers cleanup of every row that this test creates.
func stmtbUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "stmtb-"+userID[:8], "stmtb-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	for _, tag := range []struct{ name, color string }{
		{"Indoor", "#3B82F6"},
		{"Pendeln", "#6B7280"},
		{"virtuell", "#8B5CF6"},
		{"E-Bike", "#16A34A"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto)
			 VALUES ($1, $2, $3, '#FFFFFF', 'system', true, false)
			 ON CONFLICT (user_id, name) DO NOTHING`,
			userID, tag.name, tag.color); err != nil {
			t.Fatalf("insert system tag %s: %v", tag.name, err)
		}
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = $1`, userID)
	})
	return userID
}

// stmtbActivity inserts a minimal cycling activity with all modifier flags
// false and an empty tags array.
func stmtbActivity(t *testing.T, pool *pgxpool.Pool, userID string) string {
	t.Helper()
	ctx := context.Background()
	activityID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at,
			tags, is_indoor, is_electric, is_virtual, is_commute, has_gps)
		VALUES ($1, $2, 'cycling', $3, $4, ARRAY[]::text[],
			false, false, false, false, false)
	`, activityID, userID, now, now.Add(30*time.Minute)); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return activityID
}

// TestStage1SystemModifierTagBlockSyncsCommuteIndoorAndEBike verifies the
// happy-path atomic block: cycling + commuting + IsIndoor + IsElectric must
// produce a fully consistent (modifier flag, tag array, junction row,
// activity_count) tuple after Stage1 returns.
func TestStage1SystemModifierTagBlockSyncsCommuteIndoorAndEBike(t *testing.T) {
	pool := stmtbPool(t)
	db := &database.DB{Pool: pool}
	userID := stmtbUser(t, pool)
	activityID := stmtbActivity(t, pool, userID)

	// HasGPS=false so RunStage1 takes neither the weather nor the geocoder
	// path. Stage1Deps can stay nil.
	startedAt := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	data := &model.ActivityData{
		SportType:    "cycling",
		SubType:      "commuting",
		StartedAt:    startedAt,
		EndedAt:      startedAt.Add(30 * time.Minute),
		ElapsedTimeS: 1800,
		HasGPS:       false,
		IsIndoor:     true,
		IsElectric:   true,
		IsVirtual:    false,
	}

	RunStage1(context.Background(), db, userID, activityID, data,
		nil, false, true, nil, true, nil)

	ctx := context.Background()
	var (
		isCommute, isIndoor, isElectric    bool
		hasPendeln, hasIndoorTag, hasEBike bool
	)
	if err := pool.QueryRow(ctx, `
		SELECT is_commute, is_indoor, is_electric,
			'Pendeln' = ANY(COALESCE(tags, '{}')),
			'Indoor'  = ANY(COALESCE(tags, '{}')),
			'E-Bike'  = ANY(COALESCE(tags, '{}'))
		FROM activities WHERE id = $1
	`, activityID).Scan(&isCommute, &isIndoor, &isElectric,
		&hasPendeln, &hasIndoorTag, &hasEBike); err != nil {
		t.Fatalf("read activity state: %v", err)
	}
	if !isCommute {
		t.Errorf("is_commute = false; want true after cycling+commuting")
	}
	if !isIndoor {
		t.Errorf("is_indoor = false; want true")
	}
	if !isElectric {
		t.Errorf("is_electric = false; want true")
	}
	if !hasPendeln || !hasIndoorTag || !hasEBike {
		t.Errorf("tags array missing system tags: pendeln=%v indoor=%v ebike=%v",
			hasPendeln, hasIndoorTag, hasEBike)
	}

	for _, tagName := range []string{"Pendeln", "Indoor", "E-Bike"} {
		var junctionRows int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM activity_tags at JOIN tags t ON t.id = at.tag_id
			WHERE at.activity_id = $1 AND t.name = $2 AND t.is_system = true
		`, activityID, tagName).Scan(&junctionRows); err != nil {
			t.Fatalf("query junction for %s: %v", tagName, err)
		}
		if junctionRows != 1 {
			t.Errorf("activity_tags rows for %s = %d, want 1", tagName, junctionRows)
		}

		var activityCount int
		if err := pool.QueryRow(ctx, `
			SELECT activity_count FROM tags WHERE user_id = $1 AND name = $2
		`, userID, tagName).Scan(&activityCount); err != nil {
			t.Fatalf("query activity_count for %s: %v", tagName, err)
		}
		if activityCount != 1 {
			t.Errorf("tags.activity_count for %s = %d, want 1", tagName, activityCount)
		}
	}
}

// TestStage1SystemModifierTagBlockRollsBackOnInjectedFault uses the
// stage1ModifierTagFaultHook seam to inject an error mid-block and asserts
// that none of the writes survive: modifier flags stay false, tags array
// stays empty, no junction rows are created, and the per-tag
// activity_count stays at 0.
func TestStage1SystemModifierTagBlockRollsBackOnInjectedFault(t *testing.T) {
	pool := stmtbPool(t)
	db := &database.DB{Pool: pool}
	userID := stmtbUser(t, pool)
	activityID := stmtbActivity(t, pool, userID)

	// Install the fault hook for the duration of this test only. The hook
	// is stored as an atomic.Pointer in production code; we Store a typed
	// closure and restore the previous value on cleanup so concurrent tests
	// in this package never race against the read in runStage1ModifierTagBlock.
	prev := stage1ModifierTagFaultHook.Load()
	hook := stage1ModifierTagFault(func(ctx context.Context, tx pgx.Tx, gotActivityID string) error {
		if gotActivityID != activityID {
			t.Errorf("hook activity_id = %q, want %q", gotActivityID, activityID)
		}
		return errors.New("injected stage1 modifier-tag fault")
	})
	stage1ModifierTagFaultHook.Store(&hook)
	t.Cleanup(func() { stage1ModifierTagFaultHook.Store(prev) })

	startedAt := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	data := &model.ActivityData{
		SportType:    "cycling",
		SubType:      "commuting",
		StartedAt:    startedAt,
		EndedAt:      startedAt.Add(30 * time.Minute),
		ElapsedTimeS: 1800,
		HasGPS:       false,
		IsIndoor:     true,
		IsElectric:   true,
	}

	RunStage1(context.Background(), db, userID, activityID, data,
		nil, false, true, nil, true, nil)

	ctx := context.Background()
	var (
		isCommute, isIndoor, isElectric bool
		tagsLen                         int
	)
	if err := pool.QueryRow(ctx, `
		SELECT is_commute, is_indoor, is_electric, COALESCE(array_length(tags, 1), 0)
		FROM activities WHERE id = $1
	`, activityID).Scan(&isCommute, &isIndoor, &isElectric, &tagsLen); err != nil {
		t.Fatalf("read activity state: %v", err)
	}
	if isCommute || isIndoor || isElectric {
		t.Errorf("modifier flags partially applied after injected fault: commute=%v indoor=%v electric=%v",
			isCommute, isIndoor, isElectric)
	}
	if tagsLen != 0 {
		t.Errorf("activities.tags has %d entries after rollback, want 0", tagsLen)
	}

	for _, tagName := range []string{"Pendeln", "Indoor", "E-Bike"} {
		var junctionRows int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM activity_tags at JOIN tags t ON t.id = at.tag_id
			WHERE at.activity_id = $1 AND t.name = $2
		`, activityID, tagName).Scan(&junctionRows); err != nil {
			t.Fatalf("query junction for %s: %v", tagName, err)
		}
		if junctionRows != 0 {
			t.Errorf("activity_tags rows for %s = %d after rollback, want 0", tagName, junctionRows)
		}

		var activityCount int
		if err := pool.QueryRow(ctx, `
			SELECT activity_count FROM tags WHERE user_id = $1 AND name = $2
		`, userID, tagName).Scan(&activityCount); err != nil {
			t.Fatalf("query activity_count for %s: %v", tagName, err)
		}
		if activityCount != 0 {
			t.Errorf("tags.activity_count for %s = %d after rollback, want 0", tagName, activityCount)
		}
	}

	// Sanity: the activity row still exists (the tx rolled back the writes
	// inside Stage1, not the seed inserts).
	var stillThere bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM activities WHERE id = $1)`, activityID,
	).Scan(&stillThere); err != nil {
		t.Fatalf("existence check: %v", err)
	}
	if !stillThere {
		t.Fatalf("activity row disappeared after rollback")
	}

	_ = uuid.Nil
}
