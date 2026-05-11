package main

import (
	"encoding/json"
	"io"
	"sort"
)

// BoundariesJSON is the machine-readable per-ride boundary report. It
// mirrors the text table from printBoundaries but adds a stable schema for
// diff tooling. Privacy: only offsets and tolerance bands appear, never the
// raw GT or ride coordinates that fed into the matcher.
type BoundariesJSON struct {
	FormatVersion int               `json:"format_version"`
	File          string            `json:"file"`
	Rides         []RideReport      `json:"rides"`
	PerLiftCounts []LiftCountReport `json:"per_lift_counts"`
}

type RideReport struct {
	Idx        int     `json:"idx"`
	BestGT     string  `json:"best_gt"`
	Orient     string  `json:"orient"`
	StartOffM  float64 `json:"start_off_m"`
	EndOffM    float64 `json:"end_off_m"`
	MaxOffM    float64 `json:"max_off_m"`
	Tolerance  string  `json:"tolerance"`
	DurationS  int     `json:"duration_s"`
	AscentM    float64 `json:"ascent_m"`
	DescentM   float64 `json:"descent_m"`
	Confidence string  `json:"confidence,omitempty"`
}

type LiftCountReport struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
	Rides []int  `json:"rides"`
}

// writeBoundariesJSON emits the per-ride boundary table as JSON. file is
// echoed unchanged so multi-case diff drivers can route output by case key.
// All lift IDs from the GT pool appear in per_lift_counts (count=0 for
// unmatched ones) — diff consumers see explicit zeros, not absence.
func writeBoundariesJSON(w io.Writer, file string, rides []rideEndpoints, gtLifts []boundLift) error {
	rep := BoundariesJSON{
		FormatVersion: 1,
		File:          file,
		Rides:         make([]RideReport, 0, len(rides)),
	}

	matched := map[string][]int{}
	for _, r := range rides {
		id, orient, sOff, eOff, maxOff := matchRide(r, gtLifts)
		matched[id] = append(matched[id], r.idx)
		rep.Rides = append(rep.Rides, RideReport{
			Idx:        r.idx,
			BestGT:     id,
			Orient:     orient,
			StartOffM:  roundDec(sOff, 1),
			EndOffM:    roundDec(eOff, 1),
			MaxOffM:    roundDec(maxOff, 1),
			Tolerance:  toleranceBand(maxOff),
			DurationS:  r.durS,
			AscentM:    roundDec(r.ascM, 1),
			DescentM:   roundDec(r.descM, 1),
			Confidence: r.conf,
		})
	}

	ids := make([]string, 0, len(gtLifts))
	for _, L := range gtLifts {
		ids = append(ids, L.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		rs := matched[id]
		if rs == nil {
			rs = []int{}
		}
		rep.PerLiftCounts = append(rep.PerLiftCounts, LiftCountReport{
			ID:    id,
			Count: len(rs),
			Rides: rs,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(&rep)
}

// roundDec rounds f to n decimal places. Used so JSON offsets are stable
// across runs (raw float64 reprintings would jitter on the last digits).
func roundDec(f float64, n int) float64 {
	scale := 1.0
	for i := 0; i < n; i++ {
		scale *= 10
	}
	if f >= 0 {
		return float64(int64(f*scale+0.5)) / scale
	}
	return float64(int64(f*scale-0.5)) / scale
}
