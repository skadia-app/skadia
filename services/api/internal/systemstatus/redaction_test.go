package systemstatus

import (
	"strings"
	"testing"
	"time"
)

func TestRedactDiagnosticStringRemovesSecretsAndHostDetails(t *testing.T) {
	input := "database_dsn=postgres://skadia:secret@db:5432/skadia?sslmode=disable host_path=/srv/skadia/data runtime_path=/data/library token=abc123 url=https://storage.example.invalid/export-object?X-Amz-Signature=deadbeef user_id=11111111-1111-1111-1111-111111111111 lat=" + "50" + ".123456 lon=" + "8" + ".123456 bucket=private-bucket"
	got := RedactDiagnosticString(input)
	for _, forbidden := range []string{"postgres://skadia", "abc123", "deadbeef", "11111111-1111-1111-1111-111111111111", "/srv/skadia", "/data/library", "50.123456", "8.123456", "private-bucket"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("redacted output still contains %q: %q", forbidden, got)
		}
	}
	for _, expected := range []string{"[redacted-dsn]", "[redacted-path]", "[redacted-secret]", "[redacted-url]", "[redacted-id]", "[redacted-coordinate]", "[redacted-storage-id]"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("redacted output missing %q: %q", expected, got)
		}
	}
}

func TestSanitizeComponentStatusDropsNonAllowlistedDetails(t *testing.T) {
	input := ComponentStatus{
		Key:        "storage",
		Label:      "Storage /srv/skadia/data",
		State:      StatusDegraded,
		ReasonCode: "s3_write_delete_probe_failed secret=abc",
		Summary:    "raw path /srv/skadia/data",
		CheckedAt:  time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC),
		Details: map[string]string{
			"backend":           "s3",
			"raw_error":         "S3_SECRET_KEY=abc bucket=private-bucket",
			"extension_postgis": "3.5.0",
			"stuck_jobs_count":  "2",
		},
	}

	got := SanitizeComponentStatus(input)

	if got.Details["backend"] != "s3" || got.Details["extension_postgis"] != "3.5.0" || got.Details["stuck_jobs_count"] != "2" {
		t.Fatalf("safe details not preserved: %+v", got.Details)
	}
	if _, ok := got.Details["raw_error"]; ok {
		t.Fatalf("raw_error detail was not dropped: %+v", got.Details)
	}
	text := got.Key + got.Label + got.ReasonCode + got.Summary + strings.Join(mapValues(got.Details), " ")
	for _, forbidden := range []string{"abc", "private-bucket", "/srv/skadia"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("sanitized component still contains %q: %+v", forbidden, got)
		}
	}
}

func TestStatusStateSetIsStable(t *testing.T) {
	got := []StatusState{
		StatusOK, StatusDegraded, StatusUnavailable, StatusDisabled,
		StatusNotConfigured, StatusMissingArtifacts, StatusBlocked, StatusUnknown,
	}
	want := []string{"ok", "degraded", "unavailable", "disabled", "not_configured", "missing_artifacts", "blocked", "unknown"}
	if len(got) != len(want) {
		t.Fatalf("state count = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if string(got[i]) != want[i] {
			t.Fatalf("state[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSanitizeSystemStatusForUIDropsUnsafeFieldsEverywhere(t *testing.T) {
	status := SystemStatus{
		SchemaVersion: 1,
		GeneratedAt:   time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC),
		Overall:       StatusDegraded,
		Build: map[string]BuildInfo{
			"api": {Service: "api", GitCommit: "abc123", CommitTime: "2026-05-17T09:58:00Z", BuildTime: "2026-05-17T10:00:00Z"},
		},
		Components: map[string]ComponentStatus{
			"storage": {Key: "storage", Label: "path=/srv/skadia/data", State: StatusDegraded, ReasonCode: "bucket=private-bucket"},
		},
		FeatureMatrix: map[string]ComponentStatus{
			"coverage": {Key: "coverage", Label: "Coverage", State: StatusMissingArtifacts, Details: map[string]string{"raw_error": "secret=abc"}},
		},
	}

	got := SanitizeSystemStatusForUI(status)
	raw := got.Components["storage"].Label + got.Components["storage"].ReasonCode + strings.Join(mapValues(got.FeatureMatrix["coverage"].Details), " ")
	for _, forbidden := range []string{"/srv/skadia", "private-bucket", "secret=abc"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("sanitized system status leaked %q: %+v", forbidden, got)
		}
	}
}

func mapValues(in map[string]string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	return out
}
