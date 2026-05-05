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
	flag.Parse()

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
