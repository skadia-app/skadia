package handler

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"html"
	"io"
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
		w.Header().Set("Content-Type", "application/geo+json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.geojson"`, safeName))
		w.WriteHeader(http.StatusOK)
		writeSegmentGeoJSON(w, seg.Name, seg.DistanceM, seg.ElevationGainM, *seg.TrackGeoJSON)

	case "gpx":
		w.Header().Set("Content-Type", "application/gpx+xml; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.gpx"`, safeName))
		w.WriteHeader(http.StatusOK)
		writeSegmentGPX(w, seg.Name, *seg.TrackGeoJSON)

	default:
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unsupported format, use gpx or geojson", r.URL.Path)
	}
}

// ExportSegments handles POST /api/v1/segments/export
// Body: {"ids": [...], "format": "gpx"|"geojson"}
// Returns a ZIP archive containing one file per segment.
func (h *Handler) ExportSegments(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var body struct {
		IDs    []string `json:"ids"`
		Format string   `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}
	if len(body.IDs) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "ids must not be empty", r.URL.Path)
		return
	}
	if len(body.IDs) > 500 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "too many ids, max 500", r.URL.Path)
		return
	}
	format := body.Format
	if format == "" {
		format = "gpx"
	}
	if format != "gpx" && format != "geojson" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unsupported format, use gpx or geojson", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="segments.zip"`)
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	defer zw.Close()

	// Track used filenames to handle duplicates.
	usedNames := make(map[string]int)

	for _, id := range body.IDs {
		seg, err := h.DB.GetSegment(r.Context(), id, userID, false)
		if err != nil || seg == nil {
			continue
		}
		if seg.TrackGeoJSON == nil || *seg.TrackGeoJSON == "" {
			continue
		}

		baseName := sanitizeFilename(seg.Name)
		if baseName == "" {
			baseName = "segment"
		}

		// Build a unique filename within the archive.
		key := baseName
		count := usedNames[key]
		usedNames[key]++
		var entryName string
		if count == 0 {
			entryName = baseName + "." + format
		} else {
			entryName = fmt.Sprintf("%s_%d.%s", baseName, count, format)
		}

		entry, err := zw.Create(entryName)
		if err != nil {
			continue
		}

		switch format {
		case "gpx":
			writeSegmentGPX(entry, seg.Name, *seg.TrackGeoJSON)
		case "geojson":
			writeSegmentGeoJSON(entry, seg.Name, seg.DistanceM, seg.ElevationGainM, *seg.TrackGeoJSON)
		}
	}
}

// writeSegmentGPX renders a GPX 1.1 document for a segment track to w.
func writeSegmentGPX(w io.Writer, name, trackGeoJSON string) {
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

// writeSegmentGeoJSON renders a GeoJSON Feature for a segment track to w.
func writeSegmentGeoJSON(w io.Writer, name string, distanceM, elevationGainM *float64, trackGeoJSON string) {
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
			Name:           name,
			DistanceM:      distanceM,
			ElevationGainM: elevationGainM,
		},
		Geometry: json.RawMessage(trackGeoJSON),
	}
	_ = json.NewEncoder(w).Encode(f)
}
