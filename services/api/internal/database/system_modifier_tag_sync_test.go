package database

// TDD coverage for the WithSystemModifierTagSyncSuspended helper. The helper
// is the single sanctioned entry point for batch updates that may legitimately
// suppress per-row trigger work for the registry-backed system modifier tags.
// Every test boots a fresh user via testdb.Open and asserts both the
// transaction lifecycle and the resync side effects.

import (
	"context"
	"errors"
	"fmt"
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

func TestWithSystemModifierTagSyncSuspendedRollsBackOnResyncError(t *testing.T) {
	db, _, ids := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	// Inject a dummy activity ID that does not exist. The resync function
	// is supposed to ignore unknown IDs, but to test the rollback path we
	// instead pass a syntactically valid uuid that the inner fn consumes
	// to raise an exception inside the resync call. Simulating a resync
	// error is best done by overriding the function to RAISE within the
	// helper's transaction scope.

	// Plant a session-local override that makes the resync raise. The
	// helper's transaction will pick it up (the override is created on
	// the same transaction).
	err := db.WithSystemModifierTagSyncSuspended(ctx, ids, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			CREATE OR REPLACE FUNCTION skadia_resync_system_modifier_tags(p_activity_ids UUID[]) RETURNS VOID
			LANGUAGE plpgsql AS $$
			BEGIN RAISE EXCEPTION 'simulated resync failure'; END;
			$$
		`); err != nil {
			t.Fatalf("override resync function: %v", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE activities SET is_indoor = true WHERE id = ANY($1::uuid[])`, ids); err != nil {
			t.Fatalf("update inside fn: %v", err)
		}
		return nil
	})
	if err == nil {
		t.Fatalf("helper returned nil despite resync failure")
	}

	// Rollback: function override is reverted (CREATE OR REPLACE inside a
	// transaction is rolled back too) and modifier flags stay false.
	for _, id := range ids {
		var got bool
		if e := db.Pool.QueryRow(ctx, `SELECT is_indoor FROM activities WHERE id = $1`, id).Scan(&got); e != nil {
			t.Fatalf("query is_indoor: %v", e)
		}
		if got {
			t.Fatalf("activity %s is_indoor = true after rollback, want false", id)
		}
	}
	// Sanity: the resync function is back to its real body.
	var raised bool
	if _, e := db.Pool.Exec(ctx, `SELECT skadia_resync_system_modifier_tags(ARRAY[]::uuid[])`); e != nil {
		raised = true
	}
	if raised {
		t.Fatalf("real resync function got dropped when the helper's tx rolled back")
	}
}

func TestWithSystemModifierTagSyncSuspendedDeduplicatesIDs(t *testing.T) {
	db, _, ids := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	// Build a slice with intentional duplicates and an empty string that
	// must be filtered out before reaching the resync call. The capturing
	// override below re-raises with the array contents so the helper's
	// transaction rolls back, leaving the real resync function intact.
	withDupes := []string{ids[0], ids[1], ids[0], "", ids[1], ids[2], ids[0]}

	err := db.WithSystemModifierTagSyncSuspended(ctx, withDupes, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			CREATE OR REPLACE FUNCTION skadia_resync_system_modifier_tags(p_activity_ids UUID[]) RETURNS VOID
			LANGUAGE plpgsql AS $$
			DECLARE v_id uuid;
			BEGIN
				FOREACH v_id IN ARRAY p_activity_ids LOOP
					RAISE NOTICE 'smts_resync_call: %', v_id::text;
				END LOOP;
				RAISE EXCEPTION 'capture: %', array_to_string(p_activity_ids, ',');
			END;
			$$
		`); err != nil {
			return err
		}
		_, _ = tx.Exec(ctx, `UPDATE activities SET is_indoor = true WHERE id = ANY($1::uuid[])`, ids)
		return nil
	})
	if err == nil {
		t.Fatalf("expected capture exception")
	}
	// Parse the error message; pgx surfaces RAISE EXCEPTION text.
	gotIDs := parseCaptureExceptionUUIDs(t, err)
	if len(gotIDs) != len(ids) {
		t.Fatalf("resync received %d IDs (%v), want %d unique (%v)", len(gotIDs), gotIDs, len(ids), ids)
	}
	seen := map[string]bool{}
	for _, id := range gotIDs {
		if seen[id] {
			t.Fatalf("resync received duplicate id %s in %v", id, gotIDs)
		}
		seen[id] = true
	}
	for _, want := range ids {
		if !seen[want] {
			t.Fatalf("resync did not receive expected id %s; got %v", want, gotIDs)
		}
	}
}

func TestWithSystemModifierTagSyncSuspendedEmptySliceSkipsResync(t *testing.T) {
	db, _, _ := systemModifierTagSyncTestDB(t)
	ctx := context.Background()

	var fnCalled bool
	err := db.WithSystemModifierTagSyncSuspended(ctx, nil, func(ctx context.Context, tx pgx.Tx) error {
		fnCalled = true
		// Verify the skip flag is NOT set under empty input.
		var raw string
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE(current_setting('skadia.skip_system_modifier_tag_sync', true), '')`,
		).Scan(&raw); err != nil {
			return err
		}
		if raw == "true" {
			t.Errorf("empty-slice path set the skip flag, want unset")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("helper returned: %v", err)
	}
	if !fnCalled {
		t.Fatalf("helper did not invoke fn for empty slice")
	}
}

// parseCaptureExceptionUUIDs pulls the comma-separated UUID list out of the
// pgx error text raised by the resync override above.
func parseCaptureExceptionUUIDs(t *testing.T, err error) []string {
	t.Helper()
	msg := err.Error()
	const marker = "capture: "
	idx := -1
	for i := 0; i+len(marker) <= len(msg); i++ {
		if msg[i:i+len(marker)] == marker {
			idx = i + len(marker)
			break
		}
	}
	if idx < 0 {
		t.Fatalf("error %q does not contain capture marker", msg)
	}
	rest := msg[idx:]
	end := len(rest)
	for i, c := range rest {
		if c == '\n' || c == ' ' || c == '\t' {
			end = i
			break
		}
	}
	rest = rest[:end]
	out := []string{}
	cur := ""
	for _, c := range rest {
		if c == ',' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
