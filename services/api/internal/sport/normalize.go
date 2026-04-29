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
	"bike":                {tuple: Tuple{SportType: "cycling"}},
	"biking":              {tuple: Tuple{SportType: "cycling"}},
	"ride":                {tuple: Tuple{SportType: "cycling"}},
	"mountain_biking":     {tuple: Tuple{SportType: "cycling", SubType: "mountain"}},
	"gravel_cycling":      {tuple: Tuple{SportType: "cycling", SubType: "gravel_cycling"}},
	"indoor_cycling":      {tuple: Tuple{SportType: "cycling", SubType: "indoor_cycling"}, patch: ModifierPatch{IsIndoor: boolPtr(true)}},
	"trail_running":       {tuple: Tuple{SportType: "running", SubType: "trail"}},
	"treadmill_running":   {tuple: Tuple{SportType: "running", SubType: "indoor_running"}, patch: ModifierPatch{IsIndoor: boolPtr(true)}},
	"lap_swimming":        {tuple: Tuple{SportType: "swimming", SubType: "lap_swimming"}, patch: ModifierPatch{IsIndoor: boolPtr(true)}},
	"open_water_swimming": {tuple: Tuple{SportType: "swimming", SubType: "open_water"}},
	"road_cycling":        {tuple: Tuple{SportType: "cycling", SubType: "road"}},
	"roadcycling":         {tuple: Tuple{SportType: "cycling", SubType: "road"}},
	"e_mountain_biking":   {tuple: Tuple{SportType: "cycling", SubType: "mountain"}, patch: ModifierPatch{IsElectric: boolPtr(true)}},
	"e_biking":            {tuple: Tuple{SportType: "cycling", SubType: "generic"}, patch: ModifierPatch{IsElectric: boolPtr(true)}},
	"e_bike":              {tuple: Tuple{SportType: "cycling", SubType: "generic"}, patch: ModifierPatch{IsElectric: boolPtr(true)}},
}

func NormalizeSportTuple(sportType, subType string) (Tuple, ModifierPatch, error) {
	sportType = strings.TrimSpace(sportType)
	subType = strings.TrimSpace(subType)

	tuple := Tuple{SportType: sportType, SubType: subType}
	var patch ModifierPatch

	if alias, ok := legacySportAliases[sportType]; ok {
		tuple = alias.tuple
		patch = mergeModifierPatch(patch, alias.patch)
		if subType != "" && subType != "generic" {
			tuple.SubType = subType
		}
	}

	switch {
	case tuple.SportType == "running" && tuple.SubType == "treadmill":
		tuple.SubType = "indoor_running"
		patch.IsIndoor = boolPtr(true)
	}

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

func normalizeTupleForLookup(sportType, subType string) (Tuple, bool) {
	tuple, _, err := NormalizeSportTuple(sportType, subType)
	if err != nil {
		return Tuple{SportType: strings.TrimSpace(sportType), SubType: strings.TrimSpace(subType)}, false
	}
	return tuple, true
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
