package main

import (
	"fmt"
	"io"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

// dumpCSV writes a CSV of all stream points to w for GT-Audit use.
// Columns: time, lat, lng, alt, speed, distance — nil fields are written as empty.
func dumpCSV(w io.Writer, pts []lifts.StreamPoint) error {
	if _, err := fmt.Fprintln(w, "time,lat,lng,alt_m,speed_ms,distance_m"); err != nil {
		return err
	}
	for _, p := range pts {
		lat := nilF64(p.Lat)
		lng := nilF64(p.Lng)
		alt := nilF64(p.AltitudeM)
		spd := nilF64(p.SpeedMs)
		dist := nilF64(p.DistanceM)
		if _, err := fmt.Fprintf(w, "%s,%s,%s,%s,%s,%s\n",
			p.Time.UTC().Format("2006-01-02T15:04:05Z"),
			lat, lng, alt, spd, dist,
		); err != nil {
			return err
		}
	}
	return nil
}

func nilF64(f *float64) string {
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%.6f", *f)
}
