package main

import (
	"fmt"
	"io"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

// summarize prints one line per N-second bucket:
// HH:MM:SS  alt=XX (Δ+YY)  spd_avg=Z.Z  hav_per_chunk=Q.Q  sin=R
// to give a human-readable view of the altitude/speed profile suitable for
// matching against a screenshot.
func summarize(w io.Writer, pts []lifts.StreamPoint, bucketSecs int) error {
	if len(pts) == 0 {
		return nil
	}
	type bucket struct {
		t        time.Time
		altStart float64
		altEnd   float64
		altMin   float64
		altMax   float64
		distSum  float64
		spdSum   float64
		spdN     int
		samples  int
		latStart float64
		lngStart float64
		latEnd   float64
		lngEnd   float64
		hasGeo   bool
	}
	buckets := []bucket{}
	cur := bucket{}
	bucketStart := pts[0].Time
	for i, p := range pts {
		if i == 0 {
			cur.t = p.Time
			if p.AltitudeM != nil {
				cur.altStart = *p.AltitudeM
				cur.altMin = *p.AltitudeM
				cur.altMax = *p.AltitudeM
			}
			if p.Lat != nil {
				cur.latStart, cur.lngStart = *p.Lat, *p.Lng
				cur.hasGeo = true
			}
		}
		if p.Time.Sub(bucketStart).Seconds() >= float64(bucketSecs) {
			buckets = append(buckets, cur)
			cur = bucket{t: p.Time}
			bucketStart = p.Time
			if p.AltitudeM != nil {
				cur.altStart = *p.AltitudeM
				cur.altMin = *p.AltitudeM
				cur.altMax = *p.AltitudeM
			}
			if p.Lat != nil {
				cur.latStart, cur.lngStart = *p.Lat, *p.Lng
				cur.hasGeo = true
			}
		}
		if p.AltitudeM != nil {
			cur.altEnd = *p.AltitudeM
			if *p.AltitudeM < cur.altMin {
				cur.altMin = *p.AltitudeM
			}
			if *p.AltitudeM > cur.altMax {
				cur.altMax = *p.AltitudeM
			}
		}
		if p.Lat != nil {
			cur.latEnd, cur.lngEnd = *p.Lat, *p.Lng
		}
		if p.SpeedMs != nil {
			cur.spdSum += *p.SpeedMs
			cur.spdN++
		}
		if i > 0 && pts[i-1].Lat != nil && p.Lat != nil {
			cur.distSum += lifts.Haversine(*pts[i-1].Lat, *pts[i-1].Lng, *p.Lat, *p.Lng)
		}
		cur.samples++
	}
	buckets = append(buckets, cur)

	fmt.Fprintf(w, "time      alt   Δ      spd  havKm/h sin   note\n")
	for i, b := range buckets {
		altDelta := b.altEnd - b.altStart
		spdAvg := 0.0
		if b.spdN > 0 {
			spdAvg = b.spdSum / float64(b.spdN)
		}
		bucketDur := float64(bucketSecs)
		if i == len(buckets)-1 {
			bucketDur = b.t.Sub(buckets[i-1].t).Seconds()
			if bucketDur <= 0 {
				bucketDur = float64(bucketSecs)
			}
		}
		havSpd := b.distSum / bucketDur
		havKmh := havSpd * 3.6
		straight := 0.0
		if b.hasGeo {
			straight = lifts.Haversine(b.latStart, b.lngStart, b.latEnd, b.lngEnd)
		}
		sin := 0.0
		if straight > 0 {
			sin = b.distSum / straight
		}
		note := ""
		if altDelta > 5 && havSpd > 1.5 && havSpd < 4 && sin > 0 && sin < 1.05 {
			note = "lift?"
		}
		fmt.Fprintf(w, "%s  %4.0f  %+5.1f  %4.1f  %5.1f   %4.2f  %s\n",
			b.t.Format("15:04:05"), b.altEnd, altDelta, spdAvg, havKmh, sin, note)
	}
	return nil
}

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
