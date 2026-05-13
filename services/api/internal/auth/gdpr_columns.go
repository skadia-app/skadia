package auth

// ExportDenyColumns lists security-sensitive columns that must NEVER appear
// in a GDPR export. All other columns are exported automatically via SELECT *.
//
// Usually for tables in Export/External category (GDPRRegistry). System-category
// tables are never exported, but may still be listed here as defense-in-depth
// for especially sensitive columns. The registry controls which tables are
// exported; the deny-list controls which columns within those tables are
// suppressed.
//
// When adding a security-sensitive column to an Export/External table,
// add it here. The gdpr_registry_test.go verifies these columns exist.
//
// NOTE: The old S3/storage columns (s3_key, s3_bucket, s3_key_thumb, etc.)
// were dropped in migration 000236. No deny-list entry needed.
//
// NOTE: TOTP/2FA is not yet implemented. When totp_secret and recovery_codes
// are introduced, add them back here.
var ExportDenyColumns = map[string][]string{
	// system_artifacts_version is system metadata (not PII) added by Phase 4b.A.
	// It tracks the EnsureSystemArtifacts seed version for the user and is
	// internal to the rules-engine pipeline — exposing it in the GDPR export
	// would leak implementation details with no value to the user.
	"users":             {"password_hash", "system_artifacts_version"},
	"agent_audit_log":   {"args", "result_detail"},
	"oauth_connections": {"access_token", "refresh_token"},
}
