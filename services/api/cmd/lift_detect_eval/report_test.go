package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

// TestWriteReportJSONShape pins the machine-readable manifest report schema
// so a downstream diff tool can rely on field names and structure across
// versions. Synthetic case names + zero-valued rides keep the test free of
// recorded GPS data; only counts and confidence labels are asserted.
func TestWriteReportJSONShape(t *testing.T) {
	results := []Result{
		{
			Case: GroundTruthCase{File: "synth-a.gpx", Sport: "ski", ExpectedRidesMin: 2, ExpectedRidesMax: 3},
			Detected: []lifts.LiftRide{
				{Confidence: "high"},
				{Confidence: "high"},
				{Confidence: "medium"},
			},
		},
		{
			Case: GroundTruthCase{File: "synth-b.fit", Sport: "ski", ExpectedRidesMin: 1, ExpectedRidesMax: 1},
			Err:  errors.New("synthetic load failure"),
		},
	}

	var buf bytes.Buffer
	exit, err := writeReportJSON(results, &buf)
	if err != nil {
		t.Fatalf("writeReportJSON: %v", err)
	}

	var parsed ReportJSON
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("not valid JSON: %v\n--- payload ---\n%s", err, buf.String())
	}

	if parsed.FormatVersion != 1 {
		t.Errorf("format_version = %d, want 1", parsed.FormatVersion)
	}
	if len(parsed.Cases) != 2 {
		t.Fatalf("cases len = %d, want 2", len(parsed.Cases))
	}

	a := parsed.Cases[0]
	if a.File != "synth-a.gpx" {
		t.Errorf("cases[0].file = %q, want synth-a.gpx", a.File)
	}
	if a.Sport != "ski" {
		t.Errorf("cases[0].sport = %q, want ski", a.Sport)
	}
	if a.Status != "ok" {
		t.Errorf("cases[0].status = %q, want ok", a.Status)
	}
	if a.Detected != 3 {
		t.Errorf("cases[0].detected = %d, want 3", a.Detected)
	}
	if a.ExpectedMin != 2 || a.ExpectedMax != 3 {
		t.Errorf("cases[0] expected range = [%d,%d], want [2,3]", a.ExpectedMin, a.ExpectedMax)
	}
	if a.ConfidenceBuckets["high"] != 2 {
		t.Errorf("cases[0].confidence_buckets[high] = %d, want 2", a.ConfidenceBuckets["high"])
	}
	if a.ConfidenceBuckets["medium"] != 1 {
		t.Errorf("cases[0].confidence_buckets[medium] = %d, want 1", a.ConfidenceBuckets["medium"])
	}

	b := parsed.Cases[1]
	if b.Status != "err" {
		t.Errorf("cases[1].status = %q, want err", b.Status)
	}
	if b.Error == "" {
		t.Errorf("cases[1].error empty, want non-empty load-failure description")
	}
	if b.Detected != 0 {
		t.Errorf("cases[1].detected = %d, want 0 (errored case)", b.Detected)
	}

	// Summary: errored case excluded from cases-count, detected stays at 3,
	// confidence buckets aggregate across non-errored cases only.
	if parsed.Summary.Cases != 1 {
		t.Errorf("summary.cases = %d, want 1 (errors excluded)", parsed.Summary.Cases)
	}
	if parsed.Summary.Detected != 3 {
		t.Errorf("summary.detected = %d, want 3", parsed.Summary.Detected)
	}
	if parsed.Summary.ConfidenceBuckets["high"] != 2 {
		t.Errorf("summary.confidence_buckets[high] = %d, want 2", parsed.Summary.ConfidenceBuckets["high"])
	}
	if parsed.Summary.Recall != 1.0 {
		t.Errorf("summary.recall = %v, want 1.0 (1/1 OK among non-errored)", parsed.Summary.Recall)
	}
	if parsed.Summary.Precision != 1.0 {
		t.Errorf("summary.precision = %v, want 1.0", parsed.Summary.Precision)
	}

	if exit != 0 {
		t.Errorf("exit = %d, want 0 (recall=1.0, precision=1.0)", exit)
	}
}

// TestWriteReportJSONStatusBuckets covers the three non-error status values
// that downstream tooling diffs against: ok / miss / over. Asserts exit code
// reflects precision drop when one case overshoots its expected_max.
func TestWriteReportJSONStatusBuckets(t *testing.T) {
	rides3 := []lifts.LiftRide{{Confidence: "high"}, {Confidence: "high"}, {Confidence: "high"}}
	rides1 := []lifts.LiftRide{{Confidence: "medium"}}
	rides10 := make([]lifts.LiftRide, 10)
	for i := range rides10 {
		rides10[i] = lifts.LiftRide{Confidence: "bidir"}
	}

	// Choose detected counts so SUM(detected) > SUM(expected_max): 3+1+10=14
	// vs 3+3+3=9. The aggregate precision contract (see report.go) compares
	// the totals, so the over case must drag the sum above expected_max.
	results := []Result{
		{Case: GroundTruthCase{File: "synth-ok.gpx", Sport: "ski", ExpectedRidesMin: 2, ExpectedRidesMax: 3}, Detected: rides3},
		{Case: GroundTruthCase{File: "synth-miss.gpx", Sport: "ski", ExpectedRidesMin: 2, ExpectedRidesMax: 3}, Detected: rides1},
		{Case: GroundTruthCase{File: "synth-over.gpx", Sport: "ski", ExpectedRidesMin: 2, ExpectedRidesMax: 3}, Detected: rides10},
	}

	var buf bytes.Buffer
	exit, err := writeReportJSON(results, &buf)
	if err != nil {
		t.Fatalf("writeReportJSON: %v", err)
	}

	var parsed ReportJSON
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}

	want := []string{"ok", "miss", "over"}
	for i, w := range want {
		if parsed.Cases[i].Status != w {
			t.Errorf("cases[%d].status = %q, want %q", i, parsed.Cases[i].Status, w)
		}
	}

	if parsed.Summary.Precision >= 1.0 {
		t.Errorf("summary.precision = %v, want < 1.0 (sum 14 > expected_max sum 9)", parsed.Summary.Precision)
	}
	if exit == 0 {
		t.Errorf("exit = 0, want non-zero (precision < 1.0)")
	}
}

// TestPrintReportHonoursFormatFlag ensures the existing text path stays
// untouched when the new -format flag is left at its default — protects
// existing CSV-tooling callers from a silent contract change.
func TestPrintReportHonoursFormatFlag(t *testing.T) {
	results := []Result{
		{
			Case: GroundTruthCase{File: "synth.gpx", Sport: "ski", ExpectedRidesMin: 1, ExpectedRidesMax: 1},
			Detected: []lifts.LiftRide{
				{Confidence: "high"},
			},
		},
	}
	var buf bytes.Buffer
	printReportTo(results, false, &buf)
	out := buf.String()
	// Spot-check: the historical "Summary:" line must still appear; tooling
	// that greps for it (no JSON) should keep working.
	if !strings.Contains(out, "Summary:") {
		t.Errorf("text report missing Summary line:\n%s", out)
	}
}
