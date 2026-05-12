package lifts

// snap.go is the OSM-snap pipeline that turns the heuristic detector's rides
// plus the OSM lift candidates into a single deduplicated decision per ride
// (and any uncovered OSM lifts that should be filled in by Stage 3).
//
// The three stages are run in order by SnapToOSMLifts → MergeMultiRideForOSM
// → MethodACoverageGap; the consumer (lift_enrich.go) executes the SQL
// implied by each SnapResult / Stage3Result.
//
// Phase 4c. See docs/internal/2026-05-04-lift-detection-go-migration-plan.md
// §"Phase 4c — OSM-Snap-Pipeline" for the full algorithmic spec.

// SnapAction names what the pipeline decided for one heuristic ride.
type SnapAction string

const (
	// SnapActionSnap: ride covers >= 70% of one OSM lift; emit a
	// segment_efforts row plus an audit-only activity_lift_candidates row.
	SnapActionSnap SnapAction = "snap"

	// SnapActionMultiRideGroup: ride is part of a multi-ride group that
	// might add up to a full OSM lift covering — Stage 2 decides whether
	// to merge or split.
	SnapActionMultiRideGroup SnapAction = "multi_ride_group"

	// SnapActionPassthrough: ride has no OSM match good enough for
	// Stage 1 or Stage 2; it stays a heuristic-only candidate.
	SnapActionPassthrough SnapAction = "passthrough"
)

// Snap-pipeline thresholds (kept as named consts so the corpus tests can
// reference them and reviewers can see the ground rules in one place).
const (
	snapFullCoverage    = 0.70 // Stage 1 auto-snap floor
	snapPartialCoverage = 0.30 // below this no Stage-1 / Stage-2 consideration
	snapHausdorffMaxM   = 25.0 // segment-matching-style shape safety cap

	// Stage 2 cable-hold detector signals (all three must be true to merge).
	cableHoldEndDriftM     = 15.0 // signal 1: end-coord ↔ next-start-coord
	cableHoldSampleDriftM  = 10.0 // signal 2: gap-sample-pattern anchor radius
	cableHoldBearingMaxDeg = 90.0 // signal 3: no direction reversal
)

// OSMLiftCandidate is one OSM lift in the activity's PostGIS neighbourhood,
// already enriched with segment ids and length/bearing. It mirrors the SQL
// `osmCandidatesSQL` in lift_enrich.go (legacy Method A) plus the segment
// version lookup. Length is in metres, bearing in degrees [0, 360).
type OSMLiftCandidate struct {
	OSMID            int64
	SegmentID        string
	SegmentVersionID string
	Name             string
	LengthM          float64
	BearingDeg       float64
}

// RideOSMOverlap is one (ride, lift) match the SQL pre-filter produced —
// maximum deviation of the densified ride line from the lift geometry plus
// the length of the intersection between the ride buffer and the lift
// geometry. Coverage is intersection/lift-length.
type RideOSMOverlap struct {
	RideIdx         int     // index into rides slice
	OSMIdx          int     // index into osmCandidates slice
	HausdorffM      float64 // maximum ride-line deviation from lift, metres
	IntersectionLen float64 // metres of lift covered by the ride buffer
}

// Stage3PlanEntry is one OSM lift the consumer must process via the legacy
// per-lift time-window SQL (extract samples within the shape corridor, derive
// duration/ascent/bearing, INSERT one segment_efforts + one audit
// activity_lift_candidates row). Emitted only for lifts no Stage-1 or
// Stage-2 ride snapped to.
type Stage3PlanEntry struct {
	OSMCandidateIdx    int
	IsLiftSegmentMatch bool // always true (Stage 3 always produces a snap-equivalent row)
	IsMethodAOnly      bool // always true (this is the heuristic-miss safety net)
}

// SnapResult is the per-ride decision produced by SnapToOSMLifts and refined
// by MergeMultiRideForOSM. Stage 3 produces additional Stage3Result rows for
// OSM lifts that no ride covered.
type SnapResult struct {
	RideIdx int
	Action  SnapAction
	// OSMCandidateIdx links the ride to an OSM lift in the activity's
	// neighbourhood. Set whenever the ride physically corresponds to an OSM
	// lift, even when the per-ride coverage was too low for a Stage-1 snap or
	// the Stage-2 merge collapsed to passthrough — Stage 3 uses this value to
	// avoid re-planning a lift the ride already touched. -1 means "no OSM
	// relation"; >= 0 indexes osmCandidates.
	OSMCandidateIdx  int
	SegmentID        string // valid when Action == SnapActionSnap (after merge)
	SegmentVersionID string
	IsReverse        bool
	IsCableHoldMerge bool // true when emitted by Stage 2 multi-ride merge
	IsMethodAOnly    bool // false in Stages 1/2; only Stage 3 sets it
	// MergedRideIndices, when set on the canonical row of a Stage-2 merge,
	// lists every ride that contributed (including RideIdx itself).
	MergedRideIndices []int
	// SuppressedByMerge marks the non-canonical members of a merge group;
	// they should produce only an audit-only candidate row pointing at the
	// canonical merge's segment_effort_id.
	SuppressedByMerge bool
	// BlocksStage3 marks an OSM lift that Stage 2 explicitly rejected as
	// mid-station-like or under-covered. Retained for documentation purposes;
	// MethodACoverageGap now derives Stage-3 coverage from OSMCandidateIdx >= 0
	// directly, so any ride with a known OSM relation blocks Stage 3 regardless
	// of this flag.
	BlocksStage3 bool
}

// SnapToOSMLifts is Stage 1: for each heuristic ride, find the highest-
// coverage OSM lift candidate that passes the hausdorff cap. >= 0.70 →
// SnapActionSnap. 0.30..0.70 with a sibling ride on the same OSM lift →
// SnapActionMultiRideGroup (Stage 2 makes the call). Otherwise →
// SnapActionPassthrough.
//
// `samples` and `rideBearings` are accepted now so MergeMultiRideForOSM
// can re-use the same pipeline state without an extra plumbing layer; Stage 1
// itself only needs them for the per-ride bearing → IsReverse calculation.
func SnapToOSMLifts(
	rides []RideWithIndices,
	osmCandidates []OSMLiftCandidate,
	overlaps []RideOSMOverlap,
	samples []StreamPoint,
	_unused []float64, // reserved for future use; keeps the signature stable for Stage 2
) []SnapResult {
	// Index overlaps by ride so we can scan them per-ride O(1).
	byRide := make(map[int][]RideOSMOverlap, len(rides))
	for _, ov := range overlaps {
		byRide[ov.RideIdx] = append(byRide[ov.RideIdx], ov)
	}
	// Index by OSM lift to detect "another ride hits the same OSM" sibling.
	ridesPerOSM := make(map[int]int, len(osmCandidates))
	for _, ov := range overlaps {
		if ov.HausdorffM > snapHausdorffMaxM {
			continue
		}
		c := osmCandidates[ov.OSMIdx]
		if c.LengthM <= 0 {
			continue
		}
		if ov.IntersectionLen/c.LengthM < snapPartialCoverage {
			continue
		}
		ridesPerOSM[ov.OSMIdx]++
	}

	out := make([]SnapResult, 0, len(rides))
	for i := range rides {
		out = append(out, snapOneRide(i, rides[i], byRide[i], osmCandidates, ridesPerOSM, samples))
	}
	return out
}

func snapOneRide(
	rideIdx int,
	ride RideWithIndices,
	overlaps []RideOSMOverlap,
	osmCandidates []OSMLiftCandidate,
	ridesPerOSM map[int]int,
	samples []StreamPoint,
) SnapResult {
	// Default OSMCandidateIdx = -1 means "no OSM relation". Any ride that
	// reaches at least snapPartialCoverage on an OSM lift overwrites this
	// with the real index, so MethodACoverageGap can recognise the lift as
	// covered even when the per-ride coverage was too low for a Stage-1 snap.
	res := SnapResult{RideIdx: rideIdx, Action: SnapActionPassthrough, OSMCandidateIdx: -1}

	bestIdx := -1
	bestCoverage := 0.0
	bestHausdorff := 0.0
	for _, ov := range overlaps {
		if ov.HausdorffM > snapHausdorffMaxM {
			continue
		}
		c := osmCandidates[ov.OSMIdx]
		if c.LengthM <= 0 {
			continue
		}
		coverage := ov.IntersectionLen / c.LengthM
		if coverage < snapPartialCoverage {
			continue
		}
		if coverage > bestCoverage {
			bestCoverage = coverage
			bestIdx = ov.OSMIdx
			bestHausdorff = ov.HausdorffM
		}
	}
	if bestIdx == -1 {
		return res
	}
	_ = bestHausdorff // reserved for future scoring tweaks

	c := osmCandidates[bestIdx]
	if bestCoverage >= snapFullCoverage {
		res.Action = SnapActionSnap
		res.OSMCandidateIdx = bestIdx
		res.SegmentID = c.SegmentID
		res.SegmentVersionID = c.SegmentVersionID
		res.IsReverse = computeIsReverseFromSamples(samples, ride, c.BearingDeg)
		return res
	}
	// 0.30 <= bestCoverage < 0.70 — defer to Stage 2 only if a sibling
	// ride covers the same OSM lift; otherwise stay passthrough but record
	// the OSM relation so Stage 3 does not re-plan the same lift (Phase-4
	// critic finding F1: passthrough+Stage-3 double-count on partial lone
	// rides).
	if ridesPerOSM[bestIdx] >= 2 {
		res.Action = SnapActionMultiRideGroup
		res.OSMCandidateIdx = bestIdx
		return res
	}
	res.OSMCandidateIdx = bestIdx
	return res
}

// MergeMultiRideForOSM is Stage 2: among the rides Stage 1 marked as
// SnapActionMultiRideGroup, group those that target the same OSM lift and
// decide per-group whether to merge (cable-hold) or split (mid-station).
//
// Cable-hold signals (all three must hold):
//  1. Inter-ride distance: haversine(R_i.endCoord, R_{i+1}.startCoord) <= 15m
//  2. Gap-sample-pattern: the samples between R_i.endIdx+1 and
//     R_{i+1}.startIdx-1 are either absent (smart-recording paused) or all
//     <= 10m from the anchor R_i.endCoord (continuous-stream cable hold).
//  3. No direction reversal: |bearing(R_i) - bearing(R_{i+1})| <= 90°.
//
// And union-coverage of the merged ride must reach >= 0.70 of the OSM lift.
//
// When all conditions hold the canonical (earliest) ride is converted to
// Action=Snap with IsCableHoldMerge=true and MergedRideIndices listing every
// participant; the rest are marked SuppressedByMerge=true so the writer
// emits audit-only candidates pointing at the canonical's segment_effort_id.
//
// When any condition fails the rides fall back to Action=Passthrough — the
// caller writes them as heuristic-only candidates without OSM linkage.
//
// Stage 1 results that are not SnapActionMultiRideGroup pass through
// unchanged, so the function is idempotent for the snap and passthrough
// paths.
func MergeMultiRideForOSM(
	stage1 []SnapResult,
	rides []RideWithIndices,
	osmCandidates []OSMLiftCandidate,
	overlaps []RideOSMOverlap,
	samples []StreamPoint,
) []SnapResult {
	out := make([]SnapResult, len(stage1))
	copy(out, stage1)

	// Group MultiRideGroup ride-indices by OSMCandidateIdx, in the order
	// they appear in `rides` (= chronological order, since the heuristic
	// detector emits rides sorted by start time).
	groupOrder := []int{}
	groups := map[int][]int{}
	for i, r := range stage1 {
		if r.Action != SnapActionMultiRideGroup {
			continue
		}
		if _, seen := groups[r.OSMCandidateIdx]; !seen {
			groupOrder = append(groupOrder, r.OSMCandidateIdx)
		}
		groups[r.OSMCandidateIdx] = append(groups[r.OSMCandidateIdx], i)
	}

	// Build per-(ride,osm) intersection lookup for the union-coverage check.
	intersection := map[[2]int]float64{}
	for _, ov := range overlaps {
		intersection[[2]int{ov.RideIdx, ov.OSMIdx}] = ov.IntersectionLen
	}

	for _, osmIdx := range groupOrder {
		members := groups[osmIdx]
		if len(members) < 2 {
			// Defensive: a "group" with 1 member shouldn't have been
			// MultiRideGroup-tagged in Stage 1, but if a future change ever
			// produces this state we must (a) keep the consumer's heuristic-
			// only insert path working — done by the SnapActionMultiRideGroup
			// case in lift_enrich.go — and (b) prevent Stage 3 from re-planning
			// the same OSM lift, which the OSMCandidateIdx >= 0 check in
			// MethodACoverageGap now covers automatically. We additionally set
			// BlocksStage3 to make the intent self-documenting (Phase-4 critic
			// finding F39).
			lone := members[0]
			out[lone].BlocksStage3 = true
			continue
		}
		osm := osmCandidates[osmIdx]

		// Each adjacent pair must satisfy all three signals. The sample-
		// iteration in cableHoldSignalsHold needs the full rides slice to skip
		// foreign-ride samples that chronologically straddle the gap — see
		// Phase-4 critic finding F26.
		allCableHold := true
		for i := 0; i < len(members)-1; i++ {
			ra := rides[members[i]]
			rb := rides[members[i+1]]
			if !cableHoldSignalsHold(ra, rb, rides, samples) {
				allCableHold = false
				break
			}
		}
		// Union-coverage: sum of per-ride intersections / lift length, capped
		// at 1.0 (intersections may overlap in extreme synthetic cases).
		unionLen := 0.0
		for _, m := range members {
			unionLen += intersection[[2]int{m, osmIdx}]
		}
		coverage := 0.0
		if osm.LengthM > 0 {
			coverage = unionLen / osm.LengthM
			if coverage > 1.0 {
				coverage = 1.0
			}
		}
		if !allCableHold || coverage < snapFullCoverage {
			// Mid-station-like or under-covered → all rides become heuristic-only.
			for _, m := range members {
				out[m].Action = SnapActionPassthrough
				out[m].OSMCandidateIdx = osmIdx
				out[m].SegmentID = ""
				out[m].SegmentVersionID = ""
				out[m].BlocksStage3 = true
			}
			continue
		}
		// Merge — canonical = first ride, rest suppressed.
		canonical := members[0]
		out[canonical] = SnapResult{
			RideIdx:           canonical,
			Action:            SnapActionSnap,
			OSMCandidateIdx:   osmIdx,
			SegmentID:         osm.SegmentID,
			SegmentVersionID:  osm.SegmentVersionID,
			IsReverse:         computeIsReverseFromSamples(samples, rides[canonical], osm.BearingDeg),
			IsCableHoldMerge:  true,
			MergedRideIndices: append([]int(nil), members...),
		}
		for _, m := range members[1:] {
			// MergedRideIndices on a suppressed member is a single-element
			// slice pointing at the canonical ride. The persistence loop in
			// lift_enrich.go uses res.MergedRideIndices[0] to look up the
			// canonical Stage-2 segment_effort by (activity_id, started_at);
			// without this slot the loop panics index-out-of-range before
			// the audit row can be written. (Phase-4 critic finding F1.)
			out[m] = SnapResult{
				RideIdx:           m,
				Action:            SnapActionSnap,
				OSMCandidateIdx:   osmIdx,
				SegmentID:         osm.SegmentID,
				SegmentVersionID:  osm.SegmentVersionID,
				IsReverse:         out[canonical].IsReverse,
				IsCableHoldMerge:  true,
				SuppressedByMerge: true,
				MergedRideIndices: []int{canonical},
			}
		}
	}
	return out
}

// cableHoldSignalsHold returns true when all three Stage-2 cable-hold signals
// are positive between the chronologically adjacent rides ra (earlier) and
// rb (later). When samples are missing (caller passed nil) the function is
// conservative and returns false — the merge decision falls back to
// "treat as mid-station".
//
// allRides is the full ride slice supplied to MergeMultiRideForOSM. The
// gap-sample-pattern check uses it to skip samples that belong to foreign
// rides (a different OSM lift recorded chronologically between ra and rb)
// rather than treating them as cable-hold-anchor failures — see Phase-4
// critic finding F26.
func cableHoldSignalsHold(ra, rb RideWithIndices, allRides []RideWithIndices, samples []StreamPoint) bool {
	if len(samples) == 0 {
		return false
	}
	// Symmetric bounds: every index we dereference (start + end on both
	// rides) must be in range, and start <= end. Phase-4 critic finding F19:
	// the previous code only validated half of the indices and would panic
	// on malformed inputs.
	if !validRideIndices(ra, samples) || !validRideIndices(rb, samples) {
		return false
	}
	endA := samples[ra.EndSampleIdx]
	startB := samples[rb.StartSampleIdx]
	if endA.Lat == nil || endA.Lng == nil || startB.Lat == nil || startB.Lng == nil {
		return false
	}

	// Signal 1: inter-ride distance.
	if Haversine(*endA.Lat, *endA.Lng, *startB.Lat, *startB.Lng) > cableHoldEndDriftM {
		return false
	}

	// Signal 2: gap-sample-pattern. Either no samples in [endA+1, startB-1]
	// (smart-recording pause) or every gap sample within cableHoldSampleDriftM
	// of the anchor endA. Samples that belong to a foreign ride (any ride
	// other than ra/rb) are skipped — they describe a different physical
	// movement and would otherwise force a false cable-hold rejection.
	for k := ra.EndSampleIdx + 1; k < rb.StartSampleIdx; k++ {
		if sampleBelongsToForeignRide(k, ra, rb, allRides) {
			continue
		}
		s := samples[k]
		if s.Lat == nil || s.Lng == nil {
			continue // gap-style nil treats as silence
		}
		if Haversine(*endA.Lat, *endA.Lng, *s.Lat, *s.Lng) > cableHoldSampleDriftM {
			return false
		}
	}

	// Signal 3: bearing must not reverse.
	startA := samples[ra.StartSampleIdx]
	endB := samples[rb.EndSampleIdx]
	if startA.Lat == nil || startA.Lng == nil || endB.Lat == nil || endB.Lng == nil {
		return false
	}
	bearA := BearingDeg(*startA.Lat, *startA.Lng, *endA.Lat, *endA.Lng)
	bearB := BearingDeg(*startB.Lat, *startB.Lng, *endB.Lat, *endB.Lng)
	diff := bearA - bearB
	if diff < 0 {
		diff = -diff
	}
	for diff > 360 {
		diff -= 360
	}
	if diff > 180 {
		diff = 360 - diff
	}
	return diff <= cableHoldBearingMaxDeg
}

// validRideIndices is the symmetric bounds check for both endpoints of one
// ride against the samples slice. Used by cableHoldSignalsHold and
// computeIsReverseFromSamples — Phase-4 critic finding F19.
func validRideIndices(r RideWithIndices, samples []StreamPoint) bool {
	if r.StartSampleIdx < 0 || r.StartSampleIdx >= len(samples) {
		return false
	}
	if r.EndSampleIdx < 0 || r.EndSampleIdx >= len(samples) {
		return false
	}
	if r.StartSampleIdx > r.EndSampleIdx {
		return false
	}
	return true
}

// sampleBelongsToForeignRide reports whether sample index k sits inside the
// [StartSampleIdx, EndSampleIdx] interval of some ride other than ra or rb.
// Used by the Stage-2 cable-hold sample-iteration to avoid penalising the
// merge for unrelated rides that chronologically straddle the gap.
func sampleBelongsToForeignRide(k int, ra, rb RideWithIndices, allRides []RideWithIndices) bool {
	for _, other := range allRides {
		if other.StartSampleIdx == ra.StartSampleIdx && other.EndSampleIdx == ra.EndSampleIdx {
			continue
		}
		if other.StartSampleIdx == rb.StartSampleIdx && other.EndSampleIdx == rb.EndSampleIdx {
			continue
		}
		if k >= other.StartSampleIdx && k <= other.EndSampleIdx {
			return true
		}
	}
	return false
}

// computeIsReverseFromSamples derives the ride's bearing from its first/last
// stream sample and compares to the OSM lift bearing. When samples are
// unavailable (caller passed nil) the function returns false (assume same
// direction). Mirrors the IsReverseHeading rule in osm.go.
//
// The bounds check is symmetric (both endpoints validated independently) —
// Phase-4 critic finding F19. The previous asymmetric check could panic on
// malformed indices like (StartSampleIdx >= len(samples), EndSampleIdx < len).
func computeIsReverseFromSamples(samples []StreamPoint, ride RideWithIndices, liftBearingDeg float64) bool {
	if len(samples) == 0 {
		return false
	}
	if !validRideIndices(ride, samples) {
		return false
	}
	s := samples[ride.StartSampleIdx]
	e := samples[ride.EndSampleIdx]
	if s.Lat == nil || s.Lng == nil || e.Lat == nil || e.Lng == nil {
		return false
	}
	rideBearing := BearingDeg(*s.Lat, *s.Lng, *e.Lat, *e.Lng)
	return IsReverseHeading(rideBearing, liftBearingDeg)
}

// MethodACoverageGapForAllOSM is the pure-OSM variant of Stage 3: it emits
// one Stage3PlanEntry for every OSM candidate unconditionally (no covered-map
// filter). Used when the heuristic detector is disabled — rides is nil and
// the snap pipeline emits nothing, so every OSM lift must be probed.
//
// The shape-distance / ≥2 samples / ≥30 s window gates in the enrichment
// writer still apply; lifts with insufficient sample coverage or mismatching
// shape are silently dropped there, not here.
func MethodACoverageGapForAllOSM(osmCandidates []OSMLiftCandidate) []Stage3PlanEntry {
	plan := make([]Stage3PlanEntry, 0, len(osmCandidates))
	for i := range osmCandidates {
		plan = append(plan, Stage3PlanEntry{
			OSMCandidateIdx:    i,
			IsLiftSegmentMatch: true,
			IsMethodAOnly:      true,
		})
	}
	return plan
}

// MethodACoverageGap is Stage 3: for every OSM lift that no Stage-1 or
// Stage-2 ride physically corresponds to, emit a Stage3PlanEntry. The caller
// is responsible for running the per-lift time-window SQL and inserting one
// segment_efforts row plus one review-required activity_lift_candidates row
// (source='osm', confirmed=false).
//
// "Physically corresponds" means OSMCandidateIdx >= 0 — any value the snap
// pipeline assigned, regardless of whether the ride ended up as Snap, a
// passthrough with partial OSM coverage, a Stage-2 mid-station rejection, or
// a defensive single-member multi-ride group. The earlier "Action == Snap or
// BlocksStage3" rule double-counted the lone passthrough with partial
// coverage case (Phase-4 critic finding F1) and the defensive single-member
// merge case (Phase-4 critic finding F39): the heuristic detector emitted a
// candidate row AND Stage 3 added an OSM-side segment_effort for the same
// physical ride, so recomputeLiftStats summed both. Switching to the
// OSMCandidateIdx-derived covered set eliminates the duplication while still
// giving Stage 3 the chance to fill in OSM lifts no ride ever touched.
//
// Min-duration enforcement (the legacy 30s floor for very short lifts that
// are below the heuristic 60s minimum) lives in the time-window SQL the
// caller runs, not in this planning function — Stage 3 only decides which
// lifts to process.
func MethodACoverageGap(finalResults []SnapResult, osmCandidates []OSMLiftCandidate) []Stage3PlanEntry {
	covered := make(map[int]bool, len(osmCandidates))
	for _, r := range finalResults {
		if r.OSMCandidateIdx < 0 {
			continue
		}
		if r.OSMCandidateIdx >= len(osmCandidates) {
			// Defensive — should never happen since the snap pipeline only
			// stores indices it produced. Skip rather than panic.
			continue
		}
		covered[r.OSMCandidateIdx] = true
	}
	plan := make([]Stage3PlanEntry, 0, len(osmCandidates))
	for i := range osmCandidates {
		if covered[i] {
			continue
		}
		plan = append(plan, Stage3PlanEntry{
			OSMCandidateIdx:    i,
			IsLiftSegmentMatch: true,
			IsMethodAOnly:      true,
		})
	}
	return plan
}
