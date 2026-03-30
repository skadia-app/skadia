package handler

import (
	"encoding/json"
	"fmt"
	"html"
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
func writeRouteGPX(w http.ResponseWriter, route *model.PlannedRoute) {
	escapedName := html.EscapeString(route.Name)

	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="Skadia" xmlns="http://www.topografix.com/GPX/1/1">
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

	// Track segment built from the stored LineString geometry.
	fmt.Fprintf(w, `  <trk>
    <name>%s</name>
    <trkseg>
`, escapedName)

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
  </trk>
</gpx>
`)
}
