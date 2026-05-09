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
