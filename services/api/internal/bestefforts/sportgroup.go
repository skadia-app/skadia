package bestefforts

import "github.com/skadia-app/skadia/services/api/internal/sport"

// DetermineSportGroup maps a sport + context flags to a best-efforts sport group key.
// Returns "" if the sport has no best-efforts semantics.
func DetermineSportGroup(sportType, subType string, isIndoor, isVirtual, isElectric bool) string {
	if sportType == "hiking" || sportType == "mountaineering" || sportType == "snowshoeing" {
		return "hiking"
	}
	if sportType == "walking" || sportType == "floor_climbing" || sportType == "wheelchair_push" {
		return ""
	}

	cat := sport.CategoryForSport(sportType).Key

	switch cat {
	case "run":
		if isVirtual {
			return "running_virtual"
		}
		if isIndoor {
			return "running_indoor"
		}
		if sportType == "running" && subType == "trail" {
			return "running_trail"
		}
		return "running"

	case "bike":
		if isVirtual {
			return "cycling_virtual"
		}
		if isIndoor {
			return "cycling_virtual"
		}
		if isElectric {
			return "cycling_electric"
		}
		return "cycling"

	case "water":
		if sportType == "swimming" {
			if subType == "lap_swimming" {
				return "swim_pool"
			}
			return "swim_ow"
		}
		return ""
	}
	return ""
}
