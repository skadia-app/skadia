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

// GearDefaultStorageSportOptions returns a copy of the generated storage-tier
// gear-default sport options. Consumed by the internal/gear package, which
// owns the gear-defaulting logic but reads taxonomy data from here.
func GearDefaultStorageSportOptions() []GearDefaultSportOption {
	return append([]GearDefaultSportOption(nil), generatedGearDefaultStorageSportOptions...)
}

// GearDefaultManualSportOptions returns a copy of the generated manual-tier
// gear-default sport options.
func GearDefaultManualSportOptions() []GearDefaultSportOption {
	return append([]GearDefaultSportOption(nil), generatedGearDefaultManualSportOptions...)
}

// GearDefaultSportForSource maps a source-specific raw sport label to the
// canonical gear-default sport_type, if one is registered for that source.
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
	field, ok := modifierGoFieldForCapability(key, "modifier_patch")
	if !ok {
		return nil
	}
	value := reflect.ValueOf(patch).FieldByName(field)
	if !value.IsValid() {
		panic(fmt.Sprintf("ModifierPatch missing generated modifier field %s for %s", field, key))
	}
	if value.Kind() != reflect.Ptr || value.Type().Elem().Kind() != reflect.Bool {
		panic(fmt.Sprintf("ModifierPatch.%s type = %s, want *bool for %s", field, value.Type(), key))
	}
	if value.IsNil() {
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
	field, ok := modifierGoFieldForCapability(key, "modifier_patch")
	if !ok {
		return false
	}
	target := reflect.ValueOf(patch).Elem().FieldByName(field)
	if !target.IsValid() {
		panic(fmt.Sprintf("ModifierPatch missing generated modifier field %s for %s", field, key))
	}
	if !target.CanSet() || target.Kind() != reflect.Ptr || target.Type().Elem().Kind() != reflect.Bool {
		panic(fmt.Sprintf("ModifierPatch.%s type = %s, want settable *bool for %s", field, target.Type(), key))
	}
	if value == nil {
		target.Set(reflect.Zero(target.Type()))
		return true
	}
	target.Set(reflect.ValueOf(value))
	return true
}

func ModifierValuesValue(values ModifierValues, key string) bool {
	field, ok := modifierGoFieldForCapability(key, "activity_state")
	if !ok {
		return false
	}
	value := reflect.ValueOf(values).FieldByName(field)
	if !value.IsValid() {
		panic(fmt.Sprintf("ModifierValues missing generated modifier field %s for %s", field, key))
	}
	if value.Kind() != reflect.Bool {
		panic(fmt.Sprintf("ModifierValues.%s type = %s, want bool for %s", field, value.Type(), key))
	}
	return value.Bool()
}

func setModifierValuesValue(values *ModifierValues, key string, value bool) bool {
	field, ok := modifierGoFieldForCapability(key, "activity_state")
	if values == nil || !ok {
		return false
	}
	target := reflect.ValueOf(values).Elem().FieldByName(field)
	if !target.IsValid() {
		panic(fmt.Sprintf("ModifierValues missing generated modifier field %s for %s", field, key))
	}
	if !target.CanSet() || target.Kind() != reflect.Bool {
		panic(fmt.Sprintf("ModifierValues.%s type = %s, want settable bool for %s", field, target.Type(), key))
	}
	target.SetBool(value)
	return true
}

func modifierGoFieldForCapability(key, capability string) (string, bool) {
	modifier, ok := SportModifierByKey(key)
	if !ok || !SportModifierHasCapability(key, capability) {
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
