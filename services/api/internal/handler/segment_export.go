package handler

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// ExportSegment handles GET /api/v1/segments/{id}/export?format=gpx|geojson
func (h *Handler) ExportSegment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	segID := chi.URLParam(r, "id")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "gpx"
	}

	seg, err := h.DB.GetSegment(r.Context(), segID, userID, false)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	if seg.TrackGeoJSON == nil || *seg.TrackGeoJSON == "" {
		httputil.WriteError(w, http.StatusUnprocessableEntity, "No Track Data", "segment has no track geometry", r.URL.Path)
		return
	}

	safeName := sanitizeFilename(seg.Name)
	if safeName == "" {
		safeName = "segment"
	}

	switch format {
	case "geojson":
		type featureProperties struct {
			Name           string   `json:"name"`
			DistanceM      *float64 `json:"distance_m,omitempty"`
			ElevationGainM *float64 `json:"elevation_gain_m,omitempty"`
		}
		type feature struct {
			Type       string            `json:"type"`
			Properties featureProperties `json:"properties"`
			Geometry   json.RawMessage   `json:"geometry"`
		}
		f := feature{
			Type: "Feature",
			Properties: featureProperties{
				Name:           seg.Name,
				DistanceM:      seg.DistanceM,
				ElevationGainM: seg.ElevationGainM,
			},
			Geometry: json.RawMessage(*seg.TrackGeoJSON),
		}
		w.Header().Set("Content-Type", "application/geo+json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.geojson"`, safeName))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(f)

	case "gpx":
		w.Header().Set("Content-Type", "application/gpx+xml; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.gpx"`, safeName))
		w.WriteHeader(http.StatusOK)
		writeSegmentGPX(w, seg.Name, *seg.TrackGeoJSON)

	default:
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unsupported format, use gpx or geojson", r.URL.Path)
	}
}

// writeSegmentGPX renders a GPX 1.1 document for a segment track to w.
func writeSegmentGPX(w http.ResponseWriter, name, trackGeoJSON string) {
	escapedName := html.EscapeString(name)

	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="Skadia" xmlns="http://www.topografix.com/GPX/1/1">
  <metadata>
    <name>%s</name>
  </metadata>
  <trk>
    <name>%s</name>
    <trkseg>
`, escapedName, escapedName)

	var ls geoJSONLineCoords
	if err := json.Unmarshal([]byte(trackGeoJSON), &ls); err == nil && ls.Type == "LineString" {
		for _, coord := range ls.Coordinates {
			if len(coord) < 2 {
				continue
			}
			lon, lat := coord[0], coord[1]
			if len(coord) >= 3 {
				fmt.Fprintf(w, `      <trkpt lat="%.7f" lon="%.7f"><ele>%.1f</ele></trkpt>
`, lat, lon, coord[2])
			} else {
				fmt.Fprintf(w, `      <trkpt lat="%.7f" lon="%.7f"/>
`, lat, lon)
			}
		}
	}

	fmt.Fprint(w, `    </trkseg>
  </trk>
</gpx>
`)
}
