package auth

// ExportDenyColumns lists security-sensitive columns that must NEVER appear
// in a GDPR export. All other columns are exported automatically via SELECT *.
//
// ONLY for tables in Export/External category (GDPRRegistry). System-category
// tables are never exported at all — they don't need deny-list entries.
// The registry controls which tables are exported; the deny-list controls
// which columns within those tables are suppressed.
//
// When adding a security-sensitive column to an Export/External table,
// add it here. The gdpr_registry_test.go verifies these columns exist.
//
// NOTE: The old S3/storage columns (s3_key, s3_bucket, s3_key_thumb, etc.)
// were dropped in migration 000236. No deny-list entry needed.
var ExportDenyColumns = map[string][]string{
	"users":             {"password_hash", "totp_secret", "recovery_codes"},
	"oauth_connections": {"access_token", "refresh_token"},
}
