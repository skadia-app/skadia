package main

import (
	"fmt"
	"io"
	"os"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

var defaultStdout io.Writer = os.Stdout

// stdoutWriter is a small indirection so tests can inject an alternative
// without monkey-patching os.Stdout. Default points at the real stdout.
var stdoutWriter = func() io.Writer { return defaultStdout }

type Result struct {
	Case     GroundTruthCase
	Detected []lifts.LiftRide
	Err      error
}

// printReport prints one line per case and aggregate Precision/Recall metrics.
// Returns exit code 0 when Recall >= 0.9 AND Precision == 1.0, else 1.
//
// Precision definition: a single case with detected > expected_max makes
// Precision < 1.0. Recall: (cases where detected >= expected_rides_min) / total.
func printReport(results []Result, verbose bool) int {
	return printReportTo(results, verbose, stdoutWriter())
}

// printReportTo allows callers to route the eval report to stdout (default)
// or stderr (when stdout is reserved for machine-readable output like the
// snap-stats JSON). Returns the same exit-code as printReport.
func printReportTo(results []Result, verbose bool, w io.Writer) int {
	var totalExpMin, totalExpMax, totalDetected int
	var recallNumerator int

	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(w, "ERR  %-50s  %v\n", r.Case.File, r.Err)
			continue
		}

		got := len(r.Detected)
		totalExpMin += r.Case.ExpectedRidesMin
		totalExpMax += r.Case.ExpectedRidesMax
		totalDetected += got

		var status string
		switch {
		case got >= r.Case.ExpectedRidesMin && got <= r.Case.ExpectedRidesMax:
			status = "OK  "
			recallNumerator++
		case got < r.Case.ExpectedRidesMin:
			status = "MISS"
		default:
			status = "OVER"
		}

		fmt.Fprintf(w, "%s %-50s  got=%d  want=[%d,%d]\n",
			status, r.Case.File, got, r.Case.ExpectedRidesMin, r.Case.ExpectedRidesMax)

		if verbose {
			for i, ride := range r.Detected {
				fmt.Fprintf(w, "     ride[%02d] started=%s dur=%ds asc=%.1fm desc=%.1fm dist=%.0fm conf=%s\n",
					i,
					ride.StartedAt.Format("15:04:05"),
					ride.DurationS,
					ride.AscentM,
					ride.DescentM,
					ride.DistanceM,
					ride.Confidence,
				)
			}
		}
	}

	totalCases := 0
	for _, r := range results {
		if r.Err == nil {
			totalCases++
		}
	}

	var recall, precision float64
	if totalCases > 0 {
		recall = float64(recallNumerator) / float64(totalCases)
	}
	if totalDetected <= totalExpMax {
		precision = 1.0
	} else {
		precision = float64(totalExpMax) / float64(totalDetected)
	}

	fmt.Fprintf(w, "\nSummary: cases=%d  detected=%d  recall=%.2f  precision=%.2f\n",
		totalCases, totalDetected, recall, precision)

	if recall >= 0.9 && precision >= 1.0 {
		return 0
	}
	return 1
}
