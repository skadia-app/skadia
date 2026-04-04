package bestefforts

import "github.com/skadia-app/skadia/services/api/internal/sport"

// DetermineSportGroup maps a sport + context flags to a best-efforts sport group key.
// Returns "" if the sport has no best-efforts semantics.
func DetermineSportGroup(sportType, subType string, isIndoor, isVirtual, isElectric bool) string {
	cat := sport.CategoryForSport(sportType).Key

	switch cat {
	case "run":
		// hiking/mountaineering/snowshoeing form their own group
		if sportType == "hiking" || sportType == "mountaineering" || sportType == "snowshoeing" {
			return "hiking"
		}
		// walking/floor_climbing/wheelchair — no best efforts
		if sportType == "walking" || sportType == "floor_climbing" ||
			sportType == "wheelchair_push_walk" || sportType == "wheelchair_push_run" {
			return ""
		}
		if isVirtual {
			return "running_virtual"
		}
		if isIndoor {
			return "running_indoor"
		}
		return "running"

	case "bike":
		if isVirtual {
			return "cycling_virtual"
		}
		if isElectric {
			return "cycling_electric"
		}
		return "cycling"

	case "water":
		switch sportType {
		case "lap_swimming":
			return "swim_pool"
		case "open_water_swimming":
			return "swim_ow"
		case "swimming":
			if subType == "lap_swimming" {
				return "swim_pool"
			}
			return "swim_ow"
		}
		return ""
	}
	return ""
}
