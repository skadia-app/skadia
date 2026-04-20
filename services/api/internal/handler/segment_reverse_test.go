package handler

import (
	"hash/fnv"
	"testing"
)

// TestAdvisoryLockKeyForSegmentReverse pins two properties:
//  1. Determinism — same UUID input must produce the same int64 lock-key.
//  2. Namespace isolation — the key must differ from raw FNV(segID), i.e. the
//     "segment.reverse." prefix must actually be folded into the hash so this
//     handler cannot collide with other advisory-lock callers (fusion_merge,
//     shadowfs_settings, commute_rule, activity_edit_ops).
func TestAdvisoryLockKeyForSegmentReverse(t *testing.T) {
	k1 := advisoryLockKeyForSegmentReverse("00000000-0000-0000-0000-000000000001")
	k2 := advisoryLockKeyForSegmentReverse("00000000-0000-0000-0000-000000000001")
	if k1 != k2 {
		t.Fatal("lock-key should be deterministic for same input")
	}
	k3 := advisoryLockKeyForSegmentReverse("00000000-0000-0000-0000-000000000002")
	if k1 == k3 {
		t.Fatal("lock-key should differ across segments")
	}
	// The namespace "segment.reverse." must actually participate in the hash
	// — otherwise a future caller hashing the same UUID raw would collide.
	h := fnv.New64a()
	h.Write([]byte("00000000-0000-0000-0000-000000000001"))
	rawOnly := int64(h.Sum64())
	if rawOnly == k1 {
		t.Fatal("lock-key must be namespaced (prefix folded into hash), not raw FNV(segID)")
	}
}

// --- DB-integration tests (kept as t.Skip until fixture lands) ---
//
// All four tests below are documented Phase-2 test-debt. They genuinely require
// the handler-test fixture (testDB + createTestUser + auth-token helper) which
// does not yet exist in this package. Tracked in
// docs/internal/2026-04-20-segment-editor-overhaul-plan.md Final-Step checklist.

// TestReverseSegmentFlipsGeometryAndEfforts_DB is a full integration test:
//   - creates a segment with 5 forward + 3 reverse efforts,
//   - POST /segments/{id}/reverse,
//   - asserts efforts_updated = 8, is_reverse flipped on all 8 rows,
//   - asserts segment_versions row inserted with ST_Reverse track + rank-recompute ran.
func TestReverseSegmentFlipsGeometryAndEfforts_DB(t *testing.T) {
	t.Skip("Enabled when the handler-test fixture (testDB + createTestUser + authToken helper) lands — Phase-2 test-debt.")
}

func TestReverseSegmentForbiddenForNonOwner_DB(t *testing.T) {
	t.Skip("Enabled when the handler-test fixture (testDB + createTestUser + authToken helper) lands — Phase-2 test-debt.")
}

// Admin calling reverse on a foreign segment MUST succeed (multi-user self-hosted:
// admins fix mis-tagged user data).
func TestReverseSegmentAllowsAdminOnForeignSegment_DB(t *testing.T) {
	t.Skip("Enabled when the handler-test fixture (testDB + createTestUser + authToken helper) lands — Phase-2 test-debt.")
}

// Two concurrent reverse-calls on the same segment must serialize via
// pg_advisory_xact_lock. Both must commit; net effect: segment reversed twice.
func TestReverseSegmentConcurrentCallsSerialize_DB(t *testing.T) {
	t.Skip("Enabled when the handler-test fixture (testDB + createTestUser + authToken helper) lands — Phase-2 test-debt.")
}
