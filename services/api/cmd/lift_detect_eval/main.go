package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

func main() {
	manifestPath := flag.String("manifest", "LIFT_ERKENNUNG/ground_truth.yaml", "")
	root := flag.String("root", "LIFT_ERKENNUNG", "")
	verbose := flag.Bool("v", false, "")
	dump := flag.String("dump", "", "print CSV of stream points for the given file (relative to -root) and exit")
	summary := flag.String("summary", "", "print 30s-bucket summary for the given file (relative to -root) and exit")
	rides := flag.String("rides", "", "print CSV of detected ride boundaries (idx,start_t,end_t,dur_s,asc_m,desc_m,conf,start_lat,start_lng,end_lat,end_lng) for the given file and exit")
	sport := flag.String("sport", "ski", "sport profile for -rides mode (ignored in -dump/-summary)")
	flag.Parse()

	if *rides != "" {
		full := filepath.Join(*root, *rides)
		var (
			pts []lifts.StreamPoint
			err error
		)
		switch strings.ToLower(filepath.Ext(*rides)) {
		case ".fit":
			pts, err = loadFITPoints(full)
		case ".gpx":
			pts, err = loadGPXPoints(full)
		default:
			fmt.Fprintf(os.Stderr, "unsupported extension: %s\n", *rides)
			os.Exit(2)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		detected := lifts.DetectHeuristic(pts, lifts.DefaultDetectorConfig(*sport))
		// Header.
		fmt.Println("idx,start_t,end_t,dur_s,asc_m,desc_m,conf,start_lat,start_lng,start_alt,end_lat,end_lng,end_alt")
		for i, r := range detected {
			// Find first stream sample at or after StartedAt with lat/lng for start coord.
			var sLat, sLng, sAlt, eLat, eLng, eAlt float64
			for _, p := range pts {
				if p.Time.Before(r.StartedAt) {
					continue
				}
				if p.Lat != nil && p.Lng != nil {
					sLat = *p.Lat
					sLng = *p.Lng
					if p.AltitudeM != nil {
						sAlt = *p.AltitudeM
					}
					break
				}
			}
			// Find last stream sample at or before EndedAt with lat/lng for end coord.
			for k := len(pts) - 1; k >= 0; k-- {
				if pts[k].Time.After(r.EndedAt) {
					continue
				}
				if pts[k].Lat != nil && pts[k].Lng != nil {
					eLat = *pts[k].Lat
					eLng = *pts[k].Lng
					if pts[k].AltitudeM != nil {
						eAlt = *pts[k].AltitudeM
					}
					break
				}
			}
			fmt.Printf("%d,%s,%s,%d,%.1f,%.1f,%s,%.6f,%.6f,%.1f,%.6f,%.6f,%.1f\n",
				i, r.StartedAt.Format("2006-01-02T15:04:05Z"), r.EndedAt.Format("2006-01-02T15:04:05Z"),
				r.DurationS, r.AscentM, r.DescentM, r.Confidence,
				sLat, sLng, sAlt, eLat, eLng, eAlt)
		}
		return
	}

	// --dump mode: parse one file, print CSV to stdout, exit.
	if *dump != "" {
		full := filepath.Join(*root, *dump)
		var (
			pts []lifts.StreamPoint
			err error
		)
		switch strings.ToLower(filepath.Ext(*dump)) {
		case ".fit":
			pts, err = loadFITPoints(full)
		case ".gpx":
			pts, err = loadGPXPoints(full)
		default:
			fmt.Fprintf(os.Stderr, "unsupported extension: %s\n", *dump)
			os.Exit(2)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := dumpCSV(os.Stdout, pts); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}

	if *summary != "" {
		full := filepath.Join(*root, *summary)
		var (
			pts []lifts.StreamPoint
			err error
		)
		switch strings.ToLower(filepath.Ext(*summary)) {
		case ".fit":
			pts, err = loadFITPoints(full)
		case ".gpx":
			pts, err = loadGPXPoints(full)
		default:
			fmt.Fprintf(os.Stderr, "unsupported extension: %s\n", *summary)
			os.Exit(2)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := summarize(os.Stdout, pts, 30); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}

	gt, err := loadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	results := make([]Result, 0, len(gt.Cases))
	for _, c := range gt.Cases {
		full := filepath.Join(*root, c.File)
		var (
			pts []lifts.StreamPoint
		)
		switch strings.ToLower(filepath.Ext(c.File)) {
		case ".fit":
			pts, err = loadFITPoints(full)
		case ".gpx":
			pts, err = loadGPXPoints(full)
		default:
			err = fmt.Errorf("unsupported extension: %s", c.File)
		}
		if err != nil {
			results = append(results, Result{Case: c, Err: err})
			continue
		}
		rides := lifts.DetectHeuristic(pts, lifts.DefaultDetectorConfig(c.Sport))
		results = append(results, Result{Case: c, Detected: rides})
	}

	exit := printReport(results, *verbose)
	os.Exit(exit)
}
