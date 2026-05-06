package sport

import (
	"fmt"
	"strings"
)

type Predicate struct {
	Sport   string  `json:"sport"`
	SubType *string `json:"sub_type,omitempty"`
}

type ModifierFilters struct {
	IsElectric *bool `json:"is_electric,omitempty"`
	IsIndoor   *bool `json:"is_indoor,omitempty"`
	IsVirtual  *bool `json:"is_virtual,omitempty"`
	IsCommute  *bool `json:"is_commute,omitempty"`
}

type ActivityModifierState struct {
	IsElectric bool
	IsIndoor   bool
	IsVirtual  bool
	IsCommute  bool
}

func ValidatePredicates(preds []Predicate, allowEmpty bool) error {
	if len(preds) == 0 {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("predicates must not be empty")
	}

	for i, pred := range preds {
		if pred.Sport == "" {
			return fmt.Errorf("predicate %d: sport is required", i)
		}
		if isLegacySportAlias(pred.Sport) {
			return fmt.Errorf("predicate %d: legacy sport %q must be normalized before storage", i, pred.Sport)
		}
		if _, ok := registry[pred.Sport]; !ok {
			return fmt.Errorf("predicate %d: unknown sport %q", i, pred.Sport)
		}
		if pred.SubType != nil {
			if err := validatePredicateSubType(pred.Sport, *pred.SubType); err != nil {
				return fmt.Errorf("predicate %d: %w", i, err)
			}
		}
	}
	return nil
}

func NormalizePredicates(preds []Predicate) ([]Predicate, error) {
	if preds == nil {
		return nil, nil
	}

	normalized := make([]Predicate, 0, len(preds))
	for i, pred := range preds {
		if pred.Sport == "" {
			return nil, fmt.Errorf("predicate %d: sport is required", i)
		}

		subType := ""
		hasSubtype := pred.SubType != nil
		if hasSubtype {
			subType = *pred.SubType
		}

		tuple, _, err := NormalizeSportTuple(pred.Sport, subType)
		if err != nil {
			return nil, fmt.Errorf("predicate %d: %w", i, err)
		}

		out := Predicate{Sport: tuple.SportType}
		switch {
		case hasSubtype:
			out.SubType = stringPtr(tuple.SubType)
		case isLegacySportAlias(pred.Sport) && tuple.SubType != "":
			out.SubType = stringPtr(tuple.SubType)
		}
		normalized = append(normalized, out)
	}
	if err := ValidatePredicates(normalized, true); err != nil {
		return nil, err
	}
	return normalized, nil
}

func PredicatesMatch(preds []Predicate, sportType string, subType *string) bool {
	if preds == nil {
		return true
	}
	if len(preds) == 0 {
		return false
	}

	inputSubType := ""
	if subType != nil {
		inputSubType = *subType
	}
	tuple, _, err := NormalizeSportTuple(sportType, inputSubType)
	if err != nil {
		return false
	}

	for _, pred := range preds {
		normalized, err := NormalizePredicates([]Predicate{pred})
		if err != nil || len(normalized) != 1 {
			continue
		}
		if predicateMatches(normalized[0], tuple) {
			return true
		}
	}
	return false
}

func ModifiersMatch(filters ModifierFilters, state ActivityModifierState) bool {
	for _, key := range SportModifierKeysWithCapability("modifier_filter") {
		if !boolFilterMatches(modifierFilterValue(filters, key), modifierStateValue(state, key)) {
			return false
		}
	}
	return true
}

func SetModifierFilter(filters *ModifierFilters, key string, value *bool) bool {
	switch key {
	case "is_electric":
		filters.IsElectric = value
	case "is_indoor":
		filters.IsIndoor = value
	case "is_virtual":
		filters.IsVirtual = value
	case "is_commute":
		filters.IsCommute = value
	default:
		return false
	}
	return true
}

func modifierFilterValue(filters ModifierFilters, key string) *bool {
	switch key {
	case "is_electric":
		return filters.IsElectric
	case "is_indoor":
		return filters.IsIndoor
	case "is_virtual":
		return filters.IsVirtual
	case "is_commute":
		return filters.IsCommute
	default:
		return nil
	}
}

func modifierStateValue(state ActivityModifierState, key string) bool {
	switch key {
	case "is_electric":
		return state.IsElectric
	case "is_indoor":
		return state.IsIndoor
	case "is_virtual":
		return state.IsVirtual
	case "is_commute":
		return state.IsCommute
	default:
		return false
	}
}

func PredicatesFromSportTypes(sportTypes []string) ([]Predicate, error) {
	if sportTypes == nil {
		return nil, nil
	}

	preds := make([]Predicate, 0, len(sportTypes))
	for i, sportType := range sportTypes {
		tuple, _, err := NormalizeSportTuple(sportType, "")
		if err != nil {
			return nil, fmt.Errorf("sport_types[%d]: %w", i, err)
		}

		pred := Predicate{Sport: tuple.SportType}
		if isLegacySportAlias(sportType) && tuple.SubType != "" {
			pred.SubType = stringPtr(tuple.SubType)
		}
		preds = append(preds, pred)
	}
	return preds, nil
}

func PredicatesFromFilterTokens(raw string) ([]Predicate, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	preds := make([]Predicate, 0)
	for token := range strings.SplitSeq(raw, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		sportType, subType, hasSubType := strings.Cut(token, ":")
		sportType = strings.TrimSpace(sportType)
		subType = strings.TrimSpace(subType)
		if sportType == "" {
			continue
		}
		pred := Predicate{Sport: sportType}
		if hasSubType {
			pred.SubType = stringPtr(subType)
		}
		preds = append(preds, pred)
	}
	return NormalizePredicates(preds)
}

func SportTypesFromPredicates(preds []Predicate) []string {
	if preds == nil {
		return nil
	}
	seen := make(map[string]bool, len(preds))
	sportTypes := make([]string, 0, len(preds))
	for _, pred := range preds {
		if pred.Sport == "" || seen[pred.Sport] {
			continue
		}
		seen[pred.Sport] = true
		sportTypes = append(sportTypes, pred.Sport)
	}
	return sportTypes
}

func predicateMatches(pred Predicate, tuple Tuple) bool {
	if pred.Sport != tuple.SportType {
		return false
	}
	if pred.SubType == nil {
		return true
	}
	return *pred.SubType == tuple.SubType
}

func validatePredicateSubType(sportType, subType string) error {
	if subType == "" {
		return nil
	}
	def, ok := registry[sportType]
	if !ok {
		return fmt.Errorf("unknown sport %q", sportType)
	}
	if _, ok := def.SubTypes[subType]; !ok {
		return fmt.Errorf("unknown subtype %q for sport %q", subType, sportType)
	}
	return nil
}

func boolFilterMatches(filter *bool, state bool) bool {
	return filter == nil || *filter == state
}

func stringPtr(v string) *string {
	return &v
}
