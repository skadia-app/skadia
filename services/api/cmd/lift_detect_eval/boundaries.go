// Boundaries mode: for each detected ride, find the closest GT lift in the
// optional station-marker YAML (same schema lift_detect_viz consumes), match
// in both endpoint orientations (s↔p1+e↔p2 OR s↔p2+e↔p1, smaller max wins),
// and print a per-ride table plus a per-lift detected-vs-expected count
// summary. The YAML and the test-fixture file are local-only; only the
// numeric table reaches stdout, so output stays privacy-safe.

package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

type boundLift struct {
	ID string    `yaml:"id"`
	P1 []float64 `yaml:"p1"`
	P2 []float64 `yaml:"p2"`
}

type boundCase struct {
	File  string      `yaml:"file"`
	Lifts []boundLift `yaml:"lifts"`
}

type boundRoot struct {
	Cases []boundCase `yaml:"cases"`
}

// haversine in metres.
func havM(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000.0
	rad := math.Pi / 180.0
	p1 := lat1 * rad
	p2 := lat2 * rad
	dp := (lat2 - lat1) * rad
	dl := (lng2 - lng1) * rad
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * R * math.Asin(math.Sqrt(a))
}

func loadBoundCase(yamlPath, fileKey string) (boundCase, error) {
	raw, err := os.ReadFile(yamlPath)
	if err != nil {
		return boundCase{}, fmt.Errorf("read %s: %w", yamlPath, err)
	}
	var r boundRoot
	if err := yaml.Unmarshal(raw, &r); err != nil {
		return boundCase{}, fmt.Errorf("parse %s: %w", yamlPath, err)
	}
	for _, c := range r.Cases {
		if c.File == fileKey {
			if len(c.Lifts) == 0 {
				return boundCase{}, fmt.Errorf("case %q has empty lifts list", fileKey)
			}
			return c, nil
		}
	}
	return boundCase{}, fmt.Errorf("case %q not found in %s", fileKey, yamlPath)
}

type rideEndpoints struct {
	idx       int
	durS      int
	ascM      float64
	descM     float64
	conf      string
	sLat      float64
	sLng      float64
	eLat      float64
	eLng      float64
}

// matchRide returns (best lift id, orientation label, start_off, end_off, max_off).
func matchRide(r rideEndpoints, lifts []boundLift) (string, string, float64, float64, float64) {
	best := math.Inf(1)
	var bestID, bestOrient string
	var bestS, bestE float64
	for _, L := range lifts {
		if len(L.P1) < 2 || len(L.P2) < 2 {
			continue
		}
		// orientation A
		aS := havM(r.sLat, r.sLng, L.P1[0], L.P1[1])
		aE := havM(r.eLat, r.eLng, L.P2[0], L.P2[1])
		if aMax := math.Max(aS, aE); aMax < best {
			best = aMax
			bestID = L.ID
			bestOrient = "s→p1,e→p2"
			bestS, bestE = aS, aE
		}
		// orientation B
		bS := havM(r.sLat, r.sLng, L.P2[0], L.P2[1])
		bE := havM(r.eLat, r.eLng, L.P1[0], L.P1[1])
		if bMax := math.Max(bS, bE); bMax < best {
			best = bMax
			bestID = L.ID
			bestOrient = "s→p2,e→p1"
			bestS, bestE = bS, bE
		}
	}
	return bestID, bestOrient, bestS, bestE, best
}

// toleranceBand maps max_offset (m) to the calibration-history tolerance
// glyph (✓ ≤20, ○ ≤30, ⚠ ≤80, ✗ >80) — see geo-lift-detection.md.
func toleranceBand(maxOff float64) string {
	switch {
	case maxOff <= 20:
		return "✓"
	case maxOff <= 30:
		return "○"
	case maxOff <= 80:
		return "⚠"
	default:
		return "✗"
	}
}

func printBoundaries(w io.Writer, rides []rideEndpoints, lifts []boundLift) {
	fmt.Fprintf(w, "%3s %-14s %-12s %9s %9s %9s %3s  %s\n",
		"idx", "best_gt", "orient", "start_off", "end_off", "max_off", "tol", "asc/desc")
	fmt.Fprintln(w, "----------------------------------------------------------------------------------------")
	matched := map[string][]int{}
	for _, r := range rides {
		id, orient, sOff, eOff, maxOff := matchRide(r, lifts)
		matched[id] = append(matched[id], r.idx)
		fmt.Fprintf(w, "%3d %-14s %-12s %7.0fm %7.0fm %7.0fm  %s   %.0f/%.0f\n",
			r.idx, id, orient, sOff, eOff, maxOff, toleranceBand(maxOff), r.ascM, r.descM)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Per-Lift detected count:")
	ids := make([]string, 0, len(lifts))
	for _, L := range lifts {
		ids = append(ids, L.ID)
	}
	sort.Strings(ids)
	totalDet := 0
	for _, id := range ids {
		rs := matched[id]
		totalDet += len(rs)
		fmt.Fprintf(w, "  %-14s %2d  rides=%v\n", id, len(rs), rs)
	}
	fmt.Fprintf(w, "  %-14s %2d\n", "TOTAL", totalDet)
}
