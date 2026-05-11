package gearcompat

// Component holds the schema-relevant fields of a gear component needed
// for transfer-compatibility checks.
type Component struct {
	TypeKey           string
	SourceGearTypeKey string
	PresetGearTypeKey *string
}

// Gear holds the schema-relevant fields of a gear item needed for
// transfer-compatibility checks.
type Gear struct {
	TypeKey string
}

// SchemaView is the read-only schema interface used by compatibility checks.
// Implementations may wrap gear.GearSchema or a test double.
type SchemaView interface {
	GearTypeAppliesToTarget(sourceTypeKey, targetTypeKey string) bool
	ComponentTypeAppliesToGear(componentTypeKey, gearTypeKey string) bool
}

// CanTransferComponent reports whether a component may be transferred to the
// target gear according to the given schema. The decision logic:
//  1. If the component has a preset gear type, the target must be that type
//     (or a subtype of it).
//  2. Otherwise, if the component has a schema type key, the target must
//     accept that component type.
//  3. Otherwise, fall back to requiring that the source gear type is compatible
//     with the target gear type (same type or a subtype).
func CanTransferComponent(component Component, target Gear, schema SchemaView) bool {
	targetTypeKey := normalizeGearTypeKey(target.TypeKey)
	if component.PresetGearTypeKey != nil && *component.PresetGearTypeKey != "" {
		return schema.GearTypeAppliesToTarget(normalizeGearTypeKey(*component.PresetGearTypeKey), targetTypeKey)
	}
	if component.TypeKey != "" && schema.ComponentTypeAppliesToGear(component.TypeKey, targetTypeKey) {
		return true
	}
	return component.SourceGearTypeKey != "" && schema.GearTypeAppliesToTarget(normalizeGearTypeKey(component.SourceGearTypeKey), targetTypeKey)
}

func normalizeGearTypeKey(typeKey string) string {
	if typeKey == "e_bike" {
		return "bike"
	}
	return typeKey
}
