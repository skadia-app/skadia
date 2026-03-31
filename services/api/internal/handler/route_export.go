package handler

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ExportRouteGPX handles GET /api/v1/routes/{id}/gpx
// Returns a GPX file for the requested planned route.
// Ownership is enforced via GetRoute (WHERE user_id = $2).
func (h *Handler) ExportRouteGPX(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing route id", r.URL.Path)
		return
	}

	route, err := h.DB.GetRoute(r.Context(), id, userID)
	if httputil.HandleDBErr(w, r, err, "route not found") {
		return
	}

	// Sanitise filename: replace characters problematic in Content-Disposition headers.
	safeName := sanitizeFilename(route.Name)

	w.Header().Set("Content-Type", "application/gpx+xml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.gpx"`, safeName))
	w.WriteHeader(http.StatusOK)

	writeRouteGPX(w, route)
}

// geoJSONLineCoords decodes coordinates from a PostGIS ST_AsGeoJSON LineString.
// Each element is [lon, lat] or [lon, lat, ele].
type geoJSONLineCoords struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

// writeRouteGPX renders a GPX 1.1 document for route directly to w.
// All user-supplied strings are XML-escaped via html.EscapeString (Errata E16).
// When route.Segments carry Geometry data, each segment becomes a separate <trkseg>.
// Offroad segments are annotated with skadia:ele_estimated extensions per trkpt,
// and a <cmt> warning is added to the <trk> when any offroad segment exists.
// Legacy routes without per-segment geometry fall back to a single trkseg from GeometryGeoJSON.
func writeRouteGPX(w io.Writer, route *model.PlannedRoute) {
	escapedName := html.EscapeString(route.Name)

	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="Skadia" xmlns="http://www.topografix.com/GPX/1/1" xmlns:skadia="https://skadia.app/gpx/1/0">
  <metadata>
    <name>%s</name>
  </metadata>
`, escapedName)

	// Named POI waypoints as <wpt> elements.
	for _, wp := range route.Waypoints {
		if wp.POIName == nil {
			continue
		}
		fmt.Fprintf(w, `  <wpt lat="%.7f" lon="%.7f">
    <name>%s</name>
  </wpt>
`, wp.LatLng[0], wp.LatLng[1], html.EscapeString(*wp.POIName))
	}

	// Determine whether any segment has offroad mode and has geometry.
	hasSegmentGeometry := false
	hasOffroad := false
	for i := range route.Segments {
		if route.Segments[i].Geometry != nil {
			hasSegmentGeometry = true
		}
		if route.Segments[i].Mode == "offroad" {
			hasOffroad = true
		}
	}

	fmt.Fprintf(w, `  <trk>
    <name>%s</name>
`, escapedName)

	if hasOffroad {
		fmt.Fprint(w, `    <cmt>Enthält Offroad-Abschnitte — Route vor Begehung auf Karte prüfen</cmt>
`)
	}

	if hasSegmentGeometry {
		// Per-segment trkseg path.
		for i := range route.Segments {
			seg := &route.Segments[i]
			fmt.Fprint(w, `    <trkseg>
`)
			if seg.Geometry != nil {
				isOffroad := seg.Mode == "offroad"
				for _, coord := range seg.Geometry.Coordinates {
					if len(coord) < 2 {
						continue
					}
					lon, lat := coord[0], coord[1]
					if len(coord) >= 3 && coord[2] != 0 {
						if isOffroad {
							fmt.Fprintf(w, `      <trkpt lat="%.7f" lon="%.7f"><ele>%.1f</ele><extensions><skadia:ele_estimated>true</skadia:ele_estimated></extensions></trkpt>
`, lat, lon, coord[2])
						} else {
							fmt.Fprintf(w, `      <trkpt lat="%.7f" lon="%.7f"><ele>%.1f</ele></trkpt>
`, lat, lon, coord[2])
						}
					} else {
						if isOffroad {
							fmt.Fprintf(w, `      <trkpt lat="%.7f" lon="%.7f"><extensions><skadia:ele_estimated>true</skadia:ele_estimated></extensions></trkpt>
`, lat, lon)
						} else {
							fmt.Fprintf(w, `      <trkpt lat="%.7f" lon="%.7f"/>
`, lat, lon)
						}
					}
				}
			}
			fmt.Fprint(w, `    </trkseg>
`)
		}
	} else {
		// Legacy fallback: single trkseg from GeometryGeoJSON.
		fmt.Fprint(w, `    <trkseg>
`)
		if route.GeometryGeoJSON != "" {
			var ls geoJSONLineCoords
			if err := json.Unmarshal([]byte(route.GeometryGeoJSON), &ls); err == nil && ls.Type == "LineString" {
				for _, coord := range ls.Coordinates {
					if len(coord) < 2 {
						continue
					}
					lon, lat := coord[0], coord[1]
					if len(coord) >= 3 && coord[2] != 0 {
						fmt.Fprintf(w, `      <trkpt lat="%.7f" lon="%.7f"><ele>%.1f</ele></trkpt>
`, lat, lon, coord[2])
					} else {
						fmt.Fprintf(w, `      <trkpt lat="%.7f" lon="%.7f"/>
`, lat, lon)
					}
				}
			}
		}
		fmt.Fprint(w, `    </trkseg>
`)
	}

	fmt.Fprint(w, `  </trk>
</gpx>
`)
}
