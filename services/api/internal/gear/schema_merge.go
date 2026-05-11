package gear

import "fmt"

// UserSchemaDefinition carries per-type user customizations.
// Users may extend existing GearTypes with extra attributes and extra
// option-set values; they cannot remove or shadow system-defined items.
type UserSchemaDefinition struct {
	// TypeKey must match an existing GearType.Key in the base schema.
	TypeKey string

	// ExtraAttrKeys lists additional attribute keys that should be appended
	// to the GearType's AttributeKeys. Keys that already exist are silently
	// skipped. Must not match any existing system attribute key.
	ExtraAttrKeys []string

	// ExtraAttributes defines new Attribute entries that are not present in
	// the base schema. The Key of each entry must not collide with any
	// system attribute key anywhere in the base schema.
	ExtraAttributes []Attribute

	// ExtraOptionValues maps OptionSet.Key to additional OptionValues that
	// should be appended. System-defined options are preserved.
	ExtraOptionValues map[string][]OptionValue
}

// systemAttrKeys returns a set of all attribute keys defined in base.Attributes.
func systemAttrKeys(base GearSchema) map[string]struct{} {
	keys := make(map[string]struct{}, len(base.Attributes))
	for _, a := range base.Attributes {
		collectAttrKeys(a, keys)
	}
	return keys
}

func collectAttrKeys(a Attribute, out map[string]struct{}) {
	out[a.Key] = struct{}{}
	for _, sub := range a.Subfields {
		collectAttrKeys(sub, out)
	}
}

// MergeSchemaWithUserCustom applies user customizations on top of a base
// schema. The merge contract is:
//
//   - System wins: existing types, attributes, and option values are never
//     removed or modified by user overrides.
//   - Users may add new OptionValues to existing OptionSets.
//   - Users may add new Attributes to existing GearTypes, provided the
//     attribute key does not collide with any system attribute key.
//   - An override targeting an unknown type_key returns an error.
//   - An extra attribute whose key matches a system attribute key returns
//     an error (shadowing is forbidden).
func MergeSchemaWithUserCustom(base GearSchema, custom []UserSchemaDefinition) (GearSchema, error) {
	if len(custom) == 0 {
		return base, nil
	}

	sysKeys := systemAttrKeys(base)

	// Build mutable copies.
	result := GearSchema{
		AssignmentRoles: base.AssignmentRoles,
		GearTypes:       make([]GearType, len(base.GearTypes)),
		Attributes:      make([]Attribute, len(base.Attributes)),
		OptionSets:      make([]OptionSet, len(base.OptionSets)),
	}
	copy(result.GearTypes, base.GearTypes)
	copy(result.Attributes, base.Attributes)
	for i, os := range base.OptionSets {
		cp := OptionSet{Key: os.Key, Options: make([]OptionValue, len(os.Options))}
		copy(cp.Options, os.Options)
		result.OptionSets[i] = cp
	}

	for _, def := range custom {
		// Validate type_key exists.
		typeIdx := -1
		for i, gt := range result.GearTypes {
			if gt.Key == def.TypeKey {
				typeIdx = i
				break
			}
		}
		if typeIdx < 0 {
			return GearSchema{}, fmt.Errorf("gear schema merge: unknown type_key %q", def.TypeKey)
		}

		// Validate and register extra attributes.
		for _, ea := range def.ExtraAttributes {
			if _, conflict := sysKeys[ea.Key]; conflict {
				return GearSchema{}, fmt.Errorf("gear schema merge: user attribute %q shadows system attribute", ea.Key)
			}
			// Guard against double-add across multiple definitions.
			sysKeys[ea.Key] = struct{}{}
			result.Attributes = append(result.Attributes, ea)
		}

		// Append extra attr keys to the GearType (system keys are preserved by copy;
		// we only add keys that come from ExtraAttributes or ExtraAttrKeys).
		gt := result.GearTypes[typeIdx]
		for _, ek := range def.ExtraAttrKeys {
			if _, isSys := sysKeys[ek]; !isSys {
				return GearSchema{}, fmt.Errorf("gear schema merge: ExtraAttrKey %q is not a known attribute", ek)
			}
			if !containsStr(gt.AttributeKeys, ek) {
				gt.AttributeKeys = append(gt.AttributeKeys, ek)
			}
		}
		for _, ea := range def.ExtraAttributes {
			if !containsStr(gt.AttributeKeys, ea.Key) {
				gt.AttributeKeys = append(gt.AttributeKeys, ea.Key)
			}
		}
		result.GearTypes[typeIdx] = gt

		// Append extra option values.
		for osKey, extraVals := range def.ExtraOptionValues {
			found := false
			for i, os := range result.OptionSets {
				if os.Key == osKey {
					for _, v := range extraVals {
						if !hasOptionValue(os, v.Key) {
							result.OptionSets[i].Options = append(result.OptionSets[i].Options, v)
						}
					}
					found = true
					break
				}
			}
			if !found {
				// Unknown OptionSet key in override — treat as a new user-defined set.
				newSet := OptionSet{Key: osKey, Options: make([]OptionValue, len(extraVals))}
				copy(newSet.Options, extraVals)
				result.OptionSets = append(result.OptionSets, newSet)
			}
		}
	}

	return result, nil
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func hasOptionValue(os OptionSet, key string) bool {
	for _, v := range os.Options {
		if v.Key == key {
			return true
		}
	}
	return false
}
