package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

var (
	ErrRoutePublicationValidation = errors.New("route publication validation failed")
	ErrRoutePublicationConflict   = errors.New("route publication operation conflict")
)

type routePublicationSnapshot struct {
	ID                 string
	RouteID            *string
	UserID             *string
	SourceRouteVersion int64
	RouteName          string
	RouteDescription   *string
	SportType          string
	SubType            *string
	IsElectric         bool
	IsIndoor           bool
	IsVirtual          bool
	IsCommute          bool
	GeometryGeoJSON    string
	GeometryHash       string
	SnapshotHash       string
	TotalDistanceM     *float64
	TotalElevationGain *float64
	TotalElevationLoss *float64
}

type RouteDerivedCourseInput struct {
	Snapshot            *routePublicationSnapshot
	ActorUserID         string
	Visibility          string
	AccessScopeID       *string
	CompatibilityPreset string
	EquivalenceDecision string
	OperationID         string
}

type routePublicationTarget struct {
	PublicationID          string
	CourseSegmentID        string
	CourseSegmentVersionID string
	CreatedBy              string
	SegmentType            string
}

func (db *DB) PublishRouteSnapshotAsCourse(ctx context.Context, input model.PublishRouteSnapshotInput) (*model.RouteCoursePublication, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin route publication: %w", err)
	}
	defer tx.Rollback(ctx)

	publication, err := db.publishRouteSnapshotAsCourseTx(ctx, tx, input)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit route publication: %w", err)
	}
	return publication, nil
}

func (db *DB) publishRouteSnapshotAsCourseTx(ctx context.Context, tx pgx.Tx, input model.PublishRouteSnapshotInput) (*model.RouteCoursePublication, error) {
	if strings.TrimSpace(input.OperationID) == "" {
		return nil, fmt.Errorf("%w: operation_id is required", ErrRoutePublicationValidation)
	}
	inputHash, err := routePublicationInputHash(input)
	if err != nil {
		return nil, err
	}
	existing, err := loadRoutePublicationByOperationTx(ctx, tx, input.ActorUserID, input.OperationID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.OperationInputHash != inputHash {
			return nil, ErrRoutePublicationConflict
		}
		return existing, nil
	}

	snapshot, err := loadRoutePublicationSnapshotTx(ctx, tx, input.RouteSnapshotID)
	if err != nil {
		return nil, err
	}
	if snapshot.UserID == nil || *snapshot.UserID != input.ActorUserID {
		return nil, fmt.Errorf("%w: snapshot is not owned by actor", ErrRoutePublicationValidation)
	}
	if snapshot.SourceRouteVersion != input.SourceRouteVersion {
		return nil, ErrVersionMismatch
	}

	publishPolicy := strings.TrimSpace(input.PublishPolicy)
	if publishPolicy == "" {
		publishPolicy = "new_course"
	}
	equivalenceDecision := strings.TrimSpace(input.EquivalenceDecision)
	if equivalenceDecision == "" {
		equivalenceDecision = "version_specific"
	}
	visibility := strings.TrimSpace(input.Visibility)
	if visibility == "" {
		visibility = "private"
	}
	compatibilityPreset := strings.TrimSpace(input.CompatibilityPreset)
	if compatibilityPreset == "" {
		compatibilityPreset = "standard"
	}
	publishPolicy, equivalenceDecision, err = normalizeRoutePublicationPolicy(input, publishPolicy, equivalenceDecision)
	if err != nil {
		return nil, err
	}
	if err := db.validateSegmentAccessScopeForOwner(ctx, visibility, input.AccessScopeID, input.ActorUserID); err != nil {
		return nil, err
	}
	if err := validateRoutePublicationPrivacyGateTx(ctx, tx, snapshot.ID, snapshot.SnapshotHash, visibility); err != nil {
		return nil, err
	}

	compatibilityEvidence, err := json.Marshal(input.CompatibilityEvidence)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal compatibility evidence: %v", ErrRoutePublicationValidation, err)
	}
	if len(compatibilityEvidence) == 0 || string(compatibilityEvidence) == "null" {
		compatibilityEvidence = []byte("{}")
	}

	var target *routePublicationTarget
	if publishPolicy != "new_course" {
		target, err = loadRoutePublicationTargetForUpdateTx(ctx, tx, input, input.ActorUserID)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(input.CompatibilityPreset) == "" {
			compatibilityPreset = target.SegmentType
		}
	}

	var segmentID, versionID string
	switch publishPolicy {
	case "new_course":
		segmentID, versionID, err = db.CreateRouteDerivedCourseTx(ctx, tx, RouteDerivedCourseInput{
			Snapshot:            snapshot,
			ActorUserID:         input.ActorUserID,
			Visibility:          visibility,
			AccessScopeID:       input.AccessScopeID,
			CompatibilityPreset: compatibilityPreset,
			EquivalenceDecision: equivalenceDecision,
			OperationID:         input.OperationID,
		})
	default:
		continueFromVersionID := ""
		if equivalenceDecision == "continue_group" {
			continueFromVersionID = target.CourseSegmentVersionID
		}
		versionID, err = db.CreateRouteDerivedCourseVersionTx(ctx, tx, RouteDerivedCourseInput{
			Snapshot:            snapshot,
			ActorUserID:         input.ActorUserID,
			Visibility:          visibility,
			AccessScopeID:       input.AccessScopeID,
			CompatibilityPreset: compatibilityPreset,
			EquivalenceDecision: equivalenceDecision,
			OperationID:         input.OperationID,
		}, target.CourseSegmentID, continueFromVersionID, input.CompatibilityEvidence)
		segmentID = target.CourseSegmentID
	}
	if err != nil {
		return nil, err
	}

	var targetPublicationID, targetCourseSegmentID, targetCourseSegmentVersionID *string
	if target != nil {
		targetPublicationID = &target.PublicationID
		targetCourseSegmentID = &target.CourseSegmentID
		targetCourseSegmentVersionID = &target.CourseSegmentVersionID
	}

	var publicationID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO route_course_publications (
			route_id, route_snapshot_id, course_segment_id, course_segment_version_id,
			publication_state, publish_policy, equivalence_decision,
			visibility_snapshot, access_scope_id_snapshot,
			target_publication_id, target_course_segment_id, target_course_segment_version_id,
			compatibility_evidence, actor_user_id, operation_id, operation_input_hash,
			observed_route_version
		)
		VALUES (
			$1, $2, $3, $4,
			'published', $5, $6,
			$7, $8,
			$9, $10, $11,
			$12, $13, $14, $15,
			$16
		)
		RETURNING id
	`, snapshot.RouteID, snapshot.ID, segmentID, versionID,
		publishPolicy, equivalenceDecision, visibility, input.AccessScopeID,
		targetPublicationID, targetCourseSegmentID, targetCourseSegmentVersionID,
		compatibilityEvidence, input.ActorUserID, input.OperationID, inputHash, input.SourceRouteVersion,
	).Scan(&publicationID); err != nil {
		return nil, fmt.Errorf("insert route course publication: %w", err)
	}

	profile, err := db.CreateRouteProfileSnapshotTx(ctx, tx, snapshot.ID)
	if err != nil {
		return nil, err
	}
	if err := insertPublishedCourseProfileProjectionTx(ctx, tx, snapshot, profile, publicationID, segmentID, versionID, visibility, input.AccessScopeID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO route_publication_events (
			actor_user_id, operation, operation_id, operation_input_hash,
			subject_kind, subject_id, after_snapshot_hash, metadata
		)
		VALUES ($1, 'publish_course', $2, $3, 'route_course_publication', $4, $5, jsonb_build_object('course_segment_id', $6::text, 'course_segment_version_id', $7::text))
		ON CONFLICT (actor_user_id, operation_id, operation) DO NOTHING
	`, input.ActorUserID, input.OperationID, inputHash, publicationID, snapshot.SnapshotHash, segmentID, versionID); err != nil {
		return nil, fmt.Errorf("insert route publication event: %w", err)
	}
	return loadRoutePublicationByIDTx(ctx, tx, publicationID)
}

func loadRoutePublicationTargetForUpdateTx(ctx context.Context, tx pgx.Tx, input model.PublishRouteSnapshotInput, actorUserID string) (*routePublicationTarget, error) {
	if input.TargetPublicationID == nil || strings.TrimSpace(*input.TargetPublicationID) == "" ||
		input.TargetCourseSegmentID == nil || strings.TrimSpace(*input.TargetCourseSegmentID) == "" {
		return nil, fmt.Errorf("%w: existing-course publish policies require target publication and course", ErrRoutePublicationValidation)
	}
	target := &routePublicationTarget{}
	if err := tx.QueryRow(ctx, `
		SELECT rcp.id, rcp.course_segment_id, rcp.course_segment_version_id, s.created_by, s.segment_type
		FROM route_course_publications rcp
		JOIN segments s ON s.id = rcp.course_segment_id
		WHERE rcp.id = $1
		  AND rcp.course_segment_id = $2
		  AND rcp.publication_state = 'published'
		FOR UPDATE OF s
	`, *input.TargetPublicationID, *input.TargetCourseSegmentID).Scan(
		&target.PublicationID, &target.CourseSegmentID, &target.CourseSegmentVersionID, &target.CreatedBy, &target.SegmentType,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: target publication/course not found", ErrRoutePublicationValidation)
		}
		return nil, fmt.Errorf("load target route course publication: %w", err)
	}
	if target.CreatedBy != actorUserID {
		return nil, fmt.Errorf("%w: target course is not owned by actor", ErrRoutePublicationValidation)
	}
	if input.TargetCourseSegmentVersionID != nil && strings.TrimSpace(*input.TargetCourseSegmentVersionID) != "" {
		target.CourseSegmentVersionID = *input.TargetCourseSegmentVersionID
	}
	var versionBelongs bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM segment_versions
			WHERE id = $1
			  AND segment_id = $2
		)
	`, target.CourseSegmentVersionID, target.CourseSegmentID).Scan(&versionBelongs); err != nil {
		return nil, fmt.Errorf("validate target course version: %w", err)
	}
	if !versionBelongs {
		return nil, fmt.Errorf("%w: target course version does not belong to target course", ErrRoutePublicationValidation)
	}
	return target, nil
}

func (db *DB) CreateRouteDerivedCourseTx(ctx context.Context, tx pgx.Tx, input RouteDerivedCourseInput) (string, string, error) {
	if input.Snapshot == nil {
		return "", "", fmt.Errorf("%w: snapshot is required", ErrRoutePublicationValidation)
	}
	segmentType := input.CompatibilityPreset
	if segmentType == "" {
		segmentType = "standard"
	}
	if segmentType != "standard" && segmentType != "trail" && segmentType != "lap" {
		return "", "", fmt.Errorf("%w: unsupported compatibility preset %q", ErrRoutePublicationValidation, segmentType)
	}
	predicates := []sport.Predicate{{Sport: input.Snapshot.SportType, SubType: input.Snapshot.SubType}}
	predicatesJSON, err := marshalSegmentPredicates(predicates)
	if err != nil {
		return "", "", fmt.Errorf("marshal route course predicates: %w", err)
	}
	sportGroup := segmentSportGroupFromPredicates(predicates)

	var segmentID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO segments (
			created_by, name, description, sport_group, segment_type, visibility,
			access_scope_id, route_hash, allowed_predicates, bidirectional, elevation_kind
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, true, 'hilly')
		RETURNING id
	`, input.ActorUserID, input.Snapshot.RouteName, input.Snapshot.RouteDescription,
		sportGroup, segmentType, input.Visibility, input.AccessScopeID, input.Snapshot.GeometryHash, predicatesJSON,
	).Scan(&segmentID); err != nil {
		return "", "", fmt.Errorf("insert route-derived course segment: %w", err)
	}

	trackGeoJSON, err := normalizeSegmentTrackForTypeTx(ctx, tx, segmentType, input.Snapshot.GeometryGeoJSON)
	if err != nil {
		return "", "", err
	}
	var versionID string
	if err := tx.QueryRow(ctx, `
		WITH g AS (
			SELECT ST_SetSRID(ST_GeomFromGeoJSON($2), 4326) AS geom
		)
		INSERT INTO segment_versions (
			segment_id, version_number, track, start_point, end_point, valid_from, created_by,
			valid_from_at, valid_until_at, validity_basis, matching_contract_key, matching_contract_version, published_at
		)
		SELECT $1, 1, g.geom, ST_StartPoint(g.geom), ST_EndPoint(g.geom), CURRENT_DATE, $3,
		       NULL, NULL, 'editor_asserted', 'course_gps_v1', 1, NOW()
		FROM g
		RETURNING id
	`, segmentID, trackGeoJSON, input.ActorUserID).Scan(&versionID); err != nil {
		return "", "", fmt.Errorf("insert route-derived course version: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE segments
		SET distance_m = $2,
		    elevation_gain_m = $3
		WHERE id = $1
	`, segmentID, input.Snapshot.TotalDistanceM, input.Snapshot.TotalElevationGain); err != nil {
		return "", "", fmt.Errorf("update route-derived course metrics: %w", err)
	}
	if err := applyEquivalenceDecisionTx(ctx, tx, segmentID, versionID, "", input.ActorUserID, input.OperationID, input.EquivalenceDecision, "route_publication", nil); err != nil {
		return "", "", err
	}
	if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
		return "", "", err
	}
	return segmentID, versionID, nil
}

func (db *DB) CreateRouteDerivedCourseVersionTx(ctx context.Context, tx pgx.Tx, input RouteDerivedCourseInput, segmentID, continueFromVersionID string, evidence map[string]any) (string, error) {
	if input.Snapshot == nil {
		return "", fmt.Errorf("%w: snapshot is required", ErrRoutePublicationValidation)
	}
	segmentType := input.CompatibilityPreset
	if segmentType == "" {
		segmentType = "standard"
	}
	if segmentType != "standard" && segmentType != "trail" && segmentType != "lap" {
		return "", fmt.Errorf("%w: unsupported compatibility preset %q", ErrRoutePublicationValidation, segmentType)
	}
	predicates := []sport.Predicate{{Sport: input.Snapshot.SportType, SubType: input.Snapshot.SubType}}
	predicatesJSON, err := marshalSegmentPredicates(predicates)
	if err != nil {
		return "", fmt.Errorf("marshal route course predicates: %w", err)
	}
	trackGeoJSON, err := normalizeSegmentTrackForTypeTx(ctx, tx, segmentType, input.Snapshot.GeometryGeoJSON)
	if err != nil {
		return "", err
	}

	var versionID string
	if err := tx.QueryRow(ctx, `
		WITH boundary AS (
			SELECT NOW() AS at
		),
		next_version AS (
			SELECT COALESCE(MAX(version_number), 0) + 1 AS number
			FROM segment_versions
			WHERE segment_id = $1
		),
		active AS (
			SELECT id
			FROM segment_versions
			WHERE segment_id = $1
			  AND matching_contract_key = 'course_gps_v1'
			  AND matching_contract_version = 1
			  AND validity_basis <> 'rollback_retired'
			  AND (valid_from_at IS NULL OR valid_from_at <= (SELECT at FROM boundary))
			  AND (valid_until_at IS NULL OR (SELECT at FROM boundary) < valid_until_at)
			ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC,
			         version_number DESC
			LIMIT 1
		),
		next_future AS (
			SELECT MIN(valid_from_at) AS at
			FROM segment_versions
			WHERE segment_id = $1
			  AND matching_contract_key = 'course_gps_v1'
			  AND matching_contract_version = 1
			  AND validity_basis <> 'rollback_retired'
			  AND valid_from_at > (SELECT at FROM boundary)
		),
		closed AS (
			UPDATE segment_versions sv
			SET valid_until_at = boundary.at
			FROM active, boundary
			WHERE sv.id = active.id
			  AND (sv.valid_until_at IS NULL OR sv.valid_until_at > boundary.at)
			RETURNING sv.id
		),
		g AS (
			SELECT ST_SetSRID(ST_GeomFromGeoJSON($2), 4326) AS geom
		)
		INSERT INTO segment_versions (
			segment_id, version_number, track, start_point, end_point, valid_from, created_by,
			valid_from_at, valid_until_at, validity_basis, matching_contract_key, matching_contract_version, published_at
		)
		SELECT $1,
		       next_version.number,
		       g.geom,
		       ST_StartPoint(g.geom),
		       ST_EndPoint(g.geom),
		       CURRENT_DATE,
		       $3,
		       boundary.at,
		       next_future.at,
		       'editor_asserted',
		       'course_gps_v1',
		       1,
		       boundary.at
		FROM g, next_version, next_future, boundary, closed
		RETURNING id
	`, segmentID, trackGeoJSON, input.ActorUserID).Scan(&versionID); err != nil {
		return "", fmt.Errorf("insert route-derived course version: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE segments
		SET name = $2,
		    description = $3,
		    segment_type = $4,
		    visibility = $5,
		    access_scope_id = $6,
		    route_hash = $7,
		    distance_m = $8,
		    elevation_gain_m = $9,
		    sport_group = $10,
		    allowed_predicates = $11
		WHERE id = $1
	`, segmentID, input.Snapshot.RouteName, input.Snapshot.RouteDescription, segmentType, input.Visibility,
		input.AccessScopeID, input.Snapshot.GeometryHash, input.Snapshot.TotalDistanceM, input.Snapshot.TotalElevationGain,
		segmentSportGroupFromPredicates(predicates), predicatesJSON); err != nil {
		return "", fmt.Errorf("update route-derived course segment: %w", err)
	}
	if err := applyEquivalenceDecisionTx(ctx, tx, segmentID, versionID, continueFromVersionID, input.ActorUserID, input.OperationID, input.EquivalenceDecision, "route_publication", evidence); err != nil {
		return "", err
	}
	if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
		return "", err
	}
	if err := syncPublishedCourseProjectionAccessTx(ctx, tx, segmentID); err != nil {
		return "", err
	}
	return versionID, nil
}

func loadRoutePublicationSnapshotTx(ctx context.Context, tx pgx.Tx, snapshotID string) (*routePublicationSnapshot, error) {
	var snapshot routePublicationSnapshot
	if err := tx.QueryRow(ctx, `
		SELECT id, route_id, user_id, source_route_version, route_name, route_description,
		       sport_type, sub_type, is_electric, is_indoor, is_virtual, is_commute,
		       ST_AsGeoJSON(geometry), geometry_hash, snapshot_hash,
		       total_distance_m, total_elevation_gain, total_elevation_loss
		FROM planned_route_snapshots
		WHERE id = $1
	`, snapshotID).Scan(
		&snapshot.ID, &snapshot.RouteID, &snapshot.UserID, &snapshot.SourceRouteVersion,
		&snapshot.RouteName, &snapshot.RouteDescription,
		&snapshot.SportType, &snapshot.SubType,
		&snapshot.IsElectric, &snapshot.IsIndoor, &snapshot.IsVirtual, &snapshot.IsCommute,
		&snapshot.GeometryGeoJSON, &snapshot.GeometryHash, &snapshot.SnapshotHash,
		&snapshot.TotalDistanceM, &snapshot.TotalElevationGain, &snapshot.TotalElevationLoss,
	); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func loadRoutePublicationByOperationTx(ctx context.Context, tx pgx.Tx, actorUserID, operationID string) (*model.RouteCoursePublication, error) {
	var id string
	err := tx.QueryRow(ctx, `
		SELECT id
		FROM route_course_publications
		WHERE actor_user_id = $1
		  AND operation_id = $2
	`, actorUserID, operationID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load route publication operation: %w", err)
	}
	return loadRoutePublicationByIDTx(ctx, tx, id)
}

func loadRoutePublicationByIDTx(ctx context.Context, tx pgx.Tx, id string) (*model.RouteCoursePublication, error) {
	var publication model.RouteCoursePublication
	if err := tx.QueryRow(ctx, `
		SELECT id, route_id, route_snapshot_id, course_segment_id, course_segment_version_id,
		       publication_state, publish_policy, equivalence_decision,
		       visibility_snapshot, access_scope_id_snapshot,
		       target_publication_id, target_course_segment_id, target_course_segment_version_id,
		       compatibility_evidence, actor_user_id, operation_id, operation_input_hash,
		       observed_route_version, created_at, retired_at, metadata
		FROM route_course_publications
		WHERE id = $1
	`, id).Scan(
		&publication.ID, &publication.RouteID, &publication.RouteSnapshotID,
		&publication.CourseSegmentID, &publication.CourseSegmentVersionID,
		&publication.PublicationState, &publication.PublishPolicy, &publication.EquivalenceDecision,
		&publication.VisibilitySnapshot, &publication.AccessScopeIDSnapshot,
		&publication.TargetPublicationID, &publication.TargetCourseSegmentID, &publication.TargetCourseSegmentVersionID,
		&publication.CompatibilityEvidence, &publication.ActorUserID, &publication.OperationID, &publication.OperationInputHash,
		&publication.ObservedRouteVersion, &publication.CreatedAt, &publication.RetiredAt, &publication.Metadata,
	); err != nil {
		return nil, err
	}
	return &publication, nil
}

func validateRoutePublicationPolicy(input model.PublishRouteSnapshotInput, publishPolicy, equivalenceDecision string) error {
	_, _, err := normalizeRoutePublicationPolicy(input, publishPolicy, equivalenceDecision)
	return err
}

func normalizeRoutePublicationPolicy(input model.PublishRouteSnapshotInput, publishPolicy, equivalenceDecision string) (string, string, error) {
	if equivalenceDecision == "" {
		equivalenceDecision = "version_specific"
	}
	switch publishPolicy {
	case "new_course":
		if input.TargetPublicationID != nil || input.TargetCourseSegmentID != nil || input.TargetCourseSegmentVersionID != nil {
			return "", "", fmt.Errorf("%w: new_course forbids target publication/course ids", ErrRoutePublicationValidation)
		}
		if equivalenceDecision == "continue_group" {
			return "", "", fmt.Errorf("%w: new_course cannot continue an existing equivalence group", ErrRoutePublicationValidation)
		}
		if equivalenceDecision != "version_specific" && equivalenceDecision != "new_group" {
			return "", "", fmt.Errorf("%w: unsupported equivalence decision %q", ErrRoutePublicationValidation, equivalenceDecision)
		}
	case "new_version_same_course":
		if input.TargetPublicationID == nil || input.TargetCourseSegmentID == nil {
			return "", "", fmt.Errorf("%w: existing-course publish policies require target publication and course", ErrRoutePublicationValidation)
		}
		equivalenceDecision = "version_specific"
	case "continue_equivalence_group":
		if input.TargetPublicationID == nil || input.TargetCourseSegmentID == nil {
			return "", "", fmt.Errorf("%w: existing-course publish policies require target publication and course", ErrRoutePublicationValidation)
		}
		if len(input.CompatibilityEvidence) == 0 {
			return "", "", fmt.Errorf("%w: continue_equivalence_group requires compatibility evidence", ErrRoutePublicationValidation)
		}
		equivalenceDecision = "continue_group"
	case "new_equivalence_group":
		if input.TargetPublicationID == nil || input.TargetCourseSegmentID == nil {
			return "", "", fmt.Errorf("%w: existing-course publish policies require target publication and course", ErrRoutePublicationValidation)
		}
		equivalenceDecision = "new_group"
	default:
		return "", "", fmt.Errorf("%w: unsupported publish policy %q", ErrRoutePublicationValidation, publishPolicy)
	}
	return publishPolicy, equivalenceDecision, nil
}

func validateRoutePublicationPrivacyGateTx(ctx context.Context, tx pgx.Tx, snapshotID, snapshotHash, visibility string) error {
	if visibility != "public" && visibility != "shared" {
		return nil
	}
	states := make([]string, 8)
	err := tx.QueryRow(ctx, `
		SELECT route_name_state, description_state, waypoints_state, source_metadata_state,
		       geometry_state, enrichment_state, media_pointer_state, overall_state
		FROM route_publication_privacy_reviews
		WHERE route_snapshot_id = $1
		  AND snapshot_hash = $2
		  AND audience_class = $3
	`, snapshotID, snapshotHash, visibility).Scan(
		&states[0], &states[1], &states[2], &states[3],
		&states[4], &states[5], &states[6], &states[7],
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: publication privacy review is required", ErrRoutePublicationValidation)
	}
	if err != nil {
		return fmt.Errorf("load route publication privacy review: %w", err)
	}
	for _, state := range states {
		if state != "safe" {
			return fmt.Errorf("%w: %s privacy review is not publishable", ErrRoutePublicationValidation, visibility)
		}
	}
	return nil
}

func syncPublishedCourseProjectionAccessTx(ctx context.Context, tx pgx.Tx, segmentID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE route_course_profile_projection p
		SET visibility = s.visibility,
		    access_scope_id = s.access_scope_id,
		    generated_at = NOW()
		FROM segments s
		WHERE p.subject_kind = 'published_course'
		  AND p.subject_id = s.id
		  AND s.id = $1
	`, segmentID)
	if err != nil {
		return fmt.Errorf("sync published course projection access: %w", err)
	}
	return nil
}

func insertPublishedCourseProfileProjectionTx(ctx context.Context, tx pgx.Tx, snapshot *routePublicationSnapshot, profile *model.RouteProfileSnapshot, publicationID, segmentID, versionID, visibility string, accessScopeID *string) error {
	var scales map[string]routeProfileScaleSummary
	if err := json.Unmarshal(profile.DifficultyScales, &scales); err != nil {
		return fmt.Errorf("unmarshal route profile scales for course projection: %w", err)
	}
	primarySurface := routeProfilePrimaryKeyFromRaw(profile.SurfaceDistribution)
	primaryWayType := routeProfilePrimaryKeyFromRaw(profile.WayDistribution)
	_, err := tx.Exec(ctx, `
		INSERT INTO route_course_profile_projection (
			subject_kind, subject_id, subject_version_id, subject_version_key,
			publication_id, route_snapshot_id, profile_snapshot_id,
			visibility, access_scope_id,
			sport_type, sub_type, is_electric, is_indoor, is_virtual, is_commute,
			distance_m, elevation_gain_m, elevation_loss_m,
			sac_max_rank, sac_dominant_rank, sac_weighted_mean_rank,
			mtb_max_rank, mtb_dominant_rank, mtb_weighted_mean_rank,
			via_ferrata_max_rank, via_ferrata_dominant_rank, via_ferrata_weighted_mean_rank,
			forest_pct, protected_pct, primary_surface, primary_way_type, source_hash
		)
		VALUES (
			'published_course', $1, $2, $2,
			$3, $4, $5,
			$6, $7,
			$8, $9, $10, $11, $12, $13,
			$14, $15, $16,
			$17, $18, $19,
			$20, $21, $22,
			$23, $24, $25,
			$26, $27, $28, $29, $30
		)
		ON CONFLICT (subject_kind, subject_id, subject_version_key) DO UPDATE SET
			publication_id = EXCLUDED.publication_id,
			route_snapshot_id = EXCLUDED.route_snapshot_id,
			profile_snapshot_id = EXCLUDED.profile_snapshot_id,
			visibility = EXCLUDED.visibility,
			access_scope_id = EXCLUDED.access_scope_id,
			source_hash = EXCLUDED.source_hash,
			generated_at = NOW()
	`, segmentID, versionID, publicationID, snapshot.ID, profile.ID,
		visibility, accessScopeID, snapshot.SportType, snapshot.SubType,
		snapshot.IsElectric, snapshot.IsIndoor, snapshot.IsVirtual, snapshot.IsCommute,
		snapshot.TotalDistanceM, snapshot.TotalElevationGain, snapshot.TotalElevationLoss,
		routeProfileRankPtr(scales["sac"].MaxLevelRank), routeProfileRankPtr(scales["sac"].DominantLevelRank), scales["sac"].WeightedMeanRank,
		routeProfileRankPtr(scales["mtb"].MaxLevelRank), routeProfileRankPtr(scales["mtb"].DominantLevelRank), scales["mtb"].WeightedMeanRank,
		routeProfileRankPtr(scales["via_ferrata"].MaxLevelRank), routeProfileRankPtr(scales["via_ferrata"].DominantLevelRank), scales["via_ferrata"].WeightedMeanRank,
		profile.ForestPct, profile.ProtectedPct, primarySurface, primaryWayType, profile.SourceHash)
	if err != nil {
		return fmt.Errorf("insert published course profile projection: %w", err)
	}
	return nil
}

func routeProfilePrimaryKeyFromRaw(raw json.RawMessage) *string {
	var distribution map[string]routeProfileDistributionEntry
	if err := json.Unmarshal(raw, &distribution); err != nil {
		return nil
	}
	distances := make(map[string]float64, len(distribution))
	for key, entry := range distribution {
		distances[key] = entry.DistanceM
	}
	return routeProfilePrimaryKey(distances)
}

func routePublicationInputHash(input model.PublishRouteSnapshotInput) (string, error) {
	payload := struct {
		RouteSnapshotID              string         `json:"route_snapshot_id"`
		ActorUserID                  string         `json:"actor_user_id"`
		SourceRouteVersion           int64          `json:"source_route_version"`
		PublishPolicy                string         `json:"publish_policy"`
		EquivalenceDecision          string         `json:"equivalence_decision"`
		Visibility                   string         `json:"visibility"`
		AccessScopeID                *string        `json:"access_scope_id,omitempty"`
		CompatibilityPreset          string         `json:"compatibility_preset"`
		TargetPublicationID          *string        `json:"target_publication_id,omitempty"`
		TargetCourseSegmentID        *string        `json:"target_course_segment_id,omitempty"`
		TargetCourseSegmentVersionID *string        `json:"target_course_segment_version_id,omitempty"`
		CompatibilityEvidence        map[string]any `json:"compatibility_evidence,omitempty"`
	}{
		RouteSnapshotID:              input.RouteSnapshotID,
		ActorUserID:                  input.ActorUserID,
		SourceRouteVersion:           input.SourceRouteVersion,
		PublishPolicy:                input.PublishPolicy,
		EquivalenceDecision:          input.EquivalenceDecision,
		Visibility:                   input.Visibility,
		AccessScopeID:                input.AccessScopeID,
		CompatibilityPreset:          input.CompatibilityPreset,
		TargetPublicationID:          input.TargetPublicationID,
		TargetCourseSegmentID:        input.TargetCourseSegmentID,
		TargetCourseSegmentVersionID: input.TargetCourseSegmentVersionID,
		CompatibilityEvidence:        input.CompatibilityEvidence,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal route publication input hash: %w", err)
	}
	return stableHashBytes(raw), nil
}
