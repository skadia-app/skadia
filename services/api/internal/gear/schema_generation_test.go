package gear_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/geargen"
)

// repoRoot walks up from the test file's package directory to find the repo root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "config", "sport-taxonomy", "sports.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", dir)
		}
		dir = parent
	}
}

// loadTestSchema loads and validates config/gear-schema.yaml from the repo root.
func loadTestSchema(t *testing.T) *geargen.Schema {
	t.Helper()
	root := repoRoot(t)
	schemaPath := filepath.Join(root, "config", "gear-schema.yaml")
	taxonomyDir := filepath.Join(root, "config", "sport-taxonomy")
	modifiersPath := filepath.Join(root, "config", "sport-taxonomy", "modifiers.yaml")

	s, err := geargen.Load(schemaPath)
	if err != nil {
		t.Fatalf("Load gear schema: %v", err)
	}
	if err := geargen.Validate(s, taxonomyDir, modifiersPath); err != nil {
		t.Fatalf("Validate gear schema: %v", err)
	}
	return s
}

// gearTypeKeys returns a set of all gear type keys present in the schema.
func gearTypeKeys(s *geargen.Schema) map[string]struct{} {
	m := make(map[string]struct{}, len(s.GearTypes))
	for _, gt := range s.GearTypes {
		m[gt.Key] = struct{}{}
	}
	return m
}

func assertGearTypeExists(t *testing.T, keys map[string]struct{}, key string) {
	t.Helper()
	if _, ok := keys[key]; !ok {
		t.Errorf("gear_type %q not found in schema", key)
	}
}

// TestBikeHierarchyComplete verifies the full bike hierarchy including
// cyclocross and handcycle are present.
func TestBikeHierarchyComplete(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	required := []string{
		"bike",
		"bike.road",
		"bike.mtb",
		"bike.gravel",
		"bike.gravel.cyclocross",
		"bike.city",
		"bike.trekking",
		"bike.cargo",
		"bike.tandem",
		"bike.recumbent",
		"bike.kids",
		"bike.balance",
		"bike.indoor",
		"bike.handcycle",
	}
	for _, key := range required {
		assertGearTypeExists(t, keys, key)
	}
}

// TestHelmetConsolidatedAsSingleType verifies helmet is a single sport-orthogonal
// type with subcategory and safety_standard attributes.
func TestHelmetConsolidatedAsSingleType(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	// Only one helmet type (not helmet.bike, helmet.ski, etc.)
	assertGearTypeExists(t, keys, "helmet")
	for k := range keys {
		if strings.HasPrefix(k, "helmet.") {
			t.Errorf("unexpected helmet subtype %q: helmet must be a single consolidated type", k)
		}
	}

	// Helmet must carry subcategory and safety_standard attribute keys.
	var helmetType *geargen.GearType
	for i := range s.GearTypes {
		if s.GearTypes[i].Key == "helmet" {
			helmetType = &s.GearTypes[i]
			break
		}
	}
	if helmetType == nil {
		t.Fatal("helmet gear type not found")
	}

	attrSet := make(map[string]struct{}, len(helmetType.AttributeKeys))
	for _, ak := range helmetType.AttributeKeys {
		attrSet[ak] = struct{}{}
	}
	for _, required := range []string{"helmet.subcategory", "helmet.safety_standard"} {
		if _, ok := attrSet[required]; !ok {
			t.Errorf("helmet missing attribute_key %q", required)
		}
	}

	// helmet.subcategory must be a search_enum option set with the required values.
	var subcatAttr *geargen.Attribute
	for i := range s.Attributes {
		if s.Attributes[i].Key == "helmet.subcategory" {
			subcatAttr = &s.Attributes[i]
			break
		}
	}
	if subcatAttr == nil {
		t.Fatal("attribute helmet.subcategory not found")
	}
	if subcatAttr.ValueType != "search_enum" {
		t.Errorf("helmet.subcategory value_type = %q, want search_enum", subcatAttr.ValueType)
	}

	// Locate the option set and verify required options.
	var optSet *geargen.OptionSet
	for i := range s.OptionSets {
		if s.OptionSets[i].Key == subcatAttr.OptionSetKey {
			optSet = &s.OptionSets[i]
			break
		}
	}
	if optSet == nil {
		t.Fatalf("option_set %q not found", subcatAttr.OptionSetKey)
	}
	optKeys := make(map[string]struct{}, len(optSet.Options))
	for _, o := range optSet.Options {
		optKeys[o.Key] = struct{}{}
	}
	for _, required := range []string{"road_bike", "mtb", "full_face_dh", "ski", "climb", "equestrian"} {
		if _, ok := optKeys[required]; !ok {
			t.Errorf("helmet.subcategory option_set missing key %q", required)
		}
	}

	// helmet.safety_standard must be enum_with_freetext.
	var safetyAttr *geargen.Attribute
	for i := range s.Attributes {
		if s.Attributes[i].Key == "helmet.safety_standard" {
			safetyAttr = &s.Attributes[i]
			break
		}
	}
	if safetyAttr == nil {
		t.Fatal("attribute helmet.safety_standard not found")
	}
	if safetyAttr.ValueType != "enum_with_freetext" {
		t.Errorf("helmet.safety_standard value_type = %q, want enum_with_freetext", safetyAttr.ValueType)
	}
}

// TestGogglesConsolidated verifies goggles is a single sport-orthogonal type
// with a subcategory attribute.
func TestGogglesConsolidated(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	assertGearTypeExists(t, keys, "goggles")
	for k := range keys {
		if strings.HasPrefix(k, "goggles.") {
			t.Errorf("unexpected goggles subtype %q: goggles must be a single consolidated type", k)
		}
	}

	// goggles must carry goggles.subcategory attribute.
	var gogglesType *geargen.GearType
	for i := range s.GearTypes {
		if s.GearTypes[i].Key == "goggles" {
			gogglesType = &s.GearTypes[i]
			break
		}
	}
	if gogglesType == nil {
		t.Fatal("goggles gear type not found")
	}

	found := false
	for _, ak := range gogglesType.AttributeKeys {
		if ak == "goggles.subcategory" {
			found = true
		}
	}
	if !found {
		t.Error("goggles missing attribute_key goggles.subcategory")
	}

	// goggles.subcategory option set must include ski, swim, mx, dh, sun.
	var subcatAttr *geargen.Attribute
	for i := range s.Attributes {
		if s.Attributes[i].Key == "goggles.subcategory" {
			subcatAttr = &s.Attributes[i]
			break
		}
	}
	if subcatAttr == nil {
		t.Fatal("attribute goggles.subcategory not found")
	}
	var optSet *geargen.OptionSet
	for i := range s.OptionSets {
		if s.OptionSets[i].Key == subcatAttr.OptionSetKey {
			optSet = &s.OptionSets[i]
			break
		}
	}
	if optSet == nil {
		t.Fatalf("option_set %q not found", subcatAttr.OptionSetKey)
	}
	optKeys := make(map[string]struct{}, len(optSet.Options))
	for _, o := range optSet.Options {
		optKeys[o.Key] = struct{}{}
	}
	for _, required := range []string{"ski", "swim", "mx", "dh", "sun"} {
		if _, ok := optKeys[required]; !ok {
			t.Errorf("goggles.subcategory option_set missing key %q", required)
		}
	}
}

// TestShoesHierarchy verifies the shoes hierarchy is present.
func TestShoesHierarchy(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	required := []string{
		"shoes",
		"shoes.cycling",
		"shoes.cycling.gravel",
		"shoes.running",
		"shoes.hiking",
		"shoes.ski",
		"shoes.climbing",
	}
	for _, key := range required {
		assertGearTypeExists(t, keys, key)
	}
}

// TestConsumableMaterials verifies bike.consumable component types exist
// and use the consumable assignment role.
func TestConsumableMaterials(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	required := []string{
		"bike.consumable.chain_lube",
		"bike.consumable.brake_pads",
		"bike.consumable.tire_sealant",
	}
	for _, key := range required {
		assertGearTypeExists(t, keys, key)
	}

	// Each consumable must have default_assignment_role_key == "consumable".
	consumableRoleFound := false
	for _, r := range s.AssignmentRoles {
		if r.Key == "consumable" {
			consumableRoleFound = true
		}
	}
	if !consumableRoleFound {
		t.Error("assignment_role consumable not found")
	}

	for _, key := range required {
		for _, gt := range s.GearTypes {
			if gt.Key == key {
				if gt.DefaultAssignmentRoleKey != "consumable" {
					t.Errorf("gear_type %q default_assignment_role_key = %q, want consumable", key, gt.DefaultAssignmentRoleKey)
				}
			}
		}
	}
}

// TestClimbScopeContainsAllExpectedTypes verifies the climb scope gear types are present.
func TestClimbScopeContainsAllExpectedTypes(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	required := []string{
		"climb.harness",
		"climb.rope",
		"climb.crampons",
		"climb.ice_axe",
		"climb.belay_device",
		"climb.quickdraw",
		"climb.chalkbag",
		"climb.crashpad",
	}
	for _, key := range required {
		assertGearTypeExists(t, keys, key)
	}
}

// TestApparelScopeContainsBaseTypes verifies apparel scope gear types are present
// and that each has a subcategory attribute.
func TestApparelScopeContainsBaseTypes(t *testing.T) {
	s := loadTestSchema(t)
	keys := gearTypeKeys(s)

	required := []string{
		"apparel.pants",
		"apparel.jersey",
		"apparel.gloves",
		"apparel.socks",
		"apparel.jacket",
		"apparel.shorts",
	}
	for _, key := range required {
		assertGearTypeExists(t, keys, key)
	}

	// Each apparel type must have apparel.subcategory in its attribute_keys.
	attrKeySet := make(map[string]bool)
	for _, a := range s.Attributes {
		attrKeySet[a.Key] = true
	}
	if !attrKeySet["apparel.subcategory"] {
		t.Error("attribute apparel.subcategory not found in schema attributes")
	}

	typesByKey := make(map[string]*geargen.GearType, len(s.GearTypes))
	for i := range s.GearTypes {
		typesByKey[s.GearTypes[i].Key] = &s.GearTypes[i]
	}
	for _, key := range required {
		gt, ok := typesByKey[key]
		if !ok {
			continue // already reported above
		}
		found := false
		for _, ak := range gt.AttributeKeys {
			if ak == "apparel.subcategory" {
				found = true
			}
		}
		if !found {
			t.Errorf("gear_type %q missing attribute_key apparel.subcategory", key)
		}
	}
}

// TestStep1WorkedExampleEnduroRecommended verifies that the Step 1 worked
// example for soft per-gear_type defaults is present on bike.mtb.enduro and
// references the discrete attribute-value lists declared in gear-schema.yaml.
// Step 2X will fill the remaining bike subtypes; this test guards the shape
// only, not the breadth.
func TestStep1WorkedExampleEnduroRecommended(t *testing.T) {
	s := loadTestSchema(t)
	var enduro *geargen.GearType
	for i := range s.GearTypes {
		if s.GearTypes[i].Key == "bike.mtb.enduro" {
			enduro = &s.GearTypes[i]
			break
		}
	}
	if enduro == nil {
		t.Fatal("bike.mtb.enduro gear_type not found")
	}
	if enduro.Recommended == nil {
		t.Fatal("bike.mtb.enduro.Recommended is nil — worked example missing")
	}
	if len(enduro.Recommended.Attributes) < 1 {
		t.Fatalf("bike.mtb.enduro.Recommended.Attributes is empty, want >=1")
	}
	wantKeys := map[string]bool{"bike.frame_design": false, "bike.wheel_size": false}
	for _, ra := range enduro.Recommended.Attributes {
		if _, ok := wantKeys[ra.Key]; ok {
			wantKeys[ra.Key] = true
			if len(ra.Values) == 0 {
				t.Errorf("Recommended attribute %q has empty Values", ra.Key)
			}
		}
	}
	for k, seen := range wantKeys {
		if !seen {
			t.Errorf("Recommended attribute %q missing from bike.mtb.enduro", k)
		}
	}
}

// TestStep1WorkedExampleShockConditions verifies the worked-example
// applies_to_gear_when on bike.shock_eye_to_eye_mm and visible_when on
// bike.shock_stroke_mm parse and round-trip through the loader. Soft DSL
// only — no runtime enforcement is asserted.
func TestStep1WorkedExampleShockConditions(t *testing.T) {
	s := loadTestSchema(t)
	var eye, stroke *geargen.Attribute
	for i := range s.Attributes {
		switch s.Attributes[i].Key {
		case "bike.shock_eye_to_eye_mm":
			eye = &s.Attributes[i]
		case "bike.shock_stroke_mm":
			stroke = &s.Attributes[i]
		}
	}
	if eye == nil || stroke == nil {
		t.Fatal("shock attributes not found")
	}
	if len(eye.AppliesToGearWhen) != 1 {
		t.Fatalf("bike.shock_eye_to_eye_mm.AppliesToGearWhen len = %d, want 1", len(eye.AppliesToGearWhen))
	}
	c := eye.AppliesToGearWhen[0]
	if c.Ref != "gear.bike.frame_design" || c.Op != "eq" || c.Value != "full_suspension" {
		t.Errorf("AppliesToGearWhen[0] = %+v, want gear.bike.frame_design eq full_suspension", c)
	}
	if len(stroke.VisibleWhen) != 1 {
		t.Fatalf("bike.shock_stroke_mm.VisibleWhen len = %d, want 1", len(stroke.VisibleWhen))
	}
	if c := stroke.VisibleWhen[0]; c.Ref != "bike.shock_eye_to_eye_mm" || c.Op != "is_set" {
		t.Errorf("VisibleWhen[0] = %+v, want bike.shock_eye_to_eye_mm is_set", c)
	}
}

// TestGearSchemaGeneratedGoFileMatchesYAML verifies schema_generated.go is in sync
// with config/gear-schema.yaml.
func TestGearSchemaGeneratedGoFileMatchesYAML(t *testing.T) {
	root := repoRoot(t)
	schemaPath := filepath.Join(root, "config", "gear-schema.yaml")

	s, err := geargen.Load(schemaPath)
	if err != nil {
		t.Fatalf("Load gear schema: %v", err)
	}

	want, err := geargen.RenderGo(s)
	if err != nil {
		t.Fatalf("RenderGo: %v", err)
	}

	generatedPath := filepath.Join(root, "services", "api", "internal", "gear", "schema_generated.go")
	got, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("read schema_generated.go: %v", err)
	}

	if !bytesEqual(got, want) {
		line, gotLine, wantLine := firstDiffLineBytes(got, want)
		t.Fatalf(
			"services/api/internal/gear/schema_generated.go is out of sync with config/gear-schema.yaml.\n"+
				"Run ./scripts/generate-gear-schema.sh and commit the result.\n"+
				"First diff at line %d: expected %q, got %q.",
			line, wantLine, gotLine,
		)
	}
}

// TestGearSchemaGeneratedGoFileHeaderIsCanonical verifies the generated file
// starts with the canonical codegen header comment.
func TestGearSchemaGeneratedGoFileHeaderIsCanonical(t *testing.T) {
	root := repoRoot(t)
	generatedPath := filepath.Join(root, "services", "api", "internal", "gear", "schema_generated.go")
	got, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("read schema_generated.go: %v", err)
	}
	const wantHeader = "// Code generated by geargen; DO NOT EDIT."
	if !strings.HasPrefix(string(got), wantHeader) {
		t.Errorf("schema_generated.go does not start with canonical header %q", wantHeader)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func firstDiffLineBytes(got, want []byte) (line int, gotLine, wantLine string) {
	gotLines := strings.Split(string(got), "\n")
	wantLines := strings.Split(string(want), "\n")
	for i := 0; i < len(gotLines) && i < len(wantLines); i++ {
		if gotLines[i] != wantLines[i] {
			return i + 1, gotLines[i], wantLines[i]
		}
	}
	if len(gotLines) != len(wantLines) {
		return min(len(gotLines), len(wantLines)) + 1, "(EOF)", "(EOF)"
	}
	return 0, "", ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
