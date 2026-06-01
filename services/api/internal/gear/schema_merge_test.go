package gear_test

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/gear"
)

// baseSchema returns a small but representative schema for merge tests.
func baseSchema() gear.GearSchema {
	return gear.GearSchema{
		AssignmentRoles: []gear.AssignmentRole{
			{Key: "primary", LabelDE: "Hauptausrüstung", UniquePerActivity: true},
		},
		OptionSets: []gear.OptionSet{
			{Key: "helmet.subcategory", Options: []gear.OptionValue{
				{Key: "road_bike", LabelDE: "Rennrad"},
				{Key: "mtb", LabelDE: "MTB"},
			}},
		},
		UnitGroups: []gear.UnitGroup{
			{Key: "distance", LabelDE: "Distanz", AllowedUnits: []string{"m", "km"}, CanonicalUnit: "m"},
		},
		Attributes: []gear.Attribute{
			{Key: "helmet.subcategory", LabelDE: "Helmkategorie", ValueType: "enum", OptionSetKey: "helmet.subcategory"},
		},
		GearTypes: []gear.GearType{
			{Key: "helmet", LabelDE: "Helm", Scope: "other", DefaultAssignmentRoleKey: "worn",
				AttributeKeys: []string{"helmet.subcategory"}},
		},
		ComponentTypes: []gear.ComponentType{
			{Key: "helmet_pad", LabelDE: "Helmpolster", AttributeKeys: []string{"helmet.subcategory"}},
		},
	}
}

// TestUserCanAddOptionToOptionSet verifies that a user can extend an
// existing OptionSet with a new value without removing system options.
func TestUserCanAddOptionToOptionSet(t *testing.T) {
	base := baseSchema()
	custom := []gear.UserSchemaDefinition{
		{
			TypeKey: "helmet",
			ExtraOptionValues: map[string][]gear.OptionValue{
				"helmet.subcategory": {{Key: "kayak", LabelDE: "Kajak"}},
			},
		},
	}

	result, err := gear.MergeSchemaWithUserCustom(base, custom)
	if err != nil {
		t.Fatalf("MergeSchemaWithUserCustom: %v", err)
	}

	os, ok := findOptionSet(result.OptionSets, "helmet.subcategory")
	if !ok {
		t.Fatal("helmet.subcategory OptionSet missing from merged schema")
	}

	if !hasOption(os, "road_bike") {
		t.Error("system option 'road_bike' missing after merge")
	}
	if !hasOption(os, "mtb") {
		t.Error("system option 'mtb' missing after merge")
	}
	if !hasOption(os, "kayak") {
		t.Error("user-added option 'kayak' missing after merge")
	}
}

func TestUserCustomizationPreservesSchemaCatalogs(t *testing.T) {
	base := baseSchema()
	custom := []gear.UserSchemaDefinition{
		{
			TypeKey:       "helmet",
			ExtraAttrKeys: []string{"helmet.extra_note"},
			ExtraAttributes: []gear.Attribute{
				{Key: "helmet.extra_note", LabelDE: "Notiz", ValueType: "text"},
			},
		},
	}

	result, err := gear.MergeSchemaWithUserCustom(base, custom)
	if err != nil {
		t.Fatalf("MergeSchemaWithUserCustom: %v", err)
	}

	if len(result.UnitGroups) != len(base.UnitGroups) {
		t.Fatalf("UnitGroups len = %d, want %d", len(result.UnitGroups), len(base.UnitGroups))
	}
	if len(result.ComponentTypes) != len(base.ComponentTypes) {
		t.Fatalf("ComponentTypes len = %d, want %d", len(result.ComponentTypes), len(base.ComponentTypes))
	}
	if result.UnitGroups[0].Key != "distance" {
		t.Errorf("UnitGroups[0].Key = %q, want distance", result.UnitGroups[0].Key)
	}
	if result.ComponentTypes[0].Key != "helmet_pad" {
		t.Errorf("ComponentTypes[0].Key = %q, want helmet_pad", result.ComponentTypes[0].Key)
	}
}

// TestUserCannotRemoveSystemAttribute verifies that omitting a system
// attribute in the user override has no effect — the attribute survives.
func TestUserCannotRemoveSystemAttribute(t *testing.T) {
	base := baseSchema()
	// User definition for "helmet" with no AttributeKeys — attempts removal by omission.
	custom := []gear.UserSchemaDefinition{
		{
			TypeKey:       "helmet",
			ExtraAttrKeys: []string{}, // explicitly empty — no add, no keep
		},
	}

	result, err := gear.MergeSchemaWithUserCustom(base, custom)
	if err != nil {
		t.Fatalf("MergeSchemaWithUserCustom: %v", err)
	}

	gt, ok := findGearType(result.GearTypes, "helmet")
	if !ok {
		t.Fatal("helmet GearType missing from merged schema")
	}
	if !containsString(gt.AttributeKeys, "helmet.subcategory") {
		t.Errorf("system attribute 'helmet.subcategory' was removed by user override; AttributeKeys = %v", gt.AttributeKeys)
	}
}

// TestUserOverrideOfNonexistentTypeKeyRejected verifies that an override
// targeting an unknown type_key returns an error.
func TestUserOverrideOfNonexistentTypeKeyRejected(t *testing.T) {
	base := baseSchema()
	custom := []gear.UserSchemaDefinition{
		{TypeKey: "ufo"},
	}

	_, err := gear.MergeSchemaWithUserCustom(base, custom)
	if err == nil {
		t.Fatal("expected error for unknown type_key 'ufo', got nil")
	}
}

// TestUserCustomAttributeCannotShadowSystemAttributeName verifies that
// a user-defined attribute with the same key as an existing system
// attribute is rejected.
func TestUserCustomAttributeCannotShadowSystemAttributeName(t *testing.T) {
	base := baseSchema()
	custom := []gear.UserSchemaDefinition{
		{
			TypeKey: "helmet",
			ExtraAttributes: []gear.Attribute{
				// Same key as a system attribute — must be rejected.
				{Key: "helmet.subcategory", LabelDE: "Eigene Helmkategorie", ValueType: "text"},
			},
		},
	}

	_, err := gear.MergeSchemaWithUserCustom(base, custom)
	if err == nil {
		t.Fatal("expected error for shadowing system attribute 'helmet.subcategory', got nil")
	}
}

// --- helpers ---

func findOptionSet(sets []gear.OptionSet, key string) (gear.OptionSet, bool) {
	for _, s := range sets {
		if s.Key == key {
			return s, true
		}
	}
	return gear.OptionSet{}, false
}

func hasOption(os gear.OptionSet, key string) bool {
	for _, v := range os.Options {
		if v.Key == key {
			return true
		}
	}
	return false
}

func findGearType(types []gear.GearType, key string) (gear.GearType, bool) {
	for _, gt := range types {
		if gt.Key == key {
			return gt, true
		}
	}
	return gear.GearType{}, false
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
