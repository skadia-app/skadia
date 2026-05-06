// Standalone debug tool: load one file, scan for windows passing all
// classification gates, print failures with reasons. Used during Phase 3
// iteration to understand why specific activities yield 0 detected rides.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
)

func main() {
	file := flag.String("file", "", "path to FIT/GPX")
	sport := flag.String("sport", "ski", "sport profile")
	tStart := flag.String("start", "", "RFC3339 ISO start time filter (only show windows starting after)")
	tEnd := flag.String("end", "", "RFC3339 ISO end time filter")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "need -file")
		os.Exit(2)
	}
	f, err := os.Open(*file)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	var pts []lifts.StreamPoint
	switch strings.ToLower(filepath.Ext(*file)) {
	case ".fit":
		data, err := fitimport.ParseFIT(f, *file)
		if err != nil {
			panic(err)
		}
		pts = lifts.ConvertActivityDataToStream(data)
	case ".gpx":
		data, err := gpximport.ParseGPX(f, *file)
		if err != nil {
			panic(err)
		}
		pts = lifts.ConvertActivityDataToStream(data)
	default:
		fmt.Fprintln(os.Stderr, "unsupported")
		os.Exit(2)
	}
	cfg := lifts.DefaultDetectorConfig(*sport)

	var startTime, endTime time.Time
	if *tStart != "" {
		startTime, _ = time.Parse(time.RFC3339, *tStart)
	}
	if *tEnd != "" {
		endTime, _ = time.Parse(time.RFC3339, *tEnd)
	}
	tracePts(pts, cfg, startTime, endTime)
}

// tracePts walks every i, runs buildInitialWindow + computeStats + classify,
// and prints the verdict.
func tracePts(rows []lifts.StreamPoint, cfg lifts.DetectorConfig, tStart, tEnd time.Time) {
	rides := lifts.DetectHeuristic(rows, cfg)
	fmt.Printf("Total samples: %d, detected rides: %d\n", len(rows), len(rides))
	for i, r := range rides {
		// Find end-position by locating the row at EndedAt time.
		var endLat, endLng, endAlt float64
		for k := len(rows) - 1; k >= 0; k-- {
			if !rows[k].Time.After(r.EndedAt) {
				if rows[k].Lat != nil {
					endLat = *rows[k].Lat
				}
				if rows[k].Lng != nil {
					endLng = *rows[k].Lng
				}
				if rows[k].AltitudeM != nil {
					endAlt = *rows[k].AltitudeM
				}
				break
			}
		}
		fmt.Printf("  ride[%02d] start=%s dur=%ds asc=%.1f desc=%.1f conf=%s end=(%.4f,%.4f,%.0fm)\n",
			i, r.StartedAt.Format("15:04:05"), r.DurationS, r.AscentM, r.DescentM, r.Confidence,
			endLat, endLng, endAlt)
	}
	if !tStart.IsZero() {
		fmt.Println("\nTrace at indices in [start, end]:")
		for i := 0; i < len(rows); i++ {
			if rows[i].Time.Before(tStart) || (!tEnd.IsZero() && rows[i].Time.After(tEnd)) {
				continue
			}
			st, fe, k, fin, valid, ok := lifts.PublicTryRide(rows, i, cfg)
			if !ok {
				continue
			}
			fmt.Printf("  i=%d t=%s win=[%d→%d] kind=%q valid=%v dur=%.0fs alt=%+.1f mean=%.2f cv=%.3f sin=%.3f rate=%.3f speeds=%v\n",
				i, rows[i].Time.Format("15:04:05"), st, fe, k, valid,
				fin.DurationS, fin.AltPos-fin.AltNeg, fin.MeanSpeed, fin.SpeedCV, fin.Sinuosity, fin.AltRateMs, fin.Speeds)
			i = fe // skip ahead
		}
	}
	_ = math.Inf
}
