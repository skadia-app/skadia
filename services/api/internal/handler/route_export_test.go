package handler

import (
	"bytes"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestWriteRouteGPX_TrksegSeparation(t *testing.T) {
	route := &model.PlannedRoute{
		Name: "Test Route",
		Segments: []model.RouteSegment{
			{
				Mode: "routed", FromWaypoint: "a", ToWaypoint: "b",
				Geometry: &model.GeoJSONLineString{
					Type:        "LineString",
					Coordinates: [][]float64{{8.0, 47.0, 500}, {8.01, 47.01, 510}},
				},
			},
			{
				Mode: "offroad", FromWaypoint: "b", ToWaypoint: "c",
				Geometry: &model.GeoJSONLineString{
					Type:        "LineString",
					Coordinates: [][]float64{{8.01, 47.01, 510}, {8.02, 47.02, 520}},
				},
			},
			{
				Mode: "routed", FromWaypoint: "c", ToWaypoint: "d",
				Geometry: &model.GeoJSONLineString{
					Type:        "LineString",
					Coordinates: [][]float64{{8.02, 47.02, 520}, {8.03, 47.03, 530}},
				},
			},
		},
	}

	var buf bytes.Buffer
	writeRouteGPX(&buf, route)
	gpx := buf.String()

	count := strings.Count(gpx, "<trkseg>")
	if count != 3 {
		t.Errorf("expected 3 <trkseg> elements, got %d\nGPX:\n%s", count, gpx)
	}
	if !strings.Contains(gpx, "skadia:ele_estimated") {
		t.Error("expected skadia:ele_estimated extension in offroad trkpt")
	}
	if !strings.Contains(gpx, "Offroad-Abschnitte") {
		t.Error("expected offroad warning in <cmt>")
	}
}

func TestWriteRouteGPX_LegacyFallback(t *testing.T) {
	route := &model.PlannedRoute{
		Name:            "Legacy Route",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[8.0,47.0,400],[8.01,47.01,410]]}`,
		Segments: []model.RouteSegment{
			{Mode: "routed", FromWaypoint: "a", ToWaypoint: "b"},
		},
	}

	var buf bytes.Buffer
	writeRouteGPX(&buf, route)
	gpx := buf.String()

	count := strings.Count(gpx, "<trkseg>")
	if count != 1 {
		t.Errorf("legacy fallback: expected 1 <trkseg>, got %d\nGPX:\n%s", count, gpx)
	}
	if strings.Contains(gpx, "skadia:ele_estimated") {
		t.Error("legacy fallback: unexpected skadia:ele_estimated")
	}
}
