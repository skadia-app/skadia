// Package gear owns gear-defaulting logic: predicates that bind gear items to
// activities by sport_type/sub_type/modifier criteria. The neutral
// gear-applicable modifier list is generated from the subsystem registry
// (config/applicable-modifiers/subsystems.yaml) and the sport-taxonomy
// modifier audience list (config/sport-taxonomy/modifiers.yaml::subsystems)
// into applicable_modifiers_generated.go.
//
// Sport taxonomy data (Tuple, ModifierPatch, ModifierValues, GearDefaultSportOption,
// the Source-keyed alias maps) lives in internal/sport. gear → sport imports
// are allowed; the reverse is forbidden to keep sport as the foundational
// layer.
package gear

import (
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// GearDefaultPredicate selects activities that should default to a given
// gear item by sport_type, optional sub_type, and optional modifier values.
type GearDefaultPredicate struct {
	SportType string               `json:"sport_type"`
	SubType   *string              `json:"sub_type,omitempty"`
	Modifiers *sport.ModifierPatch `json:"modifiers,omitempty"`
}

// GearDefaultPredicateForSource resolves a source-specific raw sport label to
// a normalized gear-default predicate (sport_type + optional sub_type +
// modifier defaults from the alias).
func GearDefaultPredicateForSource(source sport.Source, rawSport string) (GearDefaultPredicate, bool) {
	rawSport = strings.TrimSpace(rawSport)
	if rawSport == "" {
		return GearDefaultPredicate{}, false
	}
	result, err := sport.ResolveSport(sport.ResolveInput{Source: source, RawSport: rawSport})
	if err == nil && result.Resolved && result.Tuple.SportType != "" {
		return gearDefaultPredicateFromResolved(result.Tuple, result.Patch), true
	}
	return GearDefaultPredicate{}, false
}

// NormalizeGearDefaultSports validates a list of storage-tier gear-default
// sport_type identifiers, deduping and trimming.
func NormalizeGearDefaultSports(values []string) ([]string, error) {
	return normalizeGearDefaultSports(values, sport.GearDefaultStorageSportOptions())
}

// NormalizeManualGearDefaultSports validates a list of manual-tier
// gear-default sport_type identifiers (the user-pickable subset).
func NormalizeManualGearDefaultSports(values []string) ([]string, error) {
	return normalizeGearDefaultSports(values, sport.GearDefaultManualSportOptions())
}

// GearDefaultPredicatesFromSports lifts a list of canonical sport_type
// identifiers into bare predicates (no sub_type, no modifier constraints).
func GearDefaultPredicatesFromSports(values []string) ([]GearDefaultPredicate, error) {
	sports, err := NormalizeGearDefaultSports(values)
	if err != nil {
		return nil, err
	}
	predicates := make([]GearDefaultPredicate, 0, len(sports))
	for _, sportType := range sports {
		predicates = append(predicates, GearDefaultPredicate{SportType: sportType})
	}
	return predicates, nil
}

// NormalizeGearDefaultPredicates normalizes and dedupes a slice of predicates,
// rejecting unknown sport_types and modifier keys not applicable to gear.
func NormalizeGearDefaultPredicates(predicates []GearDefaultPredicate) ([]GearDefaultPredicate, error) {
	if len(predicates) == 0 {
		return []GearDefaultPredicate{}, nil
	}
	out := make([]GearDefaultPredicate, 0, len(predicates))
	seen := map[string]struct{}{}
	for _, predicate := range predicates {
		normalized, err := normalizeGearDefaultPredicate(predicate)
		if err != nil {
			return nil, err
		}
		if normalized.SportType == "" {
			continue
		}
		key := gearDefaultPredicateKey(normalized)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	if out == nil {
		return []GearDefaultPredicate{}, nil
	}
	return out, nil
}

// GearDefaultPredicateMatchesActivity returns true iff the predicate's
// sport_type, optional sub_type, and optional gear-applicable modifier
// constraints all match the given activity context.
func GearDefaultPredicateMatchesActivity(predicate GearDefaultPredicate, sportType, subType string, modifiers sport.ModifierValues) bool {
	normalized, err := normalizeGearDefaultPredicate(predicate)
	if err != nil || normalized.SportType == "" {
		return false
	}
	activityTuple, _, err := sport.NormalizeSportTuple(sportType, subType)
	if err != nil {
		return false
	}
	if normalized.SportType != activityTuple.SportType {
		return false
	}
	if normalized.SubType != nil && *normalized.SubType != activityTuple.SubType {
		return false
	}
	if normalized.Modifiers == nil {
		return true
	}
	for _, key := range GearApplicableModifierKeys {
		want := sport.ModifierPatchValue(*normalized.Modifiers, key)
		if want != nil && *want != sport.ModifierValuesValue(modifiers, key) {
			return false
		}
	}
	return true
}

func normalizeGearDefaultSports(values []string, options []sport.GearDefaultSportOption) ([]string, error) {
	if len(values) == 0 {
		return []string{}, nil
	}
	allowed := map[string]struct{}{}
	for _, option := range options {
		allowed[option.SportType] = struct{}{}
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		sportType := strings.TrimSpace(value)
		if sportType == "" {
			continue
		}
		if strings.Contains(sportType, ":") {
			return nil, fmt.Errorf("gear default sport %q must be a top-level sport_type, not a tuple key", sportType)
		}
		if _, ok := allowed[sportType]; !ok {
			return nil, fmt.Errorf("gear default sport %q is not allowed", sportType)
		}
		if _, ok := seen[sportType]; ok {
			continue
		}
		seen[sportType] = struct{}{}
		out = append(out, sportType)
	}
	if out == nil {
		return []string{}, nil
	}
	return out, nil
}

func normalizeGearDefaultPredicate(predicate GearDefaultPredicate) (GearDefaultPredicate, error) {
	sportType := strings.TrimSpace(predicate.SportType)
	if sportType == "" {
		return GearDefaultPredicate{}, nil
	}
	subType := ""
	if predicate.SubType != nil {
		subType = strings.TrimSpace(*predicate.SubType)
	}
	tuple, _, err := sport.NormalizeSportTuple(sportType, subType)
	if err != nil {
		return GearDefaultPredicate{}, err
	}
	if _, err := NormalizeGearDefaultSports([]string{tuple.SportType}); err != nil {
		return GearDefaultPredicate{}, err
	}
	normalized := GearDefaultPredicate{SportType: tuple.SportType}
	if tuple.SubType != "" {
		sub := tuple.SubType
		normalized.SubType = &sub
	}
	patch, err := normalizeGearDefaultModifierPatch(predicate.Modifiers)
	if err != nil {
		return GearDefaultPredicate{}, err
	}
	if patch != nil {
		normalized.Modifiers = patch
	}
	return normalized, nil
}

func gearDefaultPredicateFromResolved(tuple sport.Tuple, patch sport.ModifierPatch) GearDefaultPredicate {
	predicate := GearDefaultPredicate{SportType: tuple.SportType}
	if tuple.SubType != "" {
		subType := tuple.SubType
		predicate.SubType = &subType
	}
	if normalizedPatch, err := normalizeGearDefaultModifierPatch(&patch); err == nil && normalizedPatch != nil {
		predicate.Modifiers = normalizedPatch
	}
	return predicate
}

func normalizeGearDefaultModifierPatch(patch *sport.ModifierPatch) (*sport.ModifierPatch, error) {
	if patch == nil {
		return nil, nil
	}
	normalized := sport.ModifierPatch{}
	for _, key := range sport.SportModifierKeys() {
		value := sport.ModifierPatchValue(*patch, key)
		if value == nil {
			continue
		}
		if !IsApplicableModifier(key) {
			return nil, fmt.Errorf("gear default modifier %q is not allowed", key)
		}
		sport.SetModifierPatchValue(&normalized, key, value)
	}
	if sport.ModifierPatchIsEmpty(&normalized) {
		return nil, nil
	}
	return &normalized, nil
}

func gearDefaultPredicateKey(predicate GearDefaultPredicate) string {
	key := predicate.SportType
	if predicate.SubType != nil {
		key += ":" + *predicate.SubType
	}
	if predicate.Modifiers != nil {
		for _, modifierKey := range GearApplicableModifierKeys {
			value := sport.ModifierPatchValue(*predicate.Modifiers, modifierKey)
			if value != nil {
				key += fmt.Sprintf("|%s=%t", modifierKey, *value)
			}
		}
	}
	return key
}

// AllScopes returns the de-duplicated set of scope values defined in the gear schema.
// Derived from Schema.GearTypes at call time; not generated.
func AllScopes() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, gt := range Schema.GearTypes {
		if gt.Scope == "" {
			continue
		}
		if _, ok := seen[gt.Scope]; !ok {
			seen[gt.Scope] = struct{}{}
			out = append(out, gt.Scope)
		}
	}
	return out
}

// IsValidScope reports whether scope is a known scope value in the gear schema.
func IsValidScope(scope string) bool {
	for _, s := range AllScopes() {
		if s == scope {
			return true
		}
	}
	return false
}
