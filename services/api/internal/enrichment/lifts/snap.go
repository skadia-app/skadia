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
	snapHausdorffMaxM   = 50.0 // parallel-lift safety cap

	// Stage 2 cable-hold detector signals (all three must be true to merge).
	cableHoldEndDriftM     = 15.0 // signal 1: end-coord ↔ next-start-coord
	cableHoldSampleDriftM  = 10.0 // signal 2: gap-sample-pattern anchor radius
	cableHoldBearingMaxDeg = 90.0 // signal 3: no direction reversal
)

// osmLiftCandidate is one OSM lift in the activity's PostGIS neighbourhood,
// already enriched with segment ids and length/bearing. It mirrors the SQL
// `osmCandidatesSQL` in lift_enrich.go (legacy Method A) plus the segment
// version lookup. Length is in metres, bearing in degrees [0, 360).
type osmLiftCandidate struct {
	OSMID            int64
	SegmentID        string
	SegmentVersionID string
	Name             string
	LengthM          float64
	BearingDeg       float64
}

// rideOSMOverlap is one (ride, lift) match the SQL pre-filter produced —
// hausdorff distance and the length of the intersection between the ride
// buffer and the lift geometry. Coverage is intersection/lift-length.
type rideOSMOverlap struct {
	RideIdx         int     // index into rides slice
	OSMIdx          int     // index into osmCandidates slice
	HausdorffM      float64 // ride-buffer ↔ lift hausdorff
	IntersectionLen float64 // metres of lift covered by the ride buffer
}

// SnapResult is the per-ride decision produced by SnapToOSMLifts and refined
// by MergeMultiRideForOSM. Stage 3 produces additional Stage3Result rows for
// OSM lifts that no ride covered.
type SnapResult struct {
	RideIdx          int
	Action           SnapAction
	OSMCandidateIdx  int    // index into osmCandidates; valid for snap + multi_ride_group
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
	osmCandidates []osmLiftCandidate,
	overlaps []rideOSMOverlap,
	samples []StreamPoint,
	_unused []float64, // reserved for future use; keeps the signature stable for Stage 2
) []SnapResult {
	// Index overlaps by ride so we can scan them per-ride O(1).
	byRide := make(map[int][]rideOSMOverlap, len(rides))
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
	overlaps []rideOSMOverlap,
	osmCandidates []osmLiftCandidate,
	ridesPerOSM map[int]int,
	samples []StreamPoint,
) SnapResult {
	res := SnapResult{RideIdx: rideIdx, Action: SnapActionPassthrough}

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
	// ride covers the same OSM lift; otherwise pass through unchanged.
	if ridesPerOSM[bestIdx] >= 2 {
		res.Action = SnapActionMultiRideGroup
		res.OSMCandidateIdx = bestIdx
		return res
	}
	return res
}

// MergeMultiRideForOSM is Stage 2: among the rides Stage 1 marked as
// SnapActionMultiRideGroup, group those that target the same OSM lift and
// decide per-group whether to merge (cable-hold) or split (mid-station).
//
// Cable-hold signals (all three must hold):
//   1. Inter-ride distance: haversine(R_i.endCoord, R_{i+1}.startCoord) <= 15m
//   2. Gap-sample-pattern: the samples between R_i.endIdx+1 and
//      R_{i+1}.startIdx-1 are either absent (smart-recording paused) or all
//      <= 10m from the anchor R_i.endCoord (continuous-stream cable hold).
//   3. No direction reversal: |bearing(R_i) - bearing(R_{i+1})| <= 90°.
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
	osmCandidates []osmLiftCandidate,
	overlaps []rideOSMOverlap,
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
			// MultiRideGroup-tagged. Pass through unchanged.
			continue
		}
		osm := osmCandidates[osmIdx]

		// Each adjacent pair must satisfy all three signals.
		allCableHold := true
		for i := 0; i < len(members)-1; i++ {
			ra := rides[members[i]]
			rb := rides[members[i+1]]
			if !cableHoldSignalsHold(ra, rb, samples) {
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
				out[m].OSMCandidateIdx = 0
				out[m].SegmentID = ""
				out[m].SegmentVersionID = ""
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
			out[m] = SnapResult{
				RideIdx:           m,
				Action:            SnapActionSnap,
				OSMCandidateIdx:   osmIdx,
				SegmentID:         osm.SegmentID,
				SegmentVersionID:  osm.SegmentVersionID,
				IsReverse:         out[canonical].IsReverse,
				IsCableHoldMerge:  true,
				SuppressedByMerge: true,
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
func cableHoldSignalsHold(ra, rb RideWithIndices, samples []StreamPoint) bool {
	if len(samples) == 0 {
		return false
	}
	if ra.EndSampleIdx < 0 || ra.EndSampleIdx >= len(samples) {
		return false
	}
	if rb.StartSampleIdx < 0 || rb.StartSampleIdx >= len(samples) {
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
	// of the anchor endA.
	for k := ra.EndSampleIdx + 1; k < rb.StartSampleIdx; k++ {
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

// computeIsReverseFromSamples derives the ride's bearing from its first/last
// stream sample and compares to the OSM lift bearing. When samples are
// unavailable (caller passed nil) the function returns false (assume same
// direction). Mirrors the IsReverseHeading rule in osm.go.
func computeIsReverseFromSamples(samples []StreamPoint, ride RideWithIndices, liftBearingDeg float64) bool {
	if len(samples) == 0 {
		return false
	}
	if ride.StartSampleIdx < 0 || ride.EndSampleIdx >= len(samples) {
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
