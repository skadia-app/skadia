package main

import (
	"encoding/json"
	"io"
)

// ReportJSON is the machine-readable manifest report. format_version is
// bumped whenever a downstream-incompatible field change is made; diff
// tooling refuses unknown versions instead of silently mis-aligning.
type ReportJSON struct {
	FormatVersion int           `json:"format_version"`
	Cases         []CaseReport  `json:"cases"`
	Summary       SummaryReport `json:"summary"`
}

type CaseReport struct {
	File              string         `json:"file"`
	Sport             string         `json:"sport"`
	ExpectedMin       int            `json:"expected_min"`
	ExpectedMax       int            `json:"expected_max"`
	Detected          int            `json:"detected"`
	Status            string         `json:"status"` // ok|miss|over|err
	Error             string         `json:"error,omitempty"`
	ConfidenceBuckets map[string]int `json:"confidence_buckets,omitempty"`
}

type SummaryReport struct {
	Cases             int            `json:"cases"`
	Detected          int            `json:"detected"`
	ExpectedMin       int            `json:"expected_min"`
	ExpectedMax       int            `json:"expected_max"`
	Recall            float64        `json:"recall"`
	Precision         float64        `json:"precision"`
	ConfidenceBuckets map[string]int `json:"confidence_buckets,omitempty"`
}

// writeReportJSON emits the manifest eval as JSON and returns the same
// pass/fail exit code as printReportTo (0 when recall ≥ 0.9 AND precision
// == 1.0, else 1). Error result is reserved for write failures so callers
// can distinguish I/O errors from eval verdicts.
func writeReportJSON(results []Result, w io.Writer) (int, error) {
	rep := ReportJSON{FormatVersion: 1, Cases: make([]CaseReport, 0, len(results))}

	var totalCases, totalDetected, totalExpMin, totalExpMax, recallNumerator int
	summaryBuckets := map[string]int{}

	for _, r := range results {
		c := CaseReport{
			File:        r.Case.File,
			Sport:       r.Case.Sport,
			ExpectedMin: r.Case.ExpectedRidesMin,
			ExpectedMax: r.Case.ExpectedRidesMax,
		}
		if r.Err != nil {
			c.Status = "err"
			c.Error = r.Err.Error()
			rep.Cases = append(rep.Cases, c)
			continue
		}

		got := len(r.Detected)
		c.Detected = got
		switch {
		case got >= r.Case.ExpectedRidesMin && got <= r.Case.ExpectedRidesMax:
			c.Status = "ok"
			recallNumerator++
		case got < r.Case.ExpectedRidesMin:
			c.Status = "miss"
		default:
			c.Status = "over"
		}

		buckets := map[string]int{}
		for _, ride := range r.Detected {
			buckets[ride.Confidence]++
			summaryBuckets[ride.Confidence]++
		}
		if len(buckets) > 0 {
			c.ConfidenceBuckets = buckets
		}

		totalCases++
		totalDetected += got
		totalExpMin += r.Case.ExpectedRidesMin
		totalExpMax += r.Case.ExpectedRidesMax

		rep.Cases = append(rep.Cases, c)
	}

	rep.Summary = SummaryReport{
		Cases:       totalCases,
		Detected:    totalDetected,
		ExpectedMin: totalExpMin,
		ExpectedMax: totalExpMax,
	}
	if totalCases > 0 {
		rep.Summary.Recall = float64(recallNumerator) / float64(totalCases)
	}
	if totalDetected <= totalExpMax {
		rep.Summary.Precision = 1.0
	} else {
		rep.Summary.Precision = float64(totalExpMax) / float64(totalDetected)
	}
	if len(summaryBuckets) > 0 {
		rep.Summary.ConfidenceBuckets = summaryBuckets
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&rep); err != nil {
		return 1, err
	}

	if rep.Summary.Recall >= 0.9 && rep.Summary.Precision >= 1.0 {
		return 0, nil
	}
	return 1, nil
}
