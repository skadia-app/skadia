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
	"regexp"
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
		"CREATE OR REPLACE FUNCTION sync_system_tag_rename_into_activities_tags()",
		"AFTER UPDATE OF name ON tags",
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
		"DROP FUNCTION IF EXISTS sync_system_tag_rename_into_activities_tags",
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
	//    Use the skip flag so the AFTER-DELETE reconcile trigger introduced
	//    in m.1 does not undo the damage by flipping modifier columns AND
	//    does not recompute tags.activity_count (step 4 deliberately sets
	//    counts to 9999 to verify that resync repairs them; without the skip
	//    flag the m.1 trigger would recompute on each junction delete and
	//    leave counts at a different stale value before resync even runs).
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
		t.Fatalf("damage: set skip flag: %v", err)
	}
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
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'false'`); err != nil {
		t.Fatalf("damage: unset skip flag: %v", err)
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

// normalizeFunctionDefText normalizes a pg_get_functiondef() string so that
// cosmetic differences between two semantically equivalent function bodies do
// not cause false round-trip failures (Critic Round-2 #10). Normalization
// rules applied in order:
//
//  1. Strip single-line SQL comments (-- ... \n) — pg may or may not emit them
//     depending on how the body was stored.
//  2. Collapse runs of whitespace (space, tab, newline) to a single space.
//  3. Trim leading/trailing whitespace.
//
// This is intentionally coarse: the goal is resilience to reformatting, not
// full semantic equivalence. If two bodies differ only in whitespace or
// comment placement they are considered equal; if they differ in actual SQL
// keywords, identifiers, or string literals they are still different.
func normalizeFunctionDefText(s string) string {
	// Strip -- line comments (everything from -- to end of line).
	commentRE := regexp.MustCompile(`--[^\n]*`)
	s = commentRE.ReplaceAllString(s, "")
	// Collapse all whitespace runs to a single space.
	wsRE := regexp.MustCompile(`\s+`)
	s = wsRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// snapshotSystemModifierFunctionBodies captures pg_get_functiondef() text for
// each trigger function 000353 redefines plus a 0/1 marker for the new
// skadia_resync_system_modifier_tags function. Function bodies are normalized
// before storage so round-trip comparisons are resilient to cosmetic whitespace
// or comment-placement differences in the up SQL (Critic Round-2 #10).
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
		out[name] = normalizeFunctionDefText(body)
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

	// Tag-rename propagation function presence (m.2). Round-trip down/up must
	// drop and recreate it cleanly.
	var renamePresent int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public' AND p.proname = 'sync_system_tag_rename_into_activities_tags'
	`).Scan(&renamePresent); err != nil {
		t.Fatalf("snapshot rename function presence: %v", err)
	}
	if renamePresent > 0 {
		out["__sync_system_tag_rename_into_activities_tags_present__"] = "yes"
	} else {
		out["__sync_system_tag_rename_into_activities_tags_present__"] = "no"
	}

	// m.1 trigger def (AFTER DELETE on activity_tags). If down.sql forgets to
	// drop it or up.sql forgets to recreate it, the round-trip catches the gap.
	var junctionDeleteTrigDef string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE((
			SELECT pg_get_triggerdef(t.oid)
			FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
			WHERE t.tgname = 'trg_sync_system_modifier_tags_on_junction_delete'
		), '')
	`).Scan(&junctionDeleteTrigDef); err != nil {
		t.Fatalf("snapshot trg_sync_system_modifier_tags_on_junction_delete: %v", err)
	}
	out["__trg_sync_system_modifier_tags_on_junction_delete__"] = junctionDeleteTrigDef

	// m.2 trigger def (AFTER UPDATE OF name on tags). Same round-trip guard.
	var renameTrigDef string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE((
			SELECT pg_get_triggerdef(t.oid)
			FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
			WHERE t.tgname = 'trg_sync_system_tag_rename_into_activities_tags'
		), '')
	`).Scan(&renameTrigDef); err != nil {
		t.Fatalf("snapshot trg_sync_system_tag_rename_into_activities_tags: %v", err)
	}
	out["__trg_sync_system_tag_rename_into_activities_tags__"] = renameTrigDef

	// idx_activity_tags_tag_id_activity_id existence. Stored as "yes"/"no" so
	// the reflect.DeepEqual comparison catches down forgetting to drop it or
	// up forgetting to recreate it.
	var indexExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = 'idx_activity_tags_tag_id_activity_id')
	`).Scan(&indexExists); err != nil {
		t.Fatalf("snapshot idx_activity_tags_tag_id_activity_id: %v", err)
	}
	if indexExists {
		out["__idx_activity_tags_tag_id_activity_id_exists__"] = "yes"
	} else {
		out["__idx_activity_tags_tag_id_activity_id_exists__"] = "no"
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

// assertResyncSnapshot compares two snapshots field by field and fails t with
// a descriptive diff when they diverge. Used by the mutation-convergence test
// to report exactly which layer drifted after each mutation round.
func assertResyncSnapshot(t *testing.T, label string, got, want resyncStateSnapshot) {
	t.Helper()
	if reflect.DeepEqual(got, want) {
		return
	}
	if !reflect.DeepEqual(got.tagsArrays, want.tagsArrays) {
		t.Errorf("%s: tags arrays diverged:\n  got =%v\n  want=%v", label, got.tagsArrays, want.tagsArrays)
	}
	if !reflect.DeepEqual(got.junctionPairs, want.junctionPairs) {
		t.Errorf("%s: junction pairs diverged:\n  got =%v\n  want=%v", label, got.junctionPairs, want.junctionPairs)
	}
	if !reflect.DeepEqual(got.tagCounts, want.tagCounts) {
		t.Errorf("%s: tag counts diverged:\n  got =%v\n  want=%v", label, got.tagCounts, want.tagCounts)
	}
	if !reflect.DeepEqual(got.tagIsSystem, want.tagIsSystem) {
		t.Errorf("%s: tag is_system diverged:\n  got =%v\n  want=%v", label, got.tagIsSystem, want.tagIsSystem)
	}
	if !reflect.DeepEqual(got.removalPairs, want.removalPairs) {
		t.Errorf("%s: removal pairs diverged:\n  got =%v\n  want=%v", label, got.removalPairs, want.removalPairs)
	}
	t.FailNow()
}

// injectMutationRound re-damages the state in a way that is distinct from the
// previous round. Each round targets a different activity and a different
// flavour of denormalization so the convergence assertion is non-trivial.
//
// round=1: stale removal row for indoorID; is_commute cleared in activities.tags but flag left true.
// round=2: stale E-Bike junction for runningElectricID (wrong sport); commute array entry absent.
// round=3: is_virtual=true activity has junction deleted directly (bypassing the protection trigger
//
//	via session_replication_role=replica), stale removal for Indoor.
//
// Triggers are disabled during tag-array writes so Direction A does not
// flip modifier flags back as a side effect of the damage injection.
func injectMutationRound(t *testing.T, ctx context.Context, tx pgx.Tx, f systemModifierTagSyncFixture, round int) {
	t.Helper()
	disableTriggers := func() {
		for _, stmt := range []string{
			`ALTER TABLE activities DISABLE TRIGGER trg_sync_system_flags_and_tags`,
			`ALTER TABLE activities DISABLE TRIGGER trg_sync_virtual_tag`,
		} {
			if _, err := tx.Exec(ctx, stmt); err != nil {
				t.Fatalf("mutation round %d: disable trigger: %v", round, err)
			}
		}
	}
	enableTriggers := func() {
		for _, stmt := range []string{
			`ALTER TABLE activities ENABLE TRIGGER trg_sync_system_flags_and_tags`,
			`ALTER TABLE activities ENABLE TRIGGER trg_sync_virtual_tag`,
		} {
			if _, err := tx.Exec(ctx, stmt); err != nil {
				t.Fatalf("mutation round %d: enable trigger: %v", round, err)
			}
		}
	}

	switch round {
	case 1:
		// Stale removal row for Indoor when is_indoor=true; Pendeln stripped from array.
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_tag_removals (activity_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, f.indoorID, f.indoorTagID); err != nil {
			t.Fatalf("round 1: insert stale removal: %v", err)
		}
		disableTriggers()
		if _, err := tx.Exec(ctx, `UPDATE activities SET tags = array_remove(tags, 'Pendeln') WHERE id = $1`, f.commuteID); err != nil {
			t.Fatalf("round 1: strip Pendeln: %v", err)
		}
		enableTriggers()

	case 2:
		// E-Bike junction injected for a non-cycling activity (running-electric):
		// flag is true but sport_type is wrong, so the junction must NOT be there.
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
			VALUES ($1, $2, 'mutation_test')
			ON CONFLICT DO NOTHING
		`, f.runningElectricID, f.eBikeTagID); err != nil {
			t.Fatalf("round 2: inject stale E-Bike junction: %v", err)
		}
		// Also strip Indoor from the array while is_indoor=true.
		disableTriggers()
		if _, err := tx.Exec(ctx, `UPDATE activities SET tags = array_remove(tags, 'Indoor') WHERE id = $1`, f.indoorID); err != nil {
			t.Fatalf("round 2: strip Indoor: %v", err)
		}
		enableTriggers()

	case 3:
		// Delete the virtuell junction directly via session_replication_role=replica
		// to bypass the protect_virtual_tag_removal trigger, leaving is_virtual=true
		// but no junction row. Also insert stale removal for Indoor.
		if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = 'replica'`); err != nil {
			t.Fatalf("round 3: set replica role: %v", err)
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM activity_tags at
			USING tags tg
			WHERE at.tag_id = tg.id
			  AND tg.user_id = $1
			  AND tg.name = 'virtuell'
			  AND at.activity_id = $2
		`, f.user1ID, f.virtualID); err != nil {
			t.Fatalf("round 3: delete virtuell junction via replica role: %v", err)
		}
		if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = 'origin'`); err != nil {
			t.Fatalf("round 3: restore replica role: %v", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_tag_removals (activity_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, f.indoorID, f.indoorTagID); err != nil {
			t.Fatalf("round 3: insert stale Indoor removal: %v", err)
		}

	default:
		t.Fatalf("injectMutationRound: unknown round %d", round)
	}
}

// TestSkadiaResyncSystemModifierTagsConvergesUnderMutation proves that the
// resync function converges to the same consistent state across multiple
// rounds of intervening damage — not just that successive identical calls are
// no-ops (Critic Round-2 #3).
//
// Protocol: call resync → capture canonical snapshot → inject a mutation
// that is different from the previous round → call resync again → assert
// the resulting snapshot equals the canonical one. Repeat for three mutation
// rounds. Each round's pre-state is materially different from the previous,
// and each post-state must be byte-for-byte identical.
func TestSkadiaResyncSystemModifierTagsConvergesUnderMutation(t *testing.T) {
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

	// Initial resync: repair all damage from damageSystemModifierState.
	callResync(t, ctx, tx, activityIDs...)
	canonical := snapshotResyncState(t, ctx, tx, f.user1ID, activityIDs, tagIDs)

	// Three mutation rounds. Each round: inject a different form of damage,
	// call resync, verify convergence to the canonical snapshot.
	for round := 1; round <= 3; round++ {
		injectMutationRound(t, ctx, tx, f, round)
		callResync(t, ctx, tx, activityIDs...)
		after := snapshotResyncState(t, ctx, tx, f.user1ID, activityIDs, tagIDs)
		assertResyncSnapshot(t, "round "+string(rune('0'+round)), after, canonical)
	}
}

// TestSkadiaResyncSystemModifierTagsIsIdempotent verifies that successive
// identical calls to the resync function on an already-consistent state are
// strict no-ops: snapshot_after_1 == snapshot_after_5. Complements
// TestSkadiaResyncSystemModifierTagsConvergesUnderMutation (which tests
// convergence under mutation) by separately pinning the no-op property.
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
	assertResyncSnapshot(t, "idempotency_after_5", snapAfter5, snapAfter1)
}

// TestTriggerAcquiresReservedTagLocksInDeterministicOrder runs two concurrent
// transactions, each of which flips different modifier columns on the same
// activity (touching Indoor then E-Bike in one goroutine and E-Bike then
// Indoor in the other). Without the Critic-#5 PK-order lock fence the two
// UPDATE triggers could grab row-locks in opposite orders → deadlock. With the
// fence, both transactions lock the reserved tag rows in the same UUID order so
// no cycle can form.
//
// The test starts N iterations in parallel; each iteration opens two
// transactions and races them. A deadlock surfaces as a pgx error containing
// "deadlock detected". The test asserts zero deadlock errors.
func TestTriggerAcquiresReservedTagLocksInDeterministicOrder(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	const iterations = 50

	// Shared fixture: one user, four system tags, one activity.
	userID := uuid.NewString()
	short := userID[:8]
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		VALUES ($1, $2, $3, true, 'test', false)
	`, userID, "dlk_"+short, "dlk_"+short+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
			t.Logf("cleanup user: %v", err)
		}
	})
	for _, name := range []string{"Indoor", "Pendeln", "virtuell", "E-Bike"} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
			VALUES ($1, $2, '#000000', '#FFFFFF', 'system', true, false, '', 0)
		`, userID, name); err != nil {
			t.Fatalf("insert %s tag: %v", name, err)
		}
	}

	var activityID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_indoor, is_electric, started_at)
		VALUES ($1, $2, 'dl-test', 'cycling', NULL, ARRAY['E-Bike','Indoor']::text[], true, true, now())
		RETURNING id
	`, uuid.NewString(), userID).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	// runToggle opens a transaction and flips firstCol then secondCol on the
	// activity, touching two different reserved tag rows in each direction.
	runToggle := func(errc chan<- error, firstCol, secondCol string) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			errc <- err
			return
		}
		defer func() {
			if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
				// committed — ignore
			}
		}()
		// allowed modifier columns — whitelist to prevent accidental injection.
		allowed := map[string]bool{"is_indoor": true, "is_electric": true, "is_commute": true}
		if !allowed[firstCol] || !allowed[secondCol] {
			errc <- nil // skip invalid combos silently
			return
		}
		// Each UPDATE triggers sync_system_flags_and_tags, which acquires
		// the lock fence before doing outbound work.
		for _, col := range []string{firstCol, secondCol} {
			if _, err := tx.Exec(ctx, `UPDATE activities SET `+col+` = NOT `+col+` WHERE id = $1`, activityID); err != nil {
				errc <- err
				return
			}
		}
		if err := tx.Commit(ctx); err != nil {
			errc <- err
			return
		}
		errc <- nil
	}

	for i := 0; i < iterations; i++ {
		errc := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); runToggle(errc, "is_indoor", "is_electric") }()
		go func() { defer wg.Done(); runToggle(errc, "is_electric", "is_indoor") }()
		wg.Wait()
		close(errc)
		for err := range errc {
			if err != nil && strings.Contains(err.Error(), "deadlock detected") {
				t.Fatalf("iteration %d: deadlock detected — PK-order lock fence did not prevent it: %v", i, err)
			}
			// serialization failures (40001) are acceptable under concurrent load;
			// only deadlocks (40P01) indicate the ordering bug.
		}
	}
}

// TestTriggerSkipFlagPreservesMidTxArrayConsistency verifies Critic-#13:
// under the skip flag, the trigger must not mutate activities.tags without
// also mutating activity_tags in the same step. The test scenario:
//
//  1. Insert a cycling+electric activity so it starts with E-Bike in both
//     activities.tags and activity_tags.
//  2. Set skip=true.
//  3. UPDATE sport_type to 'running' (non-cycling). Before the fix the trigger
//     stripped E-Bike from activities.tags unconditionally while leaving
//     activity_tags intact → mid-tx inconsistency. After the fix, skip
//     suppresses both.
//  4. Inside the same tx, SELECT activities.tags and activity_tags — assert
//     both agree (E-Bike present in both, or absent in both).
//  5. Commit. After commit, call the resync and assert E-Bike is gone from both
//     (the modifier flag no longer applies to a non-cycling activity).
//
// RED-by-tweak validation: after the test passes, inverting the assertion in
// step 4 ("want present" → "want absent") must fail.
func TestTriggerSkipFlagPreservesMidTxArrayConsistency(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	// Setup: user + four system tags + one cycling-electric activity.
	userID := uuid.NewString()
	short := userID[:8]
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		VALUES ($1, $2, $3, true, 'test', false)
	`, userID, "skp_"+short, "skp_"+short+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
			t.Logf("cleanup user: %v", err)
		}
	})
	for _, name := range []string{"Indoor", "Pendeln", "virtuell", "E-Bike"} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
			VALUES ($1, $2, '#000000', '#FFFFFF', 'system', true, false, '', 0)
		`, userID, name); err != nil {
			t.Fatalf("insert %s tag: %v", name, err)
		}
	}
	var activityID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_electric, started_at)
		VALUES ($1, $2, 'skip-test', 'cycling', NULL, ARRAY[]::text[], true, now())
		RETURNING id
	`, uuid.NewString(), userID).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	// After the INSERT trigger fires, E-Bike should be in activities.tags and
	// in activity_tags. Verify the pre-condition.
	var hasBikeInArray bool
	if err := pool.QueryRow(ctx, `
		SELECT 'E-Bike' = ANY(COALESCE(tags, '{}')) FROM activities WHERE id = $1
	`, activityID).Scan(&hasBikeInArray); err != nil {
		t.Fatalf("pre-condition query: %v", err)
	}
	if !hasBikeInArray {
		t.Fatalf("pre-condition: E-Bike not in activities.tags after INSERT trigger")
	}

	// Open a new transaction to exercise the skip-flag path.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	}()

	// Enable skip flag.
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
		t.Fatalf("set skip flag: %v", err)
	}

	// Change sport_type to running (non-cycling). Without the fix the trigger
	// strips E-Bike from activities.tags but not from activity_tags.
	if _, err := tx.Exec(ctx, `UPDATE activities SET sport_type = 'running' WHERE id = $1`, activityID); err != nil {
		t.Fatalf("update sport_type: %v", err)
	}

	// Mid-tx consistency check: activities.tags and activity_tags must agree.
	var arrayHasBike bool
	if err := tx.QueryRow(ctx, `
		SELECT 'E-Bike' = ANY(COALESCE(tags, '{}')) FROM activities WHERE id = $1
	`, activityID).Scan(&arrayHasBike); err != nil {
		t.Fatalf("mid-tx array query: %v", err)
	}
	var junctionHasBike bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity_tags at
			JOIN tags t ON t.id = at.tag_id
			WHERE at.activity_id = $1 AND t.user_id = $2 AND t.name = 'E-Bike' AND t.is_system = true
		)
	`, activityID, userID).Scan(&junctionHasBike); err != nil {
		t.Fatalf("mid-tx junction query: %v", err)
	}

	// Both must agree: either both present or both absent.
	if arrayHasBike != junctionHasBike {
		t.Fatalf("mid-tx inconsistency: activities.tags has E-Bike=%v but activity_tags has E-Bike=%v (Critic #13 regression)",
			arrayHasBike, junctionHasBike)
	}

	// Commit the transaction (sport_type=running, is_electric=true, skip was set).
	// The resync below will reconcile the final state.
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Post-commit: run the resync to apply the canonical final state.
	if _, err := pool.Exec(ctx, `SELECT skadia_resync_system_modifier_tags($1::uuid[])`, []string{activityID}); err != nil {
		t.Fatalf("resync: %v", err)
	}

	// After resync: non-cycling activity must not have E-Bike in either place.
	var postArrayHasBike bool
	if err := pool.QueryRow(ctx, `
		SELECT 'E-Bike' = ANY(COALESCE(tags, '{}')) FROM activities WHERE id = $1
	`, activityID).Scan(&postArrayHasBike); err != nil {
		t.Fatalf("post-commit array query: %v", err)
	}
	if postArrayHasBike {
		t.Fatalf("post-resync: E-Bike still in activities.tags for non-cycling activity")
	}
	var postJunctionHasBike bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity_tags at
			JOIN tags t ON t.id = at.tag_id
			WHERE at.activity_id = $1 AND t.user_id = $2 AND t.name = 'E-Bike' AND t.is_system = true
		)
	`, activityID, userID).Scan(&postJunctionHasBike); err != nil {
		t.Fatalf("post-commit junction query: %v", err)
	}
	if postJunctionHasBike {
		t.Fatalf("post-resync: E-Bike junction still present for non-cycling activity")
	}
}

// TestTriggerDirectionAGuardLeavesCorruptOLDUnmediated pins the Critic Round-2
// #4 trade-off: the slot-guard in the Direction-A UPDATE trigger compares
// OLD.tags membership to NEW.tags membership. When OLD.tags is NULL (a
// pre-353 inconsistency where is_indoor=true but tags was never populated),
// the slot-guard expression `'Indoor' = ANY(NULL)` evaluates to NULL (treated
// as false). If NEW.tags is '{}' (also not containing 'Indoor'), neither
// OLD nor NEW membership changed, so the guard suppresses re-derivation and
// is_indoor stays true even though tags is now empty — the activity is
// "corrupt by design" after an unrelated tag edit.
//
// This is intentional: the trigger's job is slot-change detection, not
// arbitrary state repair. Manual repair is the job of
// skadia_resync_system_modifier_tags, which IS able to converge the state
// correctly. The test asserts both halves of the contract:
//
//  1. After an unrelated tag UPDATE on a corrupt-OLD activity,
//     is_indoor=true and tags={} — no auto-heal by the trigger.
//  2. After calling skadia_resync_system_modifier_tags, the state converges
//     using the modifier flag as the source of truth: is_indoor=true is
//     preserved, 'Indoor' is added back to tags, and the junction is
//     inserted. The flag wins — corrupt was tags=NULL, not is_indoor=true.
func TestTriggerDirectionAGuardLeavesCorruptOLDUnmediated(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	// Seed an activity with is_indoor=true but tags=NULL, bypassing triggers
	// via session_replication_role=replica. This models a pre-353 row where
	// the modifier flag and the tags array were never in sync.
	activityID := uuid.NewString()
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = 'replica'`); err != nil {
		t.Fatalf("set replica role: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_indoor, started_at)
		VALUES ($1, $2, 'corrupt-old-test', 'rowing', NULL, NULL, true, now())
	`, activityID, f.userID); err != nil {
		t.Fatalf("insert corrupt activity: %v", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = 'origin'`); err != nil {
		t.Fatalf("restore origin role: %v", err)
	}

	// Verify the pre-condition: is_indoor=true, tags IS NULL.
	var preIsIndoor bool
	var preTagsIsNull bool
	if err := tx.QueryRow(ctx, `
		SELECT is_indoor, tags IS NULL FROM activities WHERE id = $1
	`, activityID).Scan(&preIsIndoor, &preTagsIsNull); err != nil {
		t.Fatalf("pre-condition query: %v", err)
	}
	if !preIsIndoor {
		t.Fatalf("pre-condition: is_indoor must be true")
	}
	if !preTagsIsNull {
		t.Fatalf("pre-condition: tags must be NULL (corrupt state)")
	}

	// Perform an unrelated tag UPDATE: set tags to '{}'. This does NOT touch
	// the 'Indoor' slot in either OLD or NEW (NULL→{} means 'Indoor'=ANY(NULL)
	// is NULL/false in OLD, 'Indoor'=ANY('{}') is false in NEW — slot stable).
	// Pre-353 the trigger would re-derive is_indoor from NEW.tags, flipping it
	// to false. Post-353 the slot-guard suppresses that re-derivation.
	if _, err := tx.Exec(ctx, `UPDATE activities SET tags = '{}'::text[] WHERE id = $1`, activityID); err != nil {
		t.Fatalf("unrelated tag update: %v", err)
	}

	// Assert Part 1: corrupt state is preserved — no auto-heal by the trigger.
	// is_indoor must still be true even though tags is now empty.
	var postUpdateIsIndoor bool
	var postUpdateTags []string
	if err := tx.QueryRow(ctx, `
		SELECT is_indoor, COALESCE(tags, ARRAY[]::text[]) FROM activities WHERE id = $1
	`, activityID).Scan(&postUpdateIsIndoor, &postUpdateTags); err != nil {
		t.Fatalf("post-update query: %v", err)
	}
	if !postUpdateIsIndoor {
		t.Fatalf("Critic #4 contract broken: trigger auto-healed is_indoor=false after unrelated tag update on corrupt-OLD activity; expected is_indoor=true (manual repair via resync only)")
	}
	if containsStr(postUpdateTags, "Indoor") {
		t.Fatalf("post-update: 'Indoor' unexpectedly appeared in tags array after unrelated update, got %v", postUpdateTags)
	}

	// Assert Part 2: after calling skadia_resync_system_modifier_tags, the
	// state converges using the modifier flag as truth (resync rebuilds the
	// tags array from is_indoor/is_commute/is_virtual/is_electric and inserts
	// matching junctions). is_indoor=true on the corrupt row therefore wins:
	// 'Indoor' is added back to tags and the junction is inserted. The flag
	// is authoritative; the corrupt half was tags=NULL, not is_indoor=true.
	callResync(t, ctx, tx, activityID)

	var postResyncIsIndoor bool
	var postResyncTags []string
	if err := tx.QueryRow(ctx, `
		SELECT is_indoor, COALESCE(tags, ARRAY[]::text[]) FROM activities WHERE id = $1
	`, activityID).Scan(&postResyncIsIndoor, &postResyncTags); err != nil {
		t.Fatalf("post-resync query: %v", err)
	}
	// Resync derives the canonical state from modifier flags. is_indoor=true
	// means the resync ADDS 'Indoor' back to tags and the junction. This is
	// correct: is_indoor=true is the truth; the corrupt state was tags=NULL,
	// not is_indoor=true being wrong.
	if !postResyncIsIndoor {
		t.Fatalf("post-resync: is_indoor was cleared by resync; flag is authoritative when tags was corrupt-NULL")
	}
	if !containsStr(postResyncTags, "Indoor") {
		t.Fatalf("post-resync: 'Indoor' not in tags array after resync with is_indoor=true, got %v", postResyncTags)
	}
	hasSystemTagLink(t, ctx, tx, f.userID, activityID, "Indoor", true)
}

// TestActivityTagsDeleteTriggerFlipsModifierColumn covers the m.1 DELETE-trigger
// contract: a direct DELETE FROM activity_tags against a registry-backed
// reserved-name junction must reconcile the activity row — flip the modifier
// column to false, strip the literal from activities.tags, and recompute the
// tag's activity_count. Without this trigger, an admin SQL DELETE would leave
// is_indoor=true while the junction is gone (denormalisation drift).
func TestActivityTagsDeleteTriggerFlipsModifierColumn(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	// Insert an indoor activity. Triggers populate the junction + tags array.
	actID := insertActivity(t, ctx, tx, f.userID, "rowing", true, false, false, false, []string{})
	hasSystemTagLink(t, ctx, tx, f.userID, actID, "Indoor", true)
	assertModifierFlag(t, ctx, tx, actID, "is_indoor", true)
	activityHasTag(t, ctx, tx, actID, "Indoor", true)

	// Direct DELETE on the junction (admin SQL path).
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_tags WHERE activity_id = $1 AND tag_id = $2
	`, actID, f.indoorTagID); err != nil {
		t.Fatalf("direct delete junction: %v", err)
	}

	// Modifier column flipped to false; literal stripped from tags array;
	// junction gone; activity_count decreased.
	assertModifierFlag(t, ctx, tx, actID, "is_indoor", false)
	activityHasTag(t, ctx, tx, actID, "Indoor", false)
	hasSystemTagLink(t, ctx, tx, f.userID, actID, "Indoor", false)
	if got := tagActivityCount(t, ctx, tx, f.indoorTagID); got != 0 {
		t.Fatalf("Indoor tag activity_count = %d, want 0", got)
	}
}

// TestActivityTagsDeleteTriggerSkipFlag covers the bulk-helper contract: under
// skadia.skip_system_modifier_tag_sync the DELETE trigger must no-op so the
// caller-managed transaction owns reconciliation through resync.
func TestActivityTagsDeleteTriggerSkipFlag(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)
	actID := insertActivity(t, ctx, tx, f.userID, "rowing", true, false, false, false, []string{})

	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
		t.Fatalf("set skip flag: %v", err)
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_tags WHERE activity_id = $1 AND tag_id = $2
	`, actID, f.indoorTagID); err != nil {
		t.Fatalf("delete with skip flag: %v", err)
	}

	// Modifier column unchanged — resync owns reconciliation.
	assertModifierFlag(t, ctx, tx, actID, "is_indoor", true)
	// Tag literal still in array (trigger did not strip).
	activityHasTag(t, ctx, tx, actID, "Indoor", true)
}

// TestActivityTagsDeleteTriggerNonReservedTagIsNoOp ensures the DELETE trigger
// only fires for the four registry-backed reserved-name system tags. Deleting
// a custom user tag's junction must not touch modifier columns or the tags
// array — the caller's UPDATE owns array maintenance for non-reserved tags.
func TestActivityTagsDeleteTriggerNonReservedTagIsNoOp(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	// Custom user tag with a non-reserved name.
	var customTagID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
		VALUES ($1, 'Berg', '#AABBCC', '#FFFFFF', 'custom', false, false, '', 0)
		RETURNING id
	`, f.userID).Scan(&customTagID); err != nil {
		t.Fatalf("insert custom tag: %v", err)
	}

	actID := insertActivity(t, ctx, tx, f.userID, "rowing", true, false, false, false, []string{"Berg"})
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES ($1, $2, 'manual')
	`, actID, customTagID); err != nil {
		t.Fatalf("insert custom junction: %v", err)
	}

	// Pre-condition: is_indoor=true, 'Indoor' AND 'Berg' in tags.
	assertModifierFlag(t, ctx, tx, actID, "is_indoor", true)
	activityHasTag(t, ctx, tx, actID, "Berg", true)

	// Delete only the custom tag junction.
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_tags WHERE activity_id = $1 AND tag_id = $2
	`, actID, customTagID); err != nil {
		t.Fatalf("delete custom junction: %v", err)
	}

	// Modifier column untouched, 'Indoor' still in array, 'Berg' still in
	// the array because the trigger does NOT manage non-reserved tags
	// (caller owns array maintenance for those).
	assertModifierFlag(t, ctx, tx, actID, "is_indoor", true)
	activityHasTag(t, ctx, tx, actID, "Indoor", true)
	activityHasTag(t, ctx, tx, actID, "Berg", true)
}

// TestActivityTagsDeleteTriggerCASCADEFromTagsDelete covers the tag-deletion
// path. Reserved-name system tags are protected by prevent_system_tag_delete
// so a direct DELETE FROM tags is impossible while is_system=true. The
// realistic path that triggers CASCADE on activity_tags for a reserved-name
// row is the demote-then-delete sequence (user removes the system shape,
// then deletes the now-custom tag). Our AFTER DELETE trigger fires per
// cascaded row. By that point the tag row is_system=false, so the trigger
// must NOT flip the modifier column for non-system rows — confirming the
// is_system=true gate matches the registry contract.
func TestActivityTagsDeleteTriggerCASCADEFromTagsDelete(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	act1 := insertActivity(t, ctx, tx, f.userID, "rowing", true, false, false, false, []string{})
	act2 := insertActivity(t, ctx, tx, f.userID, "running", true, false, false, false, []string{})

	hasSystemTagLink(t, ctx, tx, f.userID, act1, "Indoor", true)
	hasSystemTagLink(t, ctx, tx, f.userID, act2, "Indoor", true)

	// Demote: is_system=false. This makes prevent_system_tag_delete tolerate
	// the subsequent DELETE.
	if _, err := tx.Exec(ctx, `UPDATE tags SET is_system = false WHERE id = $1`, f.indoorTagID); err != nil {
		t.Fatalf("demote tag: %v", err)
	}

	// Delete the (now custom) tag. CASCADE removes both junctions; our
	// AFTER DELETE trigger fires per row but the is_system gate skips the
	// modifier reconciliation since the tag is no longer a registry-backed
	// system tag. No recursion, no error.
	if _, err := tx.Exec(ctx, `DELETE FROM tags WHERE id = $1`, f.indoorTagID); err != nil {
		t.Fatalf("delete demoted tag (CASCADE): %v", err)
	}

	// Junctions gone (CASCADE). Modifier columns and array UNCHANGED because
	// the tag is_system=false at the moment the trigger reads it.
	var stillThere bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM activity_tags WHERE activity_id = $1 AND tag_id = $2)`, act1, f.indoorTagID).Scan(&stillThere); err != nil {
		t.Fatalf("query junction existence: %v", err)
	}
	if stillThere {
		t.Fatalf("CASCADE failed: junction still present for act1")
	}
	assertModifierFlag(t, ctx, tx, act1, "is_indoor", true)
	assertModifierFlag(t, ctx, tx, act2, "is_indoor", true)
}

// TestActivityTagsDeleteTriggerEBikeStrip covers the cycling-only E-Bike
// branch: deleting the E-Bike junction on a cycling activity must flip
// is_electric=false. The tag literal is stripped from the tags array and
// activity_count is decremented.
func TestActivityTagsDeleteTriggerEBikeStrip(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	actID := insertActivity(t, ctx, tx, f.userID, "cycling", false, true, false, false, []string{})
	hasSystemTagLink(t, ctx, tx, f.userID, actID, "E-Bike", true)
	assertModifierFlag(t, ctx, tx, actID, "is_electric", true)

	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_tags WHERE activity_id = $1 AND tag_id = $2
	`, actID, f.eBikeTagID); err != nil {
		t.Fatalf("delete E-Bike junction: %v", err)
	}

	assertModifierFlag(t, ctx, tx, actID, "is_electric", false)
	activityHasTag(t, ctx, tx, actID, "E-Bike", false)
	if got := tagActivityCount(t, ctx, tx, f.eBikeTagID); got != 0 {
		t.Fatalf("E-Bike tag activity_count = %d, want 0", got)
	}
}

// TestActivityTagsDeleteTriggerEBikeStripPreservesFlagOnNonCycling encodes the
// 000350 design contract that is_electric is a generic modifier flag and is
// preserved on non-cycling activities even when an orphan E-Bike junction is
// deleted. Migration 000350's one-time cleanup pass relies on this — the
// cleanup strips orphan E-Bike junctions from non-cycling activities while
// keeping is_electric=true on those rows so user-meaningful data isn't lost.
func TestActivityTagsDeleteTriggerEBikeStripPreservesFlagOnNonCycling(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	// Create a non-cycling activity with is_electric=true. The standard
	// trigger does not link E-Bike for non-cycling, so we insert the
	// junction manually under the skip flag (defensive orphan scenario).
	actID := insertActivity(t, ctx, tx, f.userID, "running", false, true, false, false, []string{})
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
		t.Fatalf("set skip flag: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES ($1, $2, 'manual')
	`, actID, f.eBikeTagID); err != nil {
		t.Fatalf("insert orphan E-Bike junction: %v", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'false'`); err != nil {
		t.Fatalf("unset skip flag: %v", err)
	}

	// Pre-condition: orphan junction present, is_electric=true.
	hasSystemTagLink(t, ctx, tx, f.userID, actID, "E-Bike", true)
	assertModifierFlag(t, ctx, tx, actID, "is_electric", true)

	// DELETE the orphan junction. The trigger should strip the tag literal
	// from the array (if present) but PRESERVE is_electric=true.
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_tags WHERE activity_id = $1 AND tag_id = $2
	`, actID, f.eBikeTagID); err != nil {
		t.Fatalf("delete orphan E-Bike junction: %v", err)
	}

	// Flag preserved (000350 design contract); junction gone.
	assertModifierFlag(t, ctx, tx, actID, "is_electric", true)
	hasSystemTagLink(t, ctx, tx, f.userID, actID, "E-Bike", false)
	activityHasTag(t, ctx, tx, actID, "E-Bike", false)
}

// TestActivityTagsDeleteTriggerSkipsInsideOuterTriggerCascade pins the contract
// that the m.1 AFTER DELETE trigger does NOT re-update activities when the
// activity_tags DELETE was emitted from inside another trigger's body
// (pg_trigger_depth() > 1). Without this gate, an UPDATE on activities that
// fires sync_system_flags_and_tags BEFORE UPDATE — which itself executes
// DELETE FROM activity_tags as part of the sport-change orchestration —
// would re-enter the m.1 trigger and try to UPDATE the same mid-update
// activities row, raising "tuple to be updated was already modified by an
// operation triggered by the current command" (SQLSTATE 27000).
//
// The 000350 cycling-electric tag sync regression test reproduces this in
// full; this test pins the m.1-internal contract directly.
func TestActivityTagsDeleteTriggerSkipsInsideOuterTriggerCascade(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	// Cycling activity with is_electric=true → triggers populate the E-Bike
	// junction and 'E-Bike' in tags.
	actID := insertActivity(t, ctx, tx, f.userID, "cycling", false, true, false, false, []string{})
	hasSystemTagLink(t, ctx, tx, f.userID, actID, "E-Bike", true)
	activityHasTag(t, ctx, tx, actID, "E-Bike", true)
	assertModifierFlag(t, ctx, tx, actID, "is_electric", true)

	// Sport-change cascade: sync_system_flags_and_tags BEFORE UPDATE detects
	// the move away from cycling and emits DELETE FROM activity_tags. The m.1
	// trigger must skip when pg_trigger_depth() > 1, otherwise we hit
	// SQLSTATE 27000 here.
	if _, err := tx.Exec(ctx, `UPDATE activities SET sport_type = 'running' WHERE id = $1`, actID); err != nil {
		t.Fatalf("change cycling to running: %v", err)
	}

	// Outer trigger orchestrated the transition: junction gone, array
	// reflects the move, is_electric preserved on non-cycling per 000350
	// design contract.
	hasSystemTagLink(t, ctx, tx, f.userID, actID, "E-Bike", false)
	activityHasTag(t, ctx, tx, actID, "E-Bike", false)
	assertModifierFlag(t, ctx, tx, actID, "is_electric", true)
}

// TestTagRenameTriggerPropagatesIntoActivitiesTagsArray verifies that an
// UPDATE on a system tag's name propagates into every activities.tags TEXT[]
// array that contains the OLD literal. Junction tag_id stays unchanged
// (junction is keyed on tag_id, not name) and modifier columns are
// untouched (rename does not change semantic membership).
func TestTagRenameTriggerPropagatesIntoActivitiesTagsArray(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	// Insert an indoor activity. Triggers populate junction + 'Indoor' in tags.
	actID := insertActivity(t, ctx, tx, f.userID, "rowing", true, false, false, false, []string{})
	hasSystemTagLink(t, ctx, tx, f.userID, actID, "Indoor", true)
	activityHasTag(t, ctx, tx, actID, "Indoor", true)
	assertModifierFlag(t, ctx, tx, actID, "is_indoor", true)

	// Rename the system Indoor tag. Without the propagation trigger, the
	// activities.tags array would still say 'Indoor' even though the tag row
	// is now named 'Hallensport'.
	if _, err := tx.Exec(ctx, `UPDATE tags SET name = 'Hallensport' WHERE id = $1`, f.indoorTagID); err != nil {
		t.Fatalf("rename Indoor tag: %v", err)
	}

	// Array now reflects the new name.
	activityHasTag(t, ctx, tx, actID, "Hallensport", true)
	activityHasTag(t, ctx, tx, actID, "Indoor", false)

	// Junction unchanged: tag_id is the same row, still linked.
	var junctionTagID string
	if err := tx.QueryRow(ctx, `
		SELECT tag_id FROM activity_tags WHERE activity_id = $1 AND tag_id = $2
	`, actID, f.indoorTagID).Scan(&junctionTagID); err != nil {
		t.Fatalf("junction must still exist for renamed tag: %v", err)
	}
	if junctionTagID != f.indoorTagID {
		t.Fatalf("junction tag_id = %s, want %s", junctionTagID, f.indoorTagID)
	}

	// Modifier column unchanged: rename does not alter semantic membership.
	assertModifierFlag(t, ctx, tx, actID, "is_indoor", true)
}

// TestTagRenameTriggerSkipFlag verifies that under the bulk-helper skip flag
// the rename propagation is suppressed — the caller-managed transaction owns
// reconciliation through skadia_resync_system_modifier_tags or its own
// array UPDATEs.
func TestTagRenameTriggerSkipFlag(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	actID := insertActivity(t, ctx, tx, f.userID, "rowing", true, false, false, false, []string{})
	activityHasTag(t, ctx, tx, actID, "Indoor", true)

	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
		t.Fatalf("set skip flag: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE tags SET name = 'Hallensport' WHERE id = $1`, f.indoorTagID); err != nil {
		t.Fatalf("rename Indoor tag under skip: %v", err)
	}

	// Array unchanged — propagation was suppressed.
	activityHasTag(t, ctx, tx, actID, "Indoor", true)
	activityHasTag(t, ctx, tx, actID, "Hallensport", false)
}

// TestTagRenameTriggerNonSystemTagDoesNotPropagate documents the narrow-scope
// boundary: non-system tag renames intentionally do NOT propagate into
// activities.tags arrays. The bug class exists for non-system tags too, but
// fixing it is a separate UX-driven decision (does the user expect the array
// to follow a custom-tag rename, or is the array a snapshot of past state?)
// that belongs in a follow-up phase.
func TestTagRenameTriggerNonSystemTagDoesNotPropagate(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	// Insert a custom (non-system) tag.
	var customTagID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
		VALUES ($1, 'Tour', '#AABBCC', '#FFFFFF', 'custom', false, false, '', 0)
		RETURNING id
	`, f.userID).Scan(&customTagID); err != nil {
		t.Fatalf("insert custom tag: %v", err)
	}

	// Insert an activity tagged with the custom 'Tour' name; junction too.
	actID := insertActivity(t, ctx, tx, f.userID, "rowing", false, false, false, false, []string{"Tour"})
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES ($1, $2, 'manual')
	`, actID, customTagID); err != nil {
		t.Fatalf("insert custom junction: %v", err)
	}

	// Rename the custom tag.
	if _, err := tx.Exec(ctx, `UPDATE tags SET name = 'Wandern' WHERE id = $1`, customTagID); err != nil {
		t.Fatalf("rename custom tag: %v", err)
	}

	// Documented gap: array literal does NOT follow custom-tag rename.
	activityHasTag(t, ctx, tx, actID, "Tour", true)
	activityHasTag(t, ctx, tx, actID, "Wandern", false)
}

// TestTagRenameTriggerCrossUserIsolation verifies the trigger's
// WHERE user_id = NEW.user_id clause: renaming user A's system Indoor tag
// must not touch user B's activities.tags arrays even though user B has a
// row with the same name. Tags is unique on (user_id, name), so each user
// owns their own row scoped by user_id.
func TestTagRenameTriggerCrossUserIsolation(t *testing.T) {
	ctx, tx := beginMigrationTx(t)

	// Two users, each with their own system Indoor tag.
	userA := uuid.NewString()
	userB := uuid.NewString()
	for _, u := range []struct{ id, prefix string }{
		{userA, "renA_"},
		{userB, "renB_"},
	} {
		short := u.id[:8]
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, u.id, u.prefix+short, u.prefix+short+"@example.com"); err != nil {
			t.Fatalf("insert user %s: %v", u.id, err)
		}
	}
	var indoorA, indoorB string
	if err := tx.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
		VALUES ($1, 'Indoor', '#3B82F6', '#FFFFFF', 'system', true, false, '', 0)
		RETURNING id
	`, userA).Scan(&indoorA); err != nil {
		t.Fatalf("insert userA Indoor: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description, activity_count)
		VALUES ($1, 'Indoor', '#3B82F6', '#FFFFFF', 'system', true, false, '', 0)
		RETURNING id
	`, userB).Scan(&indoorB); err != nil {
		t.Fatalf("insert userB Indoor: %v", err)
	}

	// Each user has an indoor activity. Triggers populate the tag array.
	actA := insertActivity(t, ctx, tx, userA, "rowing", true, false, false, false, []string{})
	actB := insertActivity(t, ctx, tx, userB, "rowing", true, false, false, false, []string{})
	activityHasTag(t, ctx, tx, actA, "Indoor", true)
	activityHasTag(t, ctx, tx, actB, "Indoor", true)

	// Rename ONLY userA's Indoor tag.
	if _, err := tx.Exec(ctx, `UPDATE tags SET name = 'Hallensport' WHERE id = $1`, indoorA); err != nil {
		t.Fatalf("rename userA Indoor tag: %v", err)
	}

	// userA's array followed the rename.
	activityHasTag(t, ctx, tx, actA, "Hallensport", true)
	activityHasTag(t, ctx, tx, actA, "Indoor", false)

	// userB's array is UNCHANGED — cross-user isolation.
	activityHasTag(t, ctx, tx, actB, "Indoor", true)
	activityHasTag(t, ctx, tx, actB, "Hallensport", false)
}

// TestTagRenameTriggerWithMultipleArrayEntries verifies that array_replace
// preserves position and adjacent unrelated entries. Tags = ['X', 'Indoor',
// 'Y'] should become ['X', 'Hallensport', 'Y'] — same length, same order,
// only the matching slot replaced.
func TestTagRenameTriggerWithMultipleArrayEntries(t *testing.T) {
	ctx, tx := beginMigrationTx(t)
	f := makeDirectionAFixture(t, ctx, tx)

	// Insert with the array already populated. The INSERT trigger derives
	// is_indoor from the 'Indoor' literal (Direction A on INSERT).
	actID := insertActivity(t, ctx, tx, f.userID, "rowing", false, false, false, false, []string{"X", "Indoor", "Y"})
	got := queryTagsArray(t, ctx, tx, actID)
	if !reflect.DeepEqual(got, []string{"X", "Indoor", "Y"}) {
		t.Fatalf("pre-rename tags = %v, want [X Indoor Y]", got)
	}

	if _, err := tx.Exec(ctx, `UPDATE tags SET name = 'Hallensport' WHERE id = $1`, f.indoorTagID); err != nil {
		t.Fatalf("rename Indoor tag: %v", err)
	}

	// Order preserved, only the matching slot replaced.
	got = queryTagsArray(t, ctx, tx, actID)
	if !reflect.DeepEqual(got, []string{"X", "Hallensport", "Y"}) {
		t.Fatalf("post-rename tags = %v, want [X Hallensport Y]", got)
	}
}
