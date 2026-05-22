package handler

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ExportSegment handles GET /api/v1/segments/{id}/export?format=gpx|geojson
func (h *Handler) ExportSegment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	isAdmin := middleware.GetIsAdmin(r.Context())
	scopes := h.resolveVisibleAccessScopesForRequest(w, r, userID)
	if scopes == nil {
		return
	}

	segID := chi.URLParam(r, "id")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "gpx"
	}

	seg, err := h.DB.GetSegmentWithOptions(r.Context(), segID, userID, isAdmin, database.GetSegmentOptions{
		IncludeGeoJSON:        true,
		IncludeControls:       true,
		VisibleAccessScopeIDs: scopes,
	})
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
		checkpointMeta, err := segmentCheckpointExportMetadata(r.Context(), h.DB, seg)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load checkpoint export metadata", r.URL.Path)
			return
		}
		w.Header().Set("Content-Type", "application/geo+json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.geojson"`, safeName))
		w.WriteHeader(http.StatusOK)
		writeSegmentGeoJSONWithMetadata(w, seg, checkpointMeta)

	case "gpx":
		w.Header().Set("Content-Type", "application/gpx+xml; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.gpx"`, safeName))
		w.WriteHeader(http.StatusOK)
		writeSegmentGPX(w, seg)

	default:
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unsupported format, use gpx or geojson", r.URL.Path)
	}
}

// ExportSegments handles POST /api/v1/segments/export
// Body: {"ids": [...], "format": "gpx"|"geojson"}
// Returns a ZIP archive containing one file per segment.
func (h *Handler) ExportSegments(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	isAdmin := middleware.GetIsAdmin(r.Context())
	scopes := h.resolveVisibleAccessScopesForRequest(w, r, userID)
	if scopes == nil {
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

	segments, err := h.DB.GetSegmentsForExportWithAccess(r.Context(), body.IDs, userID, isAdmin, scopes)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch segments", r.URL.Path)
		return
	}
	checkpointMetaBySegmentID, err := checkpointExportMetadataBySegmentID(r.Context(), h.DB, segments)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load checkpoint export metadata", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="segments.zip"`)
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	defer zw.Close()

	// Track used filenames to handle duplicates.
	usedNames := make(map[string]int)

	for i := range segments {
		seg := &segments[i]

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
			writeSegmentGPX(entry, seg)
		case "geojson":
			writeSegmentGeoJSONWithMetadata(entry, seg, checkpointMetaBySegmentID[seg.ID])
		}
	}
}

// writeSegmentGPX renders a GPX 1.1 document for a segment track to w.
func writeSegmentGPX(w io.Writer, seg *model.Segment) {
	escapedName := html.EscapeString(seg.Name)

	if seg.CourseKind == string(model.CourseKindCheckpointSequence) {
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="Skadia" xmlns="http://www.topografix.com/GPX/1/1">
  <metadata>
    <name>%s</name>
  </metadata>
`, escapedName)
		for _, control := range seg.Controls {
			lon, lat, ok := checkpointControlExportPoint(control)
			if !ok {
				continue
			}
			label := html.EscapeString(checkpointControlExportLabel(control))
			fmt.Fprintf(w, `  <wpt lat="%.7f" lon="%.7f"><name>%s</name></wpt>
`, lat, lon, label)
		}
		fmt.Fprintf(w, `  <rte>
    <name>%s</name>
`, escapedName)
		for _, control := range seg.Controls {
			lon, lat, ok := checkpointControlExportPoint(control)
			if !ok {
				continue
			}
			label := html.EscapeString(checkpointControlExportLabel(control))
			fmt.Fprintf(w, `    <rtept lat="%.7f" lon="%.7f"><name>%s</name></rtept>
`, lat, lon, label)
		}
		fmt.Fprint(w, `  </rte>
</gpx>
`)
		return
	}

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
	if seg.TrackGeoJSON != nil {
		_ = json.Unmarshal([]byte(*seg.TrackGeoJSON), &ls)
	}
	if ls.Type == "LineString" {
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
func writeSegmentGeoJSON(w io.Writer, seg *model.Segment) {
	writeSegmentGeoJSONWithMetadata(w, seg, nil)
}

func segmentCheckpointExportMetadata(ctx context.Context, db *database.DB, seg *model.Segment) (*database.CheckpointCourseExportMetadata, error) {
	if seg.CourseKind == string(model.CourseKindCheckpointSequence) {
		return db.GetCheckpointCourseExportMetadata(ctx, seg.ID)
	}
	return nil, nil
}

func checkpointExportMetadataBySegmentID(ctx context.Context, db *database.DB, segments []model.Segment) (map[string]*database.CheckpointCourseExportMetadata, error) {
	metaBySegmentID := make(map[string]*database.CheckpointCourseExportMetadata)
	for i := range segments {
		seg := &segments[i]
		meta, err := segmentCheckpointExportMetadata(ctx, db, seg)
		if err != nil {
			return nil, err
		}
		if meta != nil {
			metaBySegmentID[seg.ID] = meta
		}
	}
	return metaBySegmentID, nil
}

func writeSegmentGeoJSONWithMetadata(w io.Writer, seg *model.Segment, checkpointMeta *database.CheckpointCourseExportMetadata) {
	if seg.CourseKind == string(model.CourseKindCheckpointSequence) {
		writeCheckpointSegmentGeoJSON(w, seg, checkpointMeta)
		return
	}
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
	_ = json.NewEncoder(w).Encode(f)
}

func writeCheckpointSegmentGeoJSON(w io.Writer, seg *model.Segment, meta *database.CheckpointCourseExportMetadata) {
	type feature struct {
		Type       string          `json:"type"`
		Properties map[string]any  `json:"properties"`
		Geometry   json.RawMessage `json:"geometry"`
	}
	features := make([]feature, 0, len(seg.Controls)+1)
	if seg.TrackGeoJSON != nil {
		features = append(features, feature{
			Type: "Feature",
			Properties: map[string]any{
				"name":                      seg.Name,
				"sport_group":               seg.SportGroup,
				"course_kind":               string(model.CourseKindCheckpointSequence),
				"matching_contract_key":     database.CheckpointSequenceV1Key,
				"matching_contract_version": database.CheckpointSequenceV1Version,
				"display_only":              true,
				"not_for_matching":          true,
			},
			Geometry: json.RawMessage(*seg.TrackGeoJSON),
		})
	}
	for _, control := range seg.Controls {
		props := map[string]any{
			"name":                      seg.Name,
			"sport_group":               seg.SportGroup,
			"course_kind":               string(model.CourseKindCheckpointSequence),
			"matching_contract_key":     database.CheckpointSequenceV1Key,
			"matching_contract_version": database.CheckpointSequenceV1Version,
			"import_semantics":          "private_copy_rebased_on_import",
			"sequence_index":            control.SequenceIndex,
			"role":                      control.Role,
			"primitive":                 control.Primitive,
			"geometry_shape":            control.GeometryShape,
			"required":                  control.Required,
		}
		if meta != nil {
			props["source_valid_from_at"] = meta.ValidFromAt
			props["source_valid_until_at"] = meta.ValidUntilAt
			props["source_validity_basis"] = meta.ValidityBasis
			props["source_eligibility_policy_key"] = meta.EligibilityPolicyKey
			props["source_eligibility_policy_version"] = meta.EligibilityPolicyVersion
			props["source_eligibility_source_hash"] = meta.EligibilitySourceHash
		}
		if control.Label != "" {
			props["label"] = control.Label
		}
		if control.HitRadiusM != nil {
			props["hit_radius_m"] = *control.HitRadiusM
		}
		if control.AreaRadiusM != nil {
			props["area_radius_m"] = *control.AreaRadiusM
		}
		features = append(features, feature{
			Type:       "Feature",
			Properties: props,
			Geometry:   control.Geometry,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":     "FeatureCollection",
		"features": features,
	})
}

func checkpointControlExportLabel(control model.CheckpointControl) string {
	if control.Label != "" {
		return control.Label
	}
	switch control.Role {
	case model.CheckpointControlStart:
		return "Start"
	case model.CheckpointControlFinish:
		return "Ziel"
	default:
		return fmt.Sprintf("%d", control.SequenceIndex+1)
	}
}

func checkpointControlExportPoint(control model.CheckpointControl) (float64, float64, bool) {
	var geom struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal(control.Geometry, &geom); err != nil {
		return 0, 0, false
	}
	if geom.Type == "Point" {
		var coords []float64
		if err := json.Unmarshal(geom.Coordinates, &coords); err != nil || len(coords) < 2 {
			return 0, 0, false
		}
		return coords[0], coords[1], true
	}
	if geom.Type == "Polygon" {
		var polygon [][][]float64
		if err := json.Unmarshal(geom.Coordinates, &polygon); err != nil || len(polygon) == 0 || len(polygon[0]) == 0 {
			return 0, 0, false
		}
		return averageGeoJSONRing(polygon[0])
	}
	if geom.Type == "MultiPolygon" {
		var multi [][][][]float64
		if err := json.Unmarshal(geom.Coordinates, &multi); err != nil || len(multi) == 0 || len(multi[0]) == 0 || len(multi[0][0]) == 0 {
			return 0, 0, false
		}
		return averageGeoJSONRing(multi[0][0])
	}
	return 0, 0, false
}

func averageGeoJSONRing(ring [][]float64) (float64, float64, bool) {
	points := ring
	if len(points) > 1 {
		first := points[0]
		last := points[len(points)-1]
		if len(first) >= 2 && len(last) >= 2 && first[0] == last[0] && first[1] == last[1] {
			points = points[:len(points)-1]
		}
	}
	if len(points) == 0 {
		return 0, 0, false
	}
	var lonSum, latSum float64
	for _, point := range points {
		if len(point) < 2 {
			return 0, 0, false
		}
		lonSum += point[0]
		latSum += point[1]
	}
	return lonSum / float64(len(points)), latSum / float64(len(points)), true
}
