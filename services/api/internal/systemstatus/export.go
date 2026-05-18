package systemstatus

import (
	"encoding/json"
	"time"
)

type SupportExport struct {
	SchemaVersion  int          `json:"schema_version"`
	GeneratedAt    string       `json:"generated_at"`
	RedactionLevel string       `json:"redaction_level"`
	Status         SystemStatus `json:"status"`
}

func BuildSupportExport(status SystemStatus) ([]byte, error) {
	redacted := SanitizeSystemStatusForUI(status)
	return json.MarshalIndent(SupportExport{
		SchemaVersion:  1,
		GeneratedAt:    redacted.GeneratedAt.Format(time.RFC3339),
		RedactionLevel: "admin_safe",
		Status:         redacted,
	}, "", "  ")
}
