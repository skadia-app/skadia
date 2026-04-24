// services/api/internal/handler/advisory_locks.go
package handler

// Advisory-Lock-Keys (int64) used across Skadia handlers. Centralized here
// to prevent key collisions across files and future growth.
//
// When adding a new lock key, choose a value from a DIFFERENT numeric range
// than existing entries (e.g. derive from a bit-shift of a hash of the lock
// name, or manually pick a distinct constant). Document the purpose inline.
//
// Note: per-user/per-type advisory locks in ExecuteFileRename are derived at
// runtime via fnv64a(userID+":"+fileType) and are NOT listed here, since they
// are not fixed constants.
//
// Registry:
//
//	bulkRenameAdvisoryLockKey — serializes global admin bulk-rename runs;
//	                            disjoint from fnv64a-derived per-user keys.
const (
	bulkRenameAdvisoryLockKey int64 = 0x1E_BB6A_CA0FE_BA5E1
)
