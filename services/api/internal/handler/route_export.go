package handler

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/routing"
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

	// Spec Rev 2.3 §6.3 Option A: parse persisted elevation profile once.
	// Used below for defensive coord[2] override on non-offroad segments.
	profile := parsePersistedProfile(route.ElevationProfile)

	if hasSegmentGeometry {
		// Per-segment trkseg path.
		for i := range route.Segments {
			seg := &route.Segments[i]
			fmt.Fprint(w, `    <trkseg>
`)
			if seg.Geometry != nil {
				isOffroad := seg.Mode == "offroad"
				// Defensive override: for non-offroad segments with an available
				// profile, replace coord[2] values via cumulative-Haversine
				// interpolation. Covers legacy routes (pre-Phase-1) where
				// elevation_profile was Lazy-Filled but segment coords in the DB
				// still carry stale carriedEle values. Happy-path (post-Phase-1
				// saved routes) is unaffected because coord[2] is already correct.
				// Offroad segments are skipped — they carry their own estimated
				// elevation source and must retain skadia:ele_estimated semantics.
				coords := seg.Geometry.Coordinates
				if profile != nil && !isOffroad && len(coords) > 0 {
					coords = overrideCoord2FromProfile(coords, profile)
				}
				for _, coord := range coords {
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
				// Spec Rev 2.3 §6.3 Option A: defensive override for legacy path too.
				// No seg.Mode available here — apply profile unconditionally (no offroad
				// segments exist in the legacy single-trkseg path).
				coords := ls.Coordinates
				if profile != nil && len(coords) > 0 {
					coords = overrideCoord2FromProfile(coords, profile)
				}
				for _, coord := range coords {
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

// parsePersistedProfile decodes route.ElevationProfile (json.RawMessage from
// the planned_routes.elevation_profile JSONB column, Migration 000325) into
// []routing.ElevationPoint. Returns nil on null/empty/parse-error so callers
// can use a simple nil-check before applying the profile.
func parsePersistedProfile(raw json.RawMessage) []routing.ElevationPoint {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var pts []routing.ElevationPoint
	if err := json.Unmarshal(raw, &pts); err != nil || len(pts) == 0 {
		return nil
	}
	return pts
}

// overrideCoord2FromProfile returns a copy of coords where each element's
// coord[2] (elevation) is replaced by a value linearly interpolated from
// profile at the cumulative Haversine distance of that point.
//
// Spec Rev 2.3 §6.3 Option A. Works on [][]float64 (GeoJSON coordinate
// format: [lon, lat, ele]) rather than the routing-internal LatLonEle slice,
// so it mirrors routing.applyProfileToCoords semantically but operates at the
// handler layer without touching routing internals.
//
// The original coords slice is never mutated; callers receive a fresh slice.
func overrideCoord2FromProfile(coords [][]float64, profile []routing.ElevationPoint) [][]float64 {
	if len(coords) == 0 || len(profile) == 0 {
		return coords
	}
	out := make([][]float64, len(coords))
	var distAccum float64
	for i, c := range coords {
		// Defensive copy: ensure at least 3 elements so coord[2] is always valid.
		nc := make([]float64, 3)
		copy(nc, c)
		if i > 0 {
			// GeoJSON order: c[0]=lon, c[1]=lat.
			// geo.DistanceHaversine expects (lat, lon).
			distAccum += geo.DistanceHaversine(
				coords[i-1][1], coords[i-1][0], // prev: lat, lon
				c[1], c[0],                     // curr: lat, lon
			)
		}
		nc[2] = routing.InterpolateProfileEleAt(profile, distAccum)
		out[i] = nc
	}
	return out
}
