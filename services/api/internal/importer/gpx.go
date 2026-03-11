package importer

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
)

// gpxDoc is a minimal GPX document structure supporting both track and route elements.
type gpxDoc struct {
	XMLName xml.Name   `xml:"gpx"`
	Tracks  []gpxTrack `xml:"trk"`
	Routes  []gpxRoute `xml:"rte"`
}

type gpxTrack struct {
	Segments []gpxTrackSeg `xml:"trkseg"`
}

type gpxTrackSeg struct {
	Points []gpxPoint `xml:"trkpt"`
}

type gpxRoute struct {
	Points []gpxPoint `xml:"rtept"`
}

type gpxPoint struct {
	Lat float64 `xml:"lat,attr"`
	Lon float64 `xml:"lon,attr"`
	Ele float64 `xml:"ele"`
}

// GPXToGeoJSON parses GPX data (trk/trkseg/trkpt and rte/rtept) and returns
// a GeoJSON LineString, total distance in meters, elevation gain, and elevation loss.
// Returns 3D coordinates [lng, lat, ele] per RFC 7946 uniform dimensionality.
// Track points are preferred; route points are used as fallback if no track points exist.
func GPXToGeoJSON(data []byte) (geojsonStr string, distM float64, gainM float64, lossM float64, err error) {
	var doc gpxDoc
	if err = xml.Unmarshal(data, &doc); err != nil {
		return "", 0, 0, 0, fmt.Errorf("gpx parse: %w", err)
	}

	// Collect all points — prefer track points, fall back to route points.
	var points []gpxPoint
	for _, trk := range doc.Tracks {
		for _, seg := range trk.Segments {
			points = append(points, seg.Points...)
		}
	}
	if len(points) == 0 {
		for _, rte := range doc.Routes {
			points = append(points, rte.Points...)
		}
	}
	if len(points) < 2 {
		return "", 0, 0, 0, fmt.Errorf("gpx has fewer than 2 track points")
	}

	// Build coordinate array [[lng, lat, ele], ...] per GeoJSON convention.
	coords := make([][]float64, 0, len(points))
	for _, p := range points {
		coords = append(coords, []float64{p.Lon, p.Lat, p.Ele})
	}

	gj := map[string]interface{}{
		"type":        "LineString",
		"coordinates": coords,
	}
	gjBytes, err := json.Marshal(gj)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("geojson marshal: %w", err)
	}

	// Calculate distance (Haversine) and elevation gain/loss.
	for i := 1; i < len(points); i++ {
		distM += HaversineM(points[i-1].Lat, points[i-1].Lon, points[i].Lat, points[i].Lon)
		dEle := points[i].Ele - points[i-1].Ele
		if dEle > 0 {
			gainM += dEle
		} else {
			lossM += -dEle
		}
	}

	return string(gjBytes), distM, gainM, lossM, nil
}
