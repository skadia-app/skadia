// Diff functions compare two eval reports (baseline vs current) and emit a
// human-readable drift table plus an exit code suitable for a pre-commit
// hook. The diff logic is schema-agnostic across algorithms — only the
// JSON shapes (ReportJSON, BoundariesJSON) are lift-specific. A different
// algorithm with the same schema can reuse these functions verbatim.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
)

// reportKind identifies which JSON schema a payload follows. Auto-detected
// by peeking at top-level keys: `summary` ⇒ manifest, `rides` ⇒ boundaries.
type reportKind int

const (
	kindUnknown reportKind = iota
	kindManifest
	kindBoundaries
)

func detectKind(raw []byte) (reportKind, error) {
	var peek map[string]json.RawMessage
	if err := json.Unmarshal(raw, &peek); err != nil {
		return kindUnknown, fmt.Errorf("parse JSON: %w", err)
	}
	_, hasSummary := peek["summary"]
	_, hasRides := peek["rides"]
	switch {
	case hasSummary && !hasRides:
		return kindManifest, nil
	case hasRides && !hasSummary:
		return kindBoundaries, nil
	default:
		return kindUnknown, fmt.Errorf("ambiguous report shape: summary=%v rides=%v", hasSummary, hasRides)
	}
}

// runDiff loads two JSON eval reports, auto-detects whether they are
// manifest- or boundaries-shape, and dispatches to the matching diff
// function. marginalBandM applies only to boundaries diffs (ignored
// otherwise). Returns the diff exit code, or an error for I/O failures
// or mixed/unknown report kinds.
func runDiff(baselinePath, currentPath string, marginalBandM float64, w io.Writer) (int, error) {
	bRaw, err := os.ReadFile(baselinePath)
	if err != nil {
		return 0, fmt.Errorf("read baseline %s: %w", baselinePath, err)
	}
	cRaw, err := os.ReadFile(currentPath)
	if err != nil {
		return 0, fmt.Errorf("read current %s: %w", currentPath, err)
	}

	bKind, err := detectKind(bRaw)
	if err != nil {
		return 0, fmt.Errorf("baseline: %w", err)
	}
	cKind, err := detectKind(cRaw)
	if err != nil {
		return 0, fmt.Errorf("current: %w", err)
	}
	if bKind != cKind {
		return 0, fmt.Errorf("baseline and current have different report kinds (manifest vs boundaries)")
	}

	switch bKind {
	case kindManifest:
		var b, c ReportJSON
		if err := json.Unmarshal(bRaw, &b); err != nil {
			return 0, fmt.Errorf("parse baseline manifest: %w", err)
		}
		if err := json.Unmarshal(cRaw, &c); err != nil {
			return 0, fmt.Errorf("parse current manifest: %w", err)
		}
		return diffManifestReports(b, c, w), nil
	case kindBoundaries:
		var b, c BoundariesJSON
		if err := json.Unmarshal(bRaw, &b); err != nil {
			return 0, fmt.Errorf("parse baseline boundaries: %w", err)
		}
		if err := json.Unmarshal(cRaw, &c); err != nil {
			return 0, fmt.Errorf("parse current boundaries: %w", err)
		}
		return diffBoundariesReports(b, c, marginalBandM, w), nil
	}
	return 0, fmt.Errorf("unknown report kind")
}

// diffManifestReports walks baseline and current case-by-case (keyed by
// File). Returns 0 when no case differs in status, detected count, or
// confidence buckets; 1 otherwise. Cases unique to one side count as
// drift in either direction.
func diffManifestReports(baseline, current ReportJSON, w io.Writer) int {
	baseByFile := map[string]CaseReport{}
	for _, c := range baseline.Cases {
		baseByFile[c.File] = c
	}
	curByFile := map[string]CaseReport{}
	for _, c := range current.Cases {
		curByFile[c.File] = c
	}

	files := unionKeys(baseByFile, curByFile)
	sort.Strings(files)

	type drift struct {
		file string
		kind string // status|count|confidence|added|removed
		msg  string
	}
	var drifts []drift

	for _, f := range files {
		b, inBase := baseByFile[f]
		c, inCur := curByFile[f]
		switch {
		case !inBase:
			drifts = append(drifts, drift{f, "added", fmt.Sprintf("status=%s detected=%d", c.Status, c.Detected)})
		case !inCur:
			drifts = append(drifts, drift{f, "removed", fmt.Sprintf("was status=%s detected=%d", b.Status, b.Detected)})
		default:
			if b.Status != c.Status {
				drifts = append(drifts, drift{f, "status", fmt.Sprintf("%s → %s (detected %d → %d)", b.Status, c.Status, b.Detected, c.Detected)})
			} else if b.Detected != c.Detected {
				drifts = append(drifts, drift{f, "count", fmt.Sprintf("detected %d → %d (status %s)", b.Detected, c.Detected, b.Status)})
			}
			if !bucketsEqual(b.ConfidenceBuckets, c.ConfidenceBuckets) {
				drifts = append(drifts, drift{f, "confidence", fmt.Sprintf("buckets %s → %s", formatBuckets(b.ConfidenceBuckets), formatBuckets(c.ConfidenceBuckets))})
			}
		}
	}

	if len(drifts) == 0 {
		fmt.Fprintln(w, "no drift")
		return 0
	}

	fmt.Fprintln(w, "DRIFT (manifest):")
	for _, d := range drifts {
		fmt.Fprintf(w, "  %-12s %-50s %s\n", "["+d.kind+"]", d.file, d.msg)
	}
	return 1
}

// diffBoundariesReports walks baseline and current per ride.idx. A
// tolerance-band flip OR a best_gt change OR an in-band offset change
// larger than marginalBandM counts as drift. Same-band offset changes ≤
// marginalBandM are reported as "marginal" but do not flip the exit code,
// matching the user-stated tolerance ("kaum verschlechtern").
func diffBoundariesReports(baseline, current BoundariesJSON, marginalBandM float64, w io.Writer) int {
	baseByIdx := map[int]RideReport{}
	for _, r := range baseline.Rides {
		baseByIdx[r.Idx] = r
	}
	curByIdx := map[int]RideReport{}
	for _, r := range current.Rides {
		curByIdx[r.Idx] = r
	}

	idxs := unionInts(baseByIdx, curByIdx)
	sort.Ints(idxs)

	type rideDrift struct {
		idx      int
		kind     string // band|match|inband|added|removed|marginal
		msg      string
		marginal bool
	}
	var drifts []rideDrift

	for _, i := range idxs {
		b, inBase := baseByIdx[i]
		c, inCur := curByIdx[i]
		switch {
		case !inBase:
			drifts = append(drifts, rideDrift{i, "added", fmt.Sprintf("best_gt=%s tol=%s max_off=%.1fm", c.BestGT, c.Tolerance, c.MaxOffM), false})
		case !inCur:
			drifts = append(drifts, rideDrift{i, "removed", fmt.Sprintf("was best_gt=%s tol=%s max_off=%.1fm", b.BestGT, b.Tolerance, b.MaxOffM), false})
		default:
			if b.Tolerance != c.Tolerance {
				drifts = append(drifts, rideDrift{i, "band", fmt.Sprintf("%s → %s (max_off %.1fm → %.1fm)", b.Tolerance, c.Tolerance, b.MaxOffM, c.MaxOffM), false})
				continue
			}
			if b.BestGT != c.BestGT {
				drifts = append(drifts, rideDrift{i, "match", fmt.Sprintf("best_gt %s → %s", b.BestGT, c.BestGT), false})
				continue
			}
			delta := math.Abs(c.MaxOffM - b.MaxOffM)
			if delta > marginalBandM {
				drifts = append(drifts, rideDrift{i, "inband", fmt.Sprintf("max_off %.1fm → %.1fm (Δ %.1fm > %.1fm marginal)", b.MaxOffM, c.MaxOffM, delta, marginalBandM), false})
			} else if delta > 0 {
				drifts = append(drifts, rideDrift{i, "marginal", fmt.Sprintf("max_off %.1fm → %.1fm (Δ %.1fm)", b.MaxOffM, c.MaxOffM, delta), true})
			}
		}
	}

	// Per-lift count diff (independent of ride-by-ride drifts).
	baseCounts := map[string]int{}
	for _, c := range baseline.PerLiftCounts {
		baseCounts[c.ID] = c.Count
	}
	type countDrift struct{ id, msg string }
	var countDrifts []countDrift
	curIDs := map[string]bool{}
	for _, c := range current.PerLiftCounts {
		curIDs[c.ID] = true
		if bc, ok := baseCounts[c.ID]; ok && bc != c.Count {
			countDrifts = append(countDrifts, countDrift{c.ID, fmt.Sprintf("%d → %d rides", bc, c.Count)})
		} else if !ok && c.Count > 0 {
			countDrifts = append(countDrifts, countDrift{c.ID, fmt.Sprintf("0 → %d rides (new lift)", c.Count)})
		}
	}
	for id, bc := range baseCounts {
		if !curIDs[id] && bc > 0 {
			countDrifts = append(countDrifts, countDrift{id, fmt.Sprintf("%d → 0 rides (lift dropped)", bc)})
		}
	}
	sort.Slice(countDrifts, func(i, j int) bool { return countDrifts[i].id < countDrifts[j].id })

	// Decide exit code: any non-marginal ride drift OR any per-lift count
	// change is a regression.
	exit := 0
	hasNonMarginal := false
	for _, d := range drifts {
		if !d.marginal {
			hasNonMarginal = true
			break
		}
	}
	if hasNonMarginal || len(countDrifts) > 0 {
		exit = 1
	}

	if len(drifts) == 0 && len(countDrifts) == 0 {
		fmt.Fprintln(w, "no drift")
		return 0
	}

	if exit == 0 {
		fmt.Fprintln(w, "marginal drifts only (no tolerance-band flips, all Δ ≤ marginalBandM):")
	} else {
		fmt.Fprintln(w, "DRIFT (boundaries):")
	}
	for _, d := range drifts {
		fmt.Fprintf(w, "  %-10s ride[%d] %s\n", "["+d.kind+"]", d.idx, d.msg)
	}
	for _, d := range countDrifts {
		fmt.Fprintf(w, "  %-10s lift=%s %s\n", "[per-lift]", d.id, d.msg)
	}
	return exit
}

// bucketsEqual reports whether two confidence-bucket maps have identical
// (key, value) sets. nil maps are treated as empty.
func bucketsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || va != vb {
			return false
		}
	}
	return true
}

func formatBuckets(m map[string]int) string {
	if len(m) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := "{"
	for i, k := range keys {
		if i > 0 {
			out += " "
		}
		out += fmt.Sprintf("%s=%d", k, m[k])
	}
	out += "}"
	return out
}

func unionKeys(a, b map[string]CaseReport) []string {
	seen := map[string]bool{}
	out := []string{}
	for k := range a {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for k := range b {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

func unionInts(a, b map[int]RideReport) []int {
	seen := map[int]bool{}
	out := []int{}
	for k := range a {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for k := range b {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}
