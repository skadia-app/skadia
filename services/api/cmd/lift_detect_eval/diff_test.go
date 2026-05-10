package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDiffManifestReports covers the case-level diff: status flip, count
// change, confidence bucket shift, plus the no-change baseline. Drift table
// content is checked by substring so layout tweaks don't break the test.
func TestDiffManifestReports(t *testing.T) {
	mk := func(status string, detected int, buckets map[string]int) CaseReport {
		return CaseReport{
			File:              "synth-a.gpx",
			Sport:             "ski",
			ExpectedMin:       2,
			ExpectedMax:       3,
			Detected:          detected,
			Status:            status,
			ConfidenceBuckets: buckets,
		}
	}
	base := mk("ok", 3, map[string]int{"high": 2, "medium": 1})

	cases := []struct {
		name            string
		current         CaseReport
		wantExit        int
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:            "no change",
			current:         base,
			wantExit:        0,
			wantContains:    []string{"no drift"},
			wantNotContains: []string{"synth-a.gpx"},
		},
		{
			name:         "status flip ok→miss",
			current:      mk("miss", 1, map[string]int{"medium": 1}),
			wantExit:     1,
			wantContains: []string{"synth-a.gpx", "ok", "miss"},
		},
		{
			name:         "count change without status flip",
			current:      mk("ok", 2, map[string]int{"high": 2}),
			wantExit:     1,
			wantContains: []string{"synth-a.gpx", "detected"},
		},
		{
			name:         "confidence bucket shift",
			current:      mk("ok", 3, map[string]int{"high": 1, "medium": 2}),
			wantExit:     1,
			wantContains: []string{"synth-a.gpx", "confidence"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseline := ReportJSON{FormatVersion: 1, Cases: []CaseReport{base}}
			current := ReportJSON{FormatVersion: 1, Cases: []CaseReport{tc.current}}
			var buf bytes.Buffer
			gotExit := diffManifestReports(baseline, current, &buf)
			if gotExit != tc.wantExit {
				t.Errorf("exit = %d, want %d\n--- output ---\n%s", gotExit, tc.wantExit, buf.String())
			}
			for _, s := range tc.wantContains {
				if !strings.Contains(buf.String(), s) {
					t.Errorf("output missing %q\n--- output ---\n%s", s, buf.String())
				}
			}
			for _, s := range tc.wantNotContains {
				if strings.Contains(buf.String(), s) {
					t.Errorf("output contains %q but should not\n--- output ---\n%s", s, buf.String())
				}
			}
		})
	}
}

// TestDiffManifestReports_AddedRemoved checks the symmetric case: a case
// appears in current but not baseline (or vice versa). Both directions
// surface in the drift table, exit code is non-zero in either direction.
func TestDiffManifestReports_AddedRemoved(t *testing.T) {
	a := CaseReport{File: "synth-a.gpx", Status: "ok", Detected: 1}
	b := CaseReport{File: "synth-b.gpx", Status: "ok", Detected: 1}

	baseline := ReportJSON{FormatVersion: 1, Cases: []CaseReport{a}}
	current := ReportJSON{FormatVersion: 1, Cases: []CaseReport{a, b}}
	var buf bytes.Buffer
	if exit := diffManifestReports(baseline, current, &buf); exit == 0 {
		t.Errorf("added case: exit = 0, want non-zero")
	}
	if !strings.Contains(buf.String(), "synth-b.gpx") {
		t.Errorf("added case not surfaced:\n%s", buf.String())
	}

	// Reverse: case removed.
	buf.Reset()
	if exit := diffManifestReports(current, baseline, &buf); exit == 0 {
		t.Errorf("removed case: exit = 0, want non-zero")
	}
	if !strings.Contains(buf.String(), "synth-b.gpx") {
		t.Errorf("removed case not surfaced:\n%s", buf.String())
	}
}

// TestDiffBoundariesReports covers per-ride boundary-drift detection.
// marginalBandM gates whether sub-band offset jitter triggers a regression.
func TestDiffBoundariesReports(t *testing.T) {
	mkRide := func(idx int, gt string, maxOff float64, tol string) RideReport {
		return RideReport{
			Idx:       idx,
			BestGT:    gt,
			MaxOffM:   maxOff,
			StartOffM: maxOff,
			EndOffM:   maxOff,
			Tolerance: tol,
		}
	}
	baseRide := mkRide(0, "alpha", 5, "✓")

	cases := []struct {
		name            string
		current         RideReport
		marginalBandM   float64
		wantExit        int
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:            "no change",
			current:         baseRide,
			marginalBandM:   10,
			wantExit:        0,
			wantContains:    []string{"no drift"},
			wantNotContains: []string{"ride[0]"},
		},
		{
			name:          "tolerance band worsens ✓→⚠",
			current:       mkRide(0, "alpha", 50, "⚠"),
			marginalBandM: 10,
			wantExit:      1,
			wantContains:  []string{"ride[0]", "✓", "⚠"},
		},
		{
			name:          "marginal drift inside band (≤ marginalBandM)",
			current:       mkRide(0, "alpha", 14, "✓"),
			marginalBandM: 10,
			// 14 - 5 = 9m ≤ 10m, same band — accepted as marginal. Exit 0.
			wantExit:        0,
			wantContains:    []string{"marginal"},
			wantNotContains: []string{"DRIFT"},
		},
		{
			name:          "drift inside band exceeds marginalBandM",
			current:       mkRide(0, "alpha", 16, "✓"),
			marginalBandM: 10,
			// 16 - 5 = 11m > 10m → counts as regression even without band flip.
			wantExit:     1,
			wantContains: []string{"ride[0]"},
		},
		{
			name:          "best_gt match changes",
			current:       mkRide(0, "beta", 5, "✓"),
			marginalBandM: 10,
			wantExit:      1,
			wantContains:  []string{"ride[0]", "alpha", "beta"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseline := BoundariesJSON{FormatVersion: 1, File: "synth.gpx", Rides: []RideReport{baseRide}}
			current := BoundariesJSON{FormatVersion: 1, File: "synth.gpx", Rides: []RideReport{tc.current}}
			var buf bytes.Buffer
			gotExit := diffBoundariesReports(baseline, current, tc.marginalBandM, &buf)
			if gotExit != tc.wantExit {
				t.Errorf("exit = %d, want %d\n--- output ---\n%s", gotExit, tc.wantExit, buf.String())
			}
			for _, s := range tc.wantContains {
				if !strings.Contains(buf.String(), s) {
					t.Errorf("output missing %q\n--- output ---\n%s", s, buf.String())
				}
			}
			for _, s := range tc.wantNotContains {
				if strings.Contains(buf.String(), s) {
					t.Errorf("output contains %q but should not\n--- output ---\n%s", s, buf.String())
				}
			}
		})
	}
}

// TestDiffBoundariesReports_BandImprovement ensures a band flip in the
// improving direction (⚠ → ✓) is also surfaced — a fix should be visible
// to the reviewer, not silently shadowed by a "no drift" verdict. Exit
// code stays non-zero because a band change is a real semantic change
// regardless of direction.
func TestDiffBoundariesReports_BandImprovement(t *testing.T) {
	baseline := BoundariesJSON{
		FormatVersion: 1, File: "synth.gpx",
		Rides: []RideReport{{Idx: 0, BestGT: "alpha", MaxOffM: 50, Tolerance: "⚠"}},
	}
	current := BoundariesJSON{
		FormatVersion: 1, File: "synth.gpx",
		Rides: []RideReport{{Idx: 0, BestGT: "alpha", MaxOffM: 4, Tolerance: "✓"}},
	}
	var buf bytes.Buffer
	if exit := diffBoundariesReports(baseline, current, 10, &buf); exit == 0 {
		t.Errorf("⚠→✓ improvement: exit = 0, want non-zero (band changed)\n%s", buf.String())
	}
	for _, want := range []string{"ride[0]", "⚠", "✓"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, buf.String())
		}
	}
}

// TestRunDiff_AutoDetectsKind covers the CLI entry: runDiff reads two
// JSON files, auto-detects whether they are manifest- or boundaries-shape
// reports, and dispatches to the matching diff function. Manifest is
// recognised by the presence of `summary`; boundaries by `rides`.
func TestRunDiff_AutoDetectsKind(t *testing.T) {
	dir := t.TempDir()

	// Manifest pair (same content → no drift, exit 0).
	manifest := []byte(`{
  "format_version": 1,
  "cases": [{"file":"synth.gpx","sport":"ski","expected_min":1,"expected_max":1,"detected":1,"status":"ok"}],
  "summary": {"cases":1,"detected":1,"recall":1.0,"precision":1.0}
}`)
	mPathA := filepath.Join(dir, "manifest_a.json")
	mPathB := filepath.Join(dir, "manifest_b.json")
	if err := os.WriteFile(mPathA, manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mPathB, manifest, 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	exit, err := runDiff(mPathA, mPathB, 10, &buf)
	if err != nil {
		t.Fatalf("runDiff (manifest): %v", err)
	}
	if exit != 0 {
		t.Errorf("manifest no-change: exit = %d, want 0\n%s", exit, buf.String())
	}
	if !strings.Contains(buf.String(), "no drift") {
		t.Errorf("manifest no-change: output missing 'no drift':\n%s", buf.String())
	}

	// Boundaries pair (drift in tolerance band → exit 1).
	bA := []byte(`{
  "format_version": 1,
  "file": "synth.gpx",
  "rides": [{"idx":0,"best_gt":"alpha","max_off_m":5,"tolerance":"✓"}],
  "per_lift_counts": [{"id":"alpha","count":1,"rides":[0]}]
}`)
	bB := []byte(`{
  "format_version": 1,
  "file": "synth.gpx",
  "rides": [{"idx":0,"best_gt":"alpha","max_off_m":50,"tolerance":"⚠"}],
  "per_lift_counts": [{"id":"alpha","count":1,"rides":[0]}]
}`)
	bPathA := filepath.Join(dir, "bounds_a.json")
	bPathB := filepath.Join(dir, "bounds_b.json")
	if err := os.WriteFile(bPathA, bA, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPathB, bB, 0o644); err != nil {
		t.Fatal(err)
	}

	buf.Reset()
	exit, err = runDiff(bPathA, bPathB, 10, &buf)
	if err != nil {
		t.Fatalf("runDiff (boundaries): %v", err)
	}
	if exit != 1 {
		t.Errorf("boundaries band drift: exit = %d, want 1\n%s", exit, buf.String())
	}
	if !strings.Contains(buf.String(), "ride[0]") {
		t.Errorf("boundaries band drift: output missing 'ride[0]':\n%s", buf.String())
	}
}

// TestRunDiff_RejectsMixedKinds prevents accidentally diffing a manifest
// against a boundaries report — the only sane outcome is a clear error.
func TestRunDiff_RejectsMixedKinds(t *testing.T) {
	dir := t.TempDir()
	manifest := []byte(`{"format_version":1,"cases":[],"summary":{"cases":0}}`)
	bounds := []byte(`{"format_version":1,"file":"synth.gpx","rides":[],"per_lift_counts":[]}`)
	mPath := filepath.Join(dir, "manifest.json")
	bPath := filepath.Join(dir, "bounds.json")
	_ = os.WriteFile(mPath, manifest, 0o644)
	_ = os.WriteFile(bPath, bounds, 0o644)

	var buf bytes.Buffer
	_, err := runDiff(mPath, bPath, 10, &buf)
	if err == nil {
		t.Errorf("mixed kinds: err = nil, want error\n%s", buf.String())
	}
}

// TestDiffBoundariesReports_PerLiftCountChange ensures changes in the
// per-lift count summary surface as drift even when no individual ride
// crossed a tolerance band — covers the case where a ride is added or
// dropped under the same lift between baseline and current.
func TestDiffBoundariesReports_PerLiftCountChange(t *testing.T) {
	r0 := RideReport{Idx: 0, BestGT: "alpha", MaxOffM: 5, Tolerance: "✓"}
	r1 := RideReport{Idx: 1, BestGT: "alpha", MaxOffM: 5, Tolerance: "✓"}

	baseline := BoundariesJSON{
		FormatVersion: 1, File: "synth.gpx",
		Rides:         []RideReport{r0, r1},
		PerLiftCounts: []LiftCountReport{{ID: "alpha", Count: 2, Rides: []int{0, 1}}},
	}
	current := BoundariesJSON{
		FormatVersion: 1, File: "synth.gpx",
		Rides:         []RideReport{r0},
		PerLiftCounts: []LiftCountReport{{ID: "alpha", Count: 1, Rides: []int{0}}},
	}

	var buf bytes.Buffer
	if exit := diffBoundariesReports(baseline, current, 10, &buf); exit == 0 {
		t.Errorf("dropped ride: exit = 0, want non-zero")
	}
	if !strings.Contains(buf.String(), "alpha") {
		t.Errorf("per-lift count drop not surfaced:\n%s", buf.String())
	}
}
