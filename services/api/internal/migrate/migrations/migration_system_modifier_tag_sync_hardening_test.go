package migrations_test

// Integration tests for the system modifier tag sync hardening migration
// (000353_system_modifier_tag_sync_hardening).
//
// The migration introduces:
//   - skadia_resync_system_modifier_tags(uuid[]) — set-based repair function
//   - skip-gated trigger bodies that bypass outbound modifier-flag→tag work
//     when skadia.skip_system_modifier_tag_sync is true, while still keeping
//     inbound activities.tags→modifier flag derivation intact
//   - cycling-only E-Bike preservation
//   - one btree index covering count/delete by tag_id on activity_tags
//
// Tests share a deterministic fixture covering every registry-backed tag_sync
// modifier (is_indoor / is_commute / is_virtual / cycling-only is_electric)
// plus cross-user manual-name collisions and intentionally damaged junction,
// array and count rows.

import (
	"context"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func readSystemModifierTagSyncMigrationSQL(t *testing.T) (upSQL, downSQL string) {
	t.Helper()
	upBytes, err := os.ReadFile("000353_system_modifier_tag_sync_hardening.up.sql")
	if err != nil {
		t.Fatalf("read up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000353_system_modifier_tag_sync_hardening.down.sql")
	if err != nil {
		t.Fatalf("read down.sql: %v", err)
	}
	return string(upBytes), string(downBytes)
}

// TestMigration_SystemModifierTagSyncDeclaresRequiredContracts is a
// structural, file-only check: every required SQL marker the integration
// tests rely on must appear in the up.sql before we even spin up the DB.
// Catches typos and copy-paste mistakes faster than a DB error.
func TestMigration_SystemModifierTagSyncDeclaresRequiredContracts(t *testing.T) {
	upSQL, downSQL := readSystemModifierTagSyncMigrationSQL(t)
	for _, needle := range []string{
		"CREATE OR REPLACE FUNCTION skadia_resync_system_modifier_tags",
		"p_activity_ids UUID[]",
		"CREATE INDEX IF NOT EXISTS idx_activity_tags_tag_id_activity_id",
		"CREATE OR REPLACE FUNCTION sync_system_flags_and_tags()",
		"CREATE OR REPLACE FUNCTION sync_system_flags_and_tags_insert_junction()",
		"CREATE OR REPLACE FUNCTION sync_virtual_tag()",
		"CREATE OR REPLACE FUNCTION sync_virtual_tag_insert()",
		"CREATE OR REPLACE FUNCTION sync_virtual_tag_insert_junction()",
		"skadia.skip_system_modifier_tag_sync",
	} {
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("up migration missing %q", needle)
		}
	}
	for _, needle := range []string{
		"DROP FUNCTION IF EXISTS skadia_resync_system_modifier_tags",
		"DROP INDEX IF EXISTS idx_activity_tags_tag_id_activity_id",
		"CREATE OR REPLACE FUNCTION sync_system_flags_and_tags()",
		"CREATE OR REPLACE FUNCTION sync_virtual_tag()",
	} {
		if !strings.Contains(downSQL, needle) {
			t.Fatalf("down migration missing %q", needle)
		}
	}
	// "DELETE FROM activity_tags" appears legitimately inside the restored
	// trigger function bodies (per-row outbound cleanup). The forbidden
	// patterns below catch only blanket bulk data destruction at file scope.
	for _, forbidden := range []string{
		"DROP TABLE",
		"TRUNCATE",
		"DELETE FROM activities ",
		"DELETE FROM activity_tag_removals ",
		"DELETE FROM tags ",
	} {
		if strings.Contains(downSQL, forbidden) {
			t.Fatalf("down migration must not destroy data, found %q", forbidden)
		}
	}
}

// systemModifierTagSyncFixture pins every row the resync tests touch.
// Each test uses one transaction with a Cleanup-Rollback so changes never
// leak into the shared per-worktree test DB.
type systemModifierTagSyncFixture struct {
	user1ID string
	user2ID string

	// user1 system tags. user1's "Pendeln" is intentionally created as a
	// manual tag with the reserved system name, to force resync to promote
	// it instead of inserting a duplicate.
	indoorTagID    string
	pendelnTagID   string
	virtuellTagID  string
	eBikeTagID     string
	unrelatedTagID string

	// user2's manual tag with a name colliding with one of user1's system
	// tags. resync MUST NOT touch this row even though the name matches.
	user2EBikeManualTagID string

	// user1 activities, one per modifier scenario.
	cyclingElectricID string // sport_type=cycling,    is_electric=true
	runningElectricID string // sport_type=running,    is_electric=true (no E-Bike)
	virtualID         string // is_virtual=true
	commuteID         string // is_commute=true
	indoorID          string // is_indoor=true
}

// makeSystemModifierTagSyncFixture assembles the deterministic two-user
// scenario described in the plan. Returns the IDs the assertions reference.
//
// It also exercises the mid-resync invariant that an already-linked virtual
// activity still has a working junction row before damage runs, so cleanup
// ordering proves it does not trip the existing delete-protection trigger.
func makeSystemModifierTagSyncFixture(t *testing.T, ctx context.Context, tx pgx.Tx) systemModifierTagSyncFixture {
	t.Helper()
	f := systemModifierTagSyncFixture{
		user1ID:               uuid.NewString(),
		user2ID:               uuid.NewString(),
		cyclingElectricID:     uuid.NewString(),
		runningElectricID:     uuid.NewString(),
		virtualID:             uuid.NewString(),
		commuteID:             uuid.NewString(),
		indoorID:              uuid.NewString(),
	}

	// Users.
	for _, u := range []struct{ id, prefix string }{
		{f.user1ID, "mig999_u1_"},
		{f.user2ID, "mig999_u2_"},
	} {
		short := u.id[:8]
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, u.id, u.prefix+short, u.prefix+short+"@example.com"); err != nil {
			t.Fatalf("insert user %s: %v", u.id, err)
		}
	}

	// user1 system tags. Indoor / virtuell / E-Bike start as is_system=true so
	// the resync verifies repair-without-promotion. "Pendeln" starts as a
	// MANUAL tag for user1 so the resync verifies promotion instead of a
	// duplicate row insert (tags is unique on (user_id, name)).
	if err := tx.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
		VALUES ($1, 'Indoor', '#3B82F6', '#FFFFFF', 'system', true, false, 'Automatisch bei Indoor-Aktivitäten', 0)
		RETURNING id
	`, f.user1ID).Scan(&f.indoorTagID); err != nil {
		t.Fatalf("insert user1 Indoor tag: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
		VALUES ($1, 'Pendeln', '#111111', '#FFFFFF', 'custom', false, true, 'pre-existing user pendeln', 0)
		RETURNING id
	`, f.user1ID).Scan(&f.pendelnTagID); err != nil {
		t.Fatalf("insert user1 Pendeln (manual) tag: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
		VALUES ($1, 'virtuell', '#8B5CF6', '#FFFFFF', 'system', true, false, 'Automatisch bei virtuellen Aktivitäten', 0)
		RETURNING id
	`, f.user1ID).Scan(&f.virtuellTagID); err != nil {
		t.Fatalf("insert user1 virtuell tag: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
		VALUES ($1, 'E-Bike', '#16A34A', '#FFFFFF', 'system', true, false, 'Automatisch bei elektrischen Radfahrten', 0)
		RETURNING id
	`, f.user1ID).Scan(&f.eBikeTagID); err != nil {
		t.Fatalf("insert user1 E-Bike tag: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
		VALUES ($1, 'Workout', '#FF00FF', '#FFFFFF', 'custom', false, false, 'unrelated', 0)
		RETURNING id
	`, f.user1ID).Scan(&f.unrelatedTagID); err != nil {
		t.Fatalf("insert user1 unrelated tag: %v", err)
	}

	// user2 manual tag with a name matching one of user1's system tags.
	if err := tx.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
		VALUES ($1, 'E-Bike', '#222222', '#FFFFFF', 'custom', false, false, 'user2 manual e-bike', 0)
		RETURNING id
	`, f.user2ID).Scan(&f.user2EBikeManualTagID); err != nil {
		t.Fatalf("insert user2 manual E-Bike tag: %v", err)
	}

	// user1 activities — INSERT triggers will populate tag arrays and
	// junction rows; the damage step below tears them apart again.
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_indoor, is_electric, is_virtual, is_commute, started_at)
		VALUES
			($1, $6, 'cycling electric',  'cycling',   NULL, ARRAY[]::text[], false, true,  false, false, now()),
			($2, $6, 'running electric',  'running',   NULL, ARRAY[]::text[], false, true,  false, false, now()),
			($3, $6, 'virtual indoor',    'cycling',   NULL, ARRAY[]::text[], false, false, true,  false, now()),
			($4, $6, 'commute ride',      'cycling',   NULL, ARRAY[]::text[], false, false, false, true,  now()),
			($5, $6, 'indoor row',        'rowing',    NULL, ARRAY[]::text[], true,  false, false, false, now())
	`, f.cyclingElectricID, f.runningElectricID, f.virtualID, f.commuteID, f.indoorID, f.user1ID); err != nil {
		t.Fatalf("insert user1 activities: %v", err)
	}

	// At this point the existing triggers will have linked Indoor / virtuell /
	// E-Bike for the activities that match. user1's Pendeln is still manual,
	// so the existing trigger could not link the commute activity to it.

	return f
}

// damageSystemModifierState breaks every flavour of denormalized row the
// resync is supposed to repair. Importantly it does NOT delete the existing
// virtuell junction row while is_virtual=true — the prevent_virtual_tag_removal
// trigger would block that, and the plan calls out that the resync repair
// order must add missing links first and only delete after is_virtual=false.
//
// Step 2 disables the system-modifier triggers locally so the tag-array
// stripping does NOT also clear the modifier flags via Direction A. After
// the damage step we re-enable the triggers; Resync is what proves the
// denormalized state is repairable from the modifier flags alone.
func damageSystemModifierState(t *testing.T, ctx context.Context, tx pgx.Tx, f systemModifierTagSyncFixture) {
	t.Helper()

	// 1. Drop every link except virtuell↔virtual (delete-protection trigger).
	//    For Indoor we want to start with a missing junction row + missing
	//    array entry, plus a stale removal row even though is_indoor=true.
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_tags at
		USING tags t
		WHERE at.tag_id = t.id
		  AND t.user_id = $1
		  AND t.name IN ('Indoor', 'E-Bike')
		  AND at.activity_id IN ($2, $3, $4)
	`, f.user1ID, f.cyclingElectricID, f.runningElectricID, f.indoorID); err != nil {
		t.Fatalf("damage: delete junctions: %v", err)
	}

	// 2. Strip names from activities.tags arrays without letting Direction A
	//    derive the modifier flags away from the values the fixture set up.
	//    Triggers stay disabled only for the duration of the bulk strip; they
	//    are re-enabled before the test calls the resync function.
	for _, stmt := range []string{
		`ALTER TABLE activities DISABLE TRIGGER trg_sync_system_flags_and_tags`,
		`ALTER TABLE activities DISABLE TRIGGER trg_sync_virtual_tag`,
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			t.Fatalf("damage: disable trigger: %v", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activities SET tags = ARRAY[]::text[]
		WHERE id IN ($1, $2, $3, $4, $5)
	`, f.cyclingElectricID, f.runningElectricID, f.virtualID, f.commuteID, f.indoorID); err != nil {
		t.Fatalf("damage: strip tag arrays: %v", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE activities ENABLE TRIGGER trg_sync_system_flags_and_tags`,
		`ALTER TABLE activities ENABLE TRIGGER trg_sync_virtual_tag`,
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			t.Fatalf("damage: re-enable trigger: %v", err)
		}
	}

	// 3. Insert stale activity_tag_removals for an Indoor activity whose
	//    is_indoor flag is true. The resync must clear that row so future
	//    sync paths are not blocked.
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_tag_removals (activity_id, tag_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, f.indoorID, f.indoorTagID); err != nil {
		t.Fatalf("damage: insert stale removal: %v", err)
	}

	// 4. Stale activity_count values that the resync recomputes. Includes
	//    the user2 E-Bike manual tag, which the resync MUST NOT touch.
	if _, err := tx.Exec(ctx, `
		UPDATE tags SET activity_count = 9999
		WHERE id IN ($1, $2, $3, $4, $5, $6, $7)
	`,
		f.indoorTagID,
		f.pendelnTagID,
		f.virtuellTagID,
		f.eBikeTagID,
		f.unrelatedTagID,
		f.user2EBikeManualTagID,
		f.user2EBikeManualTagID, // duplicate id is fine, makes len() consistent
	); err != nil {
		t.Fatalf("damage: stale counts: %v", err)
	}
}

func beginMigrationTx(t *testing.T) (context.Context, pgx.Tx) {
	t.Helper()
	pool := migrationTestPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})
	return ctx, tx
}

func callResync(t *testing.T, ctx context.Context, tx pgx.Tx, ids ...string) {
	t.Helper()
	if _, err := tx.Exec(ctx, `SELECT skadia_resync_system_modifier_tags($1::uuid[])`, ids); err != nil {
		t.Fatalf("call skadia_resync_system_modifier_tags: %v", err)
	}
}

// hasSystemTagLink asserts whether (activity_id, system_tag_with_name) is
// present in activity_tags for the given user.
func hasSystemTagLink(t *testing.T, ctx context.Context, tx pgx.Tx, userID, activityID, tagName string, want bool) {
	t.Helper()
	var got bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity_tags at
			JOIN tags t ON t.id = at.tag_id
			WHERE at.activity_id = $1
			  AND t.user_id = $2
			  AND t.name = $3
			  AND t.is_system = true
		)
	`, activityID, userID, tagName).Scan(&got); err != nil {
		t.Fatalf("query system %s link: %v", tagName, err)
	}
	if got != want {
		t.Fatalf("activity %s system %q link = %v, want %v", activityID, tagName, got, want)
	}
}

func activityHasTag(t *testing.T, ctx context.Context, tx pgx.Tx, activityID, tag string, want bool) {
	t.Helper()
	var got bool
	if err := tx.QueryRow(ctx, `
		SELECT $2 = ANY(COALESCE(tags, '{}')) FROM activities WHERE id = $1
	`, activityID, tag).Scan(&got); err != nil {
		t.Fatalf("query tags array: %v", err)
	}
	if got != want {
		t.Fatalf("activity %s tags array contains %q = %v, want %v", activityID, tag, got, want)
	}
}

func tagActivityCount(t *testing.T, ctx context.Context, tx pgx.Tx, tagID string) int {
	t.Helper()
	var n int
	if err := tx.QueryRow(ctx, `SELECT activity_count FROM tags WHERE id = $1`, tagID).Scan(&n); err != nil {
		t.Fatalf("query tag count: %v", err)
	}
	return n
}

func tagIsSystem(t *testing.T, ctx context.Context, tx pgx.Tx, tagID string) bool {
	t.Helper()
	var b bool
	if err := tx.QueryRow(ctx, `SELECT is_system FROM tags WHERE id = $1`, tagID).Scan(&b); err != nil {
		t.Fatalf("query is_system: %v", err)
	}
	return b
}

func removalRowExists(t *testing.T, ctx context.Context, tx pgx.Tx, activityID, tagID string) bool {
	t.Helper()
	var b bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity_tag_removals WHERE activity_id = $1 AND tag_id = $2)
	`, activityID, tagID).Scan(&b); err != nil {
		t.Fatalf("query removal row: %v", err)
	}
	return b
}

func TestMigration_SystemModifierTagSyncResyncRepairsAllCurrentModifiers(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeSystemModifierTagSyncFixture(t, ctx, tx)
	damageSystemModifierState(t, ctx, tx, f)

	// Sanity: damage step actually broke the rows we expect resync to fix.
	hasSystemTagLink(t, ctx, tx, f.user1ID, f.cyclingElectricID, "E-Bike", false)
	hasSystemTagLink(t, ctx, tx, f.user1ID, f.indoorID, "Indoor", false)
	if !removalRowExists(t, ctx, tx, f.indoorID, f.indoorTagID) {
		t.Fatalf("damage step did not insert stale removal row")
	}

	callResync(t, ctx, tx,
		f.cyclingElectricID, f.runningElectricID, f.virtualID, f.commuteID, f.indoorID,
	)

	// user1 modifier-driven junctions are repaired.
	hasSystemTagLink(t, ctx, tx, f.user1ID, f.cyclingElectricID, "E-Bike", true)
	hasSystemTagLink(t, ctx, tx, f.user1ID, f.runningElectricID, "E-Bike", false) // running stays unbiked
	hasSystemTagLink(t, ctx, tx, f.user1ID, f.virtualID, "virtuell", true)
	hasSystemTagLink(t, ctx, tx, f.user1ID, f.commuteID, "Pendeln", true)
	hasSystemTagLink(t, ctx, tx, f.user1ID, f.indoorID, "Indoor", true)

	// activities.tags arrays are reconciled with modifier flags.
	activityHasTag(t, ctx, tx, f.cyclingElectricID, "E-Bike", true)
	activityHasTag(t, ctx, tx, f.runningElectricID, "E-Bike", false)
	activityHasTag(t, ctx, tx, f.virtualID, "virtuell", true)
	activityHasTag(t, ctx, tx, f.commuteID, "Pendeln", true)
	activityHasTag(t, ctx, tx, f.indoorID, "Indoor", true)

	// user1's pre-existing manual Pendeln tag has been promoted to the system
	// shape, not duplicated. (tags is unique on (user_id, name), so there is
	// at most one Pendeln per user.)
	if !tagIsSystem(t, ctx, tx, f.pendelnTagID) {
		t.Fatalf("user1 Pendeln tag was not promoted to is_system=true")
	}

	// Stale removal row for the Indoor activity is gone (tag is now present
	// according to the modifier field).
	if removalRowExists(t, ctx, tx, f.indoorID, f.indoorTagID) {
		t.Fatalf("stale Indoor removal row was not cleared")
	}

	// Counts for user1 system tags are correct.
	want := map[string]int{
		f.indoorTagID:   1,
		f.pendelnTagID:  1,
		f.virtuellTagID: 1,
		f.eBikeTagID:    1,
	}
	for tagID, w := range want {
		if got := tagActivityCount(t, ctx, tx, tagID); got != w {
			t.Fatalf("tag %s activity_count = %d, want %d", tagID, got, w)
		}
	}

	// user2's manual E-Bike tag is untouched: still is_system=false and the
	// stale activity_count we set as part of damage stays untouched because
	// no resync-scope activity belongs to user2.
	if tagIsSystem(t, ctx, tx, f.user2EBikeManualTagID) {
		t.Fatalf("user2 manual E-Bike tag was wrongly promoted to is_system=true")
	}
	if got := tagActivityCount(t, ctx, tx, f.user2EBikeManualTagID); got != 9999 {
		t.Fatalf("user2 manual E-Bike count = %d, want 9999 (untouched)", got)
	}
	// user1 unrelated tag count also untouched (we set it to 9999 too).
	if got := tagActivityCount(t, ctx, tx, f.unrelatedTagID); got != 9999 {
		t.Fatalf("user1 unrelated tag count = %d, want 9999 (untouched)", got)
	}
}

func TestMigration_SystemModifierTagSyncSkipGateRequiresExplicitResync(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeSystemModifierTagSyncFixture(t, ctx, tx)

	// Baseline: existing INSERT triggers should already have linked Indoor on
	// the indoor activity. Sanity-check before we exercise the skip flag.
	hasSystemTagLink(t, ctx, tx, f.user1ID, f.indoorID, "Indoor", true)

	// With the skip flag set, flipping a modifier column must NOT update
	// activity_tags / activities.tags — but Direction A (tags-array → flags)
	// still has to fire so a tag-driven write keeps deriving flags.
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
		t.Fatalf("set skip flag: %v", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE activities SET is_commute = true WHERE id = $1`, f.indoorID); err != nil {
		t.Fatalf("flip is_commute under skip: %v", err)
	}
	hasSystemTagLink(t, ctx, tx, f.user1ID, f.indoorID, "Pendeln", false) // skipped
	activityHasTag(t, ctx, tx, f.indoorID, "Pendeln", false)              // skipped

	// Direction A still works: writing the tag array should still derive flags.
	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = ARRAY['Pendeln']::text[] WHERE id = $1`, f.runningElectricID); err != nil {
		t.Fatalf("write tags array under skip: %v", err)
	}
	var isCommute bool
	if err := tx.QueryRow(ctx, `SELECT is_commute FROM activities WHERE id = $1`, f.runningElectricID).Scan(&isCommute); err != nil {
		t.Fatalf("query is_commute: %v", err)
	}
	if !isCommute {
		t.Fatalf("inbound tags→flag derivation must run even under skip flag, is_commute = false")
	}

	// Now drop the skip and run the resync explicitly. State must converge.
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'false'`); err != nil {
		t.Fatalf("clear skip flag: %v", err)
	}
	callResync(t, ctx, tx, f.indoorID, f.runningElectricID)

	hasSystemTagLink(t, ctx, tx, f.user1ID, f.indoorID, "Pendeln", true)
	activityHasTag(t, ctx, tx, f.indoorID, "Pendeln", true)
	hasSystemTagLink(t, ctx, tx, f.user1ID, f.runningElectricID, "Pendeln", true)
}

func TestMigration_SystemModifierTagSyncRespectsCyclingOnlyEBike(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeSystemModifierTagSyncFixture(t, ctx, tx)
	damageSystemModifierState(t, ctx, tx, f)

	callResync(t, ctx, tx,
		f.cyclingElectricID, f.runningElectricID,
	)

	// Cycling-electric gets the system E-Bike link; running-electric does not.
	hasSystemTagLink(t, ctx, tx, f.user1ID, f.cyclingElectricID, "E-Bike", true)
	hasSystemTagLink(t, ctx, tx, f.user1ID, f.runningElectricID, "E-Bike", false)
	activityHasTag(t, ctx, tx, f.cyclingElectricID, "E-Bike", true)
	activityHasTag(t, ctx, tx, f.runningElectricID, "E-Bike", false)

	// is_electric stays generic on the running activity.
	var isElectric bool
	if err := tx.QueryRow(ctx, `SELECT is_electric FROM activities WHERE id = $1`, f.runningElectricID).Scan(&isElectric); err != nil {
		t.Fatalf("query is_electric: %v", err)
	}
	if !isElectric {
		t.Fatalf("running-electric is_electric = false, want true (resync must not clear non-cycling electric flag)")
	}
}

func TestMigration_SystemModifierTagSyncRecomputesOnlyAffectedSystemCounts(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeSystemModifierTagSyncFixture(t, ctx, tx)

	// Set every count in the world to 9999 — the user2 manual E-Bike tag plus
	// every user1 tag, system or not.
	if _, err := tx.Exec(ctx, `UPDATE tags SET activity_count = 9999 WHERE user_id IN ($1, $2)`, f.user1ID, f.user2ID); err != nil {
		t.Fatalf("stale counts: %v", err)
	}

	// Resync only the cycling-electric activity. Only counts for tags that
	// (a) are system tags and (b) belong to the activity's user must be
	// recomputed. Everything else stays at 9999.
	callResync(t, ctx, tx, f.cyclingElectricID)

	// Tags whose counts MUST be recomputed (user1 system tags, regardless of
	// modifier match — because the function repairs the four reserved system
	// tags for any user with an affected activity).
	mustRecomputed := []string{f.indoorTagID, f.pendelnTagID, f.virtuellTagID, f.eBikeTagID}
	for _, id := range mustRecomputed {
		if got := tagActivityCount(t, ctx, tx, id); got == 9999 {
			t.Fatalf("tag %s activity_count was not recomputed (still 9999)", id)
		}
	}

	// Tags whose counts MUST NOT be touched.
	mustUntouched := []string{f.unrelatedTagID, f.user2EBikeManualTagID}
	for _, id := range mustUntouched {
		if got := tagActivityCount(t, ctx, tx, id); got != 9999 {
			t.Fatalf("tag %s activity_count = %d, want 9999 (must not be recomputed)", id, got)
		}
	}
}

// TestMigration_SystemModifierTagSyncRejectsEmptyAndUnknownIDs proves the
// function early-outs on empty input and silently ignores unknown IDs.
func TestMigration_SystemModifierTagSyncRejectsEmptyAndUnknownIDs(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	makeSystemModifierTagSyncFixture(t, ctx, tx)

	if _, err := tx.Exec(ctx, `SELECT skadia_resync_system_modifier_tags(ARRAY[]::uuid[])`); err != nil {
		t.Fatalf("empty array call: %v", err)
	}
	if _, err := tx.Exec(ctx, `SELECT skadia_resync_system_modifier_tags(NULL::uuid[])`); err != nil {
		t.Fatalf("NULL call: %v", err)
	}
	unknownID := uuid.NewString()
	if _, err := tx.Exec(ctx, `SELECT skadia_resync_system_modifier_tags(ARRAY[$1]::uuid[])`, unknownID); err != nil {
		t.Fatalf("unknown id call: %v", err)
	}
}

// idsToSorted is a small convenience for assertions where ordering would
// otherwise make the diff messy.
func idsToSorted(ids []string) []string {
	out := append([]string{}, ids...)
	sort.Strings(out)
	return out
}

var _ = idsToSorted // suppress unused-helper lint until follow-up tests use it

// directionAFixture is a minimal single-user fixture for Direction-A slot-guard
// tests. It seeds a user, the four system tags, and one activity. Each test
// drives its own INSERT/UPDATE sequence to isolate trigger behavior.
type directionAFixture struct {
	userID         string
	pendelnTagID   string
	indoorTagID    string
	eBikeTagID     string
	virtuellTagID  string
}

func makeDirectionAFixture(t *testing.T, ctx context.Context, tx pgx.Tx) directionAFixture {
	t.Helper()
	f := directionAFixture{userID: uuid.NewString()}
	short := f.userID[:8]
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		VALUES ($1, $2, $3, true, 'test', false)
	`, f.userID, "dirA_"+short, "dirA_"+short+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	for _, row := range []struct {
		name  string
		color string
		dest  *string
	}{
		{"Pendeln", "#6B7280", &f.pendelnTagID},
		{"Indoor", "#3B82F6", &f.indoorTagID},
		{"E-Bike", "#16A34A", &f.eBikeTagID},
		{"virtuell", "#8B5CF6", &f.virtuellTagID},
	} {
		if err := tx.QueryRow(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
			VALUES ($1, $2, $3, '#FFFFFF', 'system', true, false, '', 0)
			RETURNING id
		`, f.userID, row.name, row.color).Scan(row.dest); err != nil {
			t.Fatalf("insert %s tag: %v", row.name, err)
		}
	}
	return f
}

func assertModifierFlag(t *testing.T, ctx context.Context, tx pgx.Tx, activityID string, col string, want bool) {
	t.Helper()
	// col must be one of the known boolean modifier columns to avoid SQL injection.
	allowed := map[string]bool{"is_commute": true, "is_indoor": true, "is_electric": true, "is_virtual": true}
	if !allowed[col] {
		t.Fatalf("assertModifierFlag: unknown column %q", col)
	}
	var got bool
	if err := tx.QueryRow(ctx, `SELECT `+col+` FROM activities WHERE id = $1`, activityID).Scan(&got); err != nil {
		t.Fatalf("query %s: %v", col, err)
	}
	if got != want {
		t.Fatalf("activity %s %s = %v, want %v", activityID, col, got, want)
	}
}

// insertActivity inserts a minimal activity with the given sport_type, boolean
// modifier flags, and tags TEXT[]. Tags is passed as a Go []string.
func insertActivity(t *testing.T, ctx context.Context, tx pgx.Tx, userID, sportType string, isIndoor, isElectric, isVirtual, isCommute bool, tags []string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_indoor, is_electric, is_virtual, is_commute, started_at)
		VALUES ($1, $2, 'dir-a-test', $3, NULL, $4::text[], $5, $6, $7, $8, now())
	`, id, userID, sportType, tags, isIndoor, isElectric, isVirtual, isCommute); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return id
}

// queryTagsArray returns the activities.tags TEXT[] for one activity.
func queryTagsArray(t *testing.T, ctx context.Context, tx pgx.Tx, activityID string) []string {
	t.Helper()
	var tags []string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(tags, ARRAY[]::text[]) FROM activities WHERE id = $1`, activityID).Scan(&tags); err != nil {
		t.Fatalf("query tags array: %v", err)
	}
	return tags
}

// TestSystemModifierTagSyncDirectionAUpdateSlotStableUnrelatedAdd is the Critic
// #3 primary regression test. A caller-managed transaction uses the skip flag
// so it can set is_commute=true without the outbound path adding 'Pendeln' to
// the tags array. When the caller later adds an unrelated tag ('Berg') without
// touching the 'Pendeln' slot, is_commute must not be cleared.
//
// Pre-Critic-#3: trigger always re-derived is_commute from NEW.tags on any tags
// change → is_commute became false when 'Pendeln' was absent. Post-fix: flag is
// only updated when the 'Pendeln' slot membership actually changed.
func TestSystemModifierTagSyncDirectionAUpdateSlotStableUnrelatedAdd(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	// Use skip flag so the outbound path does NOT add 'Pendeln' to tags.
	// This models a caller-managed bulk path that sets is_commute=true directly
	// and will call resync later — the initial INSERT leaves tags=['Stadt'].
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
		t.Fatalf("set skip flag: %v", err)
	}
	actID := insertActivity(t, ctx, tx, f.userID, "cycling", false, false, false, true, []string{"Stadt"})
	// Direction A on INSERT still runs: is_commute := true OR ('Pendeln'=ANY(['Stadt'])) = true.
	assertModifierFlag(t, ctx, tx, actID, "is_commute", true)
	// Outbound was suppressed: 'Pendeln' must NOT be in tags.
	if tags := queryTagsArray(t, ctx, tx, actID); containsStr(tags, "Pendeln") {
		t.Fatalf("pre-condition: outbound was skipped but 'Pendeln' found in tags, got %v", tags)
	}
	// Drop the skip flag for the subsequent unrelated tag UPDATE.
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'false'`); err != nil {
		t.Fatalf("clear skip flag: %v", err)
	}

	// UPDATE: add 'Berg'. OLD.tags=['Stadt'], NEW.tags=['Stadt','Berg'].
	// 'Pendeln' absent in both — slot did not change.
	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = array_append(tags, 'Berg') WHERE id = $1`, actID); err != nil {
		t.Fatalf("update tags: %v", err)
	}

	// is_commute must remain true (slot stable — 'Pendeln' absent in OLD and NEW).
	assertModifierFlag(t, ctx, tx, actID, "is_commute", true)
}

// TestSystemModifierTagSyncDirectionAUpdateSlotStableUnrelatedRemove verifies
// the same slot-stability guarantee as the Add test, but for a tag removal.
// The skip flag suppresses outbound so tags=['Stadt','Berg'] without 'Pendeln';
// removing 'Berg' while is_commute=true must not clear the flag.
func TestSystemModifierTagSyncDirectionAUpdateSlotStableUnrelatedRemove(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	// Setup via skip flag: is_commute=true, tags=['Stadt','Berg'] (no 'Pendeln').
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
		t.Fatalf("set skip flag: %v", err)
	}
	actID := insertActivity(t, ctx, tx, f.userID, "cycling", false, false, false, true, []string{"Stadt", "Berg"})
	assertModifierFlag(t, ctx, tx, actID, "is_commute", true)
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'false'`); err != nil {
		t.Fatalf("clear skip flag: %v", err)
	}

	// UPDATE: remove 'Berg'. OLD.tags=['Stadt','Berg'], NEW.tags=['Stadt'].
	// 'Pendeln' absent in both — slot stable.
	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = array_remove(tags, 'Berg') WHERE id = $1`, actID); err != nil {
		t.Fatalf("update tags: %v", err)
	}
	assertModifierFlag(t, ctx, tx, actID, "is_commute", true)
}

// TestSystemModifierTagSyncDirectionAUpdateSlotAdded verifies the forward case:
// adding 'Pendeln' to tags sets is_commute=true (backward-compat).
func TestSystemModifierTagSyncDirectionAUpdateSlotAdded(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	actID := insertActivity(t, ctx, tx, f.userID, "cycling", false, false, false, false, []string{})
	assertModifierFlag(t, ctx, tx, actID, "is_commute", false)

	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = ARRAY['Pendeln']::text[] WHERE id = $1`, actID); err != nil {
		t.Fatalf("update tags: %v", err)
	}
	assertModifierFlag(t, ctx, tx, actID, "is_commute", true)
}

// TestSystemModifierTagSyncDirectionAUpdateSlotRemoved verifies the removal case:
// removing 'Pendeln' from tags sets is_commute=false (backward-compat).
func TestSystemModifierTagSyncDirectionAUpdateSlotRemoved(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	actID := insertActivity(t, ctx, tx, f.userID, "cycling", false, false, false, true, []string{"Pendeln"})
	assertModifierFlag(t, ctx, tx, actID, "is_commute", true)

	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = ARRAY[]::text[] WHERE id = $1`, actID); err != nil {
		t.Fatalf("update tags: %v", err)
	}
	assertModifierFlag(t, ctx, tx, actID, "is_commute", false)
}

// TestSystemModifierTagSyncDirectionAUpdateSlotStablePendelnRetained verifies
// that adding an unrelated tag when 'Pendeln' is already in tags keeps
// is_commute=true and does not flip it.
func TestSystemModifierTagSyncDirectionAUpdateSlotStablePendelnRetained(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	actID := insertActivity(t, ctx, tx, f.userID, "cycling", false, false, false, true, []string{"Pendeln"})
	assertModifierFlag(t, ctx, tx, actID, "is_commute", true)

	// Add 'Stadt' — slot stable.
	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = array_append(tags, 'Stadt') WHERE id = $1`, actID); err != nil {
		t.Fatalf("update tags: %v", err)
	}
	assertModifierFlag(t, ctx, tx, actID, "is_commute", true)
}

// TestSystemModifierTagSyncDirectionAUpdateEBikeSlotStableOnCycling is the
// E-Bike equivalent of the Critic #3 regression test. A cycling activity has
// is_electric=true and tags=['Stadt'] (no 'E-Bike', set via skip flag). Adding
// 'Berg' must not clear is_electric.
func TestSystemModifierTagSyncDirectionAUpdateEBikeSlotStableOnCycling(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	// Setup via skip flag: is_electric=true, tags=['Stadt'] (no 'E-Bike').
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
		t.Fatalf("set skip flag: %v", err)
	}
	actID := insertActivity(t, ctx, tx, f.userID, "cycling", false, true, false, false, []string{"Stadt"})
	assertModifierFlag(t, ctx, tx, actID, "is_electric", true)
	if tags := queryTagsArray(t, ctx, tx, actID); containsStr(tags, "E-Bike") {
		t.Fatalf("pre-condition: skip-flag INSERT should not have 'E-Bike' in tags, got %v", tags)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'false'`); err != nil {
		t.Fatalf("clear skip flag: %v", err)
	}

	// UPDATE: add 'Berg'. OLD.tags=['Stadt'], NEW.tags=['Stadt','Berg'].
	// 'E-Bike' absent in both — slot stable.
	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = array_append(tags, 'Berg') WHERE id = $1`, actID); err != nil {
		t.Fatalf("update tags: %v", err)
	}
	assertModifierFlag(t, ctx, tx, actID, "is_electric", true)
}

// TestSystemModifierTagSyncDirectionAUpdateEBikeIgnoresNonCyclingSlotChange
// verifies the cycling-only rule: for a non-cycling activity with is_electric=true
// and tags=['Stadt'] (set via skip flag, no 'E-Bike' in tags), an unrelated tag
// change must not touch is_electric. The E-Bike slot guard only applies to
// cycling; non-cycling activities never derive is_electric from the E-Bike slot.
func TestSystemModifierTagSyncDirectionAUpdateEBikeIgnoresNonCyclingSlotChange(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	// Setup via skip flag: non-cycling, is_electric=true, tags=['Stadt'].
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
		t.Fatalf("set skip flag: %v", err)
	}
	actID := insertActivity(t, ctx, tx, f.userID, "running", false, true, false, false, []string{"Stadt"})
	assertModifierFlag(t, ctx, tx, actID, "is_electric", true)
	if tags := queryTagsArray(t, ctx, tx, actID); containsStr(tags, "E-Bike") {
		t.Fatalf("pre-condition: non-cycling activity should not have 'E-Bike' in tags, got %v", tags)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'false'`); err != nil {
		t.Fatalf("clear skip flag: %v", err)
	}

	// UPDATE: add 'Berg'. No E-Bike slot involvement.
	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = array_append(tags, 'Berg') WHERE id = $1`, actID); err != nil {
		t.Fatalf("update tags: %v", err)
	}
	assertModifierFlag(t, ctx, tx, actID, "is_electric", true)
	if tags := queryTagsArray(t, ctx, tx, actID); containsStr(tags, "E-Bike") {
		t.Fatalf("'E-Bike' appeared in non-cycling tags after unrelated UPDATE, got %v", tags)
	}
	_ = f
}

// TestSystemModifierTagSyncDirectionAUpdateNonCyclingEBikeStripStillWorks
// verifies that the non-cycling-strip Sonderfall still fires: if a non-cycling
// activity has 'E-Bike' injected into its tags array directly, the UPDATE trigger
// strips it.
func TestSystemModifierTagSyncDirectionAUpdateNonCyclingEBikeStripStillWorks(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	actID := insertActivity(t, ctx, tx, f.userID, "running", false, true, false, false, []string{"Stadt"})
	assertModifierFlag(t, ctx, tx, actID, "is_electric", true)

	// Disable triggers so we can inject 'E-Bike' directly without the INSERT
	// trigger stripping it — simulating a caller that bypassed the trigger.
	for _, stmt := range []string{
		`ALTER TABLE activities DISABLE TRIGGER trg_sync_system_flags_and_tags`,
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			t.Fatalf("disable trigger: %v", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = ARRAY['E-Bike','Stadt']::text[] WHERE id = $1`, actID); err != nil {
		t.Fatalf("inject E-Bike into non-cycling tags: %v", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE activities ENABLE TRIGGER trg_sync_system_flags_and_tags`,
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			t.Fatalf("re-enable trigger: %v", err)
		}
	}

	// Now do a normal UPDATE that changes tags. The non-cycling E-Bike strip must
	// fire and remove 'E-Bike' from the tags array.
	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = ARRAY['E-Bike','Stadt','Berg']::text[] WHERE id = $1`, actID); err != nil {
		t.Fatalf("update tags: %v", err)
	}
	if tags := queryTagsArray(t, ctx, tx, actID); containsStr(tags, "E-Bike") {
		t.Fatalf("non-cycling E-Bike strip did not fire, tags still contain 'E-Bike': %v", tags)
	}
	_ = f // fixture only needed for DB setup (user + tags)
}

// containsStr returns true when s is found in the slice ss.
func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// TestSkadiaResyncSystemModifierTagsActivityCountUnderConcurrency runs two
// concurrent transactions that each update a different activity owned by the same
// user and then call skadia_resync_system_modifier_tags. After both commit, the
// Indoor system tag must have activity_count = 2.
//
// The ORDER BY id FOR UPDATE fence in the resync function (Critic #12) makes lock
// acquisition deterministic so the two transactions do not deadlock.
func TestSkadiaResyncSystemModifierTagsActivityCountUnderConcurrency(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	// --- Setup: shared rows created outside the competing transactions. ---
	userID := uuid.NewString()
	short := userID[:8]
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		VALUES ($1, $2, $3, true, 'test', false)
	`, userID, "cc_"+short, "cc_"+short+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
			t.Logf("cleanup user: %v", err)
		}
	})

	// Insert four system tags (resync recomputes all four for the user).
	var indoorTagID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
		VALUES ($1, 'Indoor', '#3B82F6', '#FFFFFF', 'system', true, false, '', 0)
		RETURNING id
	`, userID).Scan(&indoorTagID); err != nil {
		t.Fatalf("insert Indoor tag: %v", err)
	}
	for _, name := range []string{"Pendeln", "virtuell", "E-Bike"} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
			VALUES ($1, $2, '#000000', '#FFFFFF', 'system', true, false, '', 0)
		`, userID, name); err != nil {
			t.Fatalf("insert %s tag: %v", name, err)
		}
	}

	// Insert two activities. Both will become indoor after the resync call.
	// Triggers are intentionally bypassed so the activities start inconsistent,
	// proving the resync—not the triggers—drives the count.
	a1ID := uuid.NewString()
	a2ID := uuid.NewString()
	for _, id := range []string{a1ID, a2ID} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_indoor, started_at)
			VALUES ($1, $2, 'cc-test', 'rowing', NULL, ARRAY[]::text[], true, now())
		`, id, userID); err != nil {
			t.Fatalf("insert activity %s: %v", id, err)
		}
	}
	// Remove any junctions that the INSERT trigger may have created for Indoor,
	// so the resync call does real insert work (not just count recompute).
	if _, err := pool.Exec(ctx, `
		DELETE FROM activity_tags at
		USING tags t
		WHERE at.tag_id = t.id AND t.user_id = $1 AND t.name = 'Indoor'
		  AND at.activity_id IN ($2, $3)
	`, userID, a1ID, a2ID); err != nil {
		t.Fatalf("clear pre-existing junctions: %v", err)
	}
	// Also reset count to 0 to make the assertion unambiguous.
	if _, err := pool.Exec(ctx, `UPDATE tags SET activity_count = 0 WHERE id = $1`, indoorTagID); err != nil {
		t.Fatalf("reset count: %v", err)
	}

	// --- Concurrent resync: each goroutine opens its own transaction. ---
	var wg sync.WaitGroup
	errs := make(chan error, 2)

	runResync := func(pool *pgxpool.Pool, activityID string) {
		defer wg.Done()
		tx, err := pool.Begin(ctx)
		if err != nil {
			errs <- err
			return
		}
		defer func() {
			if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
				// already committed — ignore
			}
		}()
		if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
			errs <- err
			return
		}
		if _, err := tx.Exec(ctx, `UPDATE activities SET is_indoor = true WHERE id = $1`, activityID); err != nil {
			errs <- err
			return
		}
		if _, err := tx.Exec(ctx, `SELECT skadia_resync_system_modifier_tags($1::uuid[])`, []string{activityID}); err != nil {
			errs <- err
			return
		}
		if err := tx.Commit(ctx); err != nil {
			errs <- err
		}
	}

	wg.Add(2)
	go runResync(pool, a1ID)
	go runResync(pool, a2ID)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent resync goroutine: %v", err)
		}
	}

	// After both transactions committed, the Indoor system tag must count exactly
	// 2 — one junction per activity inserted by the resync.
	var count int
	if err := pool.QueryRow(ctx, `SELECT activity_count FROM tags WHERE id = $1`, indoorTagID).Scan(&count); err != nil {
		t.Fatalf("query activity_count: %v", err)
	}
	if count != 2 {
		t.Fatalf("Indoor activity_count = %d, want 2 (ORDER BY id FOR UPDATE fence must prevent race)", count)
	}
}

// snapshotSystemModifierFunctionBodies captures pg_get_functiondef() text for
// each trigger function 000353 redefines plus a 0/1 marker for the new
// skadia_resync_system_modifier_tags function. The bodies are sensitive to
// CREATE-OR-REPLACE ordering — round-trip tests use deep-equality on this map.
func snapshotSystemModifierFunctionBodies(t *testing.T, ctx context.Context, tx pgx.Tx) map[string]string {
	t.Helper()
	out := make(map[string]string, 6)
	for _, name := range []string{
		"sync_system_flags_and_tags",
		"sync_system_flags_and_tags_insert_junction",
		"sync_virtual_tag",
		"sync_virtual_tag_insert",
		"sync_virtual_tag_insert_junction",
	} {
		var body string
		if err := tx.QueryRow(ctx, `SELECT pg_get_functiondef(p.oid)
			FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
			WHERE n.nspname = 'public' AND p.proname = $1`, name).Scan(&body); err != nil {
			t.Fatalf("snapshot %s: %v", name, err)
		}
		out[name] = body
	}
	var resyncPresent int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public' AND p.proname = 'skadia_resync_system_modifier_tags'
	`).Scan(&resyncPresent); err != nil {
		t.Fatalf("snapshot resync presence: %v", err)
	}
	if resyncPresent > 0 {
		out["__skadia_resync_system_modifier_tags_present__"] = "yes"
	} else {
		out["__skadia_resync_system_modifier_tags_present__"] = "no"
	}
	return out
}

// TestMigration000353DownRoundTripPreservesPre353State applies down.sql then
// up.sql inside a single rolled-back transaction and asserts that the trigger
// function bodies plus the resync function presence/absence round-trip
// cleanly. Pins down-migration correctness — Critic-Finding #9 (search_path
// drift between up and down) would manifest as
// snapshot_initial != snapshot_after_up_again here.
func TestMigration000353DownRoundTripPreservesPre353State(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && rbErr.Error() != "tx is closed" {
			t.Logf("rollback: %v", rbErr)
		}
	}()

	upSQL, downSQL := readSystemModifierTagSyncMigrationSQL(t)

	// snapshot_initial: HEAD-applied state (post-353).
	snapshotInitial := snapshotSystemModifierFunctionBodies(t, ctx, tx)
	if snapshotInitial["__skadia_resync_system_modifier_tags_present__"] != "yes" {
		t.Fatalf("pre-condition: skadia_resync_system_modifier_tags must be present at HEAD, snapshot=%v", snapshotInitial)
	}

	// Apply down.sql.
	if _, err := tx.Exec(ctx, downSQL); err != nil {
		t.Fatalf("apply down.sql: %v", err)
	}
	snapshotAfterDown := snapshotSystemModifierFunctionBodies(t, ctx, tx)

	// down must actually have changed something — otherwise the round-trip
	// claim is vacuous.
	if reflect.DeepEqual(snapshotInitial, snapshotAfterDown) {
		t.Fatalf("down.sql did not change the snapshot at all")
	}
	if snapshotAfterDown["__skadia_resync_system_modifier_tags_present__"] != "no" {
		t.Fatalf("after down: skadia_resync_system_modifier_tags must be gone, got %q",
			snapshotAfterDown["__skadia_resync_system_modifier_tags_present__"])
	}

	// Re-apply up.sql.
	if _, err := tx.Exec(ctx, upSQL); err != nil {
		t.Fatalf("re-apply up.sql: %v", err)
	}
	snapshotAfterUpAgain := snapshotSystemModifierFunctionBodies(t, ctx, tx)

	// Round-trip stable: HEAD-applied → down → up must reproduce HEAD-applied.
	if !reflect.DeepEqual(snapshotInitial, snapshotAfterUpAgain) {
		for k, want := range snapshotInitial {
			got := snapshotAfterUpAgain[k]
			if got != want {
				t.Errorf("round-trip drift in %q:\n--- initial ---\n%s\n--- after up again ---\n%s",
					k, want, got)
			}
		}
		t.FailNow()
	}
}

// snapshotResyncState captures every row in the activity_tags / activities.tags /
// tags.activity_count / tags.is_system / activity_tag_removals tables that
// skadia_resync_system_modifier_tags can legally touch for the supplied user
// and activity IDs. Returned shape is deterministic so reflect.DeepEqual can
// diff snapshot_after_1 vs snapshot_after_5 for the idempotency assertion.
type resyncStateSnapshot struct {
	tagsArrays      map[string][]string // activityID -> sorted tag names from activities.tags
	junctionPairs   []string            // sorted "<activityID>:<tagID>" pairs
	tagCounts       map[string]int      // tagID -> activity_count for system tags of the user
	tagIsSystem     map[string]bool     // tagID -> is_system for tags in scope (system + name-collision tags)
	removalPairs    []string            // sorted "<activityID>:<tagID>" pairs in activity_tag_removals
}

func snapshotResyncState(t *testing.T, ctx context.Context, tx pgx.Tx, userID string, activityIDs []string, tagIDs []string) resyncStateSnapshot {
	t.Helper()
	snap := resyncStateSnapshot{
		tagsArrays:  make(map[string][]string, len(activityIDs)),
		tagCounts:   make(map[string]int, len(tagIDs)),
		tagIsSystem: make(map[string]bool, len(tagIDs)),
	}

	for _, aid := range activityIDs {
		var tags []string
		if err := tx.QueryRow(ctx, `SELECT COALESCE(tags, ARRAY[]::text[]) FROM activities WHERE id = $1`, aid).Scan(&tags); err != nil {
			t.Fatalf("snapshot tags array for %s: %v", aid, err)
		}
		sort.Strings(tags)
		snap.tagsArrays[aid] = tags
	}

	junctionRows, err := tx.Query(ctx, `
		SELECT activity_id::text, tag_id::text FROM activity_tags
		WHERE activity_id = ANY($1::uuid[])
	`, activityIDs)
	if err != nil {
		t.Fatalf("snapshot junctions: %v", err)
	}
	for junctionRows.Next() {
		var a, tg string
		if err := junctionRows.Scan(&a, &tg); err != nil {
			t.Fatalf("scan junction: %v", err)
		}
		snap.junctionPairs = append(snap.junctionPairs, a+":"+tg)
	}
	junctionRows.Close()
	sort.Strings(snap.junctionPairs)

	for _, tid := range tagIDs {
		var count int
		var isSystem bool
		if err := tx.QueryRow(ctx, `SELECT activity_count, is_system FROM tags WHERE id = $1`, tid).Scan(&count, &isSystem); err != nil {
			t.Fatalf("snapshot tag %s: %v", tid, err)
		}
		snap.tagCounts[tid] = count
		snap.tagIsSystem[tid] = isSystem
	}

	removalRows, err := tx.Query(ctx, `
		SELECT activity_id::text, tag_id::text FROM activity_tag_removals
		WHERE activity_id = ANY($1::uuid[])
	`, activityIDs)
	if err != nil {
		t.Fatalf("snapshot removals: %v", err)
	}
	for removalRows.Next() {
		var a, tg string
		if err := removalRows.Scan(&a, &tg); err != nil {
			t.Fatalf("scan removal: %v", err)
		}
		snap.removalPairs = append(snap.removalPairs, a+":"+tg)
	}
	removalRows.Close()
	sort.Strings(snap.removalPairs)

	_ = userID // kept in signature so future scope expansions can filter without churn
	return snap
}

// TestSkadiaResyncSystemModifierTagsIsIdempotent calls the resync function
// five times in succession against the same scenario and asserts the resulting
// state after the first call equals the state after the fifth call. Pins the
// architectural idempotency claim — a future caller adding a non-idempotent
// operation (e.g. unconditional INSERT, count += 1, RAISE NOTICE side effect
// on rows) would surface as snapshot_after_1 != snapshot_after_5.
func TestSkadiaResyncSystemModifierTagsIsIdempotent(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeSystemModifierTagSyncFixture(t, ctx, tx)
	damageSystemModifierState(t, ctx, tx, f)

	activityIDs := []string{
		f.cyclingElectricID, f.runningElectricID, f.virtualID, f.commuteID, f.indoorID,
	}
	tagIDs := []string{
		f.indoorTagID, f.pendelnTagID, f.virtuellTagID, f.eBikeTagID,
		f.unrelatedTagID, f.user2EBikeManualTagID,
	}

	// First call repairs the damage.
	callResync(t, ctx, tx, activityIDs...)
	snapAfter1 := snapshotResyncState(t, ctx, tx, f.user1ID, activityIDs, tagIDs)

	// Four more calls — the function must be a no-op on already-correct state.
	for i := 2; i <= 5; i++ {
		callResync(t, ctx, tx, activityIDs...)
	}
	snapAfter5 := snapshotResyncState(t, ctx, tx, f.user1ID, activityIDs, tagIDs)

	if !reflect.DeepEqual(snapAfter1, snapAfter5) {
		// Diff per field to make the failure actionable.
		if !reflect.DeepEqual(snapAfter1.tagsArrays, snapAfter5.tagsArrays) {
			t.Errorf("tags arrays drifted across resyncs:\n  after_1=%v\n  after_5=%v", snapAfter1.tagsArrays, snapAfter5.tagsArrays)
		}
		if !reflect.DeepEqual(snapAfter1.junctionPairs, snapAfter5.junctionPairs) {
			t.Errorf("junction pairs drifted:\n  after_1=%v\n  after_5=%v", snapAfter1.junctionPairs, snapAfter5.junctionPairs)
		}
		if !reflect.DeepEqual(snapAfter1.tagCounts, snapAfter5.tagCounts) {
			t.Errorf("tag counts drifted:\n  after_1=%v\n  after_5=%v", snapAfter1.tagCounts, snapAfter5.tagCounts)
		}
		if !reflect.DeepEqual(snapAfter1.tagIsSystem, snapAfter5.tagIsSystem) {
			t.Errorf("tag is_system drifted:\n  after_1=%v\n  after_5=%v", snapAfter1.tagIsSystem, snapAfter5.tagIsSystem)
		}
		if !reflect.DeepEqual(snapAfter1.removalPairs, snapAfter5.removalPairs) {
			t.Errorf("removal pairs drifted:\n  after_1=%v\n  after_5=%v", snapAfter1.removalPairs, snapAfter5.removalPairs)
		}
		t.FailNow()
	}
}
