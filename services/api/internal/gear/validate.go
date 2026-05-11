package gear

import (
	"fmt"
	"strings"
)

// ValidationError is a structured error from gear attribute/modifier/scope/predicate validation.
type ValidationError struct {
	Field   string
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// ValidateAttributes checks that every key/value pair in attrs is allowed for
// the given gear type key. Attribute keys are resolved by walking up the parent
// chain so child types inherit parent attribute sets. Required attributes that
// are missing from attrs produce a validation error.
func ValidateAttributes(typeKey string, attrs map[string]any) *ValidationError {
	allowedKeys := resolveAttributeKeys(typeKey)

	// Collect the full Attribute definitions for show_when-aware validation.
	var allowedAttrs []Attribute
	for ak := range allowedKeys {
		if a, ok := LookupAttribute(ak); ok {
			allowedAttrs = append(allowedAttrs, a)
		}
	}
	return validateWithShowWhen(allowedAttrs, attrs)
}

// validateWithShowWhen performs attribute validation over a list of allowed
// Attribute definitions against the provided values, filtering by show_when
// conditions and enforcing required constraints only on active attributes.
//
// An attribute is "active" when it has no show_when spec, or when its
// show_when.field in vals equals show_when.value. Inactive attributes are
// silently skipped — even values submitted for inactive fields are ignored.
func validateWithShowWhen(allowedAttrs []Attribute, vals map[string]any) *ValidationError {
	// Compute the active set: attributes whose show_when condition is satisfied.
	active := make([]Attribute, 0, len(allowedAttrs))
	activeKeys := make(map[string]bool, len(allowedAttrs))
	for _, a := range allowedAttrs {
		if isAttributeActive(a, vals) {
			active = append(active, a)
			activeKeys[a.Key] = true
		}
	}

	// Required check runs only on active attributes.
	if err := validateRequiredAttributes(active, vals); err != nil {
		return err
	}

	for attrKey, val := range vals {
		// Reject keys not in the allowed set when the type has a schema entry.
		if len(allowedAttrs) > 0 && !activeKeys[attrKey] {
			// Also reject completely unknown keys (not just inactive ones).
			isAllowed := false
			for _, a := range allowedAttrs {
				if a.Key == attrKey {
					isAllowed = true
					break
				}
			}
			if isAllowed {
				// Key is defined but inactive — silently ignore.
				continue
			}
			return &ValidationError{
				Field:   attrKey,
				Code:    "gear.schema_validation_failed",
				Message: fmt.Sprintf("attribute %q is not allowed", attrKey),
			}
		}
		if !activeKeys[attrKey] {
			continue
		}
		// Find the attribute definition.
		var attr Attribute
		found := false
		for _, a := range active {
			if a.Key == attrKey {
				attr = a
				found = true
				break
			}
		}
		if !found {
			continue
		}
		if err := validateAttributeValue(attr, val); err != nil {
			return err
		}
	}
	return nil
}

// isAttributeActive returns true when the attribute should be validated given
// the current values. An attribute with no ShowWhen spec is always active.
func isAttributeActive(a Attribute, vals map[string]any) bool {
	if a.ShowWhen == nil {
		return true
	}
	siblingVal, ok := vals[a.ShowWhen.Field]
	if !ok {
		return false
	}
	strVal, ok := siblingVal.(string)
	if !ok {
		return false
	}
	return strVal == a.ShowWhen.Value
}

// validateRequiredAttributes checks that all attributes with Required=true are
// present in the provided map.
func validateRequiredAttributes(attrs []Attribute, vals map[string]any) *ValidationError {
	for _, a := range attrs {
		if !a.Required {
			continue
		}
		if _, present := vals[a.Key]; !present {
			return &ValidationError{
				Field:   a.Key,
				Code:    "gear.schema_validation_failed",
				Message: fmt.Sprintf("required attribute %q is missing", a.Key),
			}
		}
	}
	return nil
}

// ValidateModifiers checks that every key in mods is in GearApplicableModifierKeys.
func ValidateModifiers(mods map[string]any) *ValidationError {
	for key := range mods {
		if !IsApplicableModifier(key) {
			return &ValidationError{
				Field:   key,
				Code:    "gear.unknown_modifier",
				Message: fmt.Sprintf("modifier %q is not applicable to gear", key),
			}
		}
	}
	return nil
}

// ValidateScope checks that scope is a known scope value.
func ValidateScope(scope string) *ValidationError {
	if scope == "" {
		return nil
	}
	if !IsValidScope(scope) {
		return &ValidationError{
			Field:   "scope",
			Code:    "gear.unknown_scope",
			Message: fmt.Sprintf("scope %q is not a valid gear scope", scope),
		}
	}
	return nil
}

// resolveAttributeKeys returns the set of allowed attribute keys for a gear
// type, including keys inherited from parent types.
func resolveAttributeKeys(typeKey string) map[string]bool {
	result := map[string]bool{}
	visited := map[string]bool{}
	key := typeKey
	for key != "" && !visited[key] {
		visited[key] = true
		gt, ok := LookupGearType(key)
		if !ok {
			break
		}
		for _, ak := range gt.AttributeKeys {
			result[ak] = true
		}
		key = gt.ParentKey
	}
	return result
}

func validateAttributeValue(attr Attribute, val any) *ValidationError {
	switch attr.ValueType {
	case "enum":
		strVal, ok := val.(string)
		if !ok {
			return &ValidationError{
				Field:   attr.Key,
				Code:    "gear.schema_validation_failed",
				Message: fmt.Sprintf("attribute %q expects a string value", attr.Key),
			}
		}
		os, ok := LookupOptionSet(attr.OptionSetKey)
		if !ok {
			// Option set not found: we cannot validate the value.
			return nil
		}
		for _, opt := range os.Options {
			if opt.Key == strVal {
				return nil
			}
		}
		validKeys := make([]string, len(os.Options))
		for i, opt := range os.Options {
			validKeys[i] = opt.Key
		}
		return &ValidationError{
			Field:   attr.Key,
			Code:    "gear.schema_validation_failed",
			Message: fmt.Sprintf("attribute %q has unknown %s option %q (valid: %s)",
				attr.Key, attr.OptionSetKey, strVal, strings.Join(validKeys, ", ")),
		}
	case "search_enum", "enum_with_freetext":
		// Autocomplete-or-freetext: any string is accepted — the UI provides
		// suggestions but does not constrain the value to the option set.
		if _, ok := val.(string); !ok {
			return &ValidationError{
				Field:   attr.Key,
				Code:    "gear.schema_validation_failed",
				Message: fmt.Sprintf("attribute %q expects a string value", attr.Key),
			}
		}
	case "multi_enum":
		return validateMultiEnumValue(attr, val)
	case "unit_number":
		return validateUnitNumberValue(attr, val)
	case "number":
		switch val.(type) {
		case float64, float32, int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
			// valid
		default:
			return &ValidationError{
				Field:   attr.Key,
				Code:    "gear.schema_validation_failed",
				Message: fmt.Sprintf("attribute %q expects a numeric value", attr.Key),
			}
		}
	case "string":
		if _, ok := val.(string); !ok {
			return &ValidationError{
				Field:   attr.Key,
				Code:    "gear.schema_validation_failed",
				Message: fmt.Sprintf("attribute %q expects a string value", attr.Key),
			}
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			return &ValidationError{
				Field:   attr.Key,
				Code:    "gear.schema_validation_failed",
				Message: fmt.Sprintf("attribute %q expects a boolean value", attr.Key),
			}
		}
	case "geometry", "object":
		return validateObjectAttribute(attr, val)
	default:
		return &ValidationError{
			Field:   attr.Key,
			Code:    "gear.schema_validation_failed",
			Message: fmt.Sprintf("attribute %q has unsupported value_type %q", attr.Key, attr.ValueType),
		}
	}
	return nil
}

// validateUnitNumberValue validates a unit_number attribute value.
// Accepts either map[string]any{"value": <number>, "unit": <string>} or
// a plain numeric value (backwards-compatible — defaults to first allowed unit).
func validateUnitNumberValue(attr Attribute, val any) *ValidationError {
	isNumeric := func(v any) bool {
		switch v.(type) {
		case float64, float32, int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
			return true
		}
		return false
	}

	// Accept plain numeric value as legacy/simplified form.
	if isNumeric(val) {
		return nil
	}

	objVal, ok := val.(map[string]any)
	if !ok {
		return &ValidationError{
			Field:   attr.Key,
			Code:    "gear.schema_validation_failed",
			Message: fmt.Sprintf("attribute %q (unit_number) expects an object {value, unit} or a plain number", attr.Key),
		}
	}

	rawValue, hasValue := objVal["value"]
	if !hasValue {
		return &ValidationError{
			Field:   attr.Key,
			Code:    "gear.schema_validation_failed",
			Message: fmt.Sprintf("attribute %q (unit_number) object is missing required key \"value\"", attr.Key),
		}
	}
	if !isNumeric(rawValue) {
		return &ValidationError{
			Field:   attr.Key,
			Code:    "gear.schema_validation_failed",
			Message: fmt.Sprintf("attribute %q (unit_number) \"value\" must be numeric", attr.Key),
		}
	}

	unitVal, hasUnit := objVal["unit"]
	if !hasUnit {
		// No unit specified; acceptable if there are no constraints.
		return nil
	}
	unitStr, ok := unitVal.(string)
	if !ok {
		return &ValidationError{
			Field:   attr.Key,
			Code:    "gear.schema_validation_failed",
			Message: fmt.Sprintf("attribute %q (unit_number) \"unit\" must be a string", attr.Key),
		}
	}

	if len(attr.AllowedUnits) == 0 {
		// No constraint on allowed units.
		return nil
	}
	for _, u := range attr.AllowedUnits {
		if u == unitStr {
			return nil
		}
	}
	return &ValidationError{
		Field:   attr.Key,
		Code:    "gear.schema_validation_failed",
		Message: fmt.Sprintf("attribute %q (unit_number) unit %q is not in allowed units [%s]",
			attr.Key, unitStr, strings.Join(attr.AllowedUnits, ", ")),
	}
}

// validateMultiEnumValue validates a multi_enum attribute value.
// Accepts []any (or []string) where every element is a valid option key in the
// referenced OptionSet. Duplicates are rejected.
func validateMultiEnumValue(attr Attribute, val any) *ValidationError {
	arrVal, ok := val.([]any)
	if !ok {
		return &ValidationError{
			Field:   attr.Key,
			Code:    "gear.schema_validation_failed",
			Message: fmt.Sprintf("attribute %q (multi_enum) expects an array value", attr.Key),
		}
	}

	os, hasOptionSet := LookupOptionSet(attr.OptionSetKey)

	seen := make(map[string]bool, len(arrVal))
	for i, item := range arrVal {
		strItem, ok := item.(string)
		if !ok {
			return &ValidationError{
				Field:   attr.Key,
				Code:    "gear.schema_validation_failed",
				Message: fmt.Sprintf("attribute %q (multi_enum) element at index %d is not a string", attr.Key, i),
			}
		}
		if seen[strItem] {
			return &ValidationError{
				Field:   attr.Key,
				Code:    "gear.schema_validation_failed",
				Message: fmt.Sprintf("attribute %q (multi_enum) contains duplicate value %q", attr.Key, strItem),
			}
		}
		seen[strItem] = true

		if hasOptionSet {
			found := false
			for _, opt := range os.Options {
				if opt.Key == strItem {
					found = true
					break
				}
			}
			if !found {
				validKeys := make([]string, len(os.Options))
				for j, opt := range os.Options {
					validKeys[j] = opt.Key
				}
				return &ValidationError{
					Field:   attr.Key,
					Code:    "gear.schema_validation_failed",
					Message: fmt.Sprintf("attribute %q (multi_enum) has unknown option %q (valid: %s)",
						attr.Key, strItem, strings.Join(validKeys, ", ")),
				}
			}
		}
	}
	return nil
}

// validateObjectAttribute validates a map value against an attribute with
// Subfields, rejecting extra keys and recursively validating each subfield.
// Required subfields must be present in the object value.
// show_when conditions on subfields use sibling subfield values as scope.
func validateObjectAttribute(attr Attribute, val any) *ValidationError {
	objVal, ok := val.(map[string]any)
	if !ok {
		return &ValidationError{
			Field:   attr.Key,
			Code:    "gear.schema_validation_failed",
			Message: fmt.Sprintf("attribute %q expects an object value", attr.Key),
		}
	}

	// Compute active subfields using show_when within the subfield scope.
	activeSubfields := make(map[string]Attribute, len(attr.Subfields))
	for _, sf := range attr.Subfields {
		if isAttributeActive(sf, objVal) {
			activeSubfields[sf.Key] = sf
		}
	}

	// Build a lookup map for ALL declared subfield keys (for unknown-key detection).
	declared := make(map[string]bool, len(attr.Subfields))
	for _, sf := range attr.Subfields {
		declared[sf.Key] = true
	}

	// Check required subfields (only active ones).
	for _, sf := range activeSubfields {
		if sf.Required {
			if _, present := objVal[sf.Key]; !present {
				return &ValidationError{
					Field:   attr.Key + "." + sf.Key,
					Code:    "gear.schema_validation_failed",
					Message: fmt.Sprintf("attribute %q has missing required subfield %q", attr.Key, sf.Key),
				}
			}
		}
	}

	// Reject unknown subkeys and silently skip inactive ones.
	for k := range objVal {
		if !declared[k] {
			return &ValidationError{
				Field:   attr.Key + "." + k,
				Code:    "gear.schema_validation_failed",
				Message: fmt.Sprintf("attribute %q has unknown subfield %q", attr.Key, k),
			}
		}
	}

	// Validate each present subfield's value, but only for active subfields.
	for k, subVal := range objVal {
		sf, active := activeSubfields[k]
		if !active {
			// Subfield is declared but inactive — silently ignore.
			continue
		}
		if err := validateAttributeValue(sf, subVal); err != nil {
			return err
		}
	}
	return nil
}
