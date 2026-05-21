package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

type checkpointGeoJSONGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

type checkpointEffectiveGeometry struct {
	kind      string
	lng       float64
	lat       float64
	radiusM   float64
	polygon   [][][]float64
	multiPoly [][][][]float64
}

func (db *DB) CreateCheckpointCourse(ctx context.Context, userID string, input *model.CheckpointCourseCreate) (*model.Segment, error) {
	if input == nil {
		return nil, fmt.Errorf("%w: checkpoint course input is required", ErrSegmentValidation)
	}
	visibility := strings.TrimSpace(input.Visibility)
	if visibility == "" {
		visibility = "public"
		if input.SourceActivityID != nil {
			visibility = "private"
		}
	}
	if err := db.validateSegmentAccessScopeForOwner(ctx, visibility, input.AccessScopeID, userID); err != nil {
		return nil, err
	}
	if input.SourceActivityID != nil {
		var owned bool
		if err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM activities WHERE id = $1 AND user_id = $2
			)
		`, *input.SourceActivityID, userID).Scan(&owned); err != nil {
			return nil, fmt.Errorf("check checkpoint source activity ownership: %w", err)
		}
		if !owned {
			return nil, fmt.Errorf("%w: source activity does not belong to user", ErrSegmentValidation)
		}
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin checkpoint course tx: %w", err)
	}
	defer tx.Rollback(ctx)

	course, err := createCheckpointCourseTx(ctx, tx, userID, input, visibility)
	if err != nil {
		return nil, err
	}
	if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, course.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit checkpoint course: %w", err)
	}
	return course, nil
}

func CreateCheckpointCourseTx(ctx context.Context, tx pgx.Tx, userID string, input *model.CheckpointCourseCreate) (*model.Segment, error) {
	visibility := strings.TrimSpace(input.Visibility)
	if visibility == "" {
		visibility = "public"
		if input.SourceActivityID != nil {
			visibility = "private"
		}
	}
	return createCheckpointCourseTx(ctx, tx, userID, input, visibility)
}

func createCheckpointCourseTx(ctx context.Context, tx pgx.Tx, userID string, input *model.CheckpointCourseCreate, visibility string) (*model.Segment, error) {
	if err := ValidateCheckpointControls(input.Controls, true); err != nil {
		return nil, err
	}
	allowedPredicates, err := segmentPredicatesForCreate("standard", input.AllowedPredicates)
	if err != nil {
		return nil, err
	}
	allowedPredicatesJSON, err := marshalSegmentPredicates(allowedPredicates)
	if err != nil {
		return nil, fmt.Errorf("marshal checkpoint predicates: %w", err)
	}
	sportGroup := input.SportGroup
	if sportGroup == "" {
		sportGroup = "generic"
	}
	if allowedPredicates != nil {
		sportGroup = segmentSportGroupFromPredicates(allowedPredicates)
	}
	displayLine, err := checkpointDisplayLineGeoJSON(input.Controls)
	if err != nil {
		return nil, err
	}

	var s model.Segment
	var scannedPredicatesJSON []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO segments (
			created_by, name, description, sport_group, segment_type,
			segment_type_meta, source_activity_id, terrain_type, climb_type,
			elevation_kind, visibility, access_scope_id, allowed_predicates,
			bidirectional, course_kind
		)
		VALUES (
			$1, $2, $3, $4, 'standard',
			'{}'::jsonb, $5, 'standard', 'standard',
			'flat', $6, $7, $8,
			false, 'checkpoint_sequence'
		)
		RETURNING id, created_by, name, description, sport_group,
		          allowed_predicates, segment_type, segment_type_meta,
		          source_activity_id, osm_lift_id, distance_m, elevation_gain_m,
		          effort_count, athlete_count, visibility, access_scope_id,
		          route_hash, created_at, terrain_type, climb_type,
		          elevation_kind, bidirectional, course_kind
	`, userID, input.Name, input.Description, sportGroup, input.SourceActivityID,
		visibility, input.AccessScopeID, allowedPredicatesJSON).Scan(
		&s.ID, &s.CreatedBy, &s.Name, &s.Description, &s.SportGroup,
		&scannedPredicatesJSON, &s.SegmentType, &s.SegmentTypeMeta,
		&s.SourceActivityID, &s.OsmLiftID, &s.DistanceM, &s.ElevationGainM,
		&s.EffortCount, &s.AthleteCount, &s.Visibility, &s.AccessScopeID,
		&s.RouteHash, &s.CreatedAt, &s.TerrainType, &s.ClimbType,
		&s.ElevationKind, &s.Bidirectional, &s.CourseKind,
	)
	if err != nil {
		return nil, fmt.Errorf("insert checkpoint segment: %w", err)
	}
	s.AllowedPredicates, err = decodeSegmentPredicates(scannedPredicatesJSON)
	if err != nil {
		return nil, fmt.Errorf("decode checkpoint segment predicates: %w", err)
	}

	var versionID string
	var returnedTrackGeoJSON string
	err = tx.QueryRow(ctx, `
		WITH g AS (SELECT ST_SetSRID(ST_GeomFromGeoJSON($2), 4326) AS geom)
		INSERT INTO segment_versions (
			segment_id, version_number, track, start_point, end_point, valid_from, created_by,
			matching_contract_key, matching_contract_version,
			eligibility_segment_type, eligibility_sport_group,
			eligibility_allowed_predicates, eligibility_allowed_predicates_set,
			eligibility_bidirectional, eligibility_visibility, eligibility_elevation_kind
		)
		SELECT $1, 1, g.geom, ST_StartPoint(g.geom), ST_EndPoint(g.geom), CURRENT_DATE, $3,
		       'checkpoint_sequence_v1', 1,
		       'standard', $4, $5::jsonb, $6,
		       false, $7, 'flat'
		FROM g
		RETURNING id, ST_AsGeoJSON(track)
	`, s.ID, displayLine, userID, s.SportGroup, allowedPredicatesJSON, allowedPredicates != nil, visibility).Scan(&versionID, &returnedTrackGeoJSON)
	if err != nil {
		return nil, fmt.Errorf("insert checkpoint segment version: %w", err)
	}
	s.TrackGeoJSON = &returnedTrackGeoJSON

	for _, control := range input.Controls {
		if _, err := tx.Exec(ctx, `
			INSERT INTO course_version_controls (
				segment_id, segment_version_id, sequence_index, role, primitive,
				geometry_shape, label, geom, hit_radius_m, area_radius_m, required
			)
			VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, ST_SetSRID(ST_GeomFromGeoJSON($8), 4326), $9, $10, $11
			)
		`, s.ID, versionID, control.SequenceIndex, control.Role, control.Primitive,
			control.GeometryShape, control.Label, string(control.Geometry), control.HitRadiusM,
			control.AreaRadiusM, control.Required); err != nil {
			return nil, fmt.Errorf("insert checkpoint control %d: %w", control.SequenceIndex, err)
		}
	}

	if err := rebuildCheckpointCourseProjectionTx(ctx, tx, s.ID); err != nil {
		return nil, err
	}
	controls, err := getCheckpointControlsTx(ctx, tx, versionID)
	if err != nil {
		return nil, err
	}
	s.Controls = controls
	s.ControlSummary = checkpointControlSummary(controls)
	return &s, nil
}

func (db *DB) GetCheckpointControls(ctx context.Context, segmentVersionID string) ([]model.CheckpointControl, error) {
	return getCheckpointControlsTx(ctx, db.Pool, segmentVersionID)
}

func getCheckpointControlsTx(ctx context.Context, q pgxQuerier, segmentVersionID string) ([]model.CheckpointControl, error) {
	rows, err := q.Query(ctx, `
		SELECT id::text, sequence_index, role, primitive, geometry_shape, label,
		       ST_AsGeoJSON(geom), hit_radius_m, area_radius_m, required
		FROM course_version_controls
		WHERE segment_version_id = $1
		ORDER BY sequence_index
	`, segmentVersionID)
	if err != nil {
		return nil, fmt.Errorf("load checkpoint controls: %w", err)
	}
	defer rows.Close()

	var controls []model.CheckpointControl
	for rows.Next() {
		var control model.CheckpointControl
		var geometry string
		if err := rows.Scan(&control.ID, &control.SequenceIndex, &control.Role, &control.Primitive,
			&control.GeometryShape, &control.Label, &geometry, &control.HitRadiusM,
			&control.AreaRadiusM, &control.Required); err != nil {
			return nil, fmt.Errorf("scan checkpoint control: %w", err)
		}
		control.Geometry = json.RawMessage(geometry)
		controls = append(controls, control)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checkpoint controls: %w", err)
	}
	if controls == nil {
		controls = []model.CheckpointControl{}
	}
	return controls, nil
}

type pgxQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func rebuildCheckpointCourseProjectionTx(ctx context.Context, tx pgx.Tx, segmentID string) error {
	_, err := tx.Exec(ctx, `
		WITH active_version AS (
			SELECT id
			FROM segment_versions
			WHERE segment_id = $1
			  AND matching_contract_key = 'checkpoint_sequence_v1'
			  AND matching_contract_version = 1
			  AND validity_basis <> 'rollback_retired'
			  AND (valid_from_at IS NULL OR valid_from_at <= NOW())
			  AND (valid_until_at IS NULL OR NOW() < valid_until_at)
			ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC,
			         version_number DESC
			LIMIT 1
		),
		controls AS (
			SELECT c.*,
			       CASE
			           WHEN c.geometry_shape IN ('point', 'circle') THEN
			               ST_Buffer(c.geom::geography, COALESCE(c.hit_radius_m, c.area_radius_m, 0))::geometry
			           ELSE c.geom
			       END AS effective_geom
			FROM course_version_controls c
			JOIN active_version av ON av.id = c.segment_version_id
		),
		projected AS (
			SELECT
				$1::uuid AS segment_id,
				(SELECT id FROM active_version) AS active_segment_version_id,
				COUNT(*)::int AS control_count,
				COUNT(*) FILTER (WHERE required)::int AS required_control_count,
				COALESCE(MAX(COALESCE(hit_radius_m, area_radius_m, 0)), 0)::double precision AS max_hit_radius_m,
				ST_Envelope(ST_Collect(effective_geom)) AS display_bounds,
				jsonb_build_object(
					'control_count', COUNT(*)::int,
					'required_control_count', COUNT(*) FILTER (WHERE required)::int,
					'primitive_kinds', COALESCE((SELECT jsonb_agg(p.primitive ORDER BY p.primitive) FROM (SELECT DISTINCT primitive FROM controls) p), '[]'::jsonb),
					'labels', COALESCE(jsonb_agg(label ORDER BY sequence_index) FILTER (WHERE label <> ''), '[]'::jsonb)
				) AS control_summary,
				COALESCE(jsonb_agg(jsonb_build_object(
					'sequence_index', sequence_index,
					'role', role,
					'primitive', primitive,
					'geometry_shape', geometry_shape,
					'label', label,
					'centroid', ST_AsGeoJSON(ST_Centroid(geom))::jsonb,
					'geometry', CASE
						WHEN primitive = 'point' THEN ST_AsGeoJSON(effective_geom)::jsonb
						WHEN primitive = 'area' AND geometry_shape = 'circle' THEN ST_AsGeoJSON(effective_geom)::jsonb
						WHEN primitive = 'area' THEN ST_AsGeoJSON(geom)::jsonb
						ELSE NULL::jsonb
					END,
					'hit_radius_m', hit_radius_m,
					'area_radius_m', area_radius_m,
					'required', required
				) ORDER BY sequence_index), '[]'::jsonb) AS control_map_features
			FROM controls
		)
		INSERT INTO checkpoint_course_projection (
			segment_id, active_segment_version_id, control_count,
			required_control_count, max_hit_radius_m, display_bounds,
			control_summary, control_map_features, updated_at
		)
		SELECT segment_id, active_segment_version_id, control_count,
		       required_control_count, max_hit_radius_m, display_bounds,
		       control_summary, control_map_features, NOW()
		FROM projected
		WHERE active_segment_version_id IS NOT NULL
		  AND control_count > 0
		ON CONFLICT (segment_id)
		DO UPDATE SET
			active_segment_version_id = EXCLUDED.active_segment_version_id,
			control_count = EXCLUDED.control_count,
			required_control_count = EXCLUDED.required_control_count,
			max_hit_radius_m = EXCLUDED.max_hit_radius_m,
			display_bounds = EXCLUDED.display_bounds,
			control_summary = EXCLUDED.control_summary,
			control_map_features = EXCLUDED.control_map_features,
			updated_at = NOW()
	`, segmentID)
	if err != nil {
		return fmt.Errorf("rebuild checkpoint course projection: %w", err)
	}
	return nil
}

func segmentIsCheckpointCourseTx(ctx context.Context, tx pgx.Tx, segmentID string) (bool, error) {
	var courseKind string
	if err := tx.QueryRow(ctx, `SELECT course_kind FROM segments WHERE id = $1`, segmentID).Scan(&courseKind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrSegmentNotFound
		}
		return false, fmt.Errorf("load segment course kind: %w", err)
	}
	return courseKind == string(model.CourseKindCheckpointSequence), nil
}

func ValidateCheckpointControls(controls []model.CheckpointControl, rankable bool) error {
	if rankable && len(controls) < 2 {
		return fmt.Errorf("%w: rankable checkpoint course requires at least start and finish controls", ErrSegmentValidation)
	}
	if len(controls) == 0 {
		return fmt.Errorf("%w: checkpoint controls are required", ErrSegmentValidation)
	}

	effective := make([]checkpointEffectiveGeometry, len(controls))
	for i, control := range controls {
		if control.SequenceIndex != i {
			return fmt.Errorf("%w: sequence_index %d must equal position %d", ErrSegmentValidation, control.SequenceIndex, i)
		}
		if rankable && !control.Required {
			return fmt.Errorf("%w: checkpoint control %d must be required", ErrSegmentValidation, i)
		}
		if i == 0 {
			if control.Role != model.CheckpointControlStart {
				return fmt.Errorf("%w: first control must be start", ErrSegmentValidation)
			}
		} else if i == len(controls)-1 {
			if control.Role != model.CheckpointControlFinish {
				return fmt.Errorf("%w: last control must be finish", ErrSegmentValidation)
			}
		} else if control.Role != model.CheckpointControlCheckpoint {
			return fmt.Errorf("%w: intermediate controls must be checkpoint", ErrSegmentValidation)
		}

		geom, err := validateCheckpointControlGeometry(control)
		if err != nil {
			return err
		}
		effective[i] = geom
	}

	if rankable {
		for i := range effective {
			for j := i + 1; j < len(effective); j++ {
				if checkpointEffectiveGeometriesOverlap(effective[i], effective[j]) {
					return fmt.Errorf("%w: required checkpoint controls %d and %d overlap", ErrSegmentValidation, i, j)
				}
			}
		}
	}

	return nil
}

func validateCheckpointControlGeometry(control model.CheckpointControl) (checkpointEffectiveGeometry, error) {
	var geom checkpointGeoJSONGeometry
	if err := json.Unmarshal(control.Geometry, &geom); err != nil {
		return checkpointEffectiveGeometry{}, fmt.Errorf("%w: invalid control GeoJSON: %v", ErrSegmentValidation, err)
	}

	switch control.Primitive {
	case model.CheckpointControlPoint:
		if control.GeometryShape != model.CheckpointControlGeometryPoint {
			return checkpointEffectiveGeometry{}, fmt.Errorf("%w: point control must use geometry_shape point", ErrSegmentValidation)
		}
		if control.AreaRadiusM != nil {
			return checkpointEffectiveGeometry{}, fmt.Errorf("%w: point control must not set area_radius_m", ErrSegmentValidation)
		}
		if control.HitRadiusM == nil || *control.HitRadiusM <= 0 || *control.HitRadiusM > 5000 {
			return checkpointEffectiveGeometry{}, fmt.Errorf("%w: point control hit_radius_m must be > 0 and <= 5000", ErrSegmentValidation)
		}
		lng, lat, err := checkpointParsePoint(geom)
		if err != nil {
			return checkpointEffectiveGeometry{}, fmt.Errorf("%w: point control requires GeoJSON Point: %v", ErrSegmentValidation, err)
		}
		return checkpointEffectiveGeometry{kind: "circle", lng: lng, lat: lat, radiusM: *control.HitRadiusM}, nil
	case model.CheckpointControlArea:
		if control.HitRadiusM != nil {
			return checkpointEffectiveGeometry{}, fmt.Errorf("%w: area control must not set hit_radius_m", ErrSegmentValidation)
		}
		switch control.GeometryShape {
		case model.CheckpointControlGeometryCircle:
			if control.AreaRadiusM == nil || *control.AreaRadiusM <= 0 {
				return checkpointEffectiveGeometry{}, fmt.Errorf("%w: circle area area_radius_m must be > 0", ErrSegmentValidation)
			}
			lng, lat, err := checkpointParsePoint(geom)
			if err != nil {
				return checkpointEffectiveGeometry{}, fmt.Errorf("%w: circle area requires GeoJSON Point center: %v", ErrSegmentValidation, err)
			}
			return checkpointEffectiveGeometry{kind: "circle", lng: lng, lat: lat, radiusM: *control.AreaRadiusM}, nil
		case model.CheckpointControlGeometryPolygon:
			if control.AreaRadiusM != nil {
				return checkpointEffectiveGeometry{}, fmt.Errorf("%w: polygon area must not set area_radius_m", ErrSegmentValidation)
			}
			polygon, err := checkpointParsePolygon(geom)
			if err != nil {
				return checkpointEffectiveGeometry{}, fmt.Errorf("%w: polygon area requires GeoJSON Polygon: %v", ErrSegmentValidation, err)
			}
			return checkpointEffectiveGeometry{kind: "polygon", polygon: polygon}, nil
		case model.CheckpointControlGeometryMultiPolygon:
			if control.AreaRadiusM != nil {
				return checkpointEffectiveGeometry{}, fmt.Errorf("%w: multipolygon area must not set area_radius_m", ErrSegmentValidation)
			}
			multiPoly, err := checkpointParseMultiPolygon(geom)
			if err != nil {
				return checkpointEffectiveGeometry{}, fmt.Errorf("%w: multipolygon area requires GeoJSON MultiPolygon: %v", ErrSegmentValidation, err)
			}
			return checkpointEffectiveGeometry{kind: "multipolygon", multiPoly: multiPoly}, nil
		default:
			return checkpointEffectiveGeometry{}, fmt.Errorf("%w: unsupported area geometry_shape %q", ErrSegmentValidation, control.GeometryShape)
		}
	default:
		return checkpointEffectiveGeometry{}, fmt.Errorf("%w: unsupported checkpoint primitive %q", ErrSegmentValidation, control.Primitive)
	}
}

func checkpointParsePoint(geom checkpointGeoJSONGeometry) (float64, float64, error) {
	if geom.Type != "Point" {
		return 0, 0, fmt.Errorf("got %s", geom.Type)
	}
	var coords []float64
	if err := json.Unmarshal(geom.Coordinates, &coords); err != nil {
		return 0, 0, err
	}
	if len(coords) < 2 || !checkpointFinite(coords[0]) || !checkpointFinite(coords[1]) {
		return 0, 0, fmt.Errorf("coordinates must contain finite lng/lat")
	}
	return coords[0], coords[1], nil
}

func checkpointParsePolygon(geom checkpointGeoJSONGeometry) ([][][]float64, error) {
	if geom.Type != "Polygon" {
		return nil, fmt.Errorf("got %s", geom.Type)
	}
	var polygon [][][]float64
	if err := json.Unmarshal(geom.Coordinates, &polygon); err != nil {
		return nil, err
	}
	if err := checkpointValidatePolygonRings(polygon); err != nil {
		return nil, err
	}
	return polygon, nil
}

func checkpointParseMultiPolygon(geom checkpointGeoJSONGeometry) ([][][][]float64, error) {
	if geom.Type != "MultiPolygon" {
		return nil, fmt.Errorf("got %s", geom.Type)
	}
	var multiPoly [][][][]float64
	if err := json.Unmarshal(geom.Coordinates, &multiPoly); err != nil {
		return nil, err
	}
	if len(multiPoly) == 0 {
		return nil, fmt.Errorf("multipolygon must contain at least one polygon")
	}
	for _, polygon := range multiPoly {
		if err := checkpointValidatePolygonRings(polygon); err != nil {
			return nil, err
		}
	}
	return multiPoly, nil
}

func checkpointValidatePolygonRings(polygon [][][]float64) error {
	if len(polygon) == 0 {
		return fmt.Errorf("polygon must contain at least one ring")
	}
	for _, ring := range polygon {
		if len(ring) < 4 {
			return fmt.Errorf("polygon ring must contain at least four positions")
		}
		for _, coord := range ring {
			if len(coord) < 2 || !checkpointFinite(coord[0]) || !checkpointFinite(coord[1]) {
				return fmt.Errorf("polygon coordinates must contain finite lng/lat")
			}
		}
		first := ring[0]
		last := ring[len(ring)-1]
		if first[0] != last[0] || first[1] != last[1] {
			return fmt.Errorf("polygon rings must be closed")
		}
	}
	return nil
}

func checkpointDisplayLineGeoJSON(controls []model.CheckpointControl) (string, error) {
	coords := make([][]float64, 0, len(controls))
	for _, control := range controls {
		lng, lat, err := checkpointControlCentroid(control)
		if err != nil {
			return "", err
		}
		coords = append(coords, []float64{lng, lat})
	}
	raw, err := json.Marshal(map[string]any{
		"type":        "LineString",
		"coordinates": coords,
	})
	if err != nil {
		return "", fmt.Errorf("marshal checkpoint display line: %w", err)
	}
	return string(raw), nil
}

func checkpointControlCentroid(control model.CheckpointControl) (float64, float64, error) {
	var geom checkpointGeoJSONGeometry
	if err := json.Unmarshal(control.Geometry, &geom); err != nil {
		return 0, 0, fmt.Errorf("%w: invalid checkpoint control geometry: %v", ErrSegmentValidation, err)
	}
	switch control.GeometryShape {
	case model.CheckpointControlGeometryPoint, model.CheckpointControlGeometryCircle:
		return checkpointParsePoint(geom)
	case model.CheckpointControlGeometryPolygon:
		polygon, err := checkpointParsePolygon(geom)
		if err != nil {
			return 0, 0, fmt.Errorf("%w: invalid polygon centroid source: %v", ErrSegmentValidation, err)
		}
		return checkpointRingAverage(polygon[0])
	case model.CheckpointControlGeometryMultiPolygon:
		multiPoly, err := checkpointParseMultiPolygon(geom)
		if err != nil {
			return 0, 0, fmt.Errorf("%w: invalid multipolygon centroid source: %v", ErrSegmentValidation, err)
		}
		return checkpointRingAverage(multiPoly[0][0])
	default:
		return 0, 0, fmt.Errorf("%w: unsupported geometry_shape %q", ErrSegmentValidation, control.GeometryShape)
	}
}

func checkpointRingAverage(ring [][]float64) (float64, float64, error) {
	if len(ring) < 4 {
		return 0, 0, fmt.Errorf("ring too short")
	}
	var lngSum float64
	var latSum float64
	points := ring
	if ring[0][0] == ring[len(ring)-1][0] && ring[0][1] == ring[len(ring)-1][1] {
		points = ring[:len(ring)-1]
	}
	for _, point := range points {
		lngSum += point[0]
		latSum += point[1]
	}
	return lngSum / float64(len(points)), latSum / float64(len(points)), nil
}

func checkpointControlSummary(controls []model.CheckpointControl) *model.CheckpointControlSummary {
	primitiveSet := make(map[string]struct{})
	labels := make([]string, 0, len(controls))
	required := 0
	for _, control := range controls {
		primitiveSet[string(control.Primitive)] = struct{}{}
		if control.Required {
			required++
		}
		if control.Label != "" {
			labels = append(labels, control.Label)
		}
	}
	primitives := make([]string, 0, len(primitiveSet))
	for primitive := range primitiveSet {
		primitives = append(primitives, primitive)
	}
	slices.Sort(primitives)
	return &model.CheckpointControlSummary{
		ControlCount:         len(controls),
		RequiredControlCount: required,
		PrimitiveKinds:       primitives,
		Labels:               labels,
	}
}

func checkpointEffectiveGeometriesOverlap(a, b checkpointEffectiveGeometry) bool {
	if a.kind == "circle" && b.kind == "circle" {
		return checkpointDistanceM(a.lat, a.lng, b.lat, b.lng) <= a.radiusM+b.radiusM
	}
	if a.kind == "circle" {
		return checkpointCircleOverlapsArea(a, b)
	}
	if b.kind == "circle" {
		return checkpointCircleOverlapsArea(b, a)
	}
	return checkpointBoundsOverlap(checkpointBounds(a), checkpointBounds(b))
}

func checkpointCircleOverlapsArea(circle, area checkpointEffectiveGeometry) bool {
	if checkpointPointInEffectiveArea(circle.lng, circle.lat, area) {
		return true
	}
	for _, point := range checkpointAreaVertices(area) {
		if checkpointDistanceM(circle.lat, circle.lng, point[1], point[0]) <= circle.radiusM {
			return true
		}
	}
	return false
}

func checkpointPointInEffectiveArea(lng, lat float64, area checkpointEffectiveGeometry) bool {
	switch area.kind {
	case "polygon":
		return checkpointPointInPolygon(lng, lat, area.polygon)
	case "multipolygon":
		for _, polygon := range area.multiPoly {
			if checkpointPointInPolygon(lng, lat, polygon) {
				return true
			}
		}
	}
	return false
}

func checkpointPointInPolygon(lng, lat float64, polygon [][][]float64) bool {
	if len(polygon) == 0 || !checkpointPointInRing(lng, lat, polygon[0]) {
		return false
	}
	for _, hole := range polygon[1:] {
		if checkpointPointInRing(lng, lat, hole) {
			return false
		}
	}
	return true
}

func checkpointPointInRing(lng, lat float64, ring [][]float64) bool {
	inside := false
	j := len(ring) - 1
	for i := range ring {
		xi, yi := ring[i][0], ring[i][1]
		xj, yj := ring[j][0], ring[j][1]
		intersects := ((yi > lat) != (yj > lat)) && (lng < (xj-xi)*(lat-yi)/(yj-yi)+xi)
		if intersects {
			inside = !inside
		}
		j = i
	}
	return inside
}

func checkpointAreaVertices(area checkpointEffectiveGeometry) [][]float64 {
	var out [][]float64
	switch area.kind {
	case "polygon":
		for _, ring := range area.polygon {
			out = append(out, ring...)
		}
	case "multipolygon":
		for _, polygon := range area.multiPoly {
			for _, ring := range polygon {
				out = append(out, ring...)
			}
		}
	}
	return out
}

type checkpointBBox struct {
	minLng float64
	minLat float64
	maxLng float64
	maxLat float64
}

func checkpointBounds(area checkpointEffectiveGeometry) checkpointBBox {
	if area.kind == "circle" {
		delta := area.radiusM / 111_320
		return checkpointBBox{
			minLng: area.lng - delta,
			minLat: area.lat - delta,
			maxLng: area.lng + delta,
			maxLat: area.lat + delta,
		}
	}
	vertices := checkpointAreaVertices(area)
	bounds := checkpointBBox{
		minLng: vertices[0][0],
		maxLng: vertices[0][0],
		minLat: vertices[0][1],
		maxLat: vertices[0][1],
	}
	for _, point := range vertices[1:] {
		if point[0] < bounds.minLng {
			bounds.minLng = point[0]
		}
		if point[0] > bounds.maxLng {
			bounds.maxLng = point[0]
		}
		if point[1] < bounds.minLat {
			bounds.minLat = point[1]
		}
		if point[1] > bounds.maxLat {
			bounds.maxLat = point[1]
		}
	}
	return bounds
}

func checkpointBoundsOverlap(a, b checkpointBBox) bool {
	return a.minLng <= b.maxLng && a.maxLng >= b.minLng && a.minLat <= b.maxLat && a.maxLat >= b.minLat
}

func checkpointDistanceM(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusM = 6_371_000.0
	dLat := checkpointRadians(lat2 - lat1)
	dLng := checkpointRadians(lng2 - lng1)
	lat1Rad := checkpointRadians(lat1)
	lat2Rad := checkpointRadians(lat2)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func checkpointRadians(deg float64) float64 {
	return deg * math.Pi / 180
}

func checkpointFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
