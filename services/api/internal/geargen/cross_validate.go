package geargen

import (
	"fmt"
	"strings"
)

// ModifierGearTypeReference is a single gear_type key referenced from a
// sport modifier's recommended_for or not_recommended_for list.
type ModifierGearTypeReference struct {
	ModifierKey string
	ListName    string // "recommended_for" | "not_recommended_for"
	GearTypeKey string
}

// ValidateModifierGearTypeReferences ensures every gear_type key referenced
// from a sport modifier's recommended_for / not_recommended_for lists
// actually exists in the gear schema.
//
// Caller passes the set of gear_type keys from the loaded gear schema plus a
// list of (ModifierKey, ListName, GearTypeKey) triples extracted from the
// modifier registry. Returns nil on success; on failure a multi-line error
// listing each dangling reference with its source modifier and list name.
func ValidateModifierGearTypeReferences(
	gearTypeKeys map[string]struct{},
	references []ModifierGearTypeReference,
) error {
	var dangling []string
	for _, ref := range references {
		if _, ok := gearTypeKeys[ref.GearTypeKey]; !ok {
			dangling = append(dangling, fmt.Sprintf(
				"modifier %q / %s: gear_type key %q not found in gear schema",
				ref.ModifierKey, ref.ListName, ref.GearTypeKey,
			))
		}
	}
	if len(dangling) > 0 {
		return fmt.Errorf(
			"dangling gear_type references in modifier recommended_for / not_recommended_for:\n  %s",
			strings.Join(dangling, "\n  "),
		)
	}
	return nil
}
