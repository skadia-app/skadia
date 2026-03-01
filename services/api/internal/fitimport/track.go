package fitimport

import (
	"fmt"
	"math"
	"strings"

	"github.com/muktihari/fit/profile/mesgdef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractTrackPoints(records []*mesgdef.Record) []model.TrackPoint {
	points := make([]model.TrackPoint, 0, len(records))

	for _, rec := range records {
		if rec.PositionLat == int32(invalidSint32) || rec.PositionLong == int32(invalidSint32) {
			continue
		}

		lat := float64(rec.PositionLat) * semicirclesToDegrees
		lng := float64(rec.PositionLong) * semicirclesToDegrees

		if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
			continue
		}

		// Filter GPS spikes: skip points with 0,0 coordinates
		if math.Abs(lat) < 0.001 && math.Abs(lng) < 0.001 {
			continue
		}

		points = append(points, model.TrackPoint{
			Lat: lat,
			Lng: lng,
		})
	}

	return points
}

// TrackToWKT converts track points to a WKT LineString.
// WKT uses longitude-first order: LINESTRING(lng lat, lng lat, ...)
func TrackToWKT(points []model.TrackPoint) string {
	if len(points) < 2 {
		return ""
	}

	parts := make([]string, len(points))
	for i, p := range points {
		parts[i] = fmt.Sprintf("%f %f", p.Lng, p.Lat)
	}
	return "LINESTRING(" + strings.Join(parts, ",") + ")"
}
