package gear

import (
	"testing"
)

// --- helpers ----------------------------------------------------------------

func attr(key, valueType string) Attribute {
	return Attribute{Key: key, ValueType: valueType}
}

func enumAttr(key, optionSetKey string) Attribute {
	return Attribute{Key: key, ValueType: "enum", OptionSetKey: optionSetKey}
}

func attrWithSubfields(key string, subs ...Attribute) Attribute {
	return Attribute{Key: key, ValueType: "object", Subfields: subs}
}

// --- Finding #6: string and boolean ValueType type-checking -----------------

// TestValidateAttributeValue_StringAcceptsString verifies that a string
// ValueType attribute accepts a Go string value.
func TestValidateAttributeValue_StringAcceptsString(t *testing.T) {
	a := attr("climb.harness_size", "string")
	if err := validateAttributeValue(a, "M"); err != nil {
		t.Errorf("expected no error for string value, got %v", err)
	}
}

// TestValidateAttributeValue_StringRejectsInt verifies that a string
// ValueType attribute rejects an integer value (Finding #6).
func TestValidateAttributeValue_StringRejectsInt(t *testing.T) {
	a := attr("climb.harness_size", "string")
	if err := validateAttributeValue(a, 42); err == nil {
		t.Error("expected error for int value on string attribute, got nil")
	}
}

// TestValidateAttributeValue_StringRejectsBool verifies that a string
// ValueType attribute rejects a boolean value.
func TestValidateAttributeValue_StringRejectsBool(t *testing.T) {
	a := attr("some.flag", "string")
	if err := validateAttributeValue(a, true); err == nil {
		t.Error("expected error for bool value on string attribute, got nil")
	}
}

// TestValidateAttributeValue_BoolAcceptsBool verifies that a boolean
// ValueType attribute accepts a Go bool value.
func TestValidateAttributeValue_BoolAcceptsBool(t *testing.T) {
	a := attr("gear.is_primary", "boolean")
	if err := validateAttributeValue(a, true); err != nil {
		t.Errorf("expected no error for bool value, got %v", err)
	}
	if err := validateAttributeValue(a, false); err != nil {
		t.Errorf("expected no error for false bool value, got %v", err)
	}
}

// TestValidateAttributeValue_BoolRejectsStringTrue verifies that a boolean
// ValueType attribute rejects the string "true" (Finding #6).
func TestValidateAttributeValue_BoolRejectsStringTrue(t *testing.T) {
	a := attr("gear.is_primary", "boolean")
	if err := validateAttributeValue(a, "true"); err == nil {
		t.Error("expected error for string 'true' on boolean attribute, got nil")
	}
}

// TestValidateAttributeValue_BoolRejectsNumber verifies that a boolean
// ValueType attribute rejects a numeric value.
func TestValidateAttributeValue_BoolRejectsNumber(t *testing.T) {
	a := attr("gear.is_primary", "boolean")
	if err := validateAttributeValue(a, float64(1)); err == nil {
		t.Error("expected error for numeric value on boolean attribute, got nil")
	}
}

// TestValidateAttributeValue_UnknownValueTypeReturnsError verifies that an
// unknown ValueType always returns an error rather than silently passing
// (Finding #6 — default-accept branch).
func TestValidateAttributeValue_UnknownValueTypeReturnsError(t *testing.T) {
	a := attr("gear.mystery", "future_type")
	if err := validateAttributeValue(a, "anything"); err == nil {
		t.Error("expected error for unknown ValueType, got nil")
	}
}

// --- Finding #5: Subfields (object-typed) recursion -------------------------

// TestValidateAttributeValue_ObjectRejectsNonMap verifies that an object
// ValueType attribute rejects a non-map value.
func TestValidateAttributeValue_ObjectRejectsNonMap(t *testing.T) {
	a := attrWithSubfields("geo", attr("geo.size", "number"))
	if err := validateAttributeValue(a, "not a map"); err == nil {
		t.Error("expected error for non-map value on object attribute, got nil")
	}
}

// TestValidateAttributeValue_ObjectAcceptsValidSubfields verifies that a
// map value with all valid subfields passes.
func TestValidateAttributeValue_ObjectAcceptsValidSubfields(t *testing.T) {
	a := attrWithSubfields("geo",
		attr("geo.width", "number"),
		attr("geo.label", "string"),
	)
	val := map[string]any{
		"geo.width": float64(42),
		"geo.label": "ok",
	}
	if err := validateAttributeValue(a, val); err != nil {
		t.Errorf("expected no error for valid subfields, got %v", err)
	}
}

// TestValidateAttributeValue_ObjectRejectsUnknownSubkey verifies that a map
// value with a key not present in Subfields is rejected (Finding #5).
func TestValidateAttributeValue_ObjectRejectsUnknownSubkey(t *testing.T) {
	a := attrWithSubfields("geo",
		attr("geo.width", "number"),
	)
	val := map[string]any{
		"geo.width":   float64(42),
		"geo.unknown": "garbage",
	}
	if err := validateAttributeValue(a, val); err == nil {
		t.Error("expected error for unknown subkey, got nil")
	}
}

// TestValidateAttributeValue_ObjectSubfieldEnumInvalidValue verifies that
// invalid enum values inside a subfield are rejected (Finding #5).
func TestValidateAttributeValue_ObjectSubfieldEnumInvalidValue(t *testing.T) {
	// Use a real option-set from the generated schema: "position.front_rear"
	a := attrWithSubfields("geo",
		enumAttr("geo.position", "position.front_rear"),
	)
	val := map[string]any{
		"geo.position": "sideways",
	}
	if err := validateAttributeValue(a, val); err == nil {
		t.Error("expected error for invalid enum subfield value, got nil")
	}
}

// TestValidateAttributeValue_ObjectSubfieldEnumValidValue verifies that
// valid enum values inside a subfield are accepted.
func TestValidateAttributeValue_ObjectSubfieldEnumValidValue(t *testing.T) {
	a := attrWithSubfields("geo",
		enumAttr("geo.position", "position.front_rear"),
	)
	val := map[string]any{
		"geo.position": "front",
	}
	if err := validateAttributeValue(a, val); err != nil {
		t.Errorf("expected no error for valid enum subfield, got %v", err)
	}
}

// TestValidateAttributeValue_ObjectSubfieldNumberTypeMismatch verifies that
// a string value for a number subfield is rejected (Finding #5).
func TestValidateAttributeValue_ObjectSubfieldNumberTypeMismatch(t *testing.T) {
	a := attrWithSubfields("geo",
		attr("geo.size", "number"),
	)
	val := map[string]any{
		"geo.size": "large",
	}
	if err := validateAttributeValue(a, val); err == nil {
		t.Error("expected error for string value on number subfield, got nil")
	}
}

// TestValidateAttributeValue_DeepSubfieldRecursion verifies that nested
// object attributes (Subfields within Subfields) are validated recursively
// (Finding #5).
func TestValidateAttributeValue_DeepSubfieldRecursion(t *testing.T) {
	// Build: outer (object) -> inner (object) -> leaf (number)
	inner := attrWithSubfields("inner",
		attr("inner.val", "number"),
	)
	outer := attrWithSubfields("outer", inner)

	// Valid deep nesting.
	validVal := map[string]any{
		"inner": map[string]any{
			"inner.val": float64(10),
		},
	}
	if err := validateAttributeValue(outer, validVal); err != nil {
		t.Errorf("expected no error for valid deep nesting, got %v", err)
	}

	// Invalid: leaf has wrong type.
	invalidVal := map[string]any{
		"inner": map[string]any{
			"inner.val": "not-a-number",
		},
	}
	if err := validateAttributeValue(outer, invalidVal); err == nil {
		t.Error("expected error for invalid deep leaf type, got nil")
	}

	// Invalid: inner has extra key.
	extraKeyVal := map[string]any{
		"inner": map[string]any{
			"inner.val":   float64(10),
			"inner.extra": "garbage",
		},
	}
	if err := validateAttributeValue(outer, extraKeyVal); err == nil {
		t.Error("expected error for extra key in inner subfields, got nil")
	}
}

// --- Finding #10: UpdateGear type-change with stale Subfield content --------
// This is tested at the unit level in validate_test.go by verifying that
// ValidateAttributes correctly rejects stale attributes under the new type.
// The handler-level integration test is in gear_crud_test.go
// (TestUpdateGearTypeChangeRejectsStaleSubfieldContent).

// --- Task 1.1: unit_number ValueType ----------------------------------------

func unitNumberAttr(key string, allowedUnits ...string) Attribute {
	return Attribute{Key: key, ValueType: "unit_number", AllowedUnits: allowedUnits}
}

func TestValidateUnitNumberAcceptsObjectValue(t *testing.T) {
	a := unitNumberAttr("bike.travel", "mm", "in")
	val := map[string]any{"value": float64(140), "unit": "mm"}
	if err := validateAttributeValue(a, val); err != nil {
		t.Errorf("expected no error for valid unit_number object, got %v", err)
	}
}

func TestValidateUnitNumberAcceptsLegacyPlainNumber(t *testing.T) {
	a := unitNumberAttr("bike.travel", "mm", "in")
	if err := validateAttributeValue(a, float64(140)); err != nil {
		t.Errorf("expected no error for plain number (legacy), got %v", err)
	}
}

func TestValidateUnitNumberRejectsUnknownUnit(t *testing.T) {
	a := unitNumberAttr("bike.travel", "mm")
	val := map[string]any{"value": float64(140), "unit": "in"}
	if err := validateAttributeValue(a, val); err == nil {
		t.Error("expected error for unknown unit, got nil")
	}
}

func TestValidateUnitNumberRejectsNonNumericValue(t *testing.T) {
	a := unitNumberAttr("bike.travel", "mm")
	val := map[string]any{"value": "not-a-number", "unit": "mm"}
	if err := validateAttributeValue(a, val); err == nil {
		t.Error("expected error for non-numeric value in unit_number object, got nil")
	}
}

func TestValidateUnitNumberRejectsMissingValueKey(t *testing.T) {
	a := unitNumberAttr("bike.travel", "mm")
	val := map[string]any{"unit": "mm"}
	if err := validateAttributeValue(a, val); err == nil {
		t.Error("expected error for unit_number object missing 'value' key, got nil")
	}
}

func TestValidateUnitNumberRejectsString(t *testing.T) {
	a := unitNumberAttr("bike.travel", "mm")
	if err := validateAttributeValue(a, "140mm"); err == nil {
		t.Error("expected error for string value on unit_number, got nil")
	}
}

// --- Task 1.2: search_enum ValueType ----------------------------------------

func searchEnumAttr(key, optionSetKey string) Attribute {
	return Attribute{Key: key, ValueType: "search_enum", OptionSetKey: optionSetKey}
}

func TestValidateSearchEnumAcceptsValidKey(t *testing.T) {
	a := searchEnumAttr("bike.position", "position.front_rear")
	if err := validateAttributeValue(a, "front"); err != nil {
		t.Errorf("expected no error for valid search_enum key, got %v", err)
	}
}

// TestValidateSearchEnumAcceptsAnyString verifies that search_enum is loose:
// any string passes even when it is not in the referenced OptionSet.
// This matches the UX contract where freetext entry is always allowed.
func TestValidateSearchEnumAcceptsAnyString(t *testing.T) {
	a := searchEnumAttr("bike.position", "position.front_rear")
	if err := validateAttributeValue(a, "sideways"); err != nil {
		t.Errorf("expected no error for freetext search_enum value, got %v", err)
	}
}

func TestValidateSearchEnumRejectsNonString(t *testing.T) {
	a := searchEnumAttr("bike.position", "position.front_rear")
	if err := validateAttributeValue(a, 42); err == nil {
		t.Error("expected error for non-string on search_enum, got nil")
	}
}

// --- Task 1.3: multi_enum ValueType -----------------------------------------

func multiEnumAttr(key, optionSetKey string) Attribute {
	return Attribute{Key: key, ValueType: "multi_enum", OptionSetKey: optionSetKey}
}

func TestValidateMultiEnumAcceptsValidArray(t *testing.T) {
	a := multiEnumAttr("bike.position", "position.front_rear")
	val := []any{"front", "rear"}
	if err := validateAttributeValue(a, val); err != nil {
		t.Errorf("expected no error for valid multi_enum array, got %v", err)
	}
}

func TestValidateMultiEnumRejectsDuplicate(t *testing.T) {
	a := multiEnumAttr("bike.position", "position.front_rear")
	val := []any{"front", "front"}
	if err := validateAttributeValue(a, val); err == nil {
		t.Error("expected error for duplicate in multi_enum, got nil")
	}
}

func TestValidateMultiEnumRejectsUnknownKey(t *testing.T) {
	a := multiEnumAttr("bike.position", "position.front_rear")
	val := []any{"front", "sideways"}
	if err := validateAttributeValue(a, val); err == nil {
		t.Error("expected error for unknown key in multi_enum, got nil")
	}
}

func TestValidateMultiEnumRejectsNonArray(t *testing.T) {
	a := multiEnumAttr("bike.position", "position.front_rear")
	if err := validateAttributeValue(a, "front"); err == nil {
		t.Error("expected error for non-array on multi_enum, got nil")
	}
}

func TestValidateMultiEnumAcceptsEmptyArray(t *testing.T) {
	a := multiEnumAttr("bike.position", "position.front_rear")
	if err := validateAttributeValue(a, []any{}); err != nil {
		t.Errorf("expected no error for empty multi_enum array, got %v", err)
	}
}

// --- Task 1.4: required property --------------------------------------------

func TestValidateRejectsMissingRequiredTopLevel(t *testing.T) {
	// bike.frame_material is in the generated schema for gear type "bike".
	// We need an attribute that is required. We test via ValidateAttributes
	// using the real schema with a synthesized required-check.
	// Since required is a new field, test using the internal validateRequired helper
	// through ValidateAttributes on a type we control.
	// Use the "bike" gear type: bike.frame_material is in its attribute_keys in the
	// generated schema. We test the required check by synthesising an attribute
	// with Required=true via validateRequiredAttributes.
	err := validateRequiredAttributes(
		[]Attribute{{Key: "bike.frame_material", ValueType: "string", Required: true}},
		map[string]any{},
	)
	if err == nil {
		t.Error("expected error for missing required attribute, got nil")
	}
	if err.Code != "gear.schema_validation_failed" {
		t.Errorf("error Code = %q, want %q", err.Code, "gear.schema_validation_failed")
	}
}

func TestValidateRequiredAttributePresentNoError(t *testing.T) {
	err := validateRequiredAttributes(
		[]Attribute{{Key: "bike.frame_material", ValueType: "string", Required: true}},
		map[string]any{"bike.frame_material": "carbon"},
	)
	if err != nil {
		t.Errorf("expected no error when required attr is present, got %v", err)
	}
}

func TestValidateOptionalAttributeMissingNoError(t *testing.T) {
	err := validateRequiredAttributes(
		[]Attribute{{Key: "bike.notes", ValueType: "string", Required: false}},
		map[string]any{},
	)
	if err != nil {
		t.Errorf("expected no error for missing optional attr, got %v", err)
	}
}

func TestValidateObjectAttributeRejectsMissingRequiredSubfield(t *testing.T) {
	a := Attribute{
		Key:       "bike.geometry",
		ValueType: "object",
		Subfields: []Attribute{
			{Key: "reach", ValueType: "number", Required: true},
			{Key: "stack", ValueType: "number", Required: false},
		},
	}
	// Object value missing required "reach" subfield.
	val := map[string]any{"stack": float64(600)}
	if err := validateObjectAttribute(a, val); err == nil {
		t.Error("expected error for missing required subfield, got nil")
	}
}

func TestValidateObjectAttributeRequiredSubfieldPresentNoError(t *testing.T) {
	a := Attribute{
		Key:       "bike.geometry",
		ValueType: "object",
		Subfields: []Attribute{
			{Key: "reach", ValueType: "number", Required: true},
		},
	}
	val := map[string]any{"reach": float64(450)}
	if err := validateObjectAttribute(a, val); err != nil {
		t.Errorf("expected no error when required subfield present, got %v", err)
	}
}

// --- Task 1.5: show_when conditional field visibility -----------------------

// showWhenAttr builds an Attribute with a ShowWhen spec.
func showWhenAttr(key, valueType, siblingField, siblingValue string) Attribute {
	return Attribute{
		Key:       key,
		ValueType: valueType,
		ShowWhen:  &ShowWhenSpec{Field: siblingField, Value: siblingValue},
	}
}

// TestValidateRespectsShowWhenIfBranchTaken: sibling matches → dependent field
// is validated normally (invalid value produces error).
func TestValidateRespectsShowWhenIfBranchTaken(t *testing.T) {
	// Build a type with two attributes: A (enum) and B (number, show_when A == "x").
	// Submit {A: "x", B: "not-a-number"} → B is active, so type mismatch is caught.
	allowedAttrs := []Attribute{
		{Key: "test.a", ValueType: "string"},
		showWhenAttr("test.b", "number", "test.a", "x"),
	}
	vals := map[string]any{
		"test.a": "x",
		"test.b": "not-a-number",
	}
	err := validateWithShowWhen(allowedAttrs, vals)
	if err == nil {
		t.Error("expected error for invalid active show_when field, got nil")
	}
}

// TestValidateIgnoresInactiveShowWhenField: sibling doesn't match → dependent
// field is silently ignored even if the value is invalid.
func TestValidateIgnoresInactiveShowWhenField(t *testing.T) {
	allowedAttrs := []Attribute{
		{Key: "test.a", ValueType: "string"},
		showWhenAttr("test.b", "number", "test.a", "x"),
	}
	vals := map[string]any{
		"test.a": "other",
		"test.b": "not-a-number", // invalid but should be ignored
	}
	err := validateWithShowWhen(allowedAttrs, vals)
	if err != nil {
		t.Errorf("expected no error for inactive show_when field with bad value, got %v", err)
	}
}

// TestValidateRequiredShowWhenFieldMustBePresentWhenActive: required + active
// branch + missing value → error.
func TestValidateRequiredShowWhenFieldMustBePresentWhenActive(t *testing.T) {
	allowedAttrs := []Attribute{
		{Key: "test.a", ValueType: "string"},
		{Key: "test.b", ValueType: "string", Required: true, ShowWhen: &ShowWhenSpec{Field: "test.a", Value: "x"}},
	}
	vals := map[string]any{
		"test.a": "x",
		// test.b is missing but required and active
	}
	err := validateWithShowWhen(allowedAttrs, vals)
	if err == nil {
		t.Error("expected error for missing required active show_when field, got nil")
	}
}

// TestValidateInactiveRequiredShowWhenFieldDoesNotError: required + inactive
// branch + missing value → no error.
func TestValidateInactiveRequiredShowWhenFieldDoesNotError(t *testing.T) {
	allowedAttrs := []Attribute{
		{Key: "test.a", ValueType: "string"},
		{Key: "test.b", ValueType: "string", Required: true, ShowWhen: &ShowWhenSpec{Field: "test.a", Value: "x"}},
	}
	vals := map[string]any{
		"test.a": "other",
		// test.b is missing, but inactive so no error expected
	}
	err := validateWithShowWhen(allowedAttrs, vals)
	if err != nil {
		t.Errorf("expected no error for inactive required show_when field, got %v", err)
	}
}

// TestValidateObjectSubfieldShowWhenRespectsLocalScope: show_when within an
// object's subfields uses the sibling subfields' values, not top-level attrs.
func TestValidateObjectSubfieldShowWhenRespectsLocalScope(t *testing.T) {
	// Object with subfields: sub.a (string) and sub.b (number, show_when sub.a == "y").
	a := Attribute{
		Key:       "parent",
		ValueType: "object",
		Subfields: []Attribute{
			{Key: "sub.a", ValueType: "string"},
			{Key: "sub.b", ValueType: "number", ShowWhen: &ShowWhenSpec{Field: "sub.a", Value: "y"}},
		},
	}

	// Active branch: sub.a = "y" → sub.b is validated.
	activeVal := map[string]any{
		"sub.a": "y",
		"sub.b": "not-a-number", // invalid
	}
	if err := validateObjectAttribute(a, activeVal); err == nil {
		t.Error("expected error for invalid active subfield show_when, got nil")
	}

	// Inactive branch: sub.a != "y" → sub.b is ignored.
	inactiveVal := map[string]any{
		"sub.a": "z",
		"sub.b": "not-a-number", // invalid but inactive
	}
	if err := validateObjectAttribute(a, inactiveVal); err != nil {
		t.Errorf("expected no error for inactive subfield show_when, got %v", err)
	}
}

// TestValidateAttributes_SubfieldTypeChange verifies that ValidateAttributes
// for a new type key correctly rejects attributes that were valid for the old
// type but are not allowed for the new type. This mirrors the type-change
// re-validation path in UpdateGear (Finding #10).
func TestValidateAttributes_SubfieldTypeChange(t *testing.T) {
	// bike.frame_material is valid for "bike" but not for "helmet".
	attrs := map[string]any{"bike.frame_material": "carbon"}
	if err := ValidateAttributes("bike", attrs); err != nil {
		t.Fatalf("expected no error for bike type, got %v", err)
	}
	if err := ValidateAttributes("helmet", attrs); err == nil {
		t.Error("expected error for bike attr under helmet type, got nil")
	}
}
