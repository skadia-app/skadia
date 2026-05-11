package geargen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/geargen"
)

const minimalSchemaYAML = `version: 1
assignment_roles:
  - key: primary
    label_de: Hauptausrüstung
    unique_per_activity: true
option_sets:
  - key: bike.frame_material
    options:
      - key: aluminum
        label_de: Aluminium
attributes:
  - key: bike.frame_size_cm
    label_de: Rahmengröße (cm)
    value_type: number
gear_types:
  - key: bike
    label_de: Fahrrad
    scope: bike
    default_assignment_role_key: primary
    attribute_keys: [bike.frame_size_cm]
`

func TestLoadValidGearSchemaFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gear-schema.yaml")
	if err := os.WriteFile(path, []byte(minimalSchemaYAML), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	schema, err := geargen.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(schema.AssignmentRoles) != 1 {
		t.Errorf("AssignmentRoles: got %d, want 1", len(schema.AssignmentRoles))
	}
	if schema.AssignmentRoles[0].Key != "primary" {
		t.Errorf("AssignmentRoles[0].Key = %q, want %q", schema.AssignmentRoles[0].Key, "primary")
	}
	if !schema.AssignmentRoles[0].UniquePerActivity {
		t.Errorf("AssignmentRoles[0].UniquePerActivity = false, want true")
	}

	if len(schema.OptionSets) != 1 {
		t.Errorf("OptionSets: got %d, want 1", len(schema.OptionSets))
	}
	if schema.OptionSets[0].Key != "bike.frame_material" {
		t.Errorf("OptionSets[0].Key = %q, want %q", schema.OptionSets[0].Key, "bike.frame_material")
	}
	if len(schema.OptionSets[0].Options) != 1 {
		t.Errorf("OptionSets[0].Options: got %d, want 1", len(schema.OptionSets[0].Options))
	}

	if len(schema.Attributes) != 1 {
		t.Errorf("Attributes: got %d, want 1", len(schema.Attributes))
	}
	if schema.Attributes[0].Key != "bike.frame_size_cm" {
		t.Errorf("Attributes[0].Key = %q, want %q", schema.Attributes[0].Key, "bike.frame_size_cm")
	}
	if schema.Attributes[0].ValueType != "number" {
		t.Errorf("Attributes[0].ValueType = %q, want %q", schema.Attributes[0].ValueType, "number")
	}

	if len(schema.GearTypes) != 1 {
		t.Errorf("GearTypes: got %d, want 1", len(schema.GearTypes))
	}
	if schema.GearTypes[0].Key != "bike" {
		t.Errorf("GearTypes[0].Key = %q, want %q", schema.GearTypes[0].Key, "bike")
	}
	if schema.GearTypes[0].Scope != "bike" {
		t.Errorf("GearTypes[0].Scope = %q, want %q", schema.GearTypes[0].Scope, "bike")
	}
}

func TestLoadRejectsInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("not: valid: yaml: [\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := geargen.Load(path)
	if err == nil {
		t.Fatal("Load() returned nil for invalid YAML")
	}
}

func TestLoadParsesUnitNumberAttribute(t *testing.T) {
	yaml := `version: 1
assignment_roles: []
option_sets: []
attributes:
  - key: bike.suspension_travel
    label_de: Federweg
    value_type: unit_number
    unit_group: length
    allowed_units: [mm, in]
gear_types: []
`
	path := filepath.Join(t.TempDir(), "unit-number.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	schema, err := geargen.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(schema.Attributes) != 1 {
		t.Fatalf("Attributes: got %d, want 1", len(schema.Attributes))
	}
	a := schema.Attributes[0]
	if a.ValueType != "unit_number" {
		t.Errorf("ValueType = %q, want %q", a.ValueType, "unit_number")
	}
	if a.UnitGroup != "length" {
		t.Errorf("UnitGroup = %q, want %q", a.UnitGroup, "length")
	}
	if len(a.AllowedUnits) != 2 || a.AllowedUnits[0] != "mm" || a.AllowedUnits[1] != "in" {
		t.Errorf("AllowedUnits = %v, want [mm in]", a.AllowedUnits)
	}
}

func TestLoadParsesRequiredFlag(t *testing.T) {
	yaml := `version: 1
assignment_roles: []
option_sets: []
attributes:
  - key: bike.frame_material
    label_de: Rahmenmaterial
    value_type: string
    required: true
  - key: bike.notes
    label_de: Notizen
    value_type: string
gear_types: []
`
	path := filepath.Join(t.TempDir(), "required.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	schema, err := geargen.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(schema.Attributes) != 2 {
		t.Fatalf("Attributes: got %d, want 2", len(schema.Attributes))
	}
	if !schema.Attributes[0].Required {
		t.Errorf("Attributes[0].Required = false, want true")
	}
	if schema.Attributes[1].Required {
		t.Errorf("Attributes[1].Required = true, want false")
	}
}

func TestLoadParsesShowWhen(t *testing.T) {
	yaml := `version: 1
assignment_roles: []
option_sets:
  - key: bike.wheel_size
    options:
      - key: mullet
        label_de: Mullet
      - key: s29
        label_de: 29 Zoll
attributes:
  - key: bike.wheel_size
    label_de: Laufradgröße
    value_type: enum
    option_set_key: bike.wheel_size
  - key: bike.wheel_size_front
    label_de: Laufradgröße vorne
    value_type: enum
    option_set_key: bike.wheel_size
    show_when:
      field: bike.wheel_size
      value: mullet
gear_types: []
`
	path := filepath.Join(t.TempDir(), "show-when.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	schema, err := geargen.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(schema.Attributes) != 2 {
		t.Fatalf("Attributes: got %d, want 2", len(schema.Attributes))
	}
	// First attribute has no show_when.
	if schema.Attributes[0].ShowWhen != nil {
		t.Errorf("Attributes[0].ShowWhen = %v, want nil", schema.Attributes[0].ShowWhen)
	}
	// Second attribute has show_when.
	sw := schema.Attributes[1].ShowWhen
	if sw == nil {
		t.Fatal("Attributes[1].ShowWhen is nil, want non-nil")
	}
	if sw.Field != "bike.wheel_size" {
		t.Errorf("ShowWhen.Field = %q, want %q", sw.Field, "bike.wheel_size")
	}
	if sw.Value != "mullet" {
		t.Errorf("ShowWhen.Value = %q, want %q", sw.Value, "mullet")
	}
}

func TestLoadParsesGearTypeSlots(t *testing.T) {
	yaml := `version: 1
assignment_roles:
  - key: primary
    label_de: Haupt
    unique_per_activity: true
option_sets: []
attributes: []
gear_types:
  - key: bike
    label_de: Fahrrad
    scope: bike
    default_assignment_role_key: primary
    slots:
      - key: frame
        label_de: Rahmen
        component_type: bike.frame
        optional: false
      - key: shock
        label_de: Dämpfer
        component_type: bike.shock
        optional: true
`
	path := filepath.Join(t.TempDir(), "slots.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	schema, err := geargen.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(schema.GearTypes) != 1 {
		t.Fatalf("GearTypes: got %d, want 1", len(schema.GearTypes))
	}
	slots := schema.GearTypes[0].Slots
	if len(slots) != 2 {
		t.Fatalf("Slots: got %d, want 2", len(slots))
	}
	if slots[0].Key != "frame" {
		t.Errorf("Slots[0].Key = %q, want %q", slots[0].Key, "frame")
	}
	if slots[0].ComponentType != "bike.frame" {
		t.Errorf("Slots[0].ComponentType = %q, want %q", slots[0].ComponentType, "bike.frame")
	}
	if slots[0].Optional {
		t.Error("Slots[0].Optional = true, want false")
	}
	if slots[1].Key != "shock" {
		t.Errorf("Slots[1].Key = %q, want %q", slots[1].Key, "shock")
	}
	if !slots[1].Optional {
		t.Error("Slots[1].Optional = false, want true")
	}
}

func TestLoadParsesComponentTypes(t *testing.T) {
	yaml := `version: 1
assignment_roles:
  - key: primary
    label_de: Haupt
    unique_per_activity: true
option_sets: []
attributes: []
component_types:
  - key: bike.frame
    label_de: Rahmen
    attribute_keys: [bike.frame_size_cm]
  - key: bike.shock
    label_de: Dämpfer
    subtype_keys: [bike.shock.air, bike.shock.coil]
    attribute_keys: []
gear_types:
  - key: bike
    label_de: Fahrrad
    scope: bike
    default_assignment_role_key: primary
`
	path := filepath.Join(t.TempDir(), "component-types.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	schema, err := geargen.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(schema.ComponentTypes) != 2 {
		t.Fatalf("ComponentTypes: got %d, want 2", len(schema.ComponentTypes))
	}
	ct0 := schema.ComponentTypes[0]
	if ct0.Key != "bike.frame" {
		t.Errorf("ComponentTypes[0].Key = %q, want %q", ct0.Key, "bike.frame")
	}
	if ct0.LabelDE != "Rahmen" {
		t.Errorf("ComponentTypes[0].LabelDE = %q, want %q", ct0.LabelDE, "Rahmen")
	}
	if len(ct0.AttributeKeys) != 1 || ct0.AttributeKeys[0] != "bike.frame_size_cm" {
		t.Errorf("ComponentTypes[0].AttributeKeys = %v, want [bike.frame_size_cm]", ct0.AttributeKeys)
	}
	ct1 := schema.ComponentTypes[1]
	if len(ct1.SubtypeKeys) != 2 {
		t.Errorf("ComponentTypes[1].SubtypeKeys: got %d, want 2", len(ct1.SubtypeKeys))
	}
}

// TestLoadParsesObjectValueType verifies that "object" is an accepted value_type
// for subfield-only attribute containers (no top-level scalar value).
func TestLoadParsesObjectValueType(t *testing.T) {
	yaml := `version: 1
assignment_roles: []
option_sets: []
attributes:
  - key: bike.fork.features
    label_de: Ausstattung
    value_type: object
    subfields:
      - key: vibrationsdaempfung
        label_de: Vibrationsdämpfung
        value_type: boolean
gear_types: []
`
	path := filepath.Join(t.TempDir(), "object-value-type.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	schema, err := geargen.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(schema.Attributes) != 1 {
		t.Fatalf("Attributes: got %d, want 1", len(schema.Attributes))
	}
	a := schema.Attributes[0]
	if a.ValueType != "object" {
		t.Errorf("ValueType = %q, want %q", a.ValueType, "object")
	}
	if len(a.Subfields) != 1 {
		t.Fatalf("Subfields: got %d, want 1", len(a.Subfields))
	}
}

// TestLoadParsesVisibleWhenAndAppliesToGearWhen verifies that the soft-UX
// conditional DSL added in Step 1 of the schema rebuild round-trips through
// the YAML loader with the expected Op set and Value/Values shapes.
func TestLoadParsesVisibleWhenAndAppliesToGearWhen(t *testing.T) {
	yaml := `version: 1
assignment_roles: []
option_sets:
  - key: bike.frame_design
    options:
      - {key: hardtail, label_de: Hardtail}
      - {key: full_suspension, label_de: Fully}
attributes:
  - key: bike.frame_design
    label_de: Rahmenkonzept
    value_type: enum
    option_set_key: bike.frame_design
  - key: bike.shock_eye_to_eye_mm
    label_de: Dämpfer-Einbaulänge
    value_type: number
    applies_to_gear_when:
      - {ref: gear.bike.frame_design, op: eq, value: full_suspension}
  - key: bike.wheel_size_front
    label_de: Laufradgröße vorne
    value_type: number
    visible_when:
      - {ref: bike.frame_design, op: in, values: [hardtail, full_suspension]}
      - {ref: bike.shock_eye_to_eye_mm, op: is_set}
gear_types: []
`
	path := filepath.Join(t.TempDir(), "conditions.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	schema, err := geargen.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(schema.Attributes) != 3 {
		t.Fatalf("Attributes: got %d, want 3", len(schema.Attributes))
	}

	// applies_to_gear_when: gear.<attr> cross-item ref with op=eq.
	shock := schema.Attributes[1]
	if len(shock.AppliesToGearWhen) != 1 {
		t.Fatalf("AppliesToGearWhen: got %d, want 1", len(shock.AppliesToGearWhen))
	}
	got := shock.AppliesToGearWhen[0]
	if got.Ref != "gear.bike.frame_design" || got.Op != "eq" || got.Value != "full_suspension" {
		t.Errorf("AppliesToGearWhen[0] = %+v, want gear.bike.frame_design eq full_suspension", got)
	}

	// visible_when: AND-combined sibling refs, op=in (uses Values) and op=is_set.
	front := schema.Attributes[2]
	if len(front.VisibleWhen) != 2 {
		t.Fatalf("VisibleWhen: got %d, want 2", len(front.VisibleWhen))
	}
	if got := front.VisibleWhen[0]; got.Ref != "bike.frame_design" || got.Op != "in" {
		t.Errorf("VisibleWhen[0] = %+v, want bike.frame_design in [...]", got)
	}
	if vals := front.VisibleWhen[0].Values; len(vals) != 2 || vals[0] != "hardtail" || vals[1] != "full_suspension" {
		t.Errorf("VisibleWhen[0].Values = %v, want [hardtail full_suspension]", vals)
	}
	if got := front.VisibleWhen[1]; got.Ref != "bike.shock_eye_to_eye_mm" || got.Op != "is_set" {
		t.Errorf("VisibleWhen[1] = %+v, want bike.shock_eye_to_eye_mm is_set", got)
	}
	if got := front.VisibleWhen[1]; got.Value != nil || len(got.Values) != 0 {
		t.Errorf("VisibleWhen[1] should leave Value/Values empty for op=is_set, got Value=%v Values=%v", got.Value, got.Values)
	}
}

// TestLoadParsesRecommendedConstraints verifies the per-gear_type soft-default
// block parses into RecommendedConstraints with discrete attribute values.
// Values are intentionally mixed string/number to exercise the any decoder.
func TestLoadParsesRecommendedConstraints(t *testing.T) {
	yaml := `version: 1
assignment_roles:
  - key: primary
    label_de: Haupt
    unique_per_activity: true
option_sets: []
attributes:
  - key: bike.wheel_size
    label_de: Laufradgröße
    value_type: string
  - key: bike.travel_mm
    label_de: Federweg
    value_type: number
gear_types:
  - key: bike.mtb.enduro
    label_de: Enduro
    scope: bike
    default_assignment_role_key: primary
    attribute_keys: [bike.wheel_size, bike.travel_mm]
    recommended:
      attributes:
        - {key: bike.wheel_size, values: ["29", "27_5"]}
        - {key: bike.travel_mm, values: [160, 170, 180]}
`
	path := filepath.Join(t.TempDir(), "recommended.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	schema, err := geargen.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(schema.GearTypes) != 1 {
		t.Fatalf("GearTypes: got %d, want 1", len(schema.GearTypes))
	}
	gt := schema.GearTypes[0]
	if gt.Recommended == nil {
		t.Fatal("GearType.Recommended is nil, want non-nil")
	}
	if got := len(gt.Recommended.Attributes); got != 2 {
		t.Fatalf("Recommended.Attributes: got %d, want 2", got)
	}
	if a := gt.Recommended.Attributes[0]; a.Key != "bike.wheel_size" || len(a.Values) != 2 {
		t.Errorf("Recommended.Attributes[0] = %+v", a)
	}
	if a := gt.Recommended.Attributes[1]; a.Key != "bike.travel_mm" || len(a.Values) != 3 {
		t.Errorf("Recommended.Attributes[1] = %+v", a)
	}
}

// TestLoadRejectsInvalidConditionOp verifies the JSON schema rejects op values
// outside the closed set (eq, neq, in, not_in, is_set, is_unset).
func TestLoadRejectsInvalidConditionOp(t *testing.T) {
	yaml := `version: 1
assignment_roles: []
option_sets: []
attributes:
  - key: bike.frame_size_cm
    label_de: Rahmengröße
    value_type: number
    visible_when:
      - {ref: bike.other, op: gt, value: 50}
gear_types: []
`
	path := filepath.Join(t.TempDir(), "bad-op.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := geargen.Load(path); err == nil {
		t.Fatal("Load() returned nil for invalid op=gt, want schema validation error")
	}
}

// TestLoadRejectsLegacyBoolValueType verifies that "bool" (the dead-drift alias)
// is no longer accepted; "boolean" is the only spelling.
func TestLoadRejectsLegacyBoolValueType(t *testing.T) {
	yaml := `version: 1
assignment_roles: []
option_sets: []
attributes:
  - key: gear.is_primary
    label_de: Hauptausrüstung
    value_type: bool
gear_types: []
`
	path := filepath.Join(t.TempDir(), "legacy-bool.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := geargen.Load(path); err == nil {
		t.Fatal("Load() returned nil for legacy 'bool' value_type, want error")
	}
}

func TestLoadRejectsMissingVersion(t *testing.T) {
	yaml := `assignment_roles: []
option_sets: []
attributes: []
gear_types: []
`
	path := filepath.Join(t.TempDir(), "no-version.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := geargen.Load(path)
	if err == nil {
		t.Fatal("Load() returned nil for schema missing version")
	}
}
