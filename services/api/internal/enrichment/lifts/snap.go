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
