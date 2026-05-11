package gear

import (
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// lookupBaseDefaults returns the default predicates for typeKey using an
// exact-match-first strategy with parent-prefix fallback. It walks the key
// from right to left by stripping segments after the last dot until a key is
// found in the map or no more dots remain.
//
// Walk stops as soon as a key is present in the map, even if its value is an
// empty slice — an empty slice is an intentional "no defaults" override.
func lookupBaseDefaults(typeKey string) []GearDefaultPredicate {
	if typeKey == "" {
		return nil
	}
	key := typeKey
	for {
		if preds, ok := applicableSportPredicatesDefaults[key]; ok {
			return preds
		}
		dot := strings.LastIndex(key, ".")
		if dot < 0 {
			return nil
		}
		key = key[:dot]
	}
}

// ComputeApplicableSportPredicates returns the default applicable-sport
// predicates for the given gear type key. Modifier overlay: each gear-
// applicable modifier key (GearApplicableModifierKeys) present with a bool
// value in the modifiers map is attached as a ModifierPatch to every
// returned predicate.
//
// Returns an empty slice when typeKey has no taxonomy default.
// Never returns nil; always returns an initialised slice.
func ComputeApplicableSportPredicates(typeKey string, modifiers map[string]any) ([]GearDefaultPredicate, error) {
	base := lookupBaseDefaults(typeKey)
	if len(base) == 0 {
		return []GearDefaultPredicate{}, nil
	}

	// Build a modifier patch restricted to gear-applicable modifier keys.
	var patch *sport.ModifierPatch
	for _, key := range GearApplicableModifierKeys {
		val, ok := modifiers[key]
		if !ok {
			continue
		}
		b, ok := val.(bool)
		if !ok {
			continue
		}
		if patch == nil {
			p := sport.ModifierPatch{}
			patch = &p
		}
		sport.SetModifierPatchValue(patch, key, boolPtr(b))
	}

	out := make([]GearDefaultPredicate, len(base))
	for i, pred := range base {
		out[i] = pred
		if patch != nil {
			cloned := *patch
			out[i].Modifiers = &cloned
		}
	}
	return out, nil
}

func boolPtr(v bool) *bool { return &v }
