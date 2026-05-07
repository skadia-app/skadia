package sport

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type Tuple struct {
	SportType string
	SubType   string
}

type ModifierPatch struct {
	IsElectric *bool `json:"is_electric,omitempty" yaml:"is_electric,omitempty"`
	IsIndoor   *bool `json:"is_indoor,omitempty" yaml:"is_indoor,omitempty"`
	IsVirtual  *bool `json:"is_virtual,omitempty" yaml:"is_virtual,omitempty"`
	IsCommute  *bool `json:"is_commute,omitempty" yaml:"is_commute,omitempty"`
}

type ModifierValues struct {
	IsElectric bool
	IsIndoor   bool
	IsVirtual  bool
	IsCommute  bool
}

type GearDefaultPredicate struct {
	SportType string         `json:"sport_type"`
	SubType   *string        `json:"sub_type,omitempty"`
	Modifiers *ModifierPatch `json:"modifiers,omitempty"`
}

const (
	GenericSportType          = "generic"
	UnknownSportType          = "unknown"
	ParentOnlySubTypeSentinel = "generic"
)

var ErrNonSelectableTuple = errors.New("sport tuple is not selectable")

// CanonicalSportTypeID normalizes internal taxonomy identifiers, not provider labels.
func CanonicalSportTypeID(sportType string) string {
	return strings.ToLower(strings.TrimSpace(sportType))
}

// CanonicalSubTypeID normalizes storage-level parent-only subtype sentinels to empty.
func CanonicalSubTypeID(subType string) string {
	subType = strings.ToLower(strings.TrimSpace(subType))
	if subType == ParentOnlySubTypeSentinel {
		return ""
	}
	return subType
}

func hasConcreteSubType(subType string) bool {
	return CanonicalSubTypeID(subType) != ""
}

func ApplyModifierPatch(values ModifierValues, patch ModifierPatch) ModifierValues {
	for _, key := range SportModifierKeysWithCapability("modifier_patch") {
		value := modifierPatchValue(patch, key)
		if value == nil {
			continue
		}
		setModifierValuesValue(&values, key, *value)
	}
	return values
}

func ModifierPatchIsEmpty(patch *ModifierPatch) bool {
	if patch == nil {
		return true
	}
	for _, key := range SportModifierKeysWithCapability("modifier_patch") {
		if modifierPatchValue(*patch, key) != nil {
			return false
		}
	}
	return true
}

type Source string

const (
	SourceLegacy       Source = "legacy"
	SourceFITFile      Source = "fit"
	SourceGPXFile      Source = "gpx"
	SourceTCXFile      Source = "tcx"
	SourceGarminCourse Source = "garmin"
	SourceStrava       Source = "strava"
	SourceStravaRoute  Source = "strava_route"
	SourceKomoot       Source = "komoot"
)

type ResolveInput struct {
	Source   Source
	RawSport string
	RawSub   string
}

type ResolveResult struct {
	Tuple        Tuple
	Patch        ModifierPatch
	AliasMatched bool
	Resolved     bool
}

func RouteImportTuple(tuple Tuple, fallback string) (string, *string) {
	if tuple.SportType == "" {
		return fallback, nil
	}
	if tuple.SubType == "" {
		return tuple.SportType, nil
	}
	return tuple.SportType, &tuple.SubType
}

type sourceAliasDef struct {
	RawSport string
	RawSub   string
	Tuple    Tuple
	Patch    ModifierPatch
}

func NormalizeSportTuple(sportType, subType string) (Tuple, ModifierPatch, error) {
	result, err := ResolveSport(ResolveInput{Source: SourceLegacy, RawSport: sportType, RawSub: subType})
	if err != nil {
		return Tuple{}, ModifierPatch{}, err
	}
	return result.Tuple, result.Patch, nil
}

func NormalizeSelectableSportTuple(sportType, subType string) (Tuple, ModifierPatch, error) {
	tuple, patch, err := NormalizeSportTuple(sportType, subType)
	if err != nil {
		return Tuple{}, ModifierPatch{}, err
	}
	if !IsSelectableTuple(tuple.SportType, tuple.SubType) {
		return Tuple{}, ModifierPatch{}, fmt.Errorf("%w: %s/%s", ErrNonSelectableTuple, tuple.SportType, tuple.SubType)
	}
	return tuple, patch, nil
}

func ResolveSport(input ResolveInput) (ResolveResult, error) {
	rawSport := strings.TrimSpace(input.RawSport)
	rawSub := strings.TrimSpace(input.RawSub)

	if alias, ok := lookupSourceAlias(input.Source, rawSport, rawSub); ok {
		tuple := alias.Tuple
		if input.Source == SourceLegacy && alias.RawSub == "" && tuple.SubType == "" && hasConcreteSubType(rawSub) {
			// Legacy callers may already store canonical parent + subtype pairs.
			tuple.SubType = CanonicalSubTypeID(rawSub)
		}
		normalized, defaults, err := normalizeCanonicalTuple(tuple.SportType, tuple.SubType)
		if err != nil {
			return ResolveResult{}, err
		}
		return ResolveResult{
			Tuple:        normalized,
			Patch:        mergeModifierPatch(defaults, alias.Patch),
			AliasMatched: true,
			Resolved:     true,
		}, nil
	}

	switch input.Source {
	case SourceLegacy:
		tuple, patch, err := normalizeCanonicalTuple(rawSport, rawSub)
		if err != nil {
			return ResolveResult{}, err
		}
		return ResolveResult{Tuple: tuple, Patch: patch, Resolved: true}, nil
	case SourceFITFile, SourceGarminCourse:
		tuple, patch, err := normalizeCanonicalTuple(rawSport, rawSub)
		if err == nil {
			return ResolveResult{Tuple: tuple, Patch: patch, Resolved: true}, nil
		}
		if rawSub != "" {
			fallbackTuple, fallbackPatch, fallbackErr := normalizeCanonicalTuple(rawSport, "")
			if fallbackErr == nil {
				return ResolveResult{Tuple: fallbackTuple, Patch: mergeModifierPatch(fallbackPatch, patch), Resolved: true}, nil
			}
		}
		return ResolveResult{}, nil
	case SourceGPXFile, SourceTCXFile:
		tuple, patch, err := normalizeCanonicalTuple(rawSport, rawSub)
		if err == nil {
			return ResolveResult{Tuple: tuple, Patch: patch, Resolved: true}, nil
		}
		if rawSub != "" {
			fallbackTuple, fallbackPatch, fallbackErr := normalizeCanonicalTuple(rawSport, "")
			if fallbackErr == nil {
				return ResolveResult{Tuple: fallbackTuple, Patch: mergeModifierPatch(fallbackPatch, patch), Resolved: true}, nil
			}
		}
		return ResolveResult{}, nil
	case SourceStrava, SourceStravaRoute, SourceKomoot:
		return ResolveResult{}, nil
	default:
		return ResolveResult{}, fmt.Errorf("unknown sport source %q", input.Source)
	}
}

func GearDefaultSportForSource(source Source, rawSport string) (string, bool) {
	rawSport = strings.TrimSpace(rawSport)
	if rawSport == "" {
		return "", false
	}
	if byRaw, ok := generatedGearDefaultSportsBySource[source]; ok {
		if mapped, ok := byRaw[rawSport]; ok {
			return mapped, true
		}
	}
	return "", false
}

func GearDefaultPredicateForSource(source Source, rawSport string) (GearDefaultPredicate, bool) {
	rawSport = strings.TrimSpace(rawSport)
	if rawSport == "" {
		return GearDefaultPredicate{}, false
	}
	result, err := ResolveSport(ResolveInput{Source: source, RawSport: rawSport})
	if err == nil && result.Resolved && result.Tuple.SportType != "" {
		return gearDefaultPredicateFromResolved(result.Tuple, result.Patch), true
	}
	return GearDefaultPredicate{}, false
}

func GearDefaultStorageSportOptions() []GearDefaultSportOption {
	return append([]GearDefaultSportOption(nil), generatedGearDefaultStorageSportOptions...)
}

func GearDefaultManualSportOptions() []GearDefaultSportOption {
	return append([]GearDefaultSportOption(nil), generatedGearDefaultManualSportOptions...)
}

func NormalizeGearDefaultSports(values []string) ([]string, error) {
	return normalizeGearDefaultSports(values, generatedGearDefaultStorageSportOptions)
}

func NormalizeManualGearDefaultSports(values []string) ([]string, error) {
	return normalizeGearDefaultSports(values, generatedGearDefaultManualSportOptions)
}

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

func GearDefaultPredicateMatchesActivity(predicate GearDefaultPredicate, sportType, subType string, modifiers ModifierValues) bool {
	normalized, err := normalizeGearDefaultPredicate(predicate)
	if err != nil || normalized.SportType == "" {
		return false
	}
	activityTuple, _, err := NormalizeSportTuple(sportType, subType)
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
	for _, key := range SportModifierKeysWithCapability("gear_default") {
		want := ModifierPatchValue(*normalized.Modifiers, key)
		if want != nil && *want != ModifierValuesValue(modifiers, key) {
			return false
		}
	}
	return true
}

func normalizeGearDefaultSports(values []string, options []GearDefaultSportOption) ([]string, error) {
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
	tuple, _, err := NormalizeSportTuple(sportType, subType)
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

func gearDefaultPredicateFromResolved(tuple Tuple, patch ModifierPatch) GearDefaultPredicate {
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

func normalizeGearDefaultModifierPatch(patch *ModifierPatch) (*ModifierPatch, error) {
	if patch == nil {
		return nil, nil
	}
	normalized := ModifierPatch{}
	for _, key := range SportModifierKeys() {
		value := ModifierPatchValue(*patch, key)
		if value == nil {
			continue
		}
		if !SportModifierHasCapability(key, "gear_default") {
			return nil, fmt.Errorf("gear default modifier %q is not allowed", key)
		}
		SetModifierPatchValue(&normalized, key, value)
	}
	if ModifierPatchIsEmpty(&normalized) {
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
		for _, modifierKey := range SportModifierKeysWithCapability("gear_default") {
			value := ModifierPatchValue(*predicate.Modifiers, modifierKey)
			if value != nil {
				key += fmt.Sprintf("|%s=%t", modifierKey, *value)
			}
		}
	}
	return key
}

func lookupSourceAlias(source Source, rawSport, rawSub string) (sourceAliasDef, bool) {
	aliases := generatedSourceAliases[source]
	for _, alias := range aliases {
		if alias.RawSport == rawSport && alias.RawSub == rawSub {
			return alias, true
		}
	}
	if rawSub == "" {
		return sourceAliasDef{}, false
	}
	for _, alias := range aliases {
		if alias.RawSport == rawSport && alias.RawSub == "" {
			return alias, true
		}
	}
	return sourceAliasDef{}, false
}

func normalizeCanonicalTuple(sportType, subType string) (Tuple, ModifierPatch, error) {
	tuple := Tuple{SportType: CanonicalSportTypeID(sportType), SubType: CanonicalSubTypeID(subType)}
	patch := modifierDefaultsToPatch(ModifierDefaultsForTuple(tuple.SportType, tuple.SubType))

	def, ok := registry[tuple.SportType]
	if !ok {
		return Tuple{}, ModifierPatch{}, fmt.Errorf("unknown sport_type %q", sportType)
	}
	if tuple.SubType == "" {
		return tuple, patch, nil
	}
	if _, ok := def.SubTypes[tuple.SubType]; !ok {
		return Tuple{}, ModifierPatch{}, fmt.Errorf("unknown sub_type %q for sport_type %q", tuple.SubType, tuple.SportType)
	}
	return tuple, patch, nil
}

func modifierDefaultsToPatch(defaults ModifierDefaults) ModifierPatch {
	var patch ModifierPatch
	for key, value := range defaults {
		SetModifierPatchValue(&patch, key, boolPtr(value))
	}
	return patch
}

// NormalizeSportTupleForImport normalizes importer-provided sport fields while
// dropping device-specific sub-sport modes that are not canonical taxonomy
// subtypes. User-authored APIs should use NormalizeSportTuple instead so invalid
// subtypes fail loudly.
func NormalizeSportTupleForImport(sportType, subType string) (Tuple, ModifierPatch, error) {
	tuple, patch, err := NormalizeSportTuple(sportType, subType)
	if err == nil {
		return tuple, patch, nil
	}
	fallbackTuple, fallbackPatch, fallbackErr := NormalizeSportTuple(sportType, "")
	if fallbackErr != nil {
		return Tuple{}, ModifierPatch{}, err
	}
	return fallbackTuple, mergeModifierPatch(fallbackPatch, patch), nil
}

func mergeModifierPatch(a, b ModifierPatch) ModifierPatch {
	for _, key := range SportModifierKeysWithCapability("modifier_patch") {
		if value := modifierPatchValue(b, key); value != nil {
			SetModifierPatchValue(&a, key, value)
		}
	}
	return a
}

func modifierPatchValue(patch ModifierPatch, key string) *bool {
	field, ok := modifierGoField(key)
	if !ok {
		return nil
	}
	value := reflect.ValueOf(patch).FieldByName(field)
	if !value.IsValid() || value.IsNil() {
		return nil
	}
	out, _ := value.Interface().(*bool)
	return out
}

func ModifierPatchValue(patch ModifierPatch, key string) *bool {
	return modifierPatchValue(patch, key)
}

func SetModifierPatchValue(patch *ModifierPatch, key string, value *bool) bool {
	if patch == nil {
		return false
	}
	field, ok := modifierGoField(key)
	if !ok {
		return false
	}
	target := reflect.ValueOf(patch).Elem().FieldByName(field)
	if !target.IsValid() || !target.CanSet() {
		return false
	}
	if value == nil {
		target.Set(reflect.Zero(target.Type()))
		return true
	}
	target.Set(reflect.ValueOf(value))
	return true
}

func ModifierValuesValue(values ModifierValues, key string) bool {
	field, ok := modifierGoField(key)
	if !ok {
		return false
	}
	value := reflect.ValueOf(values).FieldByName(field)
	return value.IsValid() && value.Kind() == reflect.Bool && value.Bool()
}

func setModifierValuesValue(values *ModifierValues, key string, value bool) bool {
	field, ok := modifierGoField(key)
	if values == nil || !ok {
		return false
	}
	target := reflect.ValueOf(values).Elem().FieldByName(field)
	if !target.IsValid() || !target.CanSet() || target.Kind() != reflect.Bool {
		return false
	}
	target.SetBool(value)
	return true
}

func modifierGoField(key string) (string, bool) {
	modifier, ok := SportModifierByKey(key)
	if !ok || !SportModifierHasCapability(key, "modifier_patch") {
		return "", false
	}
	return modifier.GoField, true
}

func boolPtr(v bool) *bool {
	return &v
}

func isLegacySportAlias(sportType string) bool {
	sportType = strings.TrimSpace(sportType)
	for _, alias := range generatedSourceAliases[SourceLegacy] {
		if alias.RawSport == sportType && alias.RawSub == "" {
			return true
		}
	}
	return false
}
