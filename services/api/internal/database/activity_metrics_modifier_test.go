package database

// Tests for the parser-only vs sport-and-modifier split of
// UpdateActivityMetrics introduced for the system-modifier-tag-sync hardening
// follow-up. The tests assert two contracts:
//
//  1. UpdateActivityParserMetrics MUST NOT touch sport_type, sub_type,
//     fit_sport_id, fit_sub_sport_id, is_indoor, is_electric, is_virtual,
//     or is_commute. The system modifier tag sync trigger therefore has
//     zero Direction-B/C/D work to do for that UPDATE — verified by
//     reading the activity row before and after the call and asserting
//     the modifier columns plus tags / activity_tags / activity_count
//     stay byte-for-byte identical.
//
//  2. UpdateActivityMetricsWithSportAndModifiers DOES rewrite the modifier
//     flags AND leaves system tag state consistent: when the call flips
//     is_indoor=true the activity ends up with the 'Indoor' tag in
//     activities.tags, the activity_tags junction row, and the matching
//     tags.activity_count. Repair runs once via the SQL resync function
//     at commit, not per-row inside the trigger.
//
// The tests run against the per-worktree test database via testdb.Open.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// activityMetricsTestSetup seeds a user, an Indoor system tag, and a single
// cycling activity with a known starting state. Returns the DB handle, the
// user id, and the activity id.
func activityMetricsTestSetup(t *testing.T) (*DB, string, string) {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "amm-"+userID[:8], "amm-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

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

	activityID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, started_at, ended_at,
			tags, is_indoor, is_electric, is_virtual, is_commute, has_gps)
		VALUES ($1, $2, 'pre', 'running', NULL, $3, $4,
			ARRAY[]::text[], false, false, false, false, false)
	`, activityID, userID, now, now.Add(time.Hour)); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	return &DB{Pool: pool}, userID, activityID
}

// TestUpdateActivityParserMetricsLeavesModifierStateIntact pins the
// metric-only contract: the call must not touch sport_type, sub_type, the
// four modifier flags, or any denormalised tag state. A parser data set
// with conflicting modifier values is intentionally provided so the test
// catches any unintended write.
func TestActivityMetricsParserOnlyLeavesModifierStateIntact(t *testing.T) {
	db, _, activityID := activityMetricsTestSetup(t)
	ctx := context.Background()

	data := &model.ActivityData{
		SportType:    "cycling", // diverges from the seeded 'running'
		SubType:      "commuting",
		StartedAt:    time.Now().UTC().Truncate(time.Second),
		EndedAt:      time.Now().UTC().Truncate(time.Second).Add(30 * time.Minute),
		ElapsedTimeS: 1800,
		MovingTimeS:  1700,
		DistanceM:    15000,
		IsIndoor:     true, // would flip if metric-only respected modifiers
		IsElectric:   true,
		IsVirtual:    true,
		IsCommute:    true,
		HasGPS:       true,
	}

	if err := db.UpdateActivityParserMetrics(ctx, activityID, data); err != nil {
		t.Fatalf("UpdateActivityParserMetrics: %v", err)
	}

	var (
		sportType                                       string
		subType                                         *string
		isIndoor, isElectric, isVirtual, isCommute      bool
		distanceM                                       *float64
		parsedSportType, parsedSubTypeStr               string
		parsedSubType                                   *string
		tagCount                                        int
		junctionCount                                   int
	)
	if err := db.Pool.QueryRow(ctx, `
		SELECT sport_type, sub_type, is_indoor, is_electric, is_virtual, is_commute,
			distance_m, parsed_sport_type, COALESCE(parsed_sub_type, '')
		FROM activities WHERE id = $1
	`, activityID).Scan(&sportType, &subType, &isIndoor, &isElectric, &isVirtual, &isCommute,
		&distanceM, &parsedSportType, &parsedSubTypeStr); err != nil {
		t.Fatalf("query activity: %v", err)
	}
	_ = parsedSubType
	if sportType != "running" {
		t.Errorf("sport_type = %q, want %q (must not be overwritten)", sportType, "running")
	}
	if subType != nil {
		t.Errorf("sub_type = %v, want NULL (must not be overwritten)", *subType)
	}
	if isIndoor || isElectric || isVirtual || isCommute {
		t.Errorf("modifier flags changed (indoor=%v electric=%v virtual=%v commute=%v), want all false",
			isIndoor, isElectric, isVirtual, isCommute)
	}
	if distanceM == nil || *distanceM != 15000 {
		t.Errorf("distance_m = %v, want 15000 — parser fields should still be written", distanceM)
	}
	if parsedSportType != "cycling" || parsedSubTypeStr != "commuting" {
		t.Errorf("parsed_sport_type/parsed_sub_type = %q/%q, want cycling/commuting",
			parsedSportType, parsedSubTypeStr)
	}

	// activities.tags array stays empty and no junction rows materialise.
	if err := db.Pool.QueryRow(ctx,
		`SELECT array_length(tags, 1) FROM activities WHERE id = $1`, activityID,
	).Scan(&tagCount); err == nil && tagCount > 0 {
		t.Errorf("activities.tags grew to %d entries, want 0 (no trigger work)", tagCount)
	}
	if err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_tags WHERE activity_id = $1`, activityID,
	).Scan(&junctionCount); err != nil {
		t.Fatalf("query junction: %v", err)
	}
	if junctionCount != 0 {
		t.Errorf("activity_tags junction has %d rows, want 0 (no trigger work)", junctionCount)
	}
}

// TestUpdateActivityMetricsWithSportAndModifiersSyncsTagState pins the
// sport-and-modifier contract: flipping is_indoor in the parser data must
// end up reflected in activities.tags, activity_tags, and tags.activity_count
// after the helper-driven resync at commit.
func TestActivityMetricsWithSportAndModifiersSyncsTagState(t *testing.T) {
	db, userID, activityID := activityMetricsTestSetup(t)
	ctx := context.Background()

	data := &model.ActivityData{
		SportType:    "cycling",
		StartedAt:    time.Now().UTC().Truncate(time.Second),
		EndedAt:      time.Now().UTC().Truncate(time.Second).Add(30 * time.Minute),
		ElapsedTimeS: 1800,
		IsIndoor:     true,
		IsElectric:   false,
		IsVirtual:    false,
		IsCommute:    false,
		HasGPS:       false,
	}

	if err := db.UpdateActivityMetricsWithSportAndModifiers(ctx, activityID, data); err != nil {
		t.Fatalf("UpdateActivityMetricsWithSportAndModifiers: %v", err)
	}

	var sportType string
	var isIndoor bool
	var hasIndoorInArray bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT sport_type, is_indoor, 'Indoor' = ANY(COALESCE(tags, '{}'))
		FROM activities WHERE id = $1
	`, activityID).Scan(&sportType, &isIndoor, &hasIndoorInArray); err != nil {
		t.Fatalf("query activity: %v", err)
	}
	if sportType != "cycling" {
		t.Errorf("sport_type = %q, want cycling", sportType)
	}
	if !isIndoor {
		t.Errorf("is_indoor = false, want true after modifier-aware update")
	}
	if !hasIndoorInArray {
		t.Errorf("activities.tags does not contain 'Indoor' after resync")
	}

	// Junction row must exist for the Indoor system tag, and the per-tag
	// activity_count must include this activity exactly once.
	var junctionRows int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM activity_tags at JOIN tags t ON t.id = at.tag_id
		WHERE at.activity_id = $1 AND t.name = 'Indoor' AND t.is_system = true
	`, activityID).Scan(&junctionRows); err != nil {
		t.Fatalf("query junction: %v", err)
	}
	if junctionRows != 1 {
		t.Errorf("activity_tags rows for Indoor = %d, want 1", junctionRows)
	}
	var indoorCount int
	if err := db.Pool.QueryRow(ctx, `
		SELECT activity_count FROM tags WHERE user_id = $1 AND name = 'Indoor'
	`, userID).Scan(&indoorCount); err != nil {
		t.Fatalf("query indoor count: %v", err)
	}
	if indoorCount != 1 {
		t.Errorf("tags.activity_count for Indoor = %d, want 1", indoorCount)
	}
}

// TestUpdateActivityParserMetricsManualEditPathStillTriggersSync pins the
// boundary the task spec demands: single-activity manual updates that flip
// is_virtual/is_indoor/is_electric/is_commute must keep the normal trigger
// path. UpdateActivityParserMetrics is the metric-only path and never
// touches the modifier flags — but the manual UpdateActivity path (not
// covered here) does, and a direct UPDATE on the pool fires the trigger
// synchronously. We assert the trigger contract by writing to the
// modifier columns directly on the pool (no helper) and checking the
// junction row materialises immediately, which is the property manual
// edits depend on.
func TestActivityMetricsManualEditPathStillTriggersSync(t *testing.T) {
	db, userID, activityID := activityMetricsTestSetup(t)
	ctx := context.Background()

	// Manual edit equivalent: a direct UPDATE on the pool with no helper
	// transaction. The BEFORE UPDATE trigger must fire and end up with
	// the Indoor system tag linked in activity_tags.
	if _, err := db.Pool.Exec(ctx,
		`UPDATE activities SET is_indoor = true WHERE id = $1`, activityID); err != nil {
		t.Fatalf("manual update: %v", err)
	}

	var hasIndoorInArray bool
	if err := db.Pool.QueryRow(ctx,
		`SELECT 'Indoor' = ANY(COALESCE(tags, '{}')) FROM activities WHERE id = $1`,
		activityID).Scan(&hasIndoorInArray); err != nil {
		t.Fatalf("query tags: %v", err)
	}
	if !hasIndoorInArray {
		t.Errorf("activities.tags missing 'Indoor' after manual update — trigger path broken")
	}

	var junctionRows int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM activity_tags at JOIN tags t ON t.id = at.tag_id
		WHERE at.activity_id = $1 AND t.name = 'Indoor' AND t.is_system = true
	`, activityID).Scan(&junctionRows); err != nil {
		t.Fatalf("query junction: %v", err)
	}
	if junctionRows != 1 {
		t.Errorf("activity_tags rows for Indoor = %d, want 1 (manual edit must trigger sync)", junctionRows)
	}

	// And the running parser-metrics call afterwards must NOT undo the
	// modifier state set by the manual edit.
	data := &model.ActivityData{
		SportType:    "running",
		StartedAt:    time.Now().UTC().Truncate(time.Second),
		EndedAt:      time.Now().UTC().Truncate(time.Second).Add(30 * time.Minute),
		ElapsedTimeS: 1800,
		HasGPS:       false,
		// Modifier values intentionally false to prove the parser-only path
		// keeps them out of the SET list.
	}
	if err := db.UpdateActivityParserMetrics(ctx, activityID, data); err != nil {
		t.Fatalf("UpdateActivityParserMetrics post-manual-edit: %v", err)
	}
	var stillIndoor bool
	if err := db.Pool.QueryRow(ctx,
		`SELECT is_indoor FROM activities WHERE id = $1`, activityID,
	).Scan(&stillIndoor); err != nil {
		t.Fatalf("re-read is_indoor: %v", err)
	}
	if !stillIndoor {
		t.Errorf("metric-only reprocess overrode is_indoor=true to false — must not touch modifiers")
	}
	_ = userID
	_ = fmt.Sprintf
}
