package komoot

func komootRouteSportTuple(komootSport string) (string, string, bool) {
	mapping, ok := KomootSportMapping[komootSport]
	if !ok {
		return "hiking", "", false
	}
	return mapping.SportType, mapping.SubType, mapping.IsElectric
}
