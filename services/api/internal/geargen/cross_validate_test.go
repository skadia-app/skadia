package geargen_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/geargen"
	"github.com/skadia-app/skadia/services/api/internal/sporttaxgen"
)

// gearTypeKeySet builds a map[string]struct{} from a slice of GearType keys.
func gearTypeKeySet(types []geargen.GearType) map[string]struct{} {
	out := make(map[string]struct{}, len(types))
	for _, gt := range types {
		out[gt.Key] = struct{}{}
	}
	return out
}

// refsFromModifiers extracts all ModifierGearTypeReference triples from a
// sporttaxgen ModifierRegistry.
func refsFromModifiers(reg sporttaxgen.ModifierRegistry) []geargen.ModifierGearTypeReference {
	var refs []geargen.ModifierGearTypeReference
	for _, m := range reg.Modifiers {
		for _, key := range m.RecommendedFor {
			refs = append(refs, geargen.ModifierGearTypeReference{
				ModifierKey: m.Key,
				ListName:    "recommended_for",
				GearTypeKey: key,
			})
		}
		for _, key := range m.NotRecommendedFor {
			refs = append(refs, geargen.ModifierGearTypeReference{
				ModifierKey: m.Key,
				ListName:    "not_recommended_for",
				GearTypeKey: key,
			})
		}
	}
	return refs
}

// TestValidateModifierGearTypeReferencesAcceptsValid verifies that a fully-valid
// set of references (all gear_type keys exist) returns nil.
func TestValidateModifierGearTypeReferencesAcceptsValid(t *testing.T) {
	keys := map[string]struct{}{
		"bike.city.citybike": {},
		"bike.road.track":    {},
	}
	refs := []geargen.ModifierGearTypeReference{
		{ModifierKey: "is_electric", ListName: "recommended_for", GearTypeKey: "bike.city.citybike"},
		{ModifierKey: "is_electric", ListName: "not_recommended_for", GearTypeKey: "bike.road.track"},
	}
	if err := geargen.ValidateModifierGearTypeReferences(keys, refs); err != nil {
		t.Fatalf("ValidateModifierGearTypeReferences() unexpected error = %v", err)
	}
}

// TestValidateModifierGearTypeReferencesRejectsDangling verifies that a single
// dangling gear_type reference is reported with modifier key, list name, and
// the bad gear_type key.
func TestValidateModifierGearTypeReferencesRejectsDangling(t *testing.T) {
	keys := map[string]struct{}{
		"bike.city.citybike": {},
	}
	refs := []geargen.ModifierGearTypeReference{
		{ModifierKey: "is_electric", ListName: "recommended_for", GearTypeKey: "bike.city.citybike"},
		{ModifierKey: "is_electric", ListName: "recommended_for", GearTypeKey: "bike.cite"}, // intentional typo
	}
	err := geargen.ValidateModifierGearTypeReferences(keys, refs)
	if err == nil {
		t.Fatal("ValidateModifierGearTypeReferences() returned nil for dangling reference, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "is_electric") {
		t.Errorf("error should mention modifier key %q, got: %v", "is_electric", err)
	}
	if !strings.Contains(msg, "recommended_for") {
		t.Errorf("error should mention list name %q, got: %v", "recommended_for", err)
	}
	if !strings.Contains(msg, "bike.cite") {
		t.Errorf("error should mention dangling gear_type key %q, got: %v", "bike.cite", err)
	}
}

// TestValidateModifierGearTypeReferencesRejectsMultiple verifies that all
// dangling references are reported together, not just the first one.
func TestValidateModifierGearTypeReferencesRejectsMultiple(t *testing.T) {
	keys := map[string]struct{}{
		"bike.mtb.enduro": {},
	}
	refs := []geargen.ModifierGearTypeReference{
		{ModifierKey: "is_electric", ListName: "recommended_for", GearTypeKey: "bike.mtb.enduro"},
		{ModifierKey: "is_electric", ListName: "recommended_for", GearTypeKey: "bike.cite"},        // typo 1
		{ModifierKey: "is_electric", ListName: "not_recommended_for", GearTypeKey: "bike.bmx.rce"}, // typo 2
	}
	err := geargen.ValidateModifierGearTypeReferences(keys, refs)
	if err == nil {
		t.Fatal("ValidateModifierGearTypeReferences() returned nil for multiple dangling references, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bike.cite") {
		t.Errorf("error should mention first dangling key %q, got: %v", "bike.cite", err)
	}
	if !strings.Contains(msg, "bike.bmx.rce") {
		t.Errorf("error should mention second dangling key %q, got: %v", "bike.bmx.rce", err)
	}
}

// TestValidateModifierGearTypeReferencesAcceptsEmptyRefs verifies that an empty
// reference list (no recommended_for / not_recommended_for entries at all)
// is accepted regardless of the gear_type key set.
func TestValidateModifierGearTypeReferencesAcceptsEmptyRefs(t *testing.T) {
	keys := map[string]struct{}{"bike": {}}
	if err := geargen.ValidateModifierGearTypeReferences(keys, nil); err != nil {
		t.Fatalf("ValidateModifierGearTypeReferences() unexpected error on empty refs = %v", err)
	}
}

// TestModifierGearTypeReferencesAreNotDangling is the integration test. It
// loads the actual config/gear-schema.yaml and config/sport-taxonomy/modifiers.yaml
// from the repository and asserts that every gear_type key referenced in any
// modifier's recommended_for / not_recommended_for list exists in the gear
// schema. This is the CI guard that catches typos introduced in either file.
func TestModifierGearTypeReferencesAreNotDangling(t *testing.T) {
	root := repoRoot(t)

	schemaPath := root + "/config/gear-schema.yaml"
	schema, err := geargen.Load(schemaPath)
	if err != nil {
		t.Fatalf("load gear schema from %s: %v", schemaPath, err)
	}

	modifiersFilePath := root + "/config/sport-taxonomy/modifiers.yaml"
	modifiersBytes, err := os.ReadFile(modifiersFilePath)
	if err != nil {
		t.Fatalf("read modifiers from %s: %v", modifiersFilePath, err)
	}
	registry, err := sporttaxgen.LoadModifiers(bytes.NewReader(modifiersBytes))
	if err != nil {
		t.Fatalf("parse modifiers from %s: %v", modifiersFilePath, err)
	}

	gearKeys := gearTypeKeySet(schema.GearTypes)
	refs := refsFromModifiers(registry)

	if err := geargen.ValidateModifierGearTypeReferences(gearKeys, refs); err != nil {
		t.Fatalf("cross-validation failed:\n%v", err)
	}
}
