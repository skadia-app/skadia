package sport

import (
	"fmt"
	"strings"
)

type Tuple struct {
	SportType string
	SubType   string
}

type ModifierPatch struct {
	IsElectric *bool
	IsIndoor   *bool
	IsVirtual  *bool
}

type ModifierValues struct {
	IsElectric bool
	IsIndoor   bool
	IsVirtual  bool
}

func ApplyModifierPatch(values ModifierValues, patch ModifierPatch) ModifierValues {
	if patch.IsElectric != nil {
		values.IsElectric = *patch.IsElectric
	}
	if patch.IsIndoor != nil {
		values.IsIndoor = *patch.IsIndoor
	}
	if patch.IsVirtual != nil {
		values.IsVirtual = *patch.IsVirtual
	}
	return values
}

type Source string

const (
	SourceLegacy       Source = "legacy"
	SourceFITFile      Source = "fit"
	SourceGPXFile      Source = "gpx"
	SourceTCXFile      Source = "tcx"
	SourceGarminCourse Source = "garmin"
	SourceStrava       Source = "strava"
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

func ResolveSport(input ResolveInput) (ResolveResult, error) {
	rawSport := strings.TrimSpace(input.RawSport)
	rawSub := strings.TrimSpace(input.RawSub)

	if alias, ok := lookupSourceAlias(input.Source, rawSport, rawSub); ok {
		tuple := alias.Tuple
		if input.Source == SourceLegacy && alias.RawSub == "" && tuple.SubType == "" && rawSub != "" && rawSub != "generic" {
			tuple.SubType = rawSub
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
	case SourceGPXFile, SourceTCXFile, SourceStrava, SourceKomoot:
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
	if alias, ok := lookupSourceAlias(source, rawSport, ""); ok {
		return alias.Tuple.SportType, true
	}
	if source == SourceStrava {
		return "generic", true
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
	tuple := Tuple{SportType: strings.TrimSpace(sportType), SubType: strings.TrimSpace(subType)}
	if tuple.SubType == "generic" {
		tuple.SubType = ""
	}
	patch := modifierDefaultsToPatch(ModifierDefaultsForTuple(tuple.SportType, tuple.SubType))

	def, ok := registry[tuple.SportType]
	if !ok {
		return Tuple{}, ModifierPatch{}, fmt.Errorf("unknown sport_type %q", sportType)
	}
	if tuple.SubType == "" || tuple.SubType == "generic" {
		return tuple, patch, nil
	}
	if _, ok := def.SubTypes[tuple.SubType]; !ok {
		return Tuple{}, ModifierPatch{}, fmt.Errorf("unknown sub_type %q for sport_type %q", tuple.SubType, tuple.SportType)
	}
	return tuple, patch, nil
}

func modifierDefaultsToPatch(defaults ModifierDefaults) ModifierPatch {
	var patch ModifierPatch
	if defaults.IsIndoor != nil {
		patch.IsIndoor = boolPtr(*defaults.IsIndoor)
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
	if b.IsElectric != nil {
		a.IsElectric = b.IsElectric
	}
	if b.IsIndoor != nil {
		a.IsIndoor = b.IsIndoor
	}
	if b.IsVirtual != nil {
		a.IsVirtual = b.IsVirtual
	}
	return a
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
