package automation

// Phase 4b.A.4 — sweep integration tests covering the startup-hook flow.
//
// Three scenarios:
//
//	(a) Sweep on a stale user (system_artifacts_version=0) seeds tags+rules
//	    and bumps the version to current.
//	(b) A second sweep is a no-op for that user (now up-to-date, skipped
//	    by the version-gated query).
//
// The advisory-lock contention scenario is already covered by
// TestEnsureSystemArtifactsForAllUsers_AdvisoryLock in system_artifacts_test.go.

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

func TestSweep_SeedsStaleUserAndBumpsVersion(t *testing.T) {
	pool := systemArtifactsTestPool(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	db := &database.DB{Pool: pool}

	// User starts at version 0 (default).
	var before int
	if err := pool.QueryRow(ctx,
		`SELECT system_artifacts_version FROM users WHERE id = $1`, userID,
	).Scan(&before); err != nil {
		t.Fatalf("read version before: %v", err)
	}
	if before != 0 {
		t.Fatalf("setup precondition: expected version=0 on fresh user, got %d", before)
	}

	if err := EnsureSystemArtifactsForAllUsers(ctx, db); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	var after int
	if err := pool.QueryRow(ctx,
		`SELECT system_artifacts_version FROM users WHERE id = $1`, userID,
	).Scan(&after); err != nil {
		t.Fatalf("read version after: %v", err)
	}
	if after != SystemArtifactsVersion {
		t.Errorf("expected version=%d after sweep, got %d", SystemArtifactsVersion, after)
	}

	// Tags + rules must exist for this user.
	var tagCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM tags WHERE user_id = $1 AND is_system = true AND name = ANY($2)`,
		userID, namesFromTagSpecs(SystemTags),
	).Scan(&tagCount); err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if tagCount != len(SystemTags) {
		t.Errorf("post-sweep tag count = %d, want %d", tagCount, len(SystemTags))
	}
}

func TestSweep_SecondRunIsNoOp(t *testing.T) {
	pool := systemArtifactsTestPool(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	db := &database.DB{Pool: pool}

	// First sweep brings the user to current.
	if err := EnsureSystemArtifactsForAllUsers(ctx, db); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	var v1 int
	if err := pool.QueryRow(ctx,
		`SELECT system_artifacts_version FROM users WHERE id = $1`, userID,
	).Scan(&v1); err != nil {
		t.Fatalf("read v1: %v", err)
	}
	if v1 != SystemArtifactsVersion {
		t.Fatalf("after first sweep version = %d, want %d", v1, SystemArtifactsVersion)
	}

	// Capture rule IDs (UUIDs) before the second sweep — if the second sweep
	// re-INSERTed (ON CONFLICT DO NOTHING masks duplicates but still tries),
	// the rule count is the safer invariant. We also capture id+created_at
	// pairs to detect any spurious deletion/recreation cycle.
	var beforePairs []string
	rows, err := pool.Query(ctx,
		`SELECT id::text || ':' || created_at::text FROM automation_rules
		   WHERE user_id = $1 AND is_system = true ORDER BY created_at, id`,
		userID,
	)
	if err != nil {
		t.Fatalf("snapshot rules before: %v", err)
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			t.Fatalf("scan: %v", err)
		}
		beforePairs = append(beforePairs, p)
	}
	rows.Close()

	// Second sweep — user is at SystemArtifactsVersion, must be skipped by
	// the WHERE system_artifacts_version < $1 predicate.
	if err := EnsureSystemArtifactsForAllUsers(ctx, db); err != nil {
		t.Fatalf("second sweep: %v", err)
	}

	var afterPairs []string
	rows2, err := pool.Query(ctx,
		`SELECT id::text || ':' || created_at::text FROM automation_rules
		   WHERE user_id = $1 AND is_system = true ORDER BY created_at, id`,
		userID,
	)
	if err != nil {
		t.Fatalf("snapshot rules after: %v", err)
	}
	for rows2.Next() {
		var p string
		if err := rows2.Scan(&p); err != nil {
			rows2.Close()
			t.Fatalf("scan: %v", err)
		}
		afterPairs = append(afterPairs, p)
	}
	rows2.Close()

	if len(beforePairs) != len(afterPairs) {
		t.Fatalf("rule count mismatch: before=%d after=%d (sweep re-inserted)",
			len(beforePairs), len(afterPairs))
	}
	for i := range beforePairs {
		if beforePairs[i] != afterPairs[i] {
			t.Errorf("rule[%d] identity changed: %q -> %q (sweep should have skipped this user)",
				i, beforePairs[i], afterPairs[i])
		}
	}
}
