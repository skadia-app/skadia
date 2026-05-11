package gear_test

// Drift guard: verifies that applicableSportPredicatesDefaults (Go literal) is
// in sync with config/sport-taxonomy/projections/gear_defaults.yaml.
//
// How to fix a failure: update BOTH the YAML and the Go literal together, or
// run the codegen pass. See applicable_sport_predicates_defaults.go.

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/gear"
	"github.com/skadia-app/skadia/services/api/internal/sporttaxgen"
)

func TestApplicableSportPredicatesDefaultsMatchYAML(t *testing.T) {
	root := repoRoot(t)
	yamlPath := filepath.Join(root, "config", "sport-taxonomy", "projections", "gear_defaults.yaml")

	f, err := os.Open(yamlPath)
	if err != nil {
		t.Fatalf("open gear_defaults.yaml: %v", err)
	}
	defer f.Close()

	config, err := sporttaxgen.LoadGearDefaultProjectionConfig(f, []sporttaxgen.SourceAliases{{Source: "strava"}})
	if err != nil {
		t.Fatalf("LoadGearDefaultProjectionConfig: %v", err)
	}

	// Build the expected map from YAML entries: key → []gear.GearDefaultPredicate.
	// SportPredicateEntry.SportType → GearDefaultPredicate.SportType
	// SportPredicateEntry.SubType  → GearDefaultPredicate.SubType  (both *string)
	want := make(map[string][]gear.GearDefaultPredicate, len(config.ApplicableSportPredicates))
	for _, entry := range config.ApplicableSportPredicates {
		preds := make([]gear.GearDefaultPredicate, len(entry.Predicates))
		for i, p := range entry.Predicates {
			preds[i] = gear.GearDefaultPredicate{
				SportType: p.SportType,
				SubType:   p.SubType,
				// Modifiers are not stored in the YAML; they are overlaid at
				// call time in ComputeApplicableSportPredicates.
			}
		}
		want[entry.GearTypeKey] = preds
	}

	got := gear.ApplicableSportPredicatesDefaults()

	// Check for keys missing from Go literal.
	var missing []string
	for key := range want {
		if _, ok := got[key]; !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)

	// Check for keys missing from YAML.
	var extra []string
	for key := range got {
		if _, ok := want[key]; !ok {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)

	if len(missing) > 0 || len(extra) > 0 {
		t.Errorf("YAML↔Go literal drift detected:\n  in YAML but not Go: %v\n  in Go but not YAML: %v", missing, extra)
	}

	// Deep-equal predicate slices per key.
	for key, wantPreds := range want {
		gotPreds, ok := got[key]
		if !ok {
			continue // already reported above
		}
		if !reflect.DeepEqual(wantPreds, gotPreds) {
			t.Errorf("key %q predicate mismatch:\n  YAML: %#v\n  Go:   %#v", key, wantPreds, gotPreds)
		}
	}
}
