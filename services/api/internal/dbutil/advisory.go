package dbutil

import "hash/fnv"

// AdvisoryLockKey derives a stable int64 lock key from a string via FNV-64a.
// Use this with pg_advisory_lock / pg_advisory_unlock to avoid importing hash/fnv
// in call sites.
func AdvisoryLockKey(key string) int64 {
	h := fnv.New64a()
	h.Write([]byte(key)) //nolint:errcheck
	return int64(h.Sum64())
}
