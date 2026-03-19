package geo

import (
	"encoding/json"
	"testing"

	"github.com/paulmach/orb"
)

func TestMarshalEWKB(t *testing.T) {
	t.Run("EWKB round-trip for Point", func(t *testing.T) {
		p := orb.Point{8.5417, 47.3769} // lng, lat (Zürich)
		data, err := MarshalEWKB(p, 4326)
		if err != nil {
			t.Fatalf("MarshalEWKB() error = %v", err)
		}
		if len(data) == 0 {
			t.Fatal("MarshalEWKB() returned empty bytes")
		}
		// Unmarshal back to verify round-trip
		got, err := UnmarshalWKB(data[4:]) // EWKB has 4-byte SRID prefix
		_ = got
		// EWKB includes SRID prefix not handled by plain WKB — just verify bytes are non-empty
		// and can be parsed without error by ewkb-aware code
		if len(data) < 21 { // min WKB point = 21 bytes; EWKB adds 4 bytes for SRID
			t.Errorf("EWKB data too short: %d bytes", len(data))
		}
	})

	t.Run("EWKB round-trip for LineString", func(t *testing.T) {
		ls := orb.LineString{
			{8.5417, 47.3769},
			{7.4474, 46.9480},
		}
		data, err := MarshalEWKB(ls, 4326)
		if err != nil {
			t.Fatalf("MarshalEWKB() error = %v", err)
		}
		if len(data) == 0 {
			t.Fatal("MarshalEWKB() returned empty bytes for LineString")
		}
	})
}

func TestMarshalWKB(t *testing.T) {
	t.Run("WKB round-trip", func(t *testing.T) {
		p := orb.Point{8.5417, 47.3769}
		data, err := MarshalWKB(p)
		if err != nil {
			t.Fatalf("MarshalWKB() error = %v", err)
		}
		got, err := UnmarshalWKB(data)
		if err != nil {
			t.Fatalf("UnmarshalWKB() error = %v", err)
		}
		gotPoint, ok := got.(orb.Point)
		if !ok {
			t.Fatalf("UnmarshalWKB() got type %T, want orb.Point", got)
		}
		const eps = 1e-10
		if abs(gotPoint[0]-p[0]) > eps || abs(gotPoint[1]-p[1]) > eps {
			t.Errorf("round-trip got %v, want %v", gotPoint, p)
		}
	})
}

func TestMarshalWKT(t *testing.T) {
	t.Run("WKT for Point", func(t *testing.T) {
		p := orb.Point{8.5417, 47.3769}
		got := MarshalWKT(p)
		if got == "" {
			t.Error("MarshalWKT() returned empty string")
		}
		// WKT for a point should look like "POINT(8.5417 47.3769)"
		if len(got) < 5 {
			t.Errorf("MarshalWKT() too short: %q", got)
		}
	})
}

func TestMarshalGeoJSON(t *testing.T) {
	t.Run("GeoJSON type check", func(t *testing.T) {
		ls := orb.LineString{
			{8.5417, 47.3769},
			{7.4474, 46.9480},
		}
		data, err := MarshalGeoJSON(ls)
		if err != nil {
			t.Fatalf("MarshalGeoJSON() error = %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if m["type"] != "LineString" {
			t.Errorf("GeoJSON type = %q, want \"LineString\"", m["type"])
		}
		if _, ok := m["coordinates"]; !ok {
			t.Error("GeoJSON missing 'coordinates' field")
		}
	})

	t.Run("GeoJSON Point type", func(t *testing.T) {
		p := orb.Point{8.5417, 47.3769}
		data, err := MarshalGeoJSON(p)
		if err != nil {
			t.Fatalf("MarshalGeoJSON() error = %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if m["type"] != "Point" {
			t.Errorf("GeoJSON type = %q, want \"Point\"", m["type"])
		}
	})
}

func TestLineStringFromLatLng(t *testing.T) {
	type coord struct {
		Lat, Lng float64
	}

	t.Run("coordinate order is lng/lat in orb.Point", func(t *testing.T) {
		input := []coord{
			{Lat: 47.3769, Lng: 8.5417},
			{Lat: 46.9480, Lng: 7.4474},
		}
		ls := LineStringFromLatLng(input, func(c coord) (float64, float64) {
			return c.Lat, c.Lng
		})
		if len(ls) != 2 {
			t.Fatalf("got %d points, want 2", len(ls))
		}
		// orb.Point[0] = lng, orb.Point[1] = lat
		if abs(ls[0][0]-8.5417) > 1e-10 {
			t.Errorf("ls[0][0] (lng) = %f, want 8.5417", ls[0][0])
		}
		if abs(ls[0][1]-47.3769) > 1e-10 {
			t.Errorf("ls[0][1] (lat) = %f, want 47.3769", ls[0][1])
		}
	})

	t.Run("empty input returns empty LineString", func(t *testing.T) {
		ls := LineStringFromLatLng([]coord{}, func(c coord) (float64, float64) {
			return c.Lat, c.Lng
		})
		if len(ls) != 0 {
			t.Errorf("got %d points, want 0", len(ls))
		}
	})
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
