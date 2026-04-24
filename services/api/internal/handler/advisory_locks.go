// services/api/internal/handler/advisory_locks.go
package handler

// Advisory-Lock-Keys (int64) used across Skadia. Centralized here as a
// registry to document all existing keys and prevent value collisions.
//
// Convention:
//   - Fixed constants (global serialization) are listed here with their value.
//   - Per-entity keys (derived at runtime from userID, regionID, etc.) are
//     listed as "derived" with their derivation formula — actual const definitions
//     remain in their packages.
//
// When adding a new FIXED key, choose a value from a DIFFERENT numeric range
// than existing entries (e.g. derive from a bit-shift of a hash of the lock
// name, or manually pick a distinct constant).
//
// Advisory-Lock-Key Registry (R2 LOW #10)
//
//	Location                                    | Key / Derivation                           | Purpose
//	────────────────────────────────────────────────────────────────────────────────────────────────────────────
//	handler/advisory_locks.go                   | bulkRenameAdvisoryLockKey (fixed constant) | serializes global admin bulk-rename runs
//	handler/shadowfs_settings.go                | derived: fnv64a(userID+":"+fileType)       | per-user/type rename; session-level on pinned conn
//	handler/activity_edit_ops.go                | derived: int64(fnv32a(activityID))         | per-activity edit serialization
//	handler/commute_rule.go                     | derived: dbutil.AdvisoryLockKey(userID+"_commute_rule") | per-user commute rule update
//	handler/fusion_merge.go (×2)                | derived: dbutil.AdvisoryLockKey("fusion:"+userID)       | per-user fusion merge serialization
//	handler/recording_groups.go                 | derived: dbutil.AdvisoryLockKey("fusion:"+userID)       | per-user recording-group update (same key range as fusion)
//	shadowfs/unified.go                         | derived: advisoryLockKey("shadowfs:"+userID)             | per-user ShadowFS reconciliation
//	database/best_efforts.go                    | derived: dbutil.AdvisoryLockKey("best_effort:"+userID)   | per-user best-effort computation
//	database/gdpr.go                            | derived: hashtextextended('gdpr_purge:'+userID, 0)       | per-user GDPR purge serialization (PostgreSQL builtin hash)
//	osm/pipeline.go                             | derived: int64(fnv32a(regionID))           | per-OSM-region import serialization
//	pmc/service.go                              | derived: pmcLockBase ^ int64(fnv32a(userID)); pmcLockBase = 0x504D4343 | per-user PMC recalculation
//
// Note: dbutil.AdvisoryLockKey uses FNV-64a. shadowfs.advisoryLockKey also uses FNV-64a (same algorithm).
// The PMC key uses FNV-32a XOR'd with a base constant to place it in a distinct numeric range.
const (
	bulkRenameAdvisoryLockKey int64 = 0x1E_BB6A_CA0FE_BA5E1
)
