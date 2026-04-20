package handler

import (
	"errors"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/wkb"

	"github.com/skadia-app/skadia/services/api/internal/routing"
)

// errTooFewCoords ist ein Sentinel-Error für coordsToLineStringWKB bei < 2 Koordinaten.
// Caller unterscheiden ihn via errors.Is, um Approach-only-Routes (normaler Corner-Case)
// von echten WKB-Marshal-Fehlern zu trennen und entsprechend zu loggen.
var errTooFewCoords = errors.New("coordsToLineStringWKB: need >=2 points")

// coordsToLineStringWKB converts LatLonEle coordinates from a RouteResult
// into a WKB-encoded LineString suitable for Postgres ST_GeomFromWKB / pgx
// BYTEA parameters. Returns nil + errTooFewCoords if there are fewer than 2 points
// (Approach-only-Route — normaler Corner-Case). Returns nil + error for WKB marshalling failures.
//
// LatLonEle fields: {Lat, Lon, Ele float64} (verified against result.go:11).
// WKB/PostGIS convention expects [longitude, latitude] → orb.Point{Lon, Lat}.
func coordsToLineStringWKB(coords []routing.LatLonEle) ([]byte, error) {
	if len(coords) < 2 {
		return nil, errTooFewCoords
	}
	pts := make([]orb.Point, len(coords))
	for i, c := range coords {
		pts[i] = orb.Point{c.Lon, c.Lat} // WKB expects [lon, lat] order
	}
	ls := orb.LineString(pts)
	return wkb.Marshal(ls)
}
