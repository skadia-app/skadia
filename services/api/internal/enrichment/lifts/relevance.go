package lifts

// liftRelevantSports mirrors LIFT_RELEVANT_SPORTS from lifts.py:26-29.
var liftRelevantSports = map[string]bool{
	"alpine_skiing":        true,
	"hiking":               true,
	"mountaineering":       true,
	"cross_country_skiing": true,
	"snowboarding":         true,
}

// liftExcludedCyclingSubTypes mirrors LIFT_EXCLUDED_CYCLING_SUB_TYPES from lifts.py:30.
var liftExcludedCyclingSubTypes = map[string]bool{
	"indoor_cycling": true,
	"spin":           true,
}

// IsLiftRelevant reports whether an activity should be processed by the lift
// detector. Ported 1:1 from lifts.py:33-44 (_is_lift_relevant_activity).
func IsLiftRelevant(sportType, subType string, isIndoor, isElectric, isVirtual bool) bool {
	if isIndoor || isVirtual {
		return false
	}
	if sportType == "cycling" {
		return !liftExcludedCyclingSubTypes[subType]
	}
	return liftRelevantSports[sportType]
}
