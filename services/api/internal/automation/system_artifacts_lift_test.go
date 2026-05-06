package automation

// Phase 4b.B.1 — verify the Lift entry is present in the central spec list
// and the version was bumped so the startup sweep backfills existing users.

import "testing"

func TestSystemTags_ContainsLift(t *testing.T) {
	for _, t2 := range SystemTags {
		if t2.Name == "Lift" {
			return
		}
	}
	t.Errorf("SystemTags must contain a 'Lift' entry (Phase 4b.B.1)")
}

func TestSystemRules_ContainsLiftDetected(t *testing.T) {
	for _, r := range SystemRules {
		if r.SystemType == "lift_detected" {
			if r.TagName != "Lift" {
				t.Errorf("lift_detected rule must reference TagName='Lift', got %q", r.TagName)
			}
			if r.TriggerEvent != "on_enrichment_complete" {
				t.Errorf("lift_detected rule must trigger 'on_enrichment_complete', got %q", r.TriggerEvent)
			}
			return
		}
	}
	t.Errorf("SystemRules must contain a 'lift_detected' entry (Phase 4b.B.1)")
}

func TestSystemArtifactsVersion_BumpedForLift(t *testing.T) {
	if SystemArtifactsVersion < 2 {
		t.Errorf("SystemArtifactsVersion must be >= 2 after Phase 4b.B (Lift), got %d", SystemArtifactsVersion)
	}
}
