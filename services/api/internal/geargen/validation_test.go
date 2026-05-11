package geargen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/geargen"
)

// repoRoot returns the absolute path to the repository root relative to the
// test binary's source location.
func repoRoot(t *testing.T) string {
	t.Helper()
	// Walk up from the package directory to find the repo root (contains go.mod).
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// go.mod is in services/api; repo root is two levels up.
			return filepath.Join(dir, "..", "..")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

func modifiersPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "config", "sport-taxonomy", "modifiers.yaml")
}

func taxonomyDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "config", "sport-taxonomy")
}

// validSchemaForValidation returns a minimal Schema that passes all cross-ref checks.
func validSchemaForValidation() *geargen.Schema {
	return &geargen.Schema{
		AssignmentRoles: []geargen.AssignmentRole{
			{Key: "primary", LabelDE: "Hauptausrüstung"},
		},
		OptionSets: []geargen.OptionSet{},
		Attributes: []geargen.Attribute{
			{Key: "bike.frame_size_cm", LabelDE: "Rahmengröße", ValueType: "number"},
		},
		GearTypes: []geargen.GearType{
			{
				Key:                      "bike",
				LabelDE:                  "Fahrrad",
				Scope:                    "bike",
				DefaultAssignmentRoleKey: "primary",
				AttributeKeys:            []string{"bike.frame_size_cm"},
			},
		},
	}
}

func TestValidatePassesForValidSchema(t *testing.T) {
	s := validSchemaForValidation()
	if err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t)); err != nil {
		t.Fatalf("Validate() unexpected error = %v", err)
	}
}

func TestValidateChecksTaxonomyPredicates(t *testing.T) {
	s := validSchemaForValidation()
	s.GearTypes[0].ApplicableSports = []geargen.Predicate{
		{SportType: "ufo_riding"},
	}
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for unknown sport_type predicate")
	}
	if !strings.Contains(err.Error(), "ufo_riding") {
		t.Errorf("error should mention unknown sport type, got: %v", err)
	}
}

func TestValidateRejectsUnknownSubType(t *testing.T) {
	s := validSchemaForValidation()
	sub := "not_a_real_subtype"
	s.GearTypes[0].ApplicableSports = []geargen.Predicate{
		{SportType: "cycling", SubType: sub},
	}
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for unknown sub_type predicate")
	}
	if !strings.Contains(err.Error(), "not_a_real_subtype") {
		t.Errorf("error should mention unknown sub_type, got: %v", err)
	}
}

func TestValidateRejectsUnknownParentKey(t *testing.T) {
	s := validSchemaForValidation()
	s.GearTypes = append(s.GearTypes, geargen.GearType{
		Key:                      "road_bike",
		LabelDE:                  "Rennrad",
		Scope:                    "bike",
		DefaultAssignmentRoleKey: "primary",
		ParentKey:                "no_such_gear_type",
	})
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for unknown parent_key")
	}
	if !strings.Contains(err.Error(), "no_such_gear_type") {
		t.Errorf("error should mention unknown parent_key, got: %v", err)
	}
}

func TestValidateRejectsUnknownAttributeKey(t *testing.T) {
	s := validSchemaForValidation()
	s.GearTypes[0].AttributeKeys = []string{"does.not.exist"}
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for unknown attribute_key")
	}
	if !strings.Contains(err.Error(), "does.not.exist") {
		t.Errorf("error should mention unknown attribute key, got: %v", err)
	}
}

func TestValidateRejectsUnknownOptionSetKey(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key:          "bike.material",
		LabelDE:      "Material",
		ValueType:    "enum",
		OptionSetKey: "no.such.option.set",
	})
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for unknown option_set_key")
	}
	if !strings.Contains(err.Error(), "no.such.option.set") {
		t.Errorf("error should mention unknown option_set_key, got: %v", err)
	}
}

func TestValidateRejectsUnknownDefaultAssignmentRoleKey(t *testing.T) {
	s := validSchemaForValidation()
	s.GearTypes[0].DefaultAssignmentRoleKey = "no_such_role"
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for unknown default_assignment_role_key")
	}
	if !strings.Contains(err.Error(), "no_such_role") {
		t.Errorf("error should mention unknown role key, got: %v", err)
	}
}

func TestValidateRejectsUnknownModifierKey(t *testing.T) {
	s := validSchemaForValidation()
	s.GearTypes[0].ModifierKeys = []string{"no_such_modifier"}
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for unknown modifier_key")
	}
	msg := err.Error()
	if !strings.Contains(msg, "no_such_modifier") {
		t.Errorf("error should mention unknown modifier key, got: %v", err)
	}
	// Hint should reference generate-gear-schema.sh or modifiers.yaml
	if !strings.Contains(msg, "generate-gear-schema.sh") && !strings.Contains(msg, "modifiers.yaml") {
		t.Errorf("error should contain fix hint, got: %v", err)
	}
}

func TestValidateRejectsSlotWithUnknownComponentType(t *testing.T) {
	s := validSchemaForValidation()
	s.GearTypes[0].Slots = []geargen.GearTypeSlot{
		{Key: "frame", LabelDE: "Rahmen", ComponentType: "no.such.component"},
	}
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for slot with unknown component_type")
	}
	if !strings.Contains(err.Error(), "no.such.component") {
		t.Errorf("error should mention unknown component_type, got: %v", err)
	}
}

func TestValidateAcceptsSlotWithKnownComponentType(t *testing.T) {
	s := validSchemaForValidation()
	s.ComponentTypes = []geargen.ComponentType{
		{Key: "bike.frame", LabelDE: "Rahmen"},
	}
	s.GearTypes[0].Slots = []geargen.GearTypeSlot{
		{Key: "frame", LabelDE: "Rahmen", ComponentType: "bike.frame"},
	}
	if err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t)); err != nil {
		t.Fatalf("Validate() unexpected error for slot with known component_type: %v", err)
	}
}

func TestValidateRejectsComponentTypeWithUnknownAttributeKey(t *testing.T) {
	s := validSchemaForValidation()
	s.ComponentTypes = []geargen.ComponentType{
		{Key: "bike.frame", LabelDE: "Rahmen", AttributeKeys: []string{"no.such.attr"}},
	}
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for component_type with unknown attribute_key")
	}
	if !strings.Contains(err.Error(), "no.such.attr") {
		t.Errorf("error should mention unknown attribute_key, got: %v", err)
	}
}

func TestValidateRejectsComponentTypeWithUnknownSubtypeKey(t *testing.T) {
	s := validSchemaForValidation()
	s.ComponentTypes = []geargen.ComponentType{
		{Key: "bike.shock", LabelDE: "Dämpfer", SubtypeKeys: []string{"no.such.subtype"}},
	}
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for component_type with unknown subtype_key")
	}
	if !strings.Contains(err.Error(), "no.such.subtype") {
		t.Errorf("error should mention unknown subtype_key, got: %v", err)
	}
}

func TestValidateAcceptsComponentTypeWithKnownSubtypeKey(t *testing.T) {
	s := validSchemaForValidation()
	s.ComponentTypes = []geargen.ComponentType{
		{Key: "bike.shock", LabelDE: "Dämpfer", SubtypeKeys: []string{"bike.shock.air"}},
		{Key: "bike.shock.air", LabelDE: "Luftfederdämpfer"},
	}
	if err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t)); err != nil {
		t.Fatalf("Validate() unexpected error for component_type with known subtype_key: %v", err)
	}
}

func TestValidateAcceptsKnownModifierKey(t *testing.T) {
	s := validSchemaForValidation()
	s.GearTypes[0].ModifierKeys = []string{"is_electric"}
	if err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t)); err != nil {
		t.Fatalf("Validate() unexpected error for known modifier key: %v", err)
	}
}

// --- unit_groups registry --------------------------------------------------

// TestValidateRejectsUnknownUnitGroup verifies that an attribute referencing
// a unit_group that does not exist in the registry is flagged.
func TestValidateRejectsUnknownUnitGroup(t *testing.T) {
	s := validSchemaForValidation()
	s.UnitGroups = []geargen.UnitGroup{
		{Key: "length", LabelDE: "Länge", AllowedUnits: []string{"mm"}},
	}
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key:          "bike.fork.travel",
		LabelDE:      "Federweg",
		ValueType:    "unit_number",
		UnitGroup:    "no_such_group",
		AllowedUnits: []string{"mm"},
	})
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.fork.travel")
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for unknown unit_group reference")
	}
	if !strings.Contains(err.Error(), "no_such_group") {
		t.Errorf("error should mention unknown unit_group, got: %v", err)
	}
}

// TestValidateRejectsAllowedUnitOutsideUnitGroup verifies that an attribute's
// allowed_units must be a subset of the registry entry's allowed_units.
func TestValidateRejectsAllowedUnitOutsideUnitGroup(t *testing.T) {
	s := validSchemaForValidation()
	s.UnitGroups = []geargen.UnitGroup{
		{Key: "length", LabelDE: "Länge", AllowedUnits: []string{"mm", "in"}},
	}
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key:          "bike.fork.travel",
		LabelDE:      "Federweg",
		ValueType:    "unit_number",
		UnitGroup:    "length",
		AllowedUnits: []string{"mm", "lightyears"},
	})
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.fork.travel")
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for allowed_unit outside unit_group registry")
	}
	if !strings.Contains(err.Error(), "lightyears") {
		t.Errorf("error should mention out-of-group unit, got: %v", err)
	}
}

// TestValidateAcceptsConsistentUnitGroupReferences verifies that valid
// unit_group references with subset allowed_units pass validation.
func TestValidateAcceptsConsistentUnitGroupReferences(t *testing.T) {
	s := validSchemaForValidation()
	s.UnitGroups = []geargen.UnitGroup{
		{Key: "length", LabelDE: "Länge", AllowedUnits: []string{"mm", "in"}, CanonicalUnit: "mm"},
	}
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key:          "bike.fork.travel",
		LabelDE:      "Federweg",
		ValueType:    "unit_number",
		UnitGroup:    "length",
		AllowedUnits: []string{"mm"},
	})
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.fork.travel")
	if err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t)); err != nil {
		t.Fatalf("Validate() unexpected error for consistent unit_group refs: %v", err)
	}
}

// TestValidateRejectsSubfieldUnitGroupOutsideRegistry verifies that the
// registry check recurses into subfields.
func TestValidateRejectsSubfieldUnitGroupOutsideRegistry(t *testing.T) {
	s := validSchemaForValidation()
	s.UnitGroups = []geargen.UnitGroup{
		{Key: "length", LabelDE: "Länge", AllowedUnits: []string{"mm"}},
	}
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key:       "bike.fork.federung",
		LabelDE:   "Federung",
		ValueType: "object",
		Subfields: []geargen.Attribute{
			{
				Key:          "druck",
				LabelDE:      "Druck",
				ValueType:    "unit_number",
				UnitGroup:    "no_such_group",
				AllowedUnits: []string{"psi"},
			},
		},
	})
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.fork.federung")
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for subfield unit_group outside registry")
	}
	if !strings.Contains(err.Error(), "no_such_group") {
		t.Errorf("error should mention unknown subfield unit_group, got: %v", err)
	}
}

// --- show_when reference validation ----------------------------------------

// TestValidateRejectsShowWhenUnknownTopLevelField verifies that a top-level
// attribute whose show_when.field references a non-existent sibling key is
// flagged.
func TestValidateRejectsShowWhenUnknownTopLevelField(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key:       "bike.fork.luftfeder",
		LabelDE:   "Luftfeder",
		ValueType: "object",
		ShowWhen:  &geargen.ShowWhenSpec{Field: "no_such_sibling", Value: "luft"},
	})
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.fork.luftfeder")
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for show_when.field with no sibling")
	}
	if !strings.Contains(err.Error(), "no_such_sibling") {
		t.Errorf("error should mention unknown show_when.field, got: %v", err)
	}
}

// TestValidateAcceptsShowWhenWithKnownSibling verifies that show_when.field
// referencing a real top-level sibling passes.
func TestValidateAcceptsShowWhenWithKnownSibling(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes,
		geargen.Attribute{
			Key:       "bike.fork.federung_typ",
			LabelDE:   "Federungstyp",
			ValueType: "string",
		},
		geargen.Attribute{
			Key:       "bike.fork.luftfeder",
			LabelDE:   "Luftfeder",
			ValueType: "object",
			ShowWhen:  &geargen.ShowWhenSpec{Field: "bike.fork.federung_typ", Value: "luft"},
		},
	)
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys,
		"bike.fork.federung_typ", "bike.fork.luftfeder")
	if err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t)); err != nil {
		t.Fatalf("Validate() unexpected error for valid show_when sibling ref: %v", err)
	}
}

// TestValidateRejectsShowWhenSubfieldUnknownSibling verifies that a subfield's
// show_when.field must reference a sibling subfield within the same parent.
func TestValidateRejectsShowWhenSubfieldUnknownSibling(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key:       "bike.fork.federung",
		LabelDE:   "Federung",
		ValueType: "object",
		Subfields: []geargen.Attribute{
			{
				Key:       "druck",
				LabelDE:   "Druck",
				ValueType: "number",
				ShowWhen:  &geargen.ShowWhenSpec{Field: "no_such_sibling", Value: "x"},
			},
		},
	})
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.fork.federung")
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for subfield show_when with no sibling")
	}
	if !strings.Contains(err.Error(), "no_such_sibling") {
		t.Errorf("error should mention unknown show_when.field, got: %v", err)
	}
}

// TestValidateAcceptsShowWhenSubfieldKnownSibling verifies that subfield-to-
// subfield references inside the same parent pass.
func TestValidateAcceptsShowWhenSubfieldKnownSibling(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key:       "bike.fork.federung",
		LabelDE:   "Federung",
		ValueType: "object",
		Subfields: []geargen.Attribute{
			{Key: "typ", LabelDE: "Typ", ValueType: "string"},
			{
				Key:       "druck",
				LabelDE:   "Druck",
				ValueType: "number",
				ShowWhen:  &geargen.ShowWhenSpec{Field: "typ", Value: "luft"},
			},
		},
	})
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.fork.federung")
	if err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t)); err != nil {
		t.Fatalf("Validate() unexpected error for subfield show_when: %v", err)
	}
}

// --- visible_when / applies_to_gear_when ref namespace validation ---------

// TestValidateRejectsVisibleWhenUnknownBareRef ensures a bare ref that
// names no attribute in the owning gear_type's scope is rejected.
func TestValidateRejectsVisibleWhenUnknownBareRef(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key:       "bike.gated",
		LabelDE:   "Gated",
		ValueType: "string",
		VisibleWhen: []geargen.Condition{
			{Ref: "bike.no_such_sibling", Op: "is_set"},
		},
	})
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.gated")
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for visible_when bare ref with no sibling")
	}
	if !strings.Contains(err.Error(), "no_such_sibling") {
		t.Errorf("error should mention unknown bare ref, got: %v", err)
	}
}

// TestValidateRejectsAppliesToGearWhenUnknownGearRef ensures a gear.<attr>
// ref pointing at an attribute not defined on any gear_type that mounts
// the component is rejected.
func TestValidateRejectsAppliesToGearWhenUnknownGearRef(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key:       "bike.shock_length",
		LabelDE:   "Dämpfer-Länge",
		ValueType: "number",
		AppliesToGearWhen: []geargen.Condition{
			{Ref: "gear.bike.no_such_design", Op: "eq", Value: "full"},
		},
	})
	s.ComponentTypes = []geargen.ComponentType{
		{Key: "bike.shock", LabelDE: "Dämpfer", AttributeKeys: []string{"bike.shock_length"}},
	}
	s.GearTypes[0].Slots = []geargen.GearTypeSlot{
		{Key: "shock", LabelDE: "Dämpfer", ComponentType: "bike.shock"},
	}
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for applies_to_gear_when with unknown gear.<attr>")
	}
	if !strings.Contains(err.Error(), "no_such_design") {
		t.Errorf("error should mention unknown gear attribute, got: %v", err)
	}
}

// TestValidateAcceptsAppliesToGearWhenViaGearTypeParent confirms that a
// gear.<attr> ref resolves through the gear_type parent_key chain — a
// child gear_type inherits the parent's attribute_keys for ref purposes.
func TestValidateAcceptsAppliesToGearWhenViaGearTypeParent(t *testing.T) {
	s := validSchemaForValidation()
	// Parent gear_type carries the attribute; child mounts the component.
	s.Attributes = append(s.Attributes,
		geargen.Attribute{Key: "bike.frame_design", LabelDE: "Rahmen-Design", ValueType: "string"},
		geargen.Attribute{
			Key:       "bike.shock_length",
			LabelDE:   "Dämpfer-Länge",
			ValueType: "number",
			AppliesToGearWhen: []geargen.Condition{
				{Ref: "gear.bike.frame_design", Op: "eq", Value: "full_suspension"},
			},
		},
	)
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.frame_design")
	s.GearTypes = append(s.GearTypes, geargen.GearType{
		Key:                      "bike.mtb",
		LabelDE:                  "MTB",
		Scope:                    "bike",
		ParentKey:                "bike",
		DefaultAssignmentRoleKey: "primary",
		Slots: []geargen.GearTypeSlot{
			{Key: "shock", LabelDE: "Dämpfer", ComponentType: "bike.shock"},
		},
	})
	s.ComponentTypes = []geargen.ComponentType{
		{Key: "bike.shock", LabelDE: "Dämpfer", AttributeKeys: []string{"bike.shock_length"}},
	}
	if err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t)); err != nil {
		t.Fatalf("Validate() unexpected error for gear.<attr> via gear_type parent: %v", err)
	}
}

// TestValidateAcceptsComponentSubtypeMountedViaParent confirms that a
// component_type reachable only through ComponentType.ParentKey (e.g.
// bike.air_shock → bike.shock) inherits the gear_types its parent is
// slotted in.
func TestValidateAcceptsComponentSubtypeMountedViaParent(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes,
		geargen.Attribute{Key: "bike.frame_design", LabelDE: "Rahmen-Design", ValueType: "string"},
		geargen.Attribute{
			Key:       "bike.shock_length",
			LabelDE:   "Dämpfer-Länge",
			ValueType: "number",
			AppliesToGearWhen: []geargen.Condition{
				{Ref: "gear.bike.frame_design", Op: "eq", Value: "full_suspension"},
			},
		},
	)
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.frame_design")
	s.GearTypes[0].Slots = []geargen.GearTypeSlot{
		{Key: "shock", LabelDE: "Dämpfer", ComponentType: "bike.shock"},
	}
	s.ComponentTypes = []geargen.ComponentType{
		{Key: "bike.shock", LabelDE: "Dämpfer"},
		{Key: "bike.air_shock", LabelDE: "Luftdämpfer", ParentKey: "bike.shock",
			AttributeKeys: []string{"bike.shock_length"}},
	}
	if err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t)); err != nil {
		t.Fatalf("Validate() unexpected error for component_type via parent_key: %v", err)
	}
}

// TestValidateRejectsVisibleWhenUnknownComponentRef ensures component.<attr>
// refs naming an attribute not defined on any reachable component_type
// are rejected.
func TestValidateRejectsVisibleWhenUnknownComponentRef(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key:       "bike.frame_length",
		LabelDE:   "Rahmenlänge",
		ValueType: "number",
		VisibleWhen: []geargen.Condition{
			{Ref: "component.no_such_component_attr", Op: "is_set"},
		},
	})
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.frame_length")
	s.GearTypes[0].Slots = []geargen.GearTypeSlot{
		{Key: "frame", LabelDE: "Rahmen", ComponentType: "bike.frame"},
	}
	s.ComponentTypes = []geargen.ComponentType{
		{Key: "bike.frame", LabelDE: "Rahmen", AttributeKeys: []string{}},
	}
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for visible_when with unknown component.<attr>")
	}
	if !strings.Contains(err.Error(), "no_such_component_attr") {
		t.Errorf("error should mention unknown component attribute, got: %v", err)
	}
}

// --- kombi_group cardinality ----------------------------------------------

// TestValidateRejectsKombiGroupWithOneMember verifies that an attribute
// tagged with kombi_group but lacking a partner in the same gear_type
// scope is rejected.
func TestValidateRejectsKombiGroupWithOneMember(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key: "bike.solo_kombi", LabelDE: "Solo", ValueType: "number",
		KombiGroup: "shock_dimensions",
	})
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.solo_kombi")
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for orphan kombi_group")
	}
	if !strings.Contains(err.Error(), "shock_dimensions") {
		t.Errorf("error should mention the orphan kombi_group, got: %v", err)
	}
}

// TestValidateRejectsKombiGroupWithThreeMembers verifies that a kombi
// group with three or more members in the same scope is rejected.
func TestValidateRejectsKombiGroupWithThreeMembers(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes,
		geargen.Attribute{Key: "bike.k_a", LabelDE: "A", ValueType: "number", KombiGroup: "trio"},
		geargen.Attribute{Key: "bike.k_b", LabelDE: "B", ValueType: "number", KombiGroup: "trio"},
		geargen.Attribute{Key: "bike.k_c", LabelDE: "C", ValueType: "number", KombiGroup: "trio"},
	)
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.k_a", "bike.k_b", "bike.k_c")
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for kombi_group with 3 members")
	}
	if !strings.Contains(err.Error(), "trio") {
		t.Errorf("error should mention the over-full kombi_group, got: %v", err)
	}
}

// TestValidateAcceptsKombiGroupWithTwoMembers verifies that a properly
// sized kombi pair passes.
func TestValidateAcceptsKombiGroupWithTwoMembers(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes,
		geargen.Attribute{Key: "bike.k_a", LabelDE: "A", ValueType: "number", KombiGroup: "pair"},
		geargen.Attribute{Key: "bike.k_b", LabelDE: "B", ValueType: "number", KombiGroup: "pair"},
	)
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys, "bike.k_a", "bike.k_b")
	if err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t)); err != nil {
		t.Fatalf("Validate() unexpected error for valid kombi pair: %v", err)
	}
}

// TestValidateRejectsKombiGroupOnComponentTypeWithOneMember verifies the
// cardinality rule also fires on component_type scopes.
func TestValidateRejectsKombiGroupOnComponentTypeWithOneMember(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes, geargen.Attribute{
		Key: "bike.solo_on_component", LabelDE: "Solo", ValueType: "number",
		KombiGroup: "solo_group",
	})
	s.ComponentTypes = []geargen.ComponentType{
		{Key: "bike.frame", LabelDE: "Rahmen", AttributeKeys: []string{"bike.solo_on_component"}},
	}
	s.GearTypes[0].Slots = []geargen.GearTypeSlot{
		{Key: "frame", LabelDE: "Rahmen", ComponentType: "bike.frame"},
	}
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for orphan kombi_group on component_type")
	}
	if !strings.Contains(err.Error(), "solo_group") {
		t.Errorf("error should mention the orphan kombi_group, got: %v", err)
	}
}

// TestValidateRejectsCrossScopeShowWhen verifies that a subfield cannot
// reference a top-level attribute via show_when (scope is sibling-only).
// This protects the convention that runtime validation uses sibling-scope
// values for active-state resolution.
func TestValidateRejectsCrossScopeShowWhen(t *testing.T) {
	s := validSchemaForValidation()
	s.Attributes = append(s.Attributes,
		geargen.Attribute{Key: "top_level_typ", LabelDE: "Typ", ValueType: "string"},
		geargen.Attribute{
			Key:       "bike.fork.federung",
			LabelDE:   "Federung",
			ValueType: "object",
			Subfields: []geargen.Attribute{
				{
					Key:       "druck",
					LabelDE:   "Druck",
					ValueType: "number",
					// References a top-level attribute, not a subfield sibling.
					ShowWhen: &geargen.ShowWhenSpec{Field: "top_level_typ", Value: "luft"},
				},
			},
		},
	)
	s.GearTypes[0].AttributeKeys = append(s.GearTypes[0].AttributeKeys,
		"top_level_typ", "bike.fork.federung")
	err := geargen.Validate(s, taxonomyDir(t), modifiersPath(t))
	if err == nil {
		t.Fatal("Validate() returned nil for cross-scope show_when reference")
	}
	if !strings.Contains(err.Error(), "top_level_typ") {
		t.Errorf("error should mention cross-scope show_when.field, got: %v", err)
	}
}

