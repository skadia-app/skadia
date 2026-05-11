package gearcompat

import "testing"

func strPtr(v string) *string { return &v }

type fakeSchemaView struct{}

func (fakeSchemaView) GearTypeAppliesToTarget(source, target string) bool {
	return source == target || (source == "bike" && target == "bike.mtb.enduro")
}

func (fakeSchemaView) ComponentTypeAppliesToGear(component, target string) bool {
	return component == "bike.frame" && target == "bike.mtb.enduro"
}

func TestAllowsPresetComponentOnlyForMatchingGearType(t *testing.T) {
	schema := fakeSchemaView{}
	if !CanTransferComponent(Component{TypeKey: "bike.frame", SourceGearTypeKey: "bike", PresetGearTypeKey: strPtr("bike")}, Gear{TypeKey: "bike.mtb.enduro"}, schema) {
		t.Fatalf("bike frame should transfer between bikes")
	}
	if CanTransferComponent(Component{TypeKey: "bike.frame", SourceGearTypeKey: "bike", PresetGearTypeKey: strPtr("bike")}, Gear{TypeKey: "shoes"}, schema) {
		t.Fatalf("bike frame must not transfer to shoes")
	}
}

func TestAllowsUnknownCustomComponentOnlyWithinSameGearType(t *testing.T) {
	schema := fakeSchemaView{}
	if !CanTransferComponent(Component{TypeKey: "custom", SourceGearTypeKey: "ski"}, Gear{TypeKey: "ski"}, schema) {
		t.Fatalf("unknown custom component may transfer inside same gear type")
	}
	if CanTransferComponent(Component{TypeKey: "custom", SourceGearTypeKey: "ski"}, Gear{TypeKey: "bike"}, schema) {
		t.Fatalf("unknown custom component must not cross gear types")
	}
}

// presetSchemaView models a realistic parent-chain: bike.mtb.enduro → bike.mtb → bike
// and bike.road has bike as parent. GearTypeAppliesToTarget returns true exactly
// when source is an ancestor of (or equal to) target. ComponentTypeAppliesToGear
// is permissive for bike.frame on any bike-subtype to mirror the production schema.
type presetSchemaView struct{}

func (presetSchemaView) GearTypeAppliesToTarget(source, target string) bool {
	parents := map[string]string{
		"bike":             "",
		"bike.mtb":         "bike",
		"bike.mtb.enduro":  "bike.mtb",
		"bike.road":        "bike",
	}
	cur := target
	for i := 0; i < 8; i++ {
		if cur == source {
			return true
		}
		parent, ok := parents[cur]
		if !ok || parent == "" {
			return false
		}
		cur = parent
	}
	return false
}

func (presetSchemaView) ComponentTypeAppliesToGear(componentTypeKey, gearTypeKey string) bool {
	// Permit bike.frame on any bike-subtype so the preset branch is the
	// stricter gate in TestPresetGearTypeKeyOverridesTypeFallback.
	if componentTypeKey != "bike.frame" {
		return false
	}
	switch gearTypeKey {
	case "bike", "bike.mtb", "bike.mtb.enduro", "bike.road":
		return true
	}
	return false
}

// TestPresetGearTypeKeyOverridesTypeFallback verifies that when a component
// carries a preset_gear_type_key, that preset gates the transfer even when the
// component's type/source-type relation would otherwise allow it.
func TestPresetGearTypeKeyOverridesTypeFallback(t *testing.T) {
	schema := presetSchemaView{}

	// A bike frame with preset="bike.mtb" and SourceGearTypeKey="bike" must
	// NOT transfer to a road bike: the type/source-type fallback would allow it
	// (bike → bike.road via parent chain), but the preset is stricter.
	frame := Component{
		TypeKey:           "bike.frame",
		SourceGearTypeKey: "bike",
		PresetGearTypeKey: strPtr("bike.mtb"),
	}
	if CanTransferComponent(frame, Gear{TypeKey: "bike.road"}, schema) {
		t.Errorf("preset-locked mtb frame must not transfer to road bike")
	}

	// Same component must transfer to a bike.mtb.enduro target: the preset
	// "bike.mtb" is an ancestor of "bike.mtb.enduro" via the parent chain.
	if !CanTransferComponent(frame, Gear{TypeKey: "bike.mtb.enduro"}, schema) {
		t.Errorf("preset bike.mtb must apply to bike.mtb.enduro via parent chain")
	}

	// Same component must transfer to a bike.mtb target (exact match).
	if !CanTransferComponent(frame, Gear{TypeKey: "bike.mtb"}, schema) {
		t.Errorf("preset bike.mtb must apply to bike.mtb (exact match)")
	}
}

// TestPresetGearTypeKeyEmptyStringIsIgnored: an empty-string preset is treated
// as absent, so the type/source-type fallback applies as if the field were nil.
func TestPresetGearTypeKeyEmptyStringIsIgnored(t *testing.T) {
	schema := presetSchemaView{}
	empty := ""
	frame := Component{
		TypeKey:           "bike.frame",
		SourceGearTypeKey: "bike",
		PresetGearTypeKey: &empty,
	}
	// With preset ignored, type fallback (bike.frame applies to bike.road) wins.
	if !CanTransferComponent(frame, Gear{TypeKey: "bike.road"}, schema) {
		t.Errorf("empty-string preset should be ignored, falling back to type rule")
	}
}
