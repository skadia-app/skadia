package database

// Advisory lock IDs for pg_advisory_xact_lock.
//
// Postgres advisory locks use int64 keys. We allocate each feature a unique
// key so that different features can't accidentally block each other.
//
// Rules:
//   - Only add new constants here — never reuse existing ones
//   - Use a descriptive name starting with 'Lock'
//   - Add a brief comment explaining which feature uses it
const (
	// LockBootstrapAdmin serializes the first-admin creation path in
	// CreateBootstrapAdmin. Prevents races between parallel registration
	// requests when the install has zero users.
	LockBootstrapAdmin int64 = 0x666974706970655f // "skadia_" as int64
)
