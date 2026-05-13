package routing

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

var brouterProfileParamSurface = map[string]map[string]bool{
	"hiking-mountain": {
		"consider_elevation":       true,
		"consider_noise":           true,
		"consider_river":           true,
		"consider_forest":          true,
		"consider_town":            true,
		"SAC_scale_limit":          true,
		"SAC_scale_preferred":      true,
		"add_beeline":              true,
		"allow_ferries":            true,
		"allow_steps":              true,
		"hiking_routes_preference": true,
		"iswet":                    true,
		"turnInstructionMode":      true,
	},
	"hiking-sac2": {
		"consider_elevation": true,
		"downhillcost":       true,
		"downhillcutoff":     true,
		"uphillcost":         true,
		"uphillcutoff":       true,
		"turncost":           true,
	},
	"running": {
		"consider_elevation":       true,
		"consider_noise":           true,
		"consider_river":           true,
		"consider_forest":          true,
		"consider_town":            true,
		"SAC_scale_limit":          true,
		"SAC_scale_preferred":      true,
		"add_beeline":              true,
		"allow_ferries":            true,
		"allow_steps":              true,
		"hiking_routes_preference": true,
		"iswet":                    true,
		"turnInstructionMode":      true,
	},
	"walking": {
		"consider_elevation":       true,
		"consider_noise":           true,
		"consider_river":           true,
		"consider_forest":          true,
		"consider_town":            true,
		"SAC_scale_limit":          true,
		"SAC_scale_preferred":      true,
		"add_beeline":              true,
		"allow_ferries":            true,
		"allow_steps":              true,
		"hiking_routes_preference": true,
		"iswet":                    true,
		"turnInstructionMode":      true,
	},
	"trekking": {
		"allow_steps":                  true,
		"allow_ferries":                true,
		"ignore_cycleroutes":           true,
		"stick_to_cycleroutes":         true,
		"avoid_unsafe":                 true,
		"add_beeline":                  true,
		"consider_noise":               true,
		"consider_river":               true,
		"consider_forest":              true,
		"consider_town":                true,
		"consider_traffic":             true,
		"consider_elevation":           true,
		"downhillcost":                 true,
		"downhillcutoff":               true,
		"uphillcost":                   true,
		"uphillcutoff":                 true,
		"totalMass":                    true,
		"maxSpeed":                     true,
		"S_C_x":                        true,
		"C_r":                          true,
		"bikerPower":                   true,
		"turnInstructionMode":          true,
		"turnInstructionCatchingRange": true,
		"turnInstructionRoundabouts":   true,
		"considerTurnRestrictions":     true,
		"processUnusedTags":            true,
	},
	"cycling-road": {
		"allow_steps":                       true,
		"allow_ferries":                     true,
		"allow_motorways":                   true,
		"use_proposed_cycleroutes":          true,
		"consider_traffic":                  true,
		"consider_noise":                    true,
		"consider_river":                    true,
		"consider_forest":                   true,
		"consider_town":                     true,
		"consider_elevation":                true,
		"downhillcost":                      true,
		"downhillcutoff":                    true,
		"uphillcost":                        true,
		"uphillcutoff":                      true,
		"totalMass":                         true,
		"maxSpeed":                          true,
		"S_C_x":                             true,
		"C_r":                               true,
		"bikerPower":                        true,
		"turnInstructionMode":               true,
		"turnInstructionCatchingRange":      true,
		"turnInstructionRoundabouts":        true,
		"considerTurnRestrictions":          true,
		"correctMisplacedViaPoints":         true,
		"correctMisplacedViaPointsDistance": true,
		"processUnusedTags":                 true,
	},
	"cycling-mountain": {
		"use_proposed_cycleroutes":          true,
		"correctMisplacedViaPoints":         true,
		"correctMisplacedViaPointsDistance": true,
		"add_beeline":                       true,
	},
	"cycling-gravel": {
		"turnInstructionMode":               true,
		"turnInstructionRoundabouts":        true,
		"totalMass":                         true,
		"maxSpeed":                          true,
		"S_C_x":                             true,
		"C_r":                               true,
		"bikerPower":                        true,
		"use_proposed_cycleroutes":          true,
		"consider_elevation":                true,
		"consider_traffic_estimate":         true,
		"correctMisplacedViaPoints":         true,
		"correctMisplacedViaPointsDistance": true,
		"add_beeline":                       true,
		"assume_wet_conditions":             true,
		"avoid_noise":                       true,
		"avoid_steep_inclines":              true,
		"avoid_towns":                       true,
		"prefer_cycle_routes":               true,
		"prefer_forests":                    true,
		"prefer_rivers":                     true,
		"prefer_unpaved_paths":              true,
	},
}

// BRouterProfileNames returns the built-in BRouter profile keys supported by
// the proxy and the agent route-preview tool.
func BRouterProfileNames() []string {
	names := make([]string, 0, len(brouterProfileParamSurface))
	for name := range brouterProfileParamSurface {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BRouterProfileParamKeys returns the known profile:key override surface for a
// BRouter profile.
func BRouterProfileParamKeys(profile string) []string {
	surface := brouterProfileParamSurface[profile]
	keys := make([]string, 0, len(surface))
	for key := range surface {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ValidateBRouterProfileParams rejects unknown profile:key overrides before a
// request leaves the API boundary.
func ValidateBRouterProfileParams(profile string, params map[string]string) error {
	surface, ok := brouterProfileParamSurface[profile]
	if !ok {
		return fmt.Errorf("unknown routing profile %q", profile)
	}
	for key, value := range params {
		if !isSafeProfileParamToken(key) || !isSafeProfileParamToken(value) {
			return fmt.Errorf("profile_params.%s contains unsafe characters", key)
		}
		if !surface[key] {
			return fmt.Errorf("profile_params.%s is not supported by profile %q", key, profile)
		}
	}
	return nil
}

// ProfileConfigOverrideParamKeys returns the JSON keys that can be used as
// sparse BadRouter profile overrides.
func ProfileConfigOverrideParamKeys() []string {
	t := reflect.TypeOf(ProfileConfig{})
	keys := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		switch key {
		case "", "-", "profile", "name", "sport_types", "predicates", "modifier_filters":
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isSafeProfileParamToken(s string) bool {
	if len(s) == 0 || len(s) > 50 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '.' || c == '-') {
			return false
		}
	}
	return true
}
