package main

// snap_stats.go runs the OSM-snap pipeline (SnapToOSMLifts → MergeMultiRideForOSM
// → MethodACoverageGap) against real activity files plus a live osm_lifts table
// to produce empirical evidence for the F1-coverage-threshold question.
//
// Privacy: the aggregation never emits raw GPS coordinates, no specific lift
// names, and no local filenames.
// Region labels in the report are derived from the leading numeric prefix of
// the case-file's top-level directory (e.g. `7_mtb-park case/foo.fit` → `region_7`)
// or fall back to a stable hash; so the same case maps to the same region
// label across runs without leaking the directory's German name.

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

// coverageBuckets defines the histogram boundaries used in the per-region
// coverage_distribution. The bucket [lo, hi) holds rides whose best-osm
// IntersectionLen / OSMLength falls in that range. The 0.30 boundary is the
// current snapPartialCoverage threshold; the 0.70 boundary is snapFullCoverage.
var coverageBuckets = []float64{0.0, 0.10, 0.20, 0.30, 0.50, 0.70, 1.0}

// SnapStats accumulates per-region (and grand-total) decision counts plus the
// coverage histogram. The struct is JSON-serialisable for `--snap-stats-json`.
type SnapStats struct {
	Region                    string  `json:"region"`
	Files                     int     `json:"files"`
	RidesTotal                int     `json:"rides_total"`
	ActionSnap                int     `json:"action_snap"`
	ActionMultiRideGroup      int     `json:"action_multi_ride_group"`
	ActionPassthroughNoOSM    int     `json:"action_passthrough_no_overlap"`
	ActionPassthroughPartial  int     `json:"action_passthrough_partial_overlap"`
	SuppressedByMerge         int     `json:"suppressed_by_merge"`
	Stage3Planned             int     `json:"stage3_planned"`
	OSMCandidatesScanned      int     `json:"osm_candidates_scanned"`
	F1RiskCount               int     `json:"f1_risk_count"`
	F1RiskExamples            []F1Risk `json:"f1_risk_examples,omitempty"`
	CoverageHistogramKeys     []string `json:"coverage_histogram_keys"`
	CoverageHistogramCounts   []int    `json:"coverage_histogram_counts"`
}

// F1Risk records one passthrough-with-partial-overlap ride. These are the
// cases the F1 fix targets: the heuristic produced a candidate AND the OSM
// lift it overlaps would otherwise have been planned by Stage 3 (= double
// count). With the fix, OSMCandidateIdx is set on the SnapResult, so
// MethodACoverageGap excludes the lift. We record the (region, way_id, coverage)
// triplet so the threshold question has concrete data points.
//
// way_id is a public OSM identifier; recording it is privacy-safe.
type F1Risk struct {
	Region   string  `json:"region"`
	WayID    int64   `json:"way_id"`
	Coverage float64 `json:"coverage"`
}

// runSnapStats is the --snap-stats entry point. It walks the manifest, parses
// each FIT/GPX, runs DetectHeuristicWithIndices, queries osm_lifts for the
// activity bbox, runs the snap pipeline, and aggregates per-region + grand-
// total. When jsonOut != "" the result is written as JSON; otherwise a
// plain-text table is emitted to stdout.
func runSnapStats(ctx context.Context, gt *GroundTruth, root, dbURL string, jsonOut io.Writer, w io.Writer) error {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("snap-stats: connect db: %w", err)
	}
	defer pool.Close()

	regions := map[string]*SnapStats{}
	totals := newSnapStats("__total__")

	for _, c := range gt.Cases {
		full := filepath.Join(root, c.File)
		regionLabel := regionLabelFromCaseFile(c.File)
		st := regions[regionLabel]
		if st == nil {
			st = newSnapStats(regionLabel)
			regions[regionLabel] = st
		}
		st.Files++
		totals.Files++

		var pts []lifts.StreamPoint
		var err error
		switch strings.ToLower(filepath.Ext(c.File)) {
		case ".fit":
			pts, err = loadFITPoints(full)
		case ".gpx":
			pts, err = loadGPXPoints(full)
		default:
			err = fmt.Errorf("unsupported extension")
		}
		if err != nil {
			fmt.Fprintf(w, "ERR  %-50s  %v\n", c.File, err)
			continue
		}
		rides := lifts.DetectHeuristicWithIndices(pts, lifts.DefaultDetectorConfig(c.Sport))
		if len(rides) == 0 {
			continue
		}

		osmCands, err := loadCandidatesForBBox(ctx, pool, pts)
		if err != nil {
			fmt.Fprintf(w, "ERR  %-50s  load candidates: %v\n", c.File, err)
			continue
		}
		st.OSMCandidatesScanned += len(osmCands)
		totals.OSMCandidatesScanned += len(osmCands)

		overlaps, err := computeOverlapsLocal(ctx, pool, pts, rides, osmCands)
		if err != nil {
			fmt.Fprintf(w, "ERR  %-50s  compute overlaps: %v\n", c.File, err)
			continue
		}

		stage1 := lifts.SnapToOSMLifts(rides, osmCands, overlaps, pts, nil)
		stage2 := lifts.MergeMultiRideForOSM(stage1, rides, osmCands, overlaps, pts)
		stage3 := lifts.MethodACoverageGap(stage2, osmCands)

		// Build per-ride best coverage so we can both classify
		// passthrough_no_overlap vs passthrough_partial_overlap and bucket
		// the histogram. Best = highest IntersectionLen / OSMLength among
		// overlaps with HausdorffM <= snapHausdorffMaxM. We cap at 1.0.
		bestCov, bestOSM := bestCoveragePerRide(rides, osmCands, overlaps)

		// Aggregate per-action.
		for _, r := range stage2 {
			st.RidesTotal++
			totals.RidesTotal++
			switch r.Action {
			case lifts.SnapActionSnap:
				st.ActionSnap++
				totals.ActionSnap++
				if r.SuppressedByMerge {
					st.SuppressedByMerge++
					totals.SuppressedByMerge++
				}
			case lifts.SnapActionMultiRideGroup:
				st.ActionMultiRideGroup++
				totals.ActionMultiRideGroup++
			case lifts.SnapActionPassthrough:
				if r.OSMCandidateIdx >= 0 {
					st.ActionPassthroughPartial++
					totals.ActionPassthroughPartial++
					// F1-risk = passthrough with OSM relation. Pre-fix this
					// would have led to a Stage-3 double count; with the fix
					// MethodACoverageGap excludes the lift via OSMCandidateIdx.
					st.F1RiskCount++
					totals.F1RiskCount++
					cand := osmCands[r.OSMCandidateIdx]
					st.F1RiskExamples = appendF1Example(st.F1RiskExamples, F1Risk{
						Region:   regionLabel,
						WayID:    cand.OSMID,
						Coverage: bestCov[r.RideIdx],
					})
				} else {
					st.ActionPassthroughNoOSM++
					totals.ActionPassthroughNoOSM++
				}
			}
			// Histogram: bucket the best-coverage value the snap pipeline
			// considered for this ride. Rides with no in-range overlap fall
			// into the [0.0, 0.10) bucket. We do this once per ride.
			cov := bestCov[r.RideIdx]
			_ = bestOSM // reserved; kept for future per-bucket OSM debug
			incHistogram(st, cov)
			incHistogram(totals, cov)
		}

		st.Stage3Planned += len(stage3)
		totals.Stage3Planned += len(stage3)
	}

	if jsonOut != nil {
		// Stable region ordering for reproducible JSON.
		labels := make([]string, 0, len(regions))
		for k := range regions {
			labels = append(labels, k)
		}
		sort.Strings(labels)
		out := struct {
			Total   *SnapStats   `json:"total"`
			Regions []*SnapStats `json:"regions"`
		}{Total: totals}
		for _, l := range labels {
			out.Regions = append(out.Regions, regions[l])
		}
		enc := json.NewEncoder(jsonOut)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	printSnapStatsTable(w, totals, regions)
	return nil
}

func newSnapStats(label string) *SnapStats {
	keys := make([]string, len(coverageBuckets))
	counts := make([]int, len(coverageBuckets))
	for i, lo := range coverageBuckets {
		hi := 1.01
		if i+1 < len(coverageBuckets) {
			hi = coverageBuckets[i+1]
		}
		keys[i] = fmt.Sprintf("[%.2f,%.2f)", lo, hi)
	}
	return &SnapStats{
		Region:                  label,
		CoverageHistogramKeys:   keys,
		CoverageHistogramCounts: counts,
	}
}

// incHistogram increments the bucket whose [lo, hi) interval contains cov.
// cov < 0.0 is clamped to 0; cov >= 1.0 falls into the last bucket.
func incHistogram(st *SnapStats, cov float64) {
	if cov < 0 {
		cov = 0
	}
	for i := len(coverageBuckets) - 1; i >= 0; i-- {
		if cov >= coverageBuckets[i] {
			st.CoverageHistogramCounts[i]++
			return
		}
	}
	st.CoverageHistogramCounts[0]++
}

// bestCoveragePerRide returns, for each ride index, the highest
// IntersectionLen / LengthM ratio among overlaps that pass the hausdorff cap.
// Coverage is capped at 1.0. Rides without a passing overlap report 0.0 and
// bestOSM == -1.
func bestCoveragePerRide(rides []lifts.RideWithIndices, osmCands []lifts.OSMLiftCandidate, overlaps []lifts.RideOSMOverlap) (map[int]float64, map[int]int) {
	cov := make(map[int]float64, len(rides))
	osm := make(map[int]int, len(rides))
	for i := range rides {
		cov[i] = 0
		osm[i] = -1
	}
	for _, ov := range overlaps {
		if ov.HausdorffM > 50.0 {
			continue
		}
		c := osmCands[ov.OSMIdx]
		if c.LengthM <= 0 {
			continue
		}
		ratio := ov.IntersectionLen / c.LengthM
		if ratio > 1.0 {
			ratio = 1.0
		}
		if ratio > cov[ov.RideIdx] {
			cov[ov.RideIdx] = ratio
			osm[ov.RideIdx] = ov.OSMIdx
		}
	}
	return cov, osm
}

// loadCandidatesForBBox runs the activity-bbox query against osm_lifts and
// returns OSMLiftCandidates with empty SegmentID/SegmentVersionID (the eval
// path never persists, only classifies). The 0.005° buffer (~500m at
// mid-latitudes) matches the prompt; we accept the small longitudinal
// over-buffering north of 60° because the training corpus is alpine.
func loadCandidatesForBBox(ctx context.Context, pool *pgxpool.Pool, pts []lifts.StreamPoint) ([]lifts.OSMLiftCandidate, error) {
	minLat, minLon, maxLat, maxLon, ok := bbox(pts)
	if !ok {
		return nil, nil
	}
	const query = `
		SELECT
			l.osm_id,
			COALESCE(l.name, '') AS name,
			ST_Length(l.geom::geography) AS length_m,
			degrees(ST_Azimuth(ST_StartPoint(l.geom), ST_EndPoint(l.geom))) AS bearing_deg
		FROM osm_lifts l
		WHERE l.geom && ST_Expand(ST_MakeEnvelope($1, $2, $3, $4, 4326), 0.005)
		ORDER BY l.osm_id
	`
	rows, err := pool.Query(ctx, query, minLon, minLat, maxLon, maxLat)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []lifts.OSMLiftCandidate{}
	for rows.Next() {
		var c lifts.OSMLiftCandidate
		if err := rows.Scan(&c.OSMID, &c.Name, &c.LengthM, &c.BearingDeg); err != nil {
			return nil, err
		}
		// SegmentID / SegmentVersionID intentionally left blank — the eval
		// pipeline never inserts segment_efforts, only classifies actions.
		out = append(out, c)
	}
	return out, rows.Err()
}

// computeOverlapsLocal mirrors enrichment.computeRideOSMOverlaps: per ride,
// build a LineString WKT and ask PostGIS for hausdorff + intersection length
// against every osm candidate. Kept inline here to avoid pulling the whole
// enrichment package (and its database.DB dependency tree) into the eval CLI.
func computeOverlapsLocal(ctx context.Context, pool *pgxpool.Pool, stream []lifts.StreamPoint, rides []lifts.RideWithIndices, osmCands []lifts.OSMLiftCandidate) ([]lifts.RideOSMOverlap, error) {
	if len(rides) == 0 || len(osmCands) == 0 {
		return nil, nil
	}
	osmIDs := make([]int64, len(osmCands))
	for i, c := range osmCands {
		osmIDs[i] = c.OSMID
	}
	osmIdxByID := make(map[int64]int, len(osmCands))
	for i, c := range osmCands {
		osmIdxByID[c.OSMID] = i
	}
	out := []lifts.RideOSMOverlap{}
	for ri, ride := range rides {
		wkt := rideWKTLocal(stream, ride)
		if wkt == "" {
			continue
		}
		const query = `
			WITH route AS (
				SELECT ST_GeomFromText($1, 4326) AS geom
			)
			SELECT
				l.osm_id,
				ST_HausdorffDistance(
					ST_Intersection(
						ST_Buffer(route.geom::geography, 30)::geometry,
						l.geom
					),
					l.geom, 0.1
				) AS hausdorff_m,
				ST_Length(
					ST_Intersection(
						ST_Buffer(route.geom::geography, 30)::geometry,
						l.geom
					)::geography
				) AS intersection_len_m
			FROM osm_lifts l, route
			WHERE l.osm_id = ANY($2)
		`
		rows, err := pool.Query(ctx, query, wkt, osmIDs)
		if err != nil {
			return nil, fmt.Errorf("ride %d overlap query: %w", ri, err)
		}
		for rows.Next() {
			var osmID int64
			var hausdorff, isectLen *float64
			if err := rows.Scan(&osmID, &hausdorff, &isectLen); err != nil {
				rows.Close()
				return nil, err
			}
			if hausdorff == nil || isectLen == nil {
				continue
			}
			osmIdx, ok := osmIdxByID[osmID]
			if !ok {
				continue
			}
			out = append(out, lifts.RideOSMOverlap{
				RideIdx:         ri,
				OSMIdx:          osmIdx,
				HausdorffM:      *hausdorff,
				IntersectionLen: *isectLen,
			})
		}
		rows.Close()
	}
	return out, nil
}

func rideWKTLocal(stream []lifts.StreamPoint, ride lifts.RideWithIndices) string {
	if ride.StartSampleIdx < 0 || ride.EndSampleIdx >= len(stream) {
		return ""
	}
	parts := make([]string, 0, ride.EndSampleIdx-ride.StartSampleIdx+1)
	for k := ride.StartSampleIdx; k <= ride.EndSampleIdx; k++ {
		s := stream[k]
		if s.Lat == nil || s.Lng == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%.7f %.7f", *s.Lng, *s.Lat))
	}
	if len(parts) < 2 {
		return ""
	}
	return "LINESTRING(" + strings.Join(parts, ",") + ")"
}

func bbox(pts []lifts.StreamPoint) (minLat, minLon, maxLat, maxLon float64, ok bool) {
	first := true
	for _, p := range pts {
		if p.Lat == nil || p.Lng == nil {
			continue
		}
		if first {
			minLat, maxLat = *p.Lat, *p.Lat
			minLon, maxLon = *p.Lng, *p.Lng
			first = false
			continue
		}
		if *p.Lat < minLat {
			minLat = *p.Lat
		}
		if *p.Lat > maxLat {
			maxLat = *p.Lat
		}
		if *p.Lng < minLon {
			minLon = *p.Lng
		}
		if *p.Lng > maxLon {
			maxLon = *p.Lng
		}
	}
	return minLat, minLon, maxLat, maxLon, !first
}

// regionLabelFromCaseFile turns a manifest case-file path like
// "7_mtb-park case/foo.fit" into the privacy-safe label "region_7". When the
// leading directory has no leading digits (e.g. "NEGATIV/", "Ski/") we
// fall back to a stable 8-bit FNV hash so the same directory always maps
// to the same `region_h<NN>` label without leaking its real name.
func regionLabelFromCaseFile(caseFile string) string {
	parts := strings.SplitN(filepath.ToSlash(caseFile), "/", 2)
	top := parts[0]
	if m := leadingDigits.FindString(top); m != "" {
		return "region_" + m
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(top))
	return fmt.Sprintf("region_h%02d", int(h.Sum32()%100))
}

var leadingDigits = regexp.MustCompile(`^[0-9]+`)

// appendF1Example keeps at most 5 examples per region to bound stdout size.
func appendF1Example(s []F1Risk, e F1Risk) []F1Risk {
	if len(s) >= 5 {
		return s
	}
	return append(s, e)
}

// printSnapStatsTable renders a privacy-safe ASCII table to w.
func printSnapStatsTable(w io.Writer, totals *SnapStats, regions map[string]*SnapStats) {
	labels := make([]string, 0, len(regions))
	for k := range regions {
		labels = append(labels, k)
	}
	sort.Strings(labels)

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Snap-pipeline statistics (PRIVACY-SAFE — counts only, no GPS, hashed regions)")
	fmt.Fprintln(w, strings.Repeat("=", 110))
	fmt.Fprintf(w, "%-12s  %5s  %5s  %5s  %5s  %5s  %5s  %5s  %5s  %5s  %5s\n",
		"region", "files", "rides", "snap", "multi", "supp", "ptOSM", "ptNoO", "S3plan", "OSMs", "F1risk")
	for _, l := range labels {
		s := regions[l]
		fmt.Fprintf(w, "%-12s  %5d  %5d  %5d  %5d  %5d  %5d  %5d  %5d  %5d  %5d\n",
			s.Region, s.Files, s.RidesTotal,
			s.ActionSnap, s.ActionMultiRideGroup, s.SuppressedByMerge,
			s.ActionPassthroughPartial, s.ActionPassthroughNoOSM,
			s.Stage3Planned, s.OSMCandidatesScanned, s.F1RiskCount)
	}
	fmt.Fprintln(w, strings.Repeat("-", 110))
	fmt.Fprintf(w, "%-12s  %5d  %5d  %5d  %5d  %5d  %5d  %5d  %5d  %5d  %5d\n",
		"TOTAL", totals.Files, totals.RidesTotal,
		totals.ActionSnap, totals.ActionMultiRideGroup, totals.SuppressedByMerge,
		totals.ActionPassthroughPartial, totals.ActionPassthroughNoOSM,
		totals.Stage3Planned, totals.OSMCandidatesScanned, totals.F1RiskCount)
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Coverage histogram (best per-ride OSM coverage, ride-level):")
	for i, k := range totals.CoverageHistogramKeys {
		fmt.Fprintf(w, "  %s  %5d\n", k, totals.CoverageHistogramCounts[i])
	}
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "Legend:\n")
	fmt.Fprintf(w, "  snap   = SnapAction snap (Stage 1 or Stage 2 canonical)\n")
	fmt.Fprintf(w, "  multi  = SnapAction multi_ride_group (left over after Stage 2; defensive)\n")
	fmt.Fprintf(w, "  supp   = subset of snap: SuppressedByMerge=true (Stage-2 non-canonical members)\n")
	fmt.Fprintf(w, "  ptOSM  = passthrough WITH OSM relation (OSMCandidateIdx>=0; F1-fix scope)\n")
	fmt.Fprintf(w, "  ptNoO  = passthrough WITHOUT OSM relation\n")
	fmt.Fprintf(w, "  S3plan = Stage-3 planned OSM lifts (would emit OSM-side segment_effort)\n")
	fmt.Fprintf(w, "  OSMs   = osm_lifts candidates loaded across all activities (sum)\n")
	fmt.Fprintf(w, "  F1risk = passthrough rides where the F1 fix prevents a Stage-3 double-count\n")

	// Privacy-safe F1-risk sample: way_id is a public OSM identifier.
	if totals.F1RiskCount > 0 {
		fmt.Fprintln(w, "")
		fmt.Fprintf(w, "F1-risk examples (way_id is public OSM; coverage = best in-range overlap):\n")
		shown := 0
		for _, l := range labels {
			s := regions[l]
			for _, ex := range s.F1RiskExamples {
				if shown >= 20 {
					break
				}
				fmt.Fprintf(w, "  %-12s  way_id=%-12d  coverage=%.3f\n", ex.Region, ex.WayID, ex.Coverage)
				shown++
			}
			if shown >= 20 {
				break
			}
		}
	}
}
