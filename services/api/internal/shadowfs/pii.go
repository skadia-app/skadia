// services/api/internal/shadowfs/pii.go
package shadowfs

import "strings"

// piiBlacklistVariables lists template variables that must not appear in
// file path schemas. Rationale: Filesystem backups bypass DB-level GDPR
// deletion, so PII in the path itself is a compliance risk.
//
// LOW #37: extended with bio, comment, message, tags — all fields that may
// contain free-form user input and are therefore GDPR-sensitive.
// Keep sorted alphabetically for diff readability.
var piiBlacklistVariables = []string{
	"bio",
	"comment",
	"description",
	"hr_avg",
	"message",
	"notes",
	"power_avg",
	"tags",
}

// ContainsPIIVariable returns the first blacklisted variable found in the
// schema template, or "" if clean. Case-sensitive exact match against
// "{key}" — mirrors the Renderer's strings.ReplaceAll semantics in
// schema.go:49.
func ContainsPIIVariable(schema string) string {
	for _, key := range piiBlacklistVariables {
		if strings.Contains(schema, "{"+key+"}") {
			return key
		}
	}
	return ""
}

// PIIBlacklistForTest exposes piiBlacklistVariables for cross-package sync tests.
// Do NOT use in production code. Named *ForTest to make intent explicit.
// See shadowfs/pii_sync_test.go for the CI guard that keeps settings/keys.go
// and shadowfs/pii.go in sync (R2 MED #4).
func PIIBlacklistForTest() []string {
	out := make([]string, len(piiBlacklistVariables))
	copy(out, piiBlacklistVariables)
	return out
}
