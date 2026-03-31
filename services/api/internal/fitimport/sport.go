package fitimport

import (
	"strings"

	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
)

func SportToString(sport typedef.Sport) string {
	if sport == typedef.SportInvalid {
		return "unknown"
	}
	s := sport.String()
	if s == "" || strings.HasPrefix(s, "Sport(") {
		return "unknown"
	}
	return strings.ToLower(s)
}

func SubSportToString(subSport typedef.SubSport) string {
	if subSport == typedef.SubSportGeneric || subSport == typedef.SubSportInvalid {
		return "generic"
	}
	// Garmin Descent: sub_sport 63 = apnea pool diving (not in FIT SDK typedef)
	if subSport == 63 {
		return "apnea_pool"
	}
	s := subSport.String()
	if s == "" || strings.HasPrefix(s, "SubSport(") {
		return "generic"
	}
	return strings.ToLower(s)
}

func ParseSportFromSession(session *mesgdef.Session) (sportType, subType string, fitSportID, fitSubSportID int16) {
	sportType = SportToString(session.Sport)
	subType = SubSportToString(session.SubSport)
	fitSportID = int16(session.Sport)
	fitSubSportID = int16(session.SubSport)
	return
}
