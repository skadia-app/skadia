// services/api/internal/shadowfs/pii.go
package shadowfs

import "strings"

// piiBlacklistVariables lists template variables that must not appear in
// file path schemas. Rationale: Filesystem backups bypass DB-level GDPR
// deletion, so PII in the path itself is a compliance risk.
var piiBlacklistVariables = []string{
	"notes",
	"description",
	"hr_avg",
	"power_avg",
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
