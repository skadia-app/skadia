package bestefforts

// DetermineSportGroup maps a sport + context flags to a best-efforts sport group key.
// Returns "" if the sport has no best-efforts semantics.
func DetermineSportGroup(sportType, subType string, isIndoor, isVirtual, isElectric bool) string {
	group, ok := RecordGroupForActivity(sportType, subType, isIndoor, isVirtual, isElectric)
	if !ok {
		return ""
	}
	return group
}
