package geo

import (
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/ewkb"
	"github.com/paulmach/orb/encoding/wkb"
	"github.com/paulmach/orb/encoding/wkt"
	"github.com/paulmach/orb/geojson"
)

// MarshalEWKB encodes a geometry as Extended Well-Known Binary (EWKB) with
// the given SRID. Suitable for direct insertion into PostGIS columns via pgx.
func MarshalEWKB(g orb.Geometry, srid int) ([]byte, error) {
	return ewkb.Marshal(g, srid)
}

// MarshalWKB encodes a geometry as standard Well-Known Binary (WKB).
func MarshalWKB(g orb.Geometry) ([]byte, error) {
	return wkb.Marshal(g)
}

// UnmarshalWKB decodes a geometry from Well-Known Binary (WKB) bytes.
func UnmarshalWKB(data []byte) (orb.Geometry, error) {
	return wkb.Unmarshal(data)
}

// MarshalWKT encodes a geometry as Well-Known Text (WKT) string.
func MarshalWKT(g orb.Geometry) string {
	return wkt.MarshalString(g)
}

// MarshalGeoJSON encodes a geometry as GeoJSON bytes.
func MarshalGeoJSON(g orb.Geometry) ([]byte, error) {
	gj := geojson.NewGeometry(g)
	return gj.MarshalJSON()
}

// LineStringFromLatLng converts a slice of any type T to an orb.LineString,
// using extractFn to obtain (lat, lng) from each element.
// Note: orb.Point is [lng, lat] (X=longitude, Y=latitude).
func LineStringFromLatLng[T any](coords []T, extractFn func(T) (lat, lng float64)) orb.LineString {
	ls := make(orb.LineString, len(coords))
	for i, c := range coords {
		lat, lng := extractFn(c)
		ls[i] = orb.Point{lng, lat}
	}
	return ls
}
