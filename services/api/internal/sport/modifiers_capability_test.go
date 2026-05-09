package sport

import "testing"

// TestSportModifierKeysWithCapabilityRejectsUnknownCapability ensures the
// generated lookup fails loudly when given a capability name that is not in
// the schema-defined set of sport-internal capability keys. After the
// 2026-05-08 capability decoupling the four valid keys are activity_state,
// modifier_filter, modifier_patch, and taxonomy_default; the nine
// subsystem-consumption capabilities removed by that change must not be
// queried via this helper. Pre-decoupling the function returned an empty
// slice for any unknown capability, which silenced typo'd lookups and
// stale references to removed capabilities. Defensive panic catches both.
func TestSportModifierKeysWithCapabilityRejectsUnknownCapability(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("SportModifierKeysWithCapability(\"unknown_capability\") returned without panicking; " +
				"unknown capability strings must fail loudly to catch typo'd or removed-capability lookups")
		}
	}()
	_ = SportModifierKeysWithCapability("unknown_capability")
}

// TestSportModifierHasCapabilityRejectsUnknownCapability mirrors the guard
// for the per-key lookup. modifierGoFieldForCapability (sport/normalize.go)
// composes SportModifierHasCapability on the inner branch, so the same
// silent-drift hazard applied here.
func TestSportModifierHasCapabilityRejectsUnknownCapability(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("SportModifierHasCapability(\"is_indoor\", \"unknown_capability\") returned without panicking")
		}
	}()
	_ = SportModifierHasCapability("is_indoor", "unknown_capability")
}

// TestSportModifierKeysWithCapabilityAcceptsAllRetainedCapabilities documents
// the four sport-internal capability names that survive the decoupling. If
// modifiers.schema.json's capabilities required-set ever changes, this test
// must be updated in the same commit.
func TestSportModifierKeysWithCapabilityAcceptsAllRetainedCapabilities(t *testing.T) {
	for _, capability := range []string{"activity_state", "modifier_filter", "modifier_patch", "taxonomy_default"} {
		// Should not panic. We intentionally do not assert the slice contents
		// here — taxonomy_generation_test.go already pins the exact key sets.
		_ = SportModifierKeysWithCapability(capability)
	}
}
