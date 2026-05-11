package gear

import "testing"

// Tests use the compiled Schema constant from schema_generated.go.

// TestGearTypeAppliesToTarget_AncestorChain verifies that a parent gear type
// matches a descendant. In the schema: bike.mtb.enduro → bike.mtb → bike.
func TestGearTypeAppliesToTarget_AncestorChain(t *testing.T) {
	if !Schema.GearTypeAppliesToTarget("bike", "bike.mtb.enduro") {
		t.Error("expected bike to apply to bike.mtb.enduro (two hops up the chain)")
	}
	if !Schema.GearTypeAppliesToTarget("bike.mtb", "bike.mtb.enduro") {
		t.Error("expected bike.mtb to apply to bike.mtb.enduro (one hop)")
	}
	if !Schema.GearTypeAppliesToTarget("bike.mtb.enduro", "bike.mtb.enduro") {
		t.Error("expected bike.mtb.enduro to apply to itself")
	}
}

// TestGearTypeAppliesToTarget_CrossCategory verifies that unrelated categories
// do not match.
func TestGearTypeAppliesToTarget_CrossCategory(t *testing.T) {
	if Schema.GearTypeAppliesToTarget("bike", "shoes") {
		t.Error("expected bike NOT to apply to shoes")
	}
	if Schema.GearTypeAppliesToTarget("bike.road", "bike.mtb.enduro") {
		t.Error("expected bike.road NOT to apply to bike.mtb.enduro (sibling subtree)")
	}
}

// TestComponentTypeAppliesToGear_SlotInheritedFromParent verifies that a
// component type is accepted when the slot is defined on an ancestor gear
// type. bike.mtb has a "frame" slot with ComponentType="bike.frame"; the leaf
// type bike.mtb.enduro has no slots of its own but inherits via parent walk.
func TestComponentTypeAppliesToGear_SlotInheritedFromParent(t *testing.T) {
	if !Schema.ComponentTypeAppliesToGear("bike.frame", "bike.mtb.enduro") {
		t.Error("expected bike.frame to apply to bike.mtb.enduro via inherited slot from bike.mtb")
	}
	if !Schema.ComponentTypeAppliesToGear("bike.fork", "bike.mtb.enduro") {
		t.Error("expected bike.fork to apply to bike.mtb.enduro via inherited slot from bike.mtb")
	}
}

// TestComponentTypeAppliesToGear_CrossCategory verifies rejection across
// unrelated gear categories.
func TestComponentTypeAppliesToGear_CrossCategory(t *testing.T) {
	if Schema.ComponentTypeAppliesToGear("bike.frame", "shoes") {
		t.Error("expected bike.frame NOT to apply to shoes")
	}
}

// TestGearTypeAppliesToTarget_EmptyKeys verifies safe handling of empty keys.
func TestGearTypeAppliesToTarget_EmptyKeys(t *testing.T) {
	if Schema.GearTypeAppliesToTarget("", "bike") {
		t.Error("empty source should return false")
	}
	if Schema.GearTypeAppliesToTarget("bike", "") {
		t.Error("empty target should return false")
	}
}

// TestComponentTypeAppliesToGear_EmptyKeys verifies safe handling of empty keys.
func TestComponentTypeAppliesToGear_EmptyKeys(t *testing.T) {
	if Schema.ComponentTypeAppliesToGear("", "bike.road") {
		t.Error("empty component type should return false")
	}
	if Schema.ComponentTypeAppliesToGear("bike.frame", "") {
		t.Error("empty gear type should return false")
	}
}
