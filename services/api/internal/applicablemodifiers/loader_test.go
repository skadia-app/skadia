package applicablemodifiers

import (
	"reflect"
	"testing"
)

func TestLoadForSubsystemFiltersFromModifiersYaml(t *testing.T) {
	keys, err := LoadForSubsystem(
		"../../../../config/sport-taxonomy/modifiers.yaml",
		"heatmap",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"is_electric", "is_commute"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("got %v, want %v", keys, want)
	}
}

// TestLoadForSubsystemRejectsUnknownSubsystemName ensures a typo'd subsystem
// name (or a stale reference to a removed subsystem) returns an error
// instead of silently returning an empty slice. Empty-on-unknown was a
// drift hazard: a caller asking for "heatmaps" (typo) or "gear-defaults"
// (renamed away) would receive [] and behave as if no modifier matched.
func TestLoadForSubsystemRejectsUnknownSubsystemName(t *testing.T) {
	_, err := LoadForSubsystem(
		"../../../../config/sport-taxonomy/modifiers.yaml",
		"heatmaps", // typo of "heatmap"
	)
	if err == nil {
		t.Fatal("LoadForSubsystem with unknown subsystem name returned nil error; " +
			"typo'd or stale subsystem names must fail loudly")
	}
}
