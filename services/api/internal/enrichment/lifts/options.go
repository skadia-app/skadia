package lifts

// LiftDetectionOpts is the resolved per-activity decision about which
// detection paths run. Computed by EffectiveLiftDetectionOpts from master
// flag, user-level setting, and per-activity override pointers.
type LiftDetectionOpts struct {
	Heuristic bool
	OSMSnap   bool
}

// EffectiveLiftDetectionOpts resolves the active detection mode for one
// activity. Master-off forces {false,false} regardless of overrides; the
// stored override pointers are kept for audit/reset roundtrip but cannot
// re-enable detection when the master toggle is off.
//
// nil override pointer = inherit user setting; non-nil = use override
// even when it equals the user setting (predictable persistence).
func EffectiveLiftDetectionOpts(
	masterEnabled bool,
	userOSM, userHeuristic bool,
	overrideOSM, overrideHeuristic *bool,
) LiftDetectionOpts {
	if !masterEnabled {
		return LiftDetectionOpts{}
	}
	osm := userOSM
	if overrideOSM != nil {
		osm = *overrideOSM
	}
	heur := userHeuristic
	if overrideHeuristic != nil {
		heur = *overrideHeuristic
	}
	return LiftDetectionOpts{Heuristic: heur, OSMSnap: osm}
}
