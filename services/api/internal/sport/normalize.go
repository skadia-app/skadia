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
}

type legacySportAlias struct {
	tuple Tuple
	patch ModifierPatch
}

var legacySportAliases = map[string]legacySportAlias{
	"bike":                 {tuple: Tuple{SportType: "cycling"}},
	"biking":               {tuple: Tuple{SportType: "cycling"}},
	"ride":                 {tuple: Tuple{SportType: "cycling"}},
	"mountain_biking":      {tuple: Tuple{SportType: "cycling", SubType: "mountain"}},
	"gravel_cycling":       {tuple: Tuple{SportType: "cycling", SubType: "gravel_cycling"}},
	"indoor_cycling":       {tuple: Tuple{SportType: "cycling", SubType: "indoor_cycling"}, patch: ModifierPatch{IsIndoor: boolPtr(true)}},
	"trail_running":        {tuple: Tuple{SportType: "running", SubType: "trail"}},
	"treadmill_running":    {tuple: Tuple{SportType: "running", SubType: "treadmill"}, patch: ModifierPatch{IsIndoor: boolPtr(true)}},
	"lap_swimming":         {tuple: Tuple{SportType: "swimming", SubType: "lap_swimming"}},
	"open_water_swimming":  {tuple: Tuple{SportType: "swimming", SubType: "open_water"}},
	"road_cycling":         {tuple: Tuple{SportType: "cycling", SubType: "road"}},
	"roadcycling":          {tuple: Tuple{SportType: "cycling", SubType: "road"}},
	"wheelchair_push_walk": {tuple: Tuple{SportType: "wheelchair_push", SubType: "walk_speed"}},
	"wheelchair_push_run":  {tuple: Tuple{SportType: "wheelchair_push", SubType: "run_speed"}},
	"strength":             {tuple: Tuple{SportType: "strength_training"}},
	"e_mountain_biking":    {tuple: Tuple{SportType: "cycling", SubType: "mountain"}, patch: ModifierPatch{IsElectric: boolPtr(true)}},
	"e_biking":             {tuple: Tuple{SportType: "cycling"}, patch: ModifierPatch{IsElectric: boolPtr(true)}},
	"e_bike":               {tuple: Tuple{SportType: "cycling"}, patch: ModifierPatch{IsElectric: boolPtr(true)}},
}

var legacySubTypeAliases = map[Tuple]legacySportAlias{
	{SportType: "wheelchair_push_walk", SubType: "indoor_wheelchair_walk"}: {
		tuple: Tuple{SportType: "wheelchair_push", SubType: "walk_speed"},
		patch: ModifierPatch{IsIndoor: boolPtr(true)},
	},
	{SportType: "wheelchair_push_run", SubType: "indoor_wheelchair_run"}: {
		tuple: Tuple{SportType: "wheelchair_push", SubType: "run_speed"},
		patch: ModifierPatch{IsIndoor: boolPtr(true)},
	},
}

func NormalizeSportTuple(sportType, subType string) (Tuple, ModifierPatch, error) {
	sportType = strings.TrimSpace(sportType)
	subType = strings.TrimSpace(subType)

	tuple := Tuple{SportType: sportType, SubType: subType}
	var aliasPatch ModifierPatch

	if alias, ok := legacySubTypeAliases[Tuple{SportType: sportType, SubType: subType}]; ok {
		tuple = alias.tuple
		aliasPatch = mergeModifierPatch(aliasPatch, alias.patch)
	} else if alias, ok := legacySportAliases[sportType]; ok {
		tuple = alias.tuple
		aliasPatch = mergeModifierPatch(aliasPatch, alias.patch)
		if tuple.SubType == "" && subType != "" && subType != "generic" {
			tuple.SubType = subType
		}
	}

	if tuple.SubType == "generic" {
		tuple.SubType = ""
	}
	patch := mergeModifierPatch(modifierDefaultsToPatch(ModifierDefaultsForTuple(tuple.SportType, tuple.SubType)), aliasPatch)

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
		patch.IsIndoor = defaults.IsIndoor
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
	return a
}

func boolPtr(v bool) *bool {
	return &v
}

func isLegacySportAlias(sportType string) bool {
	_, ok := legacySportAliases[sportType]
	return ok
}
