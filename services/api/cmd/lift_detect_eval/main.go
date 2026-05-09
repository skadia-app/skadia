package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

func main() {
	manifestPath := flag.String("manifest", "", "local evaluation manifest")
	root := flag.String("root", "", "directory used to resolve manifest case files; default: manifest directory")
	verbose := flag.Bool("v", false, "")
	dump := flag.String("dump", "", "print CSV of stream points for the given file (relative to -root) and exit")
	summary := flag.String("summary", "", "print 30s-bucket summary for the given file (relative to -root) and exit")
	rides := flag.String("rides", "", "print CSV of detected ride boundaries (idx,start_t,end_t,dur_s,asc_m,desc_m,conf,start_lat,start_lng,end_lat,end_lng) for the given file and exit")
	boundaries := flag.String("boundaries", "", "print per-ride boundary-offset table (best matching GT lift, both endpoint orientations, ✓/○/⚠/✗ tolerance band) for the given file and exit; requires -gt and optionally -gt-key")
	gtPath := flag.String("gt", "", "path to local station-marker YAML (e.g. local-station-markers.yaml); used by -boundaries")
	gtKey := flag.String("gt-key", "", "case.file key in the GT YAML; defaults to the value of -boundaries (relative to -root)")
	sport := flag.String("sport", "ski", "sport profile for -rides / -boundaries (ignored in -dump/-summary)")
	dbURL := flag.String("db", "", "postgres URL; when set together with --snap-stats, runs the OSM-snap pipeline against the live osm_lifts table for empirical action/coverage stats")
	snapStats := flag.Bool("snap-stats", false, "after eval, run the snap pipeline against osm_lifts and print the per-region action/coverage table (requires --db)")
	snapStatsJSON := flag.Bool("snap-stats-json", false, "instead of the table, emit the snap-stats result as JSON on stdout (implies --snap-stats)")
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

	if *boundaries != "" {
		full := filepath.Join(*root, *boundaries)
		var (
			pts []lifts.StreamPoint
			err error
		)
		switch strings.ToLower(filepath.Ext(*boundaries)) {
		case ".fit":
			pts, err = loadFITPoints(full)
		case ".gpx":
			pts, err = loadGPXPoints(full)
		default:
			fmt.Fprintf(os.Stderr, "unsupported extension: %s\n", *boundaries)
			os.Exit(2)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if *gtPath == "" {
			fmt.Fprintln(os.Stderr, "-boundaries requires -gt <station-marker yaml>")
			os.Exit(2)
		}
		key := *gtKey
		if key == "" {
			key = *boundaries
		}
		gtCase, err := loadBoundCase(*gtPath, key)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		detected := lifts.DetectHeuristic(pts, lifts.DefaultDetectorConfig(*sport))
		eps := make([]rideEndpoints, 0, len(detected))
		for i, r := range detected {
			ep := rideEndpoints{idx: i, durS: r.DurationS, ascM: r.AscentM, descM: r.DescentM, conf: r.Confidence}
			for _, p := range pts {
				if p.Time.Before(r.StartedAt) {
					continue
				}
				if p.Lat != nil && p.Lng != nil {
					ep.sLat, ep.sLng = *p.Lat, *p.Lng
					break
				}
			}
			for k := len(pts) - 1; k >= 0; k-- {
				if pts[k].Time.After(r.EndedAt) {
					continue
				}
				if pts[k].Lat != nil && pts[k].Lng != nil {
					ep.eLat, ep.eLng = *pts[k].Lat, *pts[k].Lng
					break
				}
			}
			eps = append(eps, ep)
		}
		printBoundaries(os.Stdout, eps, gtCase.Lifts)
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

	if *manifestPath == "" {
		fmt.Fprintln(os.Stderr, "need -manifest for aggregate eval")
		os.Exit(2)
	}
	if *root == "" {
		*root = filepath.Dir(*manifestPath)
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

	// In --snap-stats-json mode stdout is reserved for the machine-readable
	// JSON document; the human-readable eval report goes to stderr so it
	// stays visible without contaminating the parsed JSON. Default mode keeps
	// the historical contract (eval report on stdout).
	var exit int
	if *snapStatsJSON {
		exit = printReportTo(results, *verbose, os.Stderr)
	} else {
		exit = printReport(results, *verbose)
	}

	// --snap-stats: run the OSM-snap pipeline against the live osm_lifts table
	// and aggregate decision counts + coverage histogram. The eval pass/fail
	// (exit code) is unaffected — Stage-3-double-count investigation runs in
	// addition to the existing recall/precision gate.
	if *snapStats || *snapStatsJSON {
		if *dbURL == "" {
			fmt.Fprintln(os.Stderr, "--snap-stats requires --db <postgres-url>")
			os.Exit(2)
		}
		ctx := context.Background()
		if *snapStatsJSON {
			// JSON to stdout, no table output (the table-writer is unused).
			if err := runSnapStats(ctx, gt, *root, *dbURL, os.Stdout, os.Stderr); err != nil {
				fmt.Fprintf(os.Stderr, "snap-stats: %v\n", err)
				os.Exit(2)
			}
		} else {
			// Table output to stdout, no JSON.
			if err := runSnapStats(ctx, gt, *root, *dbURL, nil, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "snap-stats: %v\n", err)
				os.Exit(2)
			}
		}
	}

	os.Exit(exit)
}
