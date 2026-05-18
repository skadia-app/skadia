package systemstatus

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
)

func TestCollectorOverallDegradesWhenAnyRequiredComponentDegraded(t *testing.T) {
	c := Collector{
		Now:   fixedNow,
		Build: map[string]BuildInfo{"api": {Service: "api", GitCommit: "abc", CommitTime: "2026-05-17T09:58:00Z", BuildTime: "2026-05-17T10:00:00Z"}},
		ComponentCollectors: []ComponentCollector{
			func(context.Context) ComponentStatus {
				return ComponentStatus{Key: "api", Label: "API", State: StatusOK, CheckedAt: fixedNow()}
			},
			func(context.Context) ComponentStatus {
				return ComponentStatus{Key: "db", Label: "Database", State: StatusDegraded, Required: true, ReasonCode: "migration_dirty", CheckedAt: fixedNow()}
			},
		},
	}

	got := c.Collect(context.Background())
	if got.Overall != StatusDegraded {
		t.Fatalf("overall = %q, want degraded", got.Overall)
	}
}

func TestFeatureMatrixFromCapabilitiesMapsUnavailableFeature(t *testing.T) {
	got := FeatureMatrixFromCapabilities(capabilities.Capabilities{
		Features: map[string]capabilities.FeatureState{
			capabilities.KeyCoverage: {Configured: true, Available: false, Reason: capabilities.ReasonMissingArtifacts},
		},
	})

	row := got[capabilities.KeyCoverage]
	if row.State != StatusMissingArtifacts || row.ReasonCode != capabilities.ReasonMissingArtifacts {
		t.Fatalf("coverage row = %+v", row)
	}
}

func TestFeatureMatrixFromCapabilitiesKeepsNotConfiguredDistinct(t *testing.T) {
	got := FeatureMatrixFromCapabilities(capabilities.Capabilities{
		Features: map[string]capabilities.FeatureState{
			capabilities.KeyDocumentML: {Configured: false, Available: false, Reason: capabilities.ReasonNotConfigured},
		},
	})

	row := got[capabilities.KeyDocumentML]
	if row.State != StatusNotConfigured || row.ReasonCode != capabilities.ReasonNotConfigured {
		t.Fatalf("document_ml row = %+v", row)
	}
}

func TestFoldOverallUsesRequiredComponentSeverity(t *testing.T) {
	cases := []struct {
		name       string
		components []ComponentStatus
		want       StatusState
	}{
		{name: "required missing artifacts degrades", components: []ComponentStatus{{State: StatusMissingArtifacts, Required: true}}, want: StatusDegraded},
		{name: "optional missing artifacts keeps ok", components: []ComponentStatus{{State: StatusMissingArtifacts}}, want: StatusOK},
		{name: "required unknown degrades", components: []ComponentStatus{{State: StatusUnknown, Required: true}}, want: StatusDegraded},
		{name: "required disabled keeps ok", components: []ComponentStatus{{State: StatusDisabled, Required: true}}, want: StatusOK},
		{name: "unavailable degrades", components: []ComponentStatus{{State: StatusUnavailable, Required: true}}, want: StatusDegraded},
		{name: "blocked degrades", components: []ComponentStatus{{State: StatusBlocked, Required: true}}, want: StatusDegraded},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := overallFromComponents(tt.components); got != tt.want {
				t.Fatalf("overall = %q, want %q", got, tt.want)
			}
		})
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
}
