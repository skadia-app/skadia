package systemstatus

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSupportExportRedactsSensitiveDetails(t *testing.T) {
	status := SystemStatus{
		SchemaVersion: 1,
		GeneratedAt:   time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC),
		Overall:       StatusDegraded,
		Build: map[string]BuildInfo{
			"api": {Service: "api", GitCommit: "c0ffee", CommitTime: "2026-05-17T09:58:00Z", BuildTime: "2026-05-17T10:00:00Z", ImageDigest: "sha256:safe"},
		},
		Components: map[string]ComponentStatus{
			"storage": {
				Key:     "storage",
				Label:   "Storage",
				State:   StatusDegraded,
				Summary: "S3_SECRET_KEY=abc path=/srv/skadia/data",
				Details: map[string]string{
					"raw_error": "S3_SECRET_KEY=abc bucket=private-bucket path=/srv/skadia/data",
					"backend":   "s3",
				},
			},
		},
	}

	b, err := BuildSupportExport(status)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, forbidden := range []string{"abc", "private-bucket", "/srv/skadia"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("export leaked %q: %s", forbidden, text)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("invalid json export: %v", err)
	}
}

func TestSupportExportRedactsComponentIdentityFields(t *testing.T) {
	status := SystemStatus{
		SchemaVersion: 1,
		GeneratedAt:   time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC),
		Overall:       StatusDegraded,
		Components: map[string]ComponentStatus{
			"storage": {
				Key:        "bucket=private-bucket",
				Label:      "path=/srv/skadia/data",
				State:      StatusDegraded,
				ReasonCode: "secret=abc",
			},
		},
	}

	b, err := BuildSupportExport(status)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, forbidden := range []string{"private-bucket", "/srv/skadia", "secret=abc"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("export leaked %q: %s", forbidden, text)
		}
	}
}
