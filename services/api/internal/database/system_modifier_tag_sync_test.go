package database

// TDD coverage for the WithSystemModifierTagSyncSuspended helper. The helper
// is the single sanctioned entry point for batch updates that may legitimately
// suppress per-row trigger work for the registry-backed system modifier tags.
// Every test boots a fresh user via testdb.Open and asserts both the
// transaction lifecycle and the resync side effects.
//
// Finding #18 tests (assertSystemModifierTagSyncSkipFlagSet) are at the bottom.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func systemModifierTagSyncTestDB(t *testing.T) (*DB, string, []string) {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "smts-"+userID[:8], "smts-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	// Seed the four reserved system tags so the resync function never has
	// to insert them; this isolates the helper test from tag-creation
	// semantics that other tests already cover.
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

	// Three cycling activities. Modifier flags start false so the helper
	// has measurable work to do.
	ids := make([]string, 3)
	for i := range ids {
		id := uuid.NewString()
		if _, err := pool.Exec(ctx, `
			INSERT INTO activities (id, user_id, title, sport_type, sub_type, tags, is_indoor, is_electric, is_virtual, is_commute, started_at)
			VALUES ($1, $2, $3, 'cycling', NULL, ARRAY[]::text[], false, false, false, false, now())
		`, id, userID, fmt.Sprintf("smts %d", i)); err != nil {
			t.Fatalf("insert activity %d: %v", i, err)
		}
		ids[i] = id
	}
	return &DB{Pool: pool}, userID, ids
}

func activityHasIndoorTagDB(t *testing.T, ctx context.Context, pool *pgxpool.Pool, activityID string) bool {
	t.Helper()
	var got bool
	if err := pool.QueryRow(ctx, `
		SELECT 'Indoor' = ANY(COALESCE(tags, '{}')) FROM activities WHERE id = $1
	`, activityID).Scan(&got); err != nil {
		t.Fatalf("query indoor tag: %v", err)
	}
	return got
}

func indoorJunctionExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, activityID string) bool {
	t.Helper()
	var got bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity_tags at JOIN tags t ON t.id = at.tag_id
			WHERE at.activity_id = $1 AND t.name = 'Indoor' AND t.is_system = true
		)
	`, activityID).Scan(&got); err != nil {
		t.Fatalf("query indoor junction: %v", err)
	}
	return got
}

func TestWithSystemModifierTagSyncSuspendedResyncsBeforeCommit(t *testing.T) {
	db, _, ids := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	err := db.WithSystemModifierTagSyncSuspended(ctx, ids, func(ctx context.Context, tx pgx.Tx) error {
		// Bulk-flip is_indoor for two of the three activities. With the
		// skip flag set, the BEFORE UPDATE trigger must NOT add 'Indoor'
		// to the tags array or insert the activity_tags junction.
		_, err := tx.Exec(ctx, `UPDATE activities SET is_indoor = true WHERE id = ANY($1::uuid[])`, ids[:2])
		return err
	})
	if err != nil {
		t.Fatalf("helper returned: %v", err)
	}

	// After the helper commits, the resync call must have repaired the
	// denormalized tag state.
	for _, id := range ids[:2] {
		if !activityHasIndoorTagDB(t, ctx, db.Pool, id) {
			t.Fatalf("activity %s tags array missing 'Indoor' after helper resync", id)
		}
		if !indoorJunctionExists(t, ctx, db.Pool, id) {
			t.Fatalf("activity %s indoor junction missing after helper resync", id)
		}
	}
	// The third activity stayed is_indoor=false: resync must not invent
	// state for it.
	if activityHasIndoorTagDB(t, ctx, db.Pool, ids[2]) {
		t.Fatalf("activity %s gained 'Indoor' tag despite is_indoor=false", ids[2])
	}
}

func TestWithSystemModifierTagSyncSuspendedRollsBackOnUpdateError(t *testing.T) {
	db, _, ids := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	wantErr := errors.New("intentional fn failure")
	err := db.WithSystemModifierTagSyncSuspended(ctx, ids, func(ctx context.Context, tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `UPDATE activities SET is_indoor = true WHERE id = ANY($1::uuid[])`, ids); e != nil {
			t.Fatalf("update failed: %v", e)
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("helper err = %v, want wrap of %v", err, wantErr)
	}

	// Roll-back means none of the activities have their modifier flag set.
	for _, id := range ids {
		var got bool
		if err := db.Pool.QueryRow(ctx, `SELECT is_indoor FROM activities WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatalf("query is_indoor: %v", err)
		}
		if got {
			t.Fatalf("activity %s is_indoor = true after rollback, want false", id)
		}
	}
}

// TestWithSystemModifierTagSyncSuspendedRejectsMalformedUUID verifies that
// UUID validation fires before the transaction opens. After Critic Round-2
// finding #8 landed, the helper validates all UUIDs via uuid.Parse before
// calling Pool.Begin — a malformed ID is therefore rejected at the validation
// stage, not at the Postgres ::uuid[] cast inside the resync call.
//
// The former TestWithSystemModifierTagSyncSuspendedRollsBackOnResyncError used
// a malformed UUID to force a mid-tx cast failure; that injection no longer
// reaches the resync step. The rollback-on-fn-error path is already covered by
// TestWithSystemModifierTagSyncSuspendedRollsBackOnUpdateError. A separate test
// that specifically exercises the resync-step error path would require a
// session-level trigger or advisory lock injection that goes beyond what the
// helper's narrow SQL surface exposes, so it is deferred (Critic Round-2 #15).
func TestWithSystemModifierTagSyncSuspendedRejectsMalformedUUID(t *testing.T) {
	db, _, ids := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	fnCalled := false
	// Mix valid UUIDs with one malformed entry at index 1.
	poisonIDs := []string{ids[0], "not-a-valid-uuid", ids[1]}
	err := db.WithSystemModifierTagSyncSuspended(ctx, poisonIDs, func(ctx context.Context, tx pgx.Tx) error {
		fnCalled = true
		return nil
	})
	if err == nil {
		t.Fatal("helper returned nil despite malformed UUID at index 1")
	}
	if fnCalled {
		t.Fatal("fn was called; validation must reject before opening tx")
	}
	// Error message must name the offending value and its index.
	errStr := err.Error()
	if !strings.Contains(errStr, "not-a-valid-uuid") {
		t.Errorf("error %q does not mention offending value", errStr)
	}
	if !strings.Contains(errStr, "index 1") {
		t.Errorf("error %q does not mention index 1", errStr)
	}

	// Sanity: the activities table is untouched — no tx was opened.
	for _, id := range ids {
		var got bool
		if e := db.Pool.QueryRow(ctx, `SELECT is_indoor FROM activities WHERE id = $1`, id).Scan(&got); e != nil {
			t.Fatalf("query is_indoor: %v", e)
		}
		if got {
			t.Fatalf("activity %s is_indoor = true; no tx should have been opened", id)
		}
	}
}

// TestWithSystemModifierTagSyncSuspendedRejectsEmptyAfterDedupe verifies that
// an all-empty-string input is rejected BEFORE opening a transaction.
// Previously an input like ["", "", ""] passed the len>0 gate, reached
// deduplication which returned nil, then the two len(deduped)>0 blocks both
// became false — fn ran without the skip flag set and without a resync call,
// silently violating the helper's contract.
func TestWithSystemModifierTagSyncSuspendedRejectsEmptyAfterDedupe(t *testing.T) {
	db, _, _ := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	fnCalled := false
	err := db.WithSystemModifierTagSyncSuspended(ctx, []string{"", "", ""}, func(ctx context.Context, tx pgx.Tx) error {
		fnCalled = true
		return nil
	})
	if err == nil {
		t.Fatal("helper returned nil for all-empty-string input; expected error")
	}
	if fnCalled {
		t.Fatal("fn was called; validation must reject before opening tx")
	}
	// Error message must indicate the post-dedupe-empty condition.
	errStr := err.Error()
	if !strings.Contains(errStr, "no valid activity IDs") && !strings.Contains(errStr, "empty") {
		t.Errorf("error %q does not describe the empty-after-dedupe condition", errStr)
	}
}

func TestWithSystemModifierTagSyncSuspendedDeduplicatesIDs(t *testing.T) {
	db, _, ids := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	// Build a slice with intentional duplicates and an empty string. The helper
	// must deduplicate before passing IDs to the resync function; we verify this
	// by asserting the correct DB state after a successful commit rather than
	// by overriding the production function (which would risk pgxpool
	// prepared-statement cache collisions).
	withDupes := []string{ids[0], ids[1], ids[0], "", ids[1], ids[2], ids[0]}

	err := db.WithSystemModifierTagSyncSuspended(ctx, withDupes, func(ctx context.Context, tx pgx.Tx) error {
		// Flip all three activities to is_indoor=true; with the skip flag set
		// the trigger will not add the tag — resync repairs this at commit.
		_, err := tx.Exec(ctx, `UPDATE activities SET is_indoor = true WHERE id = ANY($1::uuid[])`, ids)
		return err
	})
	if err != nil {
		t.Fatalf("helper with duplicates: %v", err)
	}

	// After commit the resync must have repaired all three activities exactly
	// once (deduplication means each ID is processed once by the resync).
	for _, id := range ids {
		if !activityHasIndoorTagDB(t, ctx, db.Pool, id) {
			t.Fatalf("activity %s tags missing 'Indoor' after dedup resync", id)
		}
		if !indoorJunctionExists(t, ctx, db.Pool, id) {
			t.Fatalf("activity %s indoor junction missing after dedup resync", id)
		}
	}
}

// TestDedupeSystemModifierActivityIDsUnit is a pure unit test for the
// deduplication helper — no DB required.
func TestDedupeSystemModifierActivityIDsUnit(t *testing.T) {
	id1 := "11111111-1111-1111-1111-111111111111"
	id2 := "22222222-2222-2222-2222-222222222222"
	id3 := "33333333-3333-3333-3333-333333333333"

	got := dedupeSystemModifierActivityIDs([]string{id1, id2, id1, "", id2, id3, id1})
	want := []string{id1, id2, id3}
	if len(got) != len(want) {
		t.Fatalf("deduped = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("deduped[%d] = %q, want %q", i, got[i], w)
		}
	}

	if got := dedupeSystemModifierActivityIDs(nil); got != nil {
		t.Fatalf("nil input: got %v, want nil", got)
	}
	if got := dedupeSystemModifierActivityIDs([]string{}); got != nil {
		t.Fatalf("empty input: got %v, want nil", got)
	}
}

func TestWithSystemModifierTagSyncSuspendedEmptySliceReturnsError(t *testing.T) {
	db, _, _ := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	err := db.WithSystemModifierTagSyncSuspended(ctx, []string{}, func(ctx context.Context, tx pgx.Tx) error {
		t.Fatal("fn must not be called when activityIDs is empty")
		return nil
	})
	if err == nil {
		t.Fatal("expected error for empty activityIDs, got nil")
	}
}

func TestWithSystemModifierTagSyncSuspendedNilSliceReturnsError(t *testing.T) {
	db, _, _ := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	err := db.WithSystemModifierTagSyncSuspended(ctx, nil, func(ctx context.Context, tx pgx.Tx) error {
		t.Fatal("fn must not be called when activityIDs is nil")
		return nil
	})
	if err == nil {
		t.Fatal("expected error for nil activityIDs, got nil")
	}
}

// TestWithSystemModifierTagSyncSuspendedRollsBackOnPanic pins the recover()
// branch in the helper's defer (added in Phase A, commit 7c05f4264). When fn
// panics after performing an UPDATE, the helper must:
//   - roll the transaction back (modifier flag stays at its pre-fn value)
//   - leave no resync side effect (tags array stays empty, junction count 0)
//   - re-panic so the caller sees the original failure.
//
// If the recover() branch is removed, the connection leaks an open tx and the
// rollback assertions fail.
func TestWithSystemModifierTagSyncSuspendedRollsBackOnPanic(t *testing.T) {
	db, userID, ids := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	// Look up the Indoor system tag for the count assertion.
	var indoorTagID string
	if err := db.Pool.QueryRow(ctx, `
		SELECT id FROM tags WHERE user_id = $1 AND name = 'Indoor' AND is_system = true
	`, userID).Scan(&indoorTagID); err != nil {
		t.Fatalf("lookup Indoor tag: %v", err)
	}

	target := ids[0]
	panicMarker := errors.New("synthetic panic from fn")

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = db.WithSystemModifierTagSyncSuspended(ctx, []string{target}, func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE activities SET is_indoor = true WHERE id = $1`, target); err != nil {
				t.Fatalf("update inside fn: %v", err)
			}
			panic(panicMarker)
		})
	}()

	if recovered == nil {
		t.Fatal("helper did not propagate panic from fn")
	}
	if recErr, ok := recovered.(error); !ok || !errors.Is(recErr, panicMarker) {
		t.Fatalf("recovered = %v, want %v", recovered, panicMarker)
	}

	// is_indoor must still be false: the UPDATE was inside the helper tx and
	// the recover() branch must have rolled it back.
	var isIndoor bool
	if err := db.Pool.QueryRow(ctx, `SELECT is_indoor FROM activities WHERE id = $1`, target).Scan(&isIndoor); err != nil {
		t.Fatalf("query is_indoor after panic: %v", err)
	}
	if isIndoor {
		t.Fatalf("activity %s is_indoor = true after panic, want false (helper rollback failed)", target)
	}

	// tags array must still be empty: resync ran inside the helper tx, so any
	// resync-driven tag insert is also gone after rollback. (Belt-and-braces:
	// the helper bails before resync when fn panics, but this assertion still
	// catches a regression that bypasses the rollback.)
	var tags []string
	if err := db.Pool.QueryRow(ctx, `SELECT COALESCE(tags, ARRAY[]::text[]) FROM activities WHERE id = $1`, target).Scan(&tags); err != nil {
		t.Fatalf("query tags after panic: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("activity %s tags = %v after panic, want empty", target, tags)
	}

	// Indoor system tag activity_count must still be 0 — no junction inserted.
	var count int
	if err := db.Pool.QueryRow(ctx, `SELECT activity_count FROM tags WHERE id = $1`, indoorTagID).Scan(&count); err != nil {
		t.Fatalf("query Indoor activity_count: %v", err)
	}
	if count != 0 {
		t.Fatalf("Indoor activity_count = %d after panic, want 0 (resync side effect leaked past rollback)", count)
	}

	// Sanity: the pool is still usable — a panic that left an open tx on the
	// connection would corrupt the pool state for follow-up queries. The two
	// COALESCE queries above already exercised the pool, but issue an explicit
	// no-op to make the contract obvious in the test body.
	if _, err := db.Pool.Exec(ctx, `SELECT 1`); err != nil {
		t.Fatalf("pool unusable after panic: %v", err)
	}
}

// TestLinkActivityTagTxRejectsUnflaggedTx verifies that LinkActivityTagTx returns
// an error when called on a transaction that does not have the
// skadia.skip_system_modifier_tag_sync flag set. A future caller that bypasses
// WithSystemModifierTagSyncSuspended must fail loudly rather than silently
// producing inconsistent denormalised state.
func TestLinkActivityTagTxRejectsUnflaggedTx(t *testing.T) {
	db, userID, ids := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	// Look up the Indoor system tag ID.
	var indoorTagID string
	if err := db.Pool.QueryRow(ctx, `
		SELECT id FROM tags WHERE user_id = $1 AND name = 'Indoor' AND is_system = true
	`, userID).Scan(&indoorTagID); err != nil {
		t.Fatalf("lookup Indoor tag: %v", err)
	}

	// Open a plain transaction without setting the skip flag.
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = db.LinkActivityTagTx(ctx, tx, ids[0], indoorTagID, "test")
	if err == nil {
		t.Fatal("LinkActivityTagTx returned nil on unflagged tx; expected error")
	}
	if !strings.Contains(err.Error(), "skip_system_modifier_tag_sync") {
		t.Errorf("error %q does not mention the skip flag", err.Error())
	}
}

// TestUpdateEnrichmentFieldsTxRejectsUnflaggedTx mirrors
// TestLinkActivityTagTxRejectsUnflaggedTx for the UpdateEnrichmentFieldsTx method.
func TestUpdateEnrichmentFieldsTxRejectsUnflaggedTx(t *testing.T) {
	db, _, ids := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = db.UpdateEnrichmentFieldsTx(ctx, tx, ids[0], map[string]any{"is_indoor": true})
	if err == nil {
		t.Fatal("UpdateEnrichmentFieldsTx returned nil on unflagged tx; expected error")
	}
	if !strings.Contains(err.Error(), "skip_system_modifier_tag_sync") {
		t.Errorf("error %q does not mention the skip flag", err.Error())
	}
}

// TestLinkActivityTagTxAcceptsFlaggedTx verifies that LinkActivityTagTx succeeds
// when the skip flag is set, confirming the assertion does not fire false positives.
func TestLinkActivityTagTxAcceptsFlaggedTx(t *testing.T) {
	db, userID, ids := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	var indoorTagID string
	if err := db.Pool.QueryRow(ctx, `
		SELECT id FROM tags WHERE user_id = $1 AND name = 'Indoor' AND is_system = true
	`, userID).Scan(&indoorTagID); err != nil {
		t.Fatalf("lookup Indoor tag: %v", err)
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Set the flag manually — simulating what WithSystemModifierTagSyncSuspended does.
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); err != nil {
		t.Fatalf("set skip flag: %v", err)
	}

	if err := db.LinkActivityTagTx(ctx, tx, ids[0], indoorTagID, "test"); err != nil {
		t.Fatalf("LinkActivityTagTx on flagged tx returned error: %v", err)
	}
}

