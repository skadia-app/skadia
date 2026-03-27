package graph

import (
	"math"
	"testing"
)

// cosLat for ~47°N (Innsbruck area).
var testCosLat = math.Cos(47.0 * math.Pi / 180)

// TestPerpendicularDist_PointOnSegment verifies that a point exactly on the
// segment midpoint has zero perpendicular distance.
func TestPerpendicularDist_PointOnSegment(t *testing.T) {
	// Segment from (47.0, 11.0) to (47.0, 11.01) — purely longitudinal.
	lat1, lng1 := 47.0, 11.0
	lat2, lng2 := 47.0, 11.01

	// Query point: midpoint of the segment.
	qLat := 47.0
	qLng := 11.005

	_, _, distM := perpendicularDist(qLat, qLng, lat1, lng1, lat2, lng2, testCosLat)
	if distM > 0.5 {
		t.Errorf("expected dist ~0 for point on segment, got %.4f m", distM)
	}
}

// TestPerpendicularDist_PointOffSegment verifies that a point ~11m north of a
// horizontal segment yields a perpendicular distance in the 8–15m range.
func TestPerpendicularDist_PointOffSegment(t *testing.T) {
	// Segment from (47.000, 11.000) to (47.000, 11.002).
	lat1, lng1 := 47.000, 11.000
	lat2, lng2 := 47.000, 11.002

	// Query point: ~11m north of the midpoint.
	// 1° lat ≈ 111320m → 11m ≈ 0.0001° lat.
	qLat := 47.000 + 0.0001 // ~11.1m north
	qLng := 11.001          // midpoint longitude

	_, _, distM := perpendicularDist(qLat, qLng, lat1, lng1, lat2, lng2, testCosLat)
	if distM < 8 || distM > 15 {
		t.Errorf("expected dist 8–15m for point ~11m off segment, got %.4f m", distM)
	}
}

// TestPerpendicularDist_BeyondSegmentEnd verifies that a point well beyond the
// end of the segment snaps to the endpoint (t=1 clamp), yielding a distance
// in the 50–100m range.
func TestPerpendicularDist_BeyondSegmentEnd(t *testing.T) {
	// Short segment: (47.000, 11.000) to (47.000, 11.001) — ~75m long.
	lat1, lng1 := 47.000, 11.000
	lat2, lng2 := 47.000, 11.001

	// Query point: ~0.001° further east than the endpoint and slightly north.
	qLat := 47.0001
	qLng := 11.002

	pLat, pLng, distM := perpendicularDist(qLat, qLng, lat1, lng1, lat2, lng2, testCosLat)

	// The projected point must equal the segment endpoint (lat2, lng2).
	if math.Abs(pLat-lat2) > 1e-7 || math.Abs(pLng-lng2) > 1e-7 {
		t.Errorf("expected projection at endpoint (%.6f,%.6f), got (%.6f,%.6f)",
			lat2, lng2, pLat, pLng)
	}

	if distM < 50 || distM > 150 {
		t.Errorf("expected dist 50–150m to endpoint beyond segment, got %.4f m", distM)
	}
}

// TestClosestPointOnPolyline_MultiSegment verifies that the correct segment is
// selected when the polyline has three segments and the query point is closest
// to the second segment.
func TestClosestPointOnPolyline_MultiSegment(t *testing.T) {
	// Polyline running east along 47°N:
	//   P0=(47.000,11.000) P1=(47.000,11.010) P2=(47.000,11.020) P3=(47.000,11.030)
	pts := []geoPoint{
		{47.000, 11.000},
		{47.000, 11.010},
		{47.000, 11.020},
		{47.000, 11.030},
	}

	// Query point: ~11m north of segment 2 (between P1 and P2), at longitude 11.015.
	qLat := 47.000 + 0.0001 // ~11m north
	qLng := 11.015

	projLat, projLng, distM := closestPointOnPolyline(qLat, qLng, pts, testCosLat)

	// Projection must lie on segment 2: longitude between 11.010 and 11.020.
	if projLng < 11.010-1e-5 || projLng > 11.020+1e-5 {
		t.Errorf("expected projection on segment 2 (lng 11.010–11.020), got lng=%.6f", projLng)
	}

	// Projected latitude must be very close to the segment latitude (47.000).
	if math.Abs(projLat-47.000) > 1e-5 {
		t.Errorf("expected projected lat ~47.000, got %.6f", projLat)
	}

	// Distance should be ~11m.
	if distM < 8 || distM > 15 {
		t.Errorf("expected dist 8–15m (point ~11m off segment 2), got %.4f m", distM)
	}
}
