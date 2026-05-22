package database

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// Sentinel errors for segment operations — use errors.Is() in handlers.
var (
	ErrSegmentNotFound                           = errors.New("segment not found")
	ErrSegmentNotAuthorized                      = errors.New("segment not authorized")
	ErrSegmentOSMLift                            = errors.New("OSM-imported lift segments cannot be deleted")
	ErrSegmentValidation                         = errors.New("segment validation failed")
	ErrSegmentPublishPrivacyConfirmationRequired = errors.New("segment publish privacy confirmation required")
)

const (
	segmentEffortActivePredicate = "se.superseded_at IS NULL"
	segmentEffortActiveWhere     = "superseded_at IS NULL"
)

func normalizeSegmentPredicates(preds []sport.Predicate) ([]sport.Predicate, error) {
	normalized, err := sport.NormalizePredicates(preds)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSegmentValidation, err)
	}
	if err := sport.ValidatePredicates(normalized, true); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSegmentValidation, err)
	}
	return normalized, nil
}

func marshalSegmentPredicates(preds []sport.Predicate) (any, error) {
	if preds == nil {
		return nil, nil
	}
	raw, err := json.Marshal(preds)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func decodeSegmentPredicates(raw []byte) ([]sport.Predicate, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var preds []sport.Predicate
	if err := json.Unmarshal(raw, &preds); err != nil {
		return nil, err
	}
	return preds, nil
}

func segmentSportGroupFromPredicates(preds []sport.Predicate) string {
	return sport.SegmentProjectionGroupForPredicates(preds)
}

func validateSegmentPredicatesForType(segmentType string, preds []sport.Predicate) error {
	if segmentType == "lift" {
		return nil
	}
	eligible := make(map[string]struct{})
	for _, entry := range sport.SegmentProjectionEntriesForType(segmentType) {
		key := entry.SportType
		if entry.SubType != "" {
			key += ":" + entry.SubType
		}
		eligible[key] = struct{}{}
	}
	for _, pred := range preds {
		key := pred.Sport
		if pred.SubType != nil && *pred.SubType != "" {
			key += ":" + *pred.SubType
		}
		if _, ok := eligible[key]; ok {
			continue
		}
		if _, ok := eligible[pred.Sport]; ok {
			continue
		}
		return fmt.Errorf("%w: sport predicate %s is not eligible for segment_type %s", ErrSegmentValidation, key, segmentType)
	}
	return nil
}

func segmentTypeUsesDerivedDefaultPredicates(segmentType string) bool {
	switch segmentType {
	case "wintersport", "wassersport":
		return true
	default:
		return false
	}
}

func segmentTypeFilterIncludesLift(segmentType, segmentTypes string) bool {
	if segmentType == "lift" {
		return true
	}
	for t := range strings.SplitSeq(segmentTypes, ",") {
		if strings.TrimSpace(t) == "lift" {
			return true
		}
	}
	return false
}

func activeCourseGPSV1VersionLateral(joinKind, segmentIDExpr, atExpr, selectList string) string {
	return fmt.Sprintf(`
		%s LATERAL (
			SELECT %s
			FROM segment_versions
			WHERE segment_id = %s
			  AND matching_contract_key = 'course_gps_v1'
			  AND matching_contract_version = 1
			  AND validity_basis <> 'rollback_retired'
			  AND (valid_from_at IS NULL OR valid_from_at <= %s)
			  AND (valid_until_at IS NULL OR %s < valid_until_at)
			ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC,
			         version_number DESC
			LIMIT 1
		) sv ON true`, joinKind, selectList, segmentIDExpr, atExpr, atExpr)
}

func activeSegmentCourseVersionLateral(joinKind, segmentIDExpr, courseKindExpr, atExpr, selectList string) string {
	return fmt.Sprintf(`
		%s LATERAL (
			SELECT %s
			FROM segment_versions
			WHERE segment_id = %s
			  AND matching_contract_key = CASE
			      WHEN %s = 'checkpoint_sequence' THEN 'checkpoint_sequence_v1'
			      ELSE 'course_gps_v1'
			  END
			  AND matching_contract_version = 1
			  AND validity_basis <> 'rollback_retired'
			  AND (valid_from_at IS NULL OR valid_from_at <= %s)
			  AND (valid_until_at IS NULL OR %s < valid_until_at)
			ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC,
			         version_number DESC
			LIMIT 1
		) sv ON true`, joinKind, selectList, segmentIDExpr, courseKindExpr, atExpr, atExpr)
}

func defaultSegmentPredicatesForType(segmentType string) ([]sport.Predicate, error) {
	if !segmentTypeUsesDerivedDefaultPredicates(segmentType) {
		return nil, nil
	}
	entries := sport.SegmentProjectionEntriesForType(segmentType)
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: segment_type %s has no default predicates", ErrSegmentValidation, segmentType)
	}
	preds := make([]sport.Predicate, 0, len(entries))
	for _, entry := range entries {
		pred := sport.Predicate{Sport: entry.SportType}
		if entry.SubType != "" {
			subType := entry.SubType
			pred.SubType = &subType
		}
		preds = append(preds, pred)
	}
	normalized, err := normalizeSegmentPredicates(preds)
	if err != nil {
		return nil, err
	}
	if err := validateSegmentPredicatesForType(segmentType, normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func segmentPredicatesForCreate(segmentType string, requested []sport.Predicate) ([]sport.Predicate, error) {
	if requested == nil && segmentTypeUsesDerivedDefaultPredicates(segmentType) {
		return defaultSegmentPredicatesForType(segmentType)
	}
	normalized, err := normalizeSegmentPredicates(requested)
	if err != nil {
		return nil, err
	}
	if requested != nil {
		if err := validateSegmentPredicatesForType(segmentType, normalized); err != nil {
			return nil, err
		}
	}
	return normalized, nil
}

func segmentPredicatesForUpdate(segmentType string, raw json.RawMessage) ([]sport.Predicate, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	if string(raw) == "null" {
		if segmentTypeUsesDerivedDefaultPredicates(segmentType) {
			preds, err := defaultSegmentPredicatesForType(segmentType)
			return preds, true, err
		}
		return nil, true, nil
	}
	var requested []sport.Predicate
	if err := json.Unmarshal(raw, &requested); err != nil {
		return nil, false, fmt.Errorf("decode segment predicates: %w", err)
	}
	normalized, err := normalizeSegmentPredicates(requested)
	if err != nil {
		return nil, false, fmt.Errorf("normalize segment predicates: %w", err)
	}
	if err := validateSegmentPredicatesForType(segmentType, normalized); err != nil {
		return nil, false, err
	}
	return normalized, true, nil
}

func validateSegmentTypeChangePredicates(segmentType string, preds []sport.Predicate) error {
	if len(preds) == 0 {
		return nil
	}
	return validateSegmentPredicatesForType(segmentType, preds)
}

func normalizeSegmentTrackForTypeTx(ctx context.Context, tx pgx.Tx, segmentType, trackGeoJSON string) (string, error) {
	if segmentType != "lap" {
		return trackGeoJSON, nil
	}
	var gapM float64
	if err := tx.QueryRow(ctx, `
		SELECT ST_Distance(
			ST_StartPoint(ST_SetSRID(ST_GeomFromGeoJSON($1), 4326))::geography,
			ST_EndPoint(ST_SetSRID(ST_GeomFromGeoJSON($1), 4326))::geography
		)`, trackGeoJSON).Scan(&gapM); err != nil {
		return "", fmt.Errorf("check lap gap: %w", err)
	}
	if gapM > 50 {
		return "", fmt.Errorf("%w: lap segment must be a closed loop; start-end gap is %.0fm (max 50m)", ErrSegmentValidation, gapM)
	}
	if gapM <= 0.1 {
		return trackGeoJSON, nil
	}
	var normalized string
	if err := tx.QueryRow(ctx, `
		SELECT ST_AsGeoJSON(
			ST_AddPoint(
				ST_SetSRID(ST_GeomFromGeoJSON($1), 4326),
				ST_StartPoint(ST_SetSRID(ST_GeomFromGeoJSON($1), 4326))
			)
		)`, trackGeoJSON).Scan(&normalized); err != nil {
		return "", fmt.Errorf("auto-close lap track: %w", err)
	}
	return normalized, nil
}

func (db *DB) ListSegments(ctx context.Context, p model.ListSegmentsParams) (*model.ListSegmentsResult, error) {
	// args: $1 = userID (always used by CTE). $2 = isAdmin is appended ONLY when the
	// visibility clause actually references it — otherwise pgx passes an orphan param
	// and PG raises 42P18 "could not determine data type of parameter $2".
	userArg := any(p.UserID)
	if strings.TrimSpace(p.UserID) == "" {
		userArg = nil
	}
	args := []any{userArg}
	argIdx := 2

	// --- CTE: user_favs ---
	cte := `
WITH user_favs AS (
    SELECT segment_id FROM segment_favorites WHERE user_id = $1
)`

	// --- SELECT ---
	selectCols := `
SELECT s.id, s.created_by, s.name, s.description, s.sport_group,
       s.allowed_predicates,
       s.segment_type, s.segment_type_meta,
       s.source_activity_id, s.osm_lift_id,
       s.distance_m, s.elevation_gain_m,
       s.effort_count + COALESCE(ue.private_ridden_count, 0) AS effort_count, s.athlete_count,
       COALESCE(ue.ridden_count, 0) AS ridden_count,
       (uf.segment_id IS NOT NULL) AS is_favorite,
       s.visibility, s.access_scope_id, s.route_hash, s.created_at,`

	if p.IncludeGeoJSON {
		selectCols += `
       ST_AsGeoJSON(sv.track) AS track_geojson,`
	} else {
		selectCols += `
       NULL::text AS track_geojson,`
	}
	needsCheckpointProjection := p.CourseKind == string(model.CourseKindCheckpointSequence) ||
		slices.Contains(p.CourseKinds, string(model.CourseKindCheckpointSequence))
	hasLineCourseFilter := len(p.CourseKinds) == 0 && p.CourseKind != string(model.CourseKindCheckpointSequence)
	if p.CourseKind == string(model.CourseKindLine) || slices.Contains(p.CourseKinds, string(model.CourseKindLine)) {
		hasLineCourseFilter = true
	}
	hasCheckpointCourseFilter := needsCheckpointProjection
	if needsCheckpointProjection {
		selectCols += `
       s.terrain_type, s.climb_type, s.elevation_kind,
       s.course_kind, cp.control_summary, COALESCE(cp.control_map_features, '[]'::jsonb)`
	} else {
		selectCols += `
       s.terrain_type, s.climb_type, s.elevation_kind,
       s.course_kind, NULL::jsonb AS control_summary, '[]'::jsonb AS control_map_features`
	}

	// --- FROM + JOINs ---
	from := `
FROM segments s
LEFT JOIN LATERAL (
    SELECT COUNT(*) FILTER (WHERE rn = 1)::int AS ridden_count,
           COUNT(*) FILTER (WHERE rn = 1 AND visibility != 'public')::int AS private_ridden_count
    FROM (
        SELECT a.visibility,
               ROW_NUMBER() OVER (
                   PARTITION BY COALESCE(rg.id::text, a.id::text), se.started_at, se.elapsed_time_s, se.is_reverse
                   ORDER BY
                       CASE
                           WHEN rg.primary_activity_id IS NOT NULL AND a.id = rg.primary_activity_id THEN 0
                           WHEN COALESCE(a.is_group_secondary, false) = false THEN 1
                           ELSE 2
                       END,
                       se.created_at ASC,
                       se.id ASC
               ) AS rn
        FROM segment_efforts se
        JOIN activities a ON se.activity_id = a.id
        LEFT JOIN recording_group_members rgm ON rgm.activity_id = a.id
        LEFT JOIN recording_groups rg ON rg.id = rgm.group_id
        WHERE se.segment_id = s.id
          AND a.user_id = $1
          AND se.superseded_at IS NULL
    ) ranked_efforts
) ue ON true
LEFT JOIN user_favs uf ON uf.segment_id = s.id`

	if p.IncludeGeoJSON {
		from += `
LEFT JOIN LATERAL (
    SELECT track
    FROM segment_versions
    WHERE segment_id = s.id
      AND matching_contract_key = CASE
          WHEN s.course_kind = 'checkpoint_sequence' THEN 'checkpoint_sequence_v1'
          ELSE 'course_gps_v1'
      END
      AND matching_contract_version = 1
      AND validity_basis <> 'rollback_retired'
      AND (valid_from_at IS NULL OR valid_from_at <= NOW())
      AND (valid_until_at IS NULL OR NOW() < valid_until_at)
    ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC,
             version_number DESC
    LIMIT 1
) sv ON true`
	}
	if needsCheckpointProjection {
		from += `
LEFT JOIN checkpoint_course_projection cp ON cp.segment_id = s.id`
	}

	// --- WHERE ---
	// Admin sees all segments (public + private from all users).
	// Segments contain only geometry/metadata — no personal health data.
	// Admin access is covered by the existing audit log.
	// isAdminPlaceholder returns "$N" for isAdmin, appending p.IsAdmin to args
	// on first call so the param is bound. Cached via closure over shared state.
	var isAdminBound bool
	var isAdminPlaceholderIdx int
	isAdminPlaceholder := func() string {
		if !isAdminBound {
			isAdminPlaceholderIdx = argIdx
			args = append(args, p.IsAdmin)
			argIdx++
			isAdminBound = true
		}
		return fmt.Sprintf("$%d", isAdminPlaceholderIdx)
	}
	resolvedScopesExpr := "NULL::uuid[]"
	if len(p.VisibleAccessScopeIDs) > 0 {
		args = append(args, p.VisibleAccessScopeIDs)
		resolvedScopesExpr = fmt.Sprintf("$%d::uuid[]", argIdx)
		argIdx++
	}

	var visibilityClause string
	if p.VisibilityFilter != "" {
		parts := make([]string, 0, 2)
		for v := range strings.SplitSeq(p.VisibilityFilter, ",") {
			switch strings.TrimSpace(v) {
			case "public":
				parts = append(parts, "s.visibility = 'public'")
			case "private":
				parts = append(parts, fmt.Sprintf("(s.visibility = 'private' AND (s.created_by = $1 OR %s = true))", isAdminPlaceholder()))
			case "shared":
				parts = append(parts, sharedSegmentAccessWithScopesSQL("s", "$1", isAdminPlaceholder(), resolvedScopesExpr))
			}
		}
		switch len(parts) {
		case 0:
			// Defense-in-depth: unknown visibility tokens return no results rather
			// than silently showing everything.
			visibilityClause = "FALSE"
		default:
			visibilityClause = "(" + strings.Join(parts, " OR ") + ")"
		}
	} else {
		visibilityClause = visibleSegmentAccessWithScopesSQL("s", "$1", isAdminPlaceholder(), resolvedScopesExpr)
	}
	where := fmt.Sprintf(" WHERE %s", visibilityClause)

	if p.SportGroup != "" {
		where += fmt.Sprintf(" AND s.sport_group = $%d", argIdx)
		args = append(args, p.SportGroup)
		argIdx++
	}
	if len(p.CourseKinds) > 0 {
		placeholders := make([]string, len(p.CourseKinds))
		for i, kind := range p.CourseKinds {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, kind)
			argIdx++
		}
		where += " AND s.course_kind IN (" + strings.Join(placeholders, ", ") + ")"
	} else if p.CourseKind != "" {
		where += fmt.Sprintf(" AND s.course_kind = $%d", argIdx)
		args = append(args, p.CourseKind)
		argIdx++
	} else {
		where += " AND s.course_kind = 'line'"
	}
	if p.SegmentTypes != "" {
		types := strings.Split(p.SegmentTypes, ",")
		placeholders := make([]string, len(types))
		for i, t := range types {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, strings.TrimSpace(t))
			argIdx++
		}
		typeFilter := "s.segment_type IN (" + strings.Join(placeholders, ", ") + ")"
		if hasCheckpointCourseFilter {
			where += " AND (s.course_kind <> 'line' OR " + typeFilter + ")"
		} else {
			where += " AND " + typeFilter
		}
	} else if p.SegmentType != "" {
		typeFilter := fmt.Sprintf("s.segment_type = $%d", argIdx)
		args = append(args, p.SegmentType)
		argIdx++
		if hasCheckpointCourseFilter {
			where += " AND (s.course_kind <> 'line' OR " + typeFilter + ")"
		} else {
			where += " AND " + typeFilter
		}
	}
	if p.Name != "" {
		where += fmt.Sprintf(" AND s.name ILIKE $%d", argIdx)
		args = append(args, "%"+p.Name+"%")
		argIdx++
	}
	if p.BBox != "" {
		parts := strings.Split(p.BBox, ",")
		if len(parts) == 4 {
			if hasCheckpointCourseFilter && !hasLineCourseFilter {
				where += fmt.Sprintf(
					` AND cp.display_bounds IS NOT NULL AND ST_Intersects(cp.display_bounds, ST_MakeEnvelope($%d, $%d, $%d, $%d, 4326))`,
					argIdx, argIdx+1, argIdx+2, argIdx+3,
				)
				args = append(args, parts[0], parts[1], parts[2], parts[3])
				argIdx += 4
			} else {
				// Wir nutzen sv wenn vorhanden, sonst sv_bbox (redundant aber funktional korrekt getrennt)
				bboxTable := "sv"
				if !p.IncludeGeoJSON {
					bboxTable = "sv_bbox"
					from += `
	LEFT JOIN LATERAL (
		SELECT track
		FROM segment_versions
		WHERE segment_id = s.id
		  AND matching_contract_key = CASE
		      WHEN s.course_kind = 'checkpoint_sequence' THEN 'checkpoint_sequence_v1'
		      ELSE 'course_gps_v1'
		  END
		  AND matching_contract_version = 1
		  AND validity_basis <> 'rollback_retired'
		  AND (valid_from_at IS NULL OR valid_from_at <= NOW())
		  AND (valid_until_at IS NULL OR NOW() < valid_until_at)
		ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC,
		         version_number DESC
		LIMIT 1
	) sv_bbox ON true`
				}
				if hasCheckpointCourseFilter {
					where += fmt.Sprintf(
						` AND (
							(s.course_kind = 'line' AND %s.track IS NOT NULL AND ST_Intersects(%s.track, ST_MakeEnvelope($%d, $%d, $%d, $%d, 4326)))
							OR (s.course_kind = 'checkpoint_sequence' AND cp.display_bounds IS NOT NULL AND ST_Intersects(cp.display_bounds, ST_MakeEnvelope($%d, $%d, $%d, $%d, 4326)))
						)`,
						bboxTable, bboxTable, argIdx, argIdx+1, argIdx+2, argIdx+3,
						argIdx, argIdx+1, argIdx+2, argIdx+3,
					)
				} else {
					where += fmt.Sprintf(
						` AND %s.track IS NOT NULL AND ST_Intersects(%s.track, ST_MakeEnvelope($%d, $%d, $%d, $%d, 4326))`,
						bboxTable, bboxTable, argIdx, argIdx+1, argIdx+2, argIdx+3,
					)
				}
				args = append(args, parts[0], parts[1], parts[2], parts[3])
				argIdx += 4
			}
		}
	}
	if p.CompletedOnly {
		where += " AND COALESCE(ue.ridden_count, 0) > 0"
	} else if p.LiftsOnlyRidden {
		where += " AND (s.segment_type != 'lift' OR COALESCE(ue.ridden_count, 0) > 0)"
	}

	// Split-approach note (B4): visibility is split by Go (handler dedup + clamping required it).
	// SAC / MTB / Surface stay PG-side (string_to_array) — handler whitelist validators already
	// ensure the comma-string is safe, and the asymmetry is intentional: Go-split was needed
	// for visibility dedup; for the others no dedup is required and PG-split is fine.
	//
	// COALESCE for explicit null-safety; plan §2.2 Step 3B.
	// DifficultyMatch "any": SAC OR MTB in a single OR-block (when both are set).
	// DifficultyMatch "all" or "": classic AND (current behaviour).
	// C1: Use meta ? 'key' AND meta->>'key' = ANY(...) instead of COALESCE(meta->>'key','') = ANY(...).
	// The COALESCE wrap hides the expression from the partial-index planner — the index predicate
	// from Migration 000323 is (meta->>'sac_scale') WHERE meta ? 'sac_scale', which matches
	// exactly the two-part predicate below and enables an Index Scan instead of Seq Scan.
	// No new migration is needed: the 000323 indexes already have the correct expression form.
	if p.SacScale != "" && p.MtbScale != "" && p.DifficultyMatch == "any" {
		where += fmt.Sprintf(
			" AND ("+
				"(s.segment_type_meta ? 'sac_scale' AND s.segment_type_meta->>'sac_scale' = ANY(string_to_array($%d, ',')))"+
				" OR "+
				"(s.segment_type_meta ? 'mtb_scale' AND s.segment_type_meta->>'mtb_scale' = ANY(string_to_array($%d, ',')))"+
				")",
			argIdx, argIdx+1,
		)
		args = append(args, p.SacScale, p.MtbScale)
		argIdx += 2
	} else {
		if p.SacScale != "" {
			where += fmt.Sprintf(
				" AND s.segment_type_meta ? 'sac_scale' AND s.segment_type_meta->>'sac_scale' = ANY(string_to_array($%d, ','))",
				argIdx,
			)
			args = append(args, p.SacScale)
			argIdx++
		}
		if p.MtbScale != "" {
			where += fmt.Sprintf(
				" AND s.segment_type_meta ? 'mtb_scale' AND s.segment_type_meta->>'mtb_scale' = ANY(string_to_array($%d, ','))",
				argIdx,
			)
			args = append(args, p.MtbScale)
			argIdx++
		}
	}
	if p.Surface != "" {
		where += fmt.Sprintf(
			" AND s.segment_type_meta ? 'surface' AND s.segment_type_meta->>'surface' = ANY(string_to_array($%d, ','))",
			argIdx,
		)
		args = append(args, p.Surface)
		argIdx++
	}
	if p.ElevationKind != "" {
		where += fmt.Sprintf(" AND s.elevation_kind = $%d", argIdx)
		args = append(args, p.ElevationKind)
		argIdx++
		if !segmentTypeFilterIncludesLift(p.SegmentType, p.SegmentTypes) {
			where += " AND s.segment_type <> 'lift'"
		}
	}

	// --- Cursor ---
	if p.Cursor != "" {
		cursorTS, cursorID, err := decodeCursor(p.Cursor)
		if err == nil {
			where += fmt.Sprintf(" AND (s.created_at, s.id) < ($%d, $%d)", argIdx, argIdx+1)
			args = append(args, cursorTS, cursorID)
			argIdx += 2
		}
	}

	// --- ORDER + LIMIT ---
	orderLimit := "\nORDER BY s.created_at DESC, s.id DESC"
	limit := p.Limit
	if limit == 0 {
		limit = 50 // default
	}
	if limit > 0 {
		// limit+1 fuer has_more Erkennung
		orderLimit += fmt.Sprintf(" LIMIT %d", limit+1)
	}

	query := cte + selectCols + from + where + orderLimit

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list segments: %w", err)
	}
	defer rows.Close()

	var segments []model.Segment
	for rows.Next() {
		var s model.Segment
		var allowedPredicatesJSON []byte
		var controlSummaryJSON []byte
		var controlMapFeaturesJSON []byte
		err := rows.Scan(
			&s.ID, &s.CreatedBy, &s.Name, &s.Description, &s.SportGroup,
			&allowedPredicatesJSON,
			&s.SegmentType, &s.SegmentTypeMeta,
			&s.SourceActivityID, &s.OsmLiftID,
			&s.DistanceM, &s.ElevationGainM,
			&s.EffortCount, &s.AthleteCount,
			&s.RiddenCount, &s.IsFavorite,
			&s.Visibility, &s.AccessScopeID, &s.RouteHash, &s.CreatedAt,
			&s.TrackGeoJSON,
			&s.TerrainType, &s.ClimbType, &s.ElevationKind,
			&s.CourseKind, &controlSummaryJSON, &controlMapFeaturesJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("scan segment: %w", err)
		}
		s.AllowedPredicates, err = decodeSegmentPredicates(allowedPredicatesJSON)
		if err != nil {
			return nil, fmt.Errorf("decode segment predicates: %w", err)
		}
		if len(controlSummaryJSON) > 0 && string(controlSummaryJSON) != "null" {
			var summary model.CheckpointControlSummary
			if err := json.Unmarshal(controlSummaryJSON, &summary); err != nil {
				return nil, fmt.Errorf("decode checkpoint control summary: %w", err)
			}
			s.ControlSummary = &summary
		}
		if len(controlMapFeaturesJSON) > 0 && string(controlMapFeaturesJSON) != "null" {
			if err := json.Unmarshal(controlMapFeaturesJSON, &s.ControlMapFeatures); err != nil {
				return nil, fmt.Errorf("decode checkpoint control map features: %w", err)
			}
		}
		segments = append(segments, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list segments: %w", err)
	}
	if segments == nil {
		segments = []model.Segment{}
	}

	// has_more + cursor berechnen
	hasMore := limit > 0 && len(segments) > limit
	var nextCursor string
	if hasMore {
		segments = segments[:limit] // ueberzaehliges Element abschneiden
		last := segments[len(segments)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	return &model.ListSegmentsResult{
		Segments:   segments,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (db *DB) CreateSegment(ctx context.Context, userID string, create *model.SegmentCreate) (*model.Segment, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	segmentTypeMeta := create.SegmentTypeMeta
	if segmentTypeMeta == nil {
		segmentTypeMeta = json.RawMessage("{}")
	}

	// Resolve bidirectional: if caller passes nil, use sport-group default (true for most).
	// We store it explicitly so later edits can override without knowing the original default.
	bidirectional := true
	if create.Bidirectional != nil {
		bidirectional = *create.Bidirectional
	}
	allowedPredicates, err := segmentPredicatesForCreate(create.SegmentType, create.AllowedPredicates)
	if err != nil {
		return nil, err
	}
	allowedPredicatesJSON, err := marshalSegmentPredicates(allowedPredicates)
	if err != nil {
		return nil, fmt.Errorf("marshal segment predicates: %w", err)
	}
	segmentSportGroup := create.SportGroup
	if segmentSportGroup == "" {
		segmentSportGroup = "generic"
	}
	if allowedPredicates != nil {
		segmentSportGroup = segmentSportGroupFromPredicates(allowedPredicates)
	}
	var elevationKind any
	if create.ElevationKind != nil {
		elevationKind = *create.ElevationKind
	}
	visibility := strings.TrimSpace(create.Visibility)
	if visibility == "" {
		visibility = "public"
	}
	if err := db.validateSegmentAccessScopeForOwner(ctx, visibility, create.AccessScopeID, userID); err != nil {
		return nil, err
	}

	// Insert segment
	var s model.Segment
	var scannedPredicatesJSON []byte
	err = tx.QueryRow(ctx, `
			INSERT INTO segments (
				created_by, name, description, sport_group, segment_type,
				segment_type_meta, source_activity_id, terrain_type, climb_type,
				elevation_kind, visibility, access_scope_id, allowed_predicates,
				bidirectional, source_activity_publish_confirmed_at,
				source_activity_publish_confirmed_by
			)
			VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9,
				$10, $11, $12, $13,
				$14,
				CASE WHEN $7::uuid IS NOT NULL AND $11 IN ('public', 'shared') AND $15 THEN NOW() ELSE NULL END,
				CASE WHEN $7::uuid IS NOT NULL AND $11 IN ('public', 'shared') AND $15 THEN $1::uuid ELSE NULL END
			)
			RETURNING id, created_by, name, description, sport_group,
			          allowed_predicates,
			          segment_type, segment_type_meta,
		          source_activity_id, osm_lift_id,
		          distance_m, elevation_gain_m,
		          effort_count, athlete_count,
		          visibility, access_scope_id, route_hash, created_at,
		          terrain_type, climb_type, elevation_kind`,
		userID, create.Name, create.Description, segmentSportGroup,
		create.SegmentType, segmentTypeMeta, create.SourceActivityID,
		create.TerrainType, create.ClimbType, elevationKind, visibility, create.AccessScopeID,
		allowedPredicatesJSON, bidirectional, create.SourceActivityPublishConfirmed,
	).Scan(
		&s.ID, &s.CreatedBy, &s.Name, &s.Description, &s.SportGroup,
		&scannedPredicatesJSON,
		&s.SegmentType, &s.SegmentTypeMeta,
		&s.SourceActivityID, &s.OsmLiftID,
		&s.DistanceM, &s.ElevationGainM,
		&s.EffortCount, &s.AthleteCount,
		&s.Visibility, &s.AccessScopeID, &s.RouteHash, &s.CreatedAt,
		&s.TerrainType, &s.ClimbType, &s.ElevationKind,
	)
	if err != nil {
		return nil, fmt.Errorf("insert segment: %w", err)
	}
	s.AllowedPredicates, err = decodeSegmentPredicates(scannedPredicatesJSON)
	if err != nil {
		return nil, fmt.Errorf("decode segment predicates: %w", err)
	}

	trackGeoJSON, err := normalizeSegmentTrackForTypeTx(ctx, tx, create.SegmentType, create.GeoJSONTrack)
	if err != nil {
		return nil, err
	}

	// Insert first version with track geometry
	var versionID string
	var returnedTrackGeoJSON string
	err = tx.QueryRow(ctx, `
		WITH g AS (SELECT ST_SetSRID(ST_GeomFromGeoJSON($2), 4326) AS geom)
		INSERT INTO segment_versions (segment_id, version_number, track, start_point, end_point, valid_from, created_by)
		SELECT $1, 1, g.geom, ST_StartPoint(g.geom), ST_EndPoint(g.geom), CURRENT_DATE, $3
		FROM g
		RETURNING id, ST_AsGeoJSON(track)`,
		s.ID, trackGeoJSON, userID,
	).Scan(&versionID, &returnedTrackGeoJSON)
	if err != nil {
		return nil, fmt.Errorf("insert segment version: %w", err)
	}
	s.TrackGeoJSON = &returnedTrackGeoJSON
	if create.SourceActivityID != nil && (visibility == "public" || visibility == "shared") && create.SourceActivityPublishConfirmed {
		if _, err := tx.Exec(ctx, `
			UPDATE segments
			SET source_activity_publish_confirmed_segment_version_id = $2
			WHERE id = $1
		`, s.ID, versionID); err != nil {
			return nil, fmt.Errorf("store source activity publish confirmation version: %w", err)
		}
	}

	// Update distance_m via PostGIS
	_, err = tx.Exec(ctx, `
		UPDATE segments SET distance_m = (
			SELECT ST_Length(track::geography)
			FROM segment_versions WHERE segment_id = $1 AND version_number = 1
		) WHERE id = $1`, s.ID)
	if err != nil {
		return nil, fmt.Errorf("update distance: %w", err)
	}

	// Re-fetch distance
	tx.QueryRow(ctx, `SELECT distance_m FROM segments WHERE id = $1`, s.ID).Scan(&s.DistanceM)

	if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, s.ID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &s, nil
}

type GetSegmentOptions struct {
	IncludeGeoJSON        bool
	IncludeControls       bool
	IncludeOverlaySummary bool
	VisibleAccessScopeIDs []string
}

func (db *DB) GetSegment(ctx context.Context, segmentID, userID string, isAdmin bool, includeGeoJSON bool) (*model.Segment, error) {
	return db.GetSegmentWithOptions(ctx, segmentID, userID, isAdmin, GetSegmentOptions{
		IncludeGeoJSON: includeGeoJSON,
	})
}

func (db *DB) GetSegmentWithOptions(ctx context.Context, segmentID, userID string, isAdmin bool, opts GetSegmentOptions) (*model.Segment, error) {
	var s model.Segment

	trackSelect := "NULL::text"
	if opts.IncludeGeoJSON {
		trackSelect = "ST_AsGeoJSON(sv.track)"
	}

	args := []any{segmentID, userID, isAdmin}
	resolvedScopesExpr := "NULL::uuid[]"
	if len(opts.VisibleAccessScopeIDs) > 0 {
		args = append(args, opts.VisibleAccessScopeIDs)
		resolvedScopesExpr = fmt.Sprintf("$%d::uuid[]", len(args))
	}

	query := fmt.Sprintf(`
                SELECT s.id, s.created_by, s.name, s.description, s.sport_group,
                       s.allowed_predicates,
                       s.segment_type, s.segment_type_meta,
                       s.source_activity_id, s.osm_lift_id,
                       s.distance_m, s.elevation_gain_m,
                       s.effort_count + COALESCE((
                           SELECT COUNT(*)::int
                           FROM (
                               SELECT a.visibility,
                                      ROW_NUMBER() OVER (
                                          PARTITION BY COALESCE(rg.id::text, a.id::text), se.started_at, se.elapsed_time_s, se.leaderboard_direction
                                          ORDER BY
                                              CASE
                                                  WHEN rg.primary_activity_id IS NOT NULL AND a.id = rg.primary_activity_id THEN 0
                                                  WHEN COALESCE(a.is_group_secondary, false) = false THEN 1
                                                  ELSE 2
                                              END,
                                              se.created_at ASC,
                                              se.id ASC
                                      ) AS rn
                               FROM segment_efforts se
                               JOIN activities a ON se.activity_id = a.id
                               LEFT JOIN recording_group_members rgm ON rgm.activity_id = a.id
                               LEFT JOIN recording_groups rg ON rg.id = rgm.group_id
                               WHERE se.segment_id = s.id AND a.user_id = NULLIF($2::text, '')::uuid
                                 AND se.superseded_at IS NULL
                           ) user_efforts
                           WHERE rn = 1 AND visibility != 'public'
                       ), 0) AS effort_count, s.athlete_count,
                       COALESCE((
                           SELECT COUNT(*)::int
                           FROM (
                               SELECT ROW_NUMBER() OVER (
                                          PARTITION BY COALESCE(rg.id::text, a.id::text), se.started_at, se.elapsed_time_s, se.leaderboard_direction
                                          ORDER BY
                                              CASE
                                                  WHEN rg.primary_activity_id IS NOT NULL AND a.id = rg.primary_activity_id THEN 0
                                                  WHEN COALESCE(a.is_group_secondary, false) = false THEN 1
                                                  ELSE 2
                                              END,
                                              se.created_at ASC,
                                              se.id ASC
                                      ) AS rn
                               FROM segment_efforts se
                               JOIN activities a ON se.activity_id = a.id
                               LEFT JOIN recording_group_members rgm ON rgm.activity_id = a.id
                               LEFT JOIN recording_groups rg ON rg.id = rgm.group_id
                               WHERE se.segment_id = s.id AND a.user_id = NULLIF($2::text, '')::uuid
                                 AND se.superseded_at IS NULL
                           ) user_efforts
                           WHERE rn = 1
                       ), 0) AS ridden_count,
                       EXISTS(
                           SELECT 1 FROM segment_favorites sf
                           WHERE sf.segment_id = s.id AND sf.user_id = NULLIF($2::text, '')::uuid
                       ) AS is_favorite,
                       s.visibility, s.access_scope_id, s.route_hash, s.created_at,
                       %s AS track_geojson,
                       s.terrain_type, s.climb_type, s.elevation_kind, s.bidirectional,
                       s.course_kind, sv.id::text AS active_segment_version_id,
	                       cp.control_summary
	                FROM segments s
	                %s
	                LEFT JOIN checkpoint_course_projection cp
	                       ON cp.segment_id = s.id
	                      AND s.course_kind = 'checkpoint_sequence'
                WHERE s.id = $1
                  AND `+visibleSegmentAccessWithScopesSQL("s", "NULLIF($2::text, '')::uuid", "$3", resolvedScopesExpr),
		trackSelect,
		activeSegmentCourseVersionLateral("LEFT JOIN", "s.id", "s.course_kind", "NOW()", "id, track"),
	)

	var allowedPredicatesJSON []byte
	var activeSegmentVersionID *string
	var controlSummaryJSON []byte
	err := db.Pool.QueryRow(ctx, query, args...).Scan(
		&s.ID, &s.CreatedBy, &s.Name, &s.Description, &s.SportGroup,
		&allowedPredicatesJSON,
		&s.SegmentType, &s.SegmentTypeMeta,
		&s.SourceActivityID, &s.OsmLiftID,
		&s.DistanceM, &s.ElevationGainM,
		&s.EffortCount, &s.AthleteCount,
		&s.RiddenCount, &s.IsFavorite,
		&s.Visibility, &s.AccessScopeID, &s.RouteHash, &s.CreatedAt,
		&s.TrackGeoJSON,
		&s.TerrainType, &s.ClimbType, &s.ElevationKind, &s.Bidirectional,
		&s.CourseKind, &activeSegmentVersionID, &controlSummaryJSON,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get segment: %w", err)
	}
	s.AllowedPredicates, err = decodeSegmentPredicates(allowedPredicatesJSON)
	if err != nil {
		return nil, fmt.Errorf("decode segment predicates: %w", err)
	}
	if len(controlSummaryJSON) > 0 && string(controlSummaryJSON) != "null" {
		var summary model.CheckpointControlSummary
		if err := json.Unmarshal(controlSummaryJSON, &summary); err != nil {
			return nil, fmt.Errorf("decode checkpoint control summary: %w", err)
		}
		s.ControlSummary = &summary
	}
	if opts.IncludeControls {
		if s.CourseKind != string(model.CourseKindCheckpointSequence) {
			s.Controls = []model.CheckpointControl{}
		} else {
			if activeSegmentVersionID == nil || *activeSegmentVersionID == "" {
				return nil, fmt.Errorf("%w: checkpoint course has no active version", ErrSegmentValidation)
			}
			controls, err := db.GetCheckpointControls(ctx, *activeSegmentVersionID)
			if err != nil {
				return nil, err
			}
			s.Controls = controls
		}
	}
	if opts.IncludeOverlaySummary {
		summary, err := db.GetTargetOverlaySummary(ctx, "course", s.ID, nil)
		if err != nil {
			return nil, err
		}
		s.OverlaySummary = summary
	}
	return &s, nil
}

func (db *DB) UpdateSegment(ctx context.Context, segmentID, userID string, isAdmin bool, update *model.SegmentUpdate) (*model.Segment, error) {
	if segmentUpdateTouchesVersionedFields(update) && !update.NewVersion {
		return nil, fmt.Errorf("%w: segment matching, eligibility, privacy, and leaderboard metadata must use the versioned segment workflow", ErrSegmentValidation)
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin segment update tx: %w", err)
	}
	defer tx.Rollback(ctx)

	setClauses := []string{}
	args := []any{}
	argIdx := 1
	if update.Visibility != nil || update.AccessScopeID != nil {
		var ownerUserID, currentVisibility string
		var currentAccessScopeID *string
		var err error
		if isAdmin {
			err = tx.QueryRow(ctx, `SELECT created_by, visibility, access_scope_id FROM segments WHERE id = $1 FOR UPDATE`, segmentID).
				Scan(&ownerUserID, &currentVisibility, &currentAccessScopeID)
		} else {
			err = tx.QueryRow(ctx, `SELECT created_by, visibility, access_scope_id FROM segments WHERE id = $1 AND created_by = $2 FOR UPDATE`, segmentID, userID).
				Scan(&ownerUserID, &currentVisibility, &currentAccessScopeID)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get segment access state for update: %w", err)
		}
		effectiveVisibility := currentVisibility
		if update.Visibility != nil {
			effectiveVisibility = strings.TrimSpace(*update.Visibility)
		}
		effectiveAccessScopeID := currentAccessScopeID
		if update.AccessScopeID != nil {
			effectiveAccessScopeID = update.AccessScopeID
		}
		if effectiveVisibility != "shared" {
			if update.AccessScopeID != nil {
				return nil, validateSegmentAccessScopePair(effectiveVisibility, update.AccessScopeID)
			}
			effectiveAccessScopeID = nil
		}
		if err := db.validateSegmentAccessScopeForOwner(ctx, effectiveVisibility, effectiveAccessScopeID, ownerUserID); err != nil {
			return nil, err
		}
	}

	if update.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *update.Name)
		argIdx++
	}
	if update.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *update.Description)
		argIdx++
	}
	if update.SegmentType != nil {
		setClauses = append(setClauses, fmt.Sprintf("segment_type = $%d", argIdx))
		args = append(args, *update.SegmentType)
		argIdx++
	}
	if update.SegmentTypeMeta != nil {
		setClauses = append(setClauses, fmt.Sprintf("segment_type_meta = $%d", argIdx))
		args = append(args, *update.SegmentTypeMeta)
		argIdx++
	}
	if update.TerrainType != nil {
		setClauses = append(setClauses, fmt.Sprintf("terrain_type = $%d", argIdx))
		args = append(args, *update.TerrainType)
		argIdx++
	}
	if update.ClimbType != nil {
		setClauses = append(setClauses, fmt.Sprintf("climb_type = $%d", argIdx))
		args = append(args, *update.ClimbType)
		argIdx++
	}
	if update.ElevationKind != nil {
		setClauses = append(setClauses, fmt.Sprintf("elevation_kind = $%d", argIdx))
		args = append(args, *update.ElevationKind)
		argIdx++
	}
	// Note: private→public MUST go through PublishSegment (handler blocks it via 400).
	// This SET clause only handles public→private or same-value updates.
	if update.Visibility != nil {
		setClauses = append(setClauses, fmt.Sprintf("visibility = $%d", argIdx))
		args = append(args, strings.TrimSpace(*update.Visibility))
		argIdx++
		if strings.TrimSpace(*update.Visibility) != "shared" {
			setClauses = append(setClauses, "access_scope_id = NULL")
		}
	}
	if update.AccessScopeID != nil {
		setClauses = append(setClauses, fmt.Sprintf("access_scope_id = $%d", argIdx))
		args = append(args, *update.AccessScopeID)
		argIdx++
	}
	if len(update.AllowedPredicates) > 0 {
		effectiveSegmentType := ""
		if update.SegmentType != nil {
			effectiveSegmentType = *update.SegmentType
		} else {
			var ok bool
			effectiveSegmentType, ok, err = segmentTypeForUpdateTx(ctx, tx, segmentID, userID, isAdmin)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, nil
			}
		}
		allowedPredicates, shouldSet, err := segmentPredicatesForUpdate(effectiveSegmentType, update.AllowedPredicates)
		if err != nil {
			return nil, err
		}
		if shouldSet {
			allowedPredicatesJSON, err := marshalSegmentPredicates(allowedPredicates)
			if err != nil {
				return nil, fmt.Errorf("marshal segment predicates: %w", err)
			}
			if allowedPredicates != nil {
				setClauses = append(setClauses, fmt.Sprintf("sport_group = $%d", argIdx))
				args = append(args, segmentSportGroupFromPredicates(allowedPredicates))
				argIdx++
			}
			setClauses = append(setClauses, fmt.Sprintf("allowed_predicates = $%d", argIdx))
			args = append(args, allowedPredicatesJSON)
			argIdx++
		}
	}
	if update.SegmentType != nil && len(update.AllowedPredicates) == 0 {
		existingPredicates, ok, err := segmentPredicatesForUpdateTx(ctx, tx, segmentID, userID, isAdmin)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		if existingPredicates == nil && segmentTypeUsesDerivedDefaultPredicates(*update.SegmentType) {
			derivedPredicates, err := defaultSegmentPredicatesForType(*update.SegmentType)
			if err != nil {
				return nil, err
			}
			allowedPredicatesJSON, err := marshalSegmentPredicates(derivedPredicates)
			if err != nil {
				return nil, fmt.Errorf("marshal segment predicates: %w", err)
			}
			setClauses = append(setClauses, fmt.Sprintf("sport_group = $%d", argIdx))
			args = append(args, segmentSportGroupFromPredicates(derivedPredicates))
			argIdx++
			setClauses = append(setClauses, fmt.Sprintf("allowed_predicates = $%d", argIdx))
			args = append(args, allowedPredicatesJSON)
			argIdx++
		}
		if err := validateSegmentTypeChangePredicates(*update.SegmentType, existingPredicates); err != nil {
			return nil, err
		}
	}
	if update.Bidirectional != nil {
		setClauses = append(setClauses, fmt.Sprintf("bidirectional = $%d", argIdx))
		args = append(args, *update.Bidirectional)
		argIdx++
	}

	if len(setClauses) == 0 {
		if update.NewVersion {
			if err := db.createNewSegmentVersionTx(ctx, tx, segmentID); err != nil {
				return nil, err
			}
			if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
				return nil, err
			}
			if err := syncPublishedCourseProjectionAccessTx(ctx, tx, segmentID); err != nil {
				return nil, err
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, fmt.Errorf("commit segment update tx: %w", err)
			}
		}
		return db.GetSegment(ctx, segmentID, userID, isAdmin, false)
	}

	// Owner-check bypassed for admins (handler already authorized the edit).
	var query string
	if isAdmin {
		query = fmt.Sprintf("UPDATE segments SET %s WHERE id = $%d",
			strings.Join(setClauses, ", "), argIdx)
		args = append(args, segmentID)
	} else {
		query = fmt.Sprintf("UPDATE segments SET %s WHERE id = $%d AND created_by = $%d",
			strings.Join(setClauses, ", "), argIdx, argIdx+1)
		args = append(args, segmentID, userID)
	}

	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update segment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}
	if update.NewVersion {
		if err := db.createNewSegmentVersionTx(ctx, tx, segmentID); err != nil {
			return nil, err
		}
	} else {
		if err := syncActiveSegmentVersionEligibilitySnapshotTx(ctx, tx, segmentID); err != nil {
			return nil, err
		}
	}
	if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
		return nil, err
	}
	if err := syncPublishedCourseProjectionAccessTx(ctx, tx, segmentID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit segment update tx: %w", err)
	}

	return db.GetSegment(ctx, segmentID, userID, isAdmin, false)
}

func segmentUpdateTouchesVersionedFields(update *model.SegmentUpdate) bool {
	if update == nil {
		return false
	}
	return update.SegmentType != nil ||
		update.SegmentTypeMeta != nil ||
		update.Visibility != nil ||
		update.AccessScopeID != nil ||
		len(update.AllowedPredicates) > 0 ||
		update.Bidirectional != nil
}

func segmentTypeForUpdateTx(ctx context.Context, tx pgx.Tx, segmentID, userID string, isAdmin bool) (string, bool, error) {
	var segmentType string
	var err error
	if isAdmin {
		err = tx.QueryRow(ctx, `SELECT segment_type FROM segments WHERE id = $1 FOR UPDATE`, segmentID).Scan(&segmentType)
	} else {
		err = tx.QueryRow(ctx, `SELECT segment_type FROM segments WHERE id = $1 AND created_by = $2 FOR UPDATE`, segmentID, userID).Scan(&segmentType)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get segment type for update: %w", err)
	}
	return segmentType, true, nil
}

func segmentPredicatesForUpdateTx(ctx context.Context, tx pgx.Tx, segmentID, userID string, isAdmin bool) ([]sport.Predicate, bool, error) {
	var raw []byte
	var err error
	if isAdmin {
		err = tx.QueryRow(ctx, `SELECT allowed_predicates FROM segments WHERE id = $1 FOR UPDATE`, segmentID).Scan(&raw)
	} else {
		err = tx.QueryRow(ctx, `SELECT allowed_predicates FROM segments WHERE id = $1 AND created_by = $2 FOR UPDATE`, segmentID, userID).Scan(&raw)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get segment predicates for update: %w", err)
	}
	preds, err := decodeSegmentPredicates(raw)
	if err != nil {
		return nil, false, fmt.Errorf("decode segment predicates for update: %w", err)
	}
	return preds, true, nil
}

// createNewSegmentVersion copies the latest segment_version into a new version row.
func (db *DB) createNewSegmentVersion(ctx context.Context, segmentID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create new segment version tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := db.createNewSegmentVersionTx(ctx, tx, segmentID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create new segment version tx: %w", err)
	}
	return nil
}

// createNewSegmentVersionTx copies the latest segment_version into a new version row.
func (db *DB) createNewSegmentVersionTx(ctx context.Context, tx pgx.Tx, segmentID string) error {
	var actorUserID string
	if err := tx.QueryRow(ctx, `SELECT created_by FROM segments WHERE id = $1`, segmentID).Scan(&actorUserID); err != nil {
		return fmt.Errorf("select segment owner for new version: %w", err)
	}
	_, _, err := db.createNewSegmentVersionWithPolicyTx(ctx, tx, segmentID, actorUserID, "manual_new_version", "version_specific")
	return err
}

func (db *DB) createNewSegmentVersionWithPolicyTx(ctx context.Context, tx pgx.Tx, segmentID, actorUserID, operation, equivalenceDecision string) (string, string, error) {
	operationID := stableOperationHash(segmentID, actorUserID, operation, time.Now().UTC().Format(time.RFC3339Nano))
	isCheckpoint, err := segmentIsCheckpointCourseTx(ctx, tx, segmentID)
	if err != nil {
		return "", "", err
	}
	if isCheckpoint {
		return "", "", fmt.Errorf("%w: checkpoint course controls require a checkpoint version operation", ErrSegmentValidation)
	}
	before, err := LoadSegmentVersionSnapshotTx(ctx, tx, segmentID)
	if err != nil {
		return "", "", err
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
			SELECT id, segment_id, version_number, track, start_point, end_point, created_by
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
		)
		INSERT INTO segment_versions (
			segment_id, version_number, track, start_point, end_point, valid_from, created_by,
			valid_from_at, valid_until_at, validity_basis, matching_contract_key, matching_contract_version, published_at
		)
		SELECT segment_id,
		       next_version.number,
		       track,
		       start_point,
		       end_point,
		       CURRENT_DATE,
		       created_by,
		       boundary.at,
		       next_future.at,
		       'editor_asserted',
		       'course_gps_v1',
		       1,
		       boundary.at
		FROM active, next_version, next_future, boundary, closed
		RETURNING id`, segmentID).Scan(&versionID); err != nil {
		return "", "", fmt.Errorf("create new segment version: %w", err)
	}
	if err := applyEquivalenceDecisionTx(ctx, tx, segmentID, versionID, "", actorUserID, operationID, equivalenceDecision, operation, nil); err != nil {
		return "", "", err
	}
	after, err := LoadSegmentVersionSnapshotTx(ctx, tx, segmentID)
	if err != nil {
		return "", "", err
	}
	beforeHash, err := segmentVersionSnapshotHash(before)
	if err != nil {
		return "", "", err
	}
	inputHash, err := segmentVersionOperationInputHash(operation, map[string]any{
		"operation":            operation,
		"equivalence_decision": equivalenceDecision,
	})
	if err != nil {
		return "", "", err
	}
	if _, err := insertSegmentVersionRevisionTx(ctx, tx, segmentID, actorUserID, operation, operation, before, after, nil, nil, nil, nil, operationID, inputHash, nil, beforeHash); err != nil {
		return "", "", err
	}
	groupID, err := activeLeaderboardEquivalenceGroupTx(ctx, tx, segmentID, versionID)
	if err != nil {
		return "", "", err
	}
	return versionID, groupID, nil
}

func (db *DB) DeleteSegment(ctx context.Context, userID, segmentID string, isAdmin bool) error {
	var segType string
	var osmLiftID *int64
	var createdBy string
	err := db.Pool.QueryRow(ctx,
		`SELECT segment_type, osm_lift_id, created_by FROM segments WHERE id = $1`,
		segmentID).Scan(&segType, &osmLiftID, &createdBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSegmentNotFound
	}
	if err != nil {
		return fmt.Errorf("delete segment: %w", err)
	}
	if !isAdmin && createdBy != userID {
		return ErrSegmentNotAuthorized
	}
	if segType == "lift" && osmLiftID != nil {
		return ErrSegmentOSMLift
	}

	tag, err := db.Pool.Exec(ctx, `DELETE FROM segments WHERE id = $1`, segmentID)
	if err != nil {
		return fmt.Errorf("delete segment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSegmentNotFound
	}

	// Audit: log admin deletions of other users' segments (DSGVO Art. 5(2) Rechenschaftspflicht).
	if isAdmin && createdBy != userID {
		slog.Info("admin deleted segment", "admin_id", userID, "segment_id", segmentID, "segment_owner", createdBy)
	}

	return nil
}

// DeleteSegments bulk-deletes segments owned by userID.
// OSM-imported lift segments (segment_type='lift' AND osm_lift_id IS NOT NULL) are silently skipped.
// Returns the number of rows actually deleted.
func (db *DB) DeleteSegments(ctx context.Context, ids []string, userID string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res, err := db.Pool.Exec(ctx,
		`DELETE FROM segments
		 WHERE id = ANY($1::uuid[])
		   AND created_by = $2
		   AND NOT (segment_type = 'lift' AND osm_lift_id IS NOT NULL)`,
		ids, userID)
	if err != nil {
		return 0, fmt.Errorf("delete segments: %w", err)
	}
	return res.RowsAffected(), nil
}

// PublishSegment sets a private segment to public.
// Owner may publish; admins may publish any private segment (isAdmin skips ownership check).
// Returns false if segment not found, not owned (and not admin), or already public.
func (db *DB) PublishSegment(ctx context.Context, segmentID, actorID string, isAdmin bool) (bool, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin publish segment tx: %w", err)
	}
	defer tx.Rollback(ctx)
	published, _, _, err := db.publishSegmentTx(ctx, tx, segmentID, actorID, isAdmin, false)
	if err != nil {
		return false, err
	}
	if err := syncActiveSegmentVersionEligibilitySnapshotTx(ctx, tx, segmentID); err != nil {
		return false, err
	}
	if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
		return false, err
	}
	if err := syncPublishedCourseProjectionAccessTx(ctx, tx, segmentID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit publish segment tx: %w", err)
	}
	return published, nil
}

func (db *DB) PublishSegmentAndSeedRecalculationRun(ctx context.Context, segmentID, actorID string, isAdmin bool, operationID string, sourceActivityPublishConfirmed bool, metadata map[string]any) (bool, string, int, error) {
	if strings.TrimSpace(operationID) == "" {
		return false, "", 0, fmt.Errorf("%w: operation_id is required", ErrSegmentValidation)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return false, "", 0, fmt.Errorf("begin publish segment tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	published, ownerID, name, err := db.publishSegmentTx(ctx, tx, segmentID, actorID, isAdmin, sourceActivityPublishConfirmed)
	if err != nil {
		return false, "", 0, err
	}
	if !published {
		return false, "", 0, nil
	}
	if err := syncActiveSegmentVersionEligibilitySnapshotTx(ctx, tx, segmentID); err != nil {
		return false, "", 0, err
	}
	if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
		return false, "", 0, err
	}
	if err := syncPublishedCourseProjectionAccessTx(ctx, tx, segmentID); err != nil {
		return false, "", 0, err
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	if _, ok := metadata["visibility"]; !ok {
		metadata["visibility"] = "user_visible"
	}
	if _, ok := metadata["source"]; !ok {
		metadata["source"] = "segment_publish"
	}
	if _, ok := metadata["entity_name"]; !ok {
		metadata["entity_name"] = name
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return false, "", 0, fmt.Errorf("marshal publish recalculation metadata: %w", err)
	}
	scope := SegmentRecalculationScope{
		SegmentID:       segmentID,
		OwnerUserID:     ownerID,
		IncludeAllUsers: true,
		OperationID:     operationID,
	}
	idempotencyKey := segmentRecalculationIdempotencyKey(scope)
	runID, total, err := seedSegmentRecalculationRunTx(ctx, tx, actorID, scope, "segment publish recompute", idempotencyKey, metadataJSON)
	if err != nil {
		return false, "", 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, "", 0, fmt.Errorf("commit publish segment tx: %w", err)
	}
	return true, runID, total, nil
}

func (db *DB) publishSegmentTx(ctx context.Context, tx pgx.Tx, segmentID, actorID string, isAdmin bool, sourceActivityPublishConfirmed bool) (bool, string, string, error) {
	if err := ensureSegmentPublishAllowedTx(ctx, tx, segmentID); err != nil {
		return false, "", "", err
	}
	query := `
		SELECT s.created_by::text, s.name, s.source_activity_id::text,
		       s.source_activity_publish_confirmed_segment_version_id::text,
		       sv.id::text
		FROM segments s
		JOIN LATERAL (
			SELECT id
			FROM segment_versions
			WHERE segment_id = s.id
			  AND matching_contract_key = CASE
			      WHEN s.course_kind = 'checkpoint_sequence' THEN 'checkpoint_sequence_v1'
			      ELSE 'course_gps_v1'
			  END
			  AND matching_contract_version = 1
			  AND validity_basis <> 'rollback_retired'
			  AND (valid_from_at IS NULL OR valid_from_at <= NOW())
			  AND (valid_until_at IS NULL OR NOW() < valid_until_at)
			ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC,
			         version_number DESC
			LIMIT 1
		) sv ON true
		WHERE s.id = $1
		  AND s.visibility IN ('private', 'shared')`
	args := []any{segmentID}
	if !isAdmin {
		query += " AND s.created_by = $2"
		args = append(args, actorID)
	}
	query += " FOR UPDATE OF s"

	var ownerID, name string
	var sourceActivityID, confirmedVersionID, activeVersionID *string
	if err := tx.QueryRow(ctx, query, args...).Scan(&ownerID, &name, &sourceActivityID, &confirmedVersionID, &activeVersionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", "", nil
		}
		return false, "", "", fmt.Errorf("load segment publish state: %w", err)
	}

	needsConfirmationUpdate := false
	if sourceActivityID != nil && activeVersionID != nil {
		intersects, err := sourceActivityIntersectsActivePrivacyZonesTx(ctx, tx, ownerID, *sourceActivityID)
		if err != nil {
			return false, "", "", err
		}
		if intersects && (confirmedVersionID == nil || *confirmedVersionID != *activeVersionID) {
			if !sourceActivityPublishConfirmed {
				return false, "", "", ErrSegmentPublishPrivacyConfirmationRequired
			}
			needsConfirmationUpdate = true
		}
	}

	if needsConfirmationUpdate {
		_, err := tx.Exec(ctx, `
			UPDATE segments
			SET visibility = 'public',
			    access_scope_id = NULL,
			    source_activity_publish_confirmed_at = NOW(),
			    source_activity_publish_confirmed_by = $2,
			    source_activity_publish_confirmed_segment_version_id = $3
			WHERE id = $1
		`, segmentID, actorID, activeVersionID)
		if err != nil {
			return false, "", "", fmt.Errorf("publish segment: %w", err)
		}
		return true, ownerID, name, nil
	}

	if _, err := tx.Exec(ctx, `UPDATE segments SET visibility = 'public', access_scope_id = NULL WHERE id = $1`, segmentID); err != nil {
		return false, "", "", fmt.Errorf("publish segment: %w", err)
	}
	return true, ownerID, name, nil
}

func sourceActivityIntersectsActivePrivacyZonesTx(ctx context.Context, q Querier, userID, activityID string) (bool, error) {
	var intersects bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM activities a
			JOIN activity_tracks t ON t.activity_id = a.id
			JOIN user_privacy_zones z ON z.user_id = a.user_id
			WHERE a.id = $2
			  AND a.user_id = $1
			  AND z.is_active = true
			  AND t.track_full IS NOT NULL
			  AND ST_Intersects(
			      t.track_full,
			      CASE
			          WHEN z.zone_type = 'polygon' AND z.polygon IS NOT NULL THEN z.polygon
			          ELSE ST_Buffer(z.center::geography, z.radius_m)::geometry
			      END
			  )
		)
	`, userID, activityID).Scan(&intersects)
	if err != nil {
		return false, fmt.Errorf("check source activity privacy-zone intersection: %w", err)
	}
	return intersects, nil
}

func ensureSegmentPublishAllowedTx(ctx context.Context, tx pgx.Tx, segmentID string) error {
	var routeDerived bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM route_course_publications
			WHERE course_segment_id = $1
			  AND publication_state = 'published'
		)
	`, segmentID).Scan(&routeDerived); err != nil {
		return fmt.Errorf("check route-derived segment publish: %w", err)
	}
	if routeDerived {
		return fmt.Errorf("%w: route-derived courses must be published through route publication", ErrRoutePublicationValidation)
	}
	return nil
}

func (db *DB) GetSegmentLeaderboard(ctx context.Context, segmentID, sportType string, isReverse *bool, gearID string, onlyUserID string) ([]model.SegmentEffort, error) {
	return db.GetSegmentLeaderboardWithQuery(ctx, model.SegmentLeaderboardQuery{
		SegmentID:    segmentID,
		SportType:    sportType,
		IsReverse:    isReverse,
		GearID:       gearID,
		OnlyUserID:   onlyUserID,
		Scope:        "current_version",
		ElectricMode: "all",
	})
}

func (db *DB) GetSegmentLeaderboardWithQuery(ctx context.Context, q model.SegmentLeaderboardQuery) ([]model.SegmentEffort, error) {
	scope := strings.TrimSpace(q.Scope)
	if scope == "" {
		scope = "current_version"
	}
	electricMode := strings.TrimSpace(q.ElectricMode)
	if electricMode == "" {
		electricMode = "all"
	}
	if electricMode != "all" && electricMode != "exclude_electric" && electricMode != "only_electric" {
		return nil, fmt.Errorf("%w: invalid leaderboard electric mode", ErrSegmentValidation)
	}
	var courseKind string
	var activeVersionID string
	if err := db.Pool.QueryRow(ctx, `
		SELECT s.course_kind, COALESCE(sv.id::text, '')
		FROM segments s
		`+activeSegmentCourseVersionLateral("LEFT JOIN", "s.id", "s.course_kind", "NOW()", "id")+`
		WHERE s.id = $1
	`, q.SegmentID).Scan(&courseKind, &activeVersionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSegmentNotFound
		}
		return nil, fmt.Errorf("load segment leaderboard course kind: %w", err)
	}
	groupID := q.LeaderboardEquivalenceGroupID
	useVersionFilter := false
	versionFilterID := ""
	if courseKind == string(model.CourseKindCheckpointSequence) {
		switch scope {
		case "current_version":
			if activeVersionID == "" {
				return []model.SegmentEffort{}, nil
			}
			useVersionFilter = true
			versionFilterID = activeVersionID
		case "version":
			if strings.TrimSpace(q.VersionID) == "" {
				return nil, fmt.Errorf("%w: version leaderboard scope requires version id", ErrSegmentValidation)
			}
			useVersionFilter = true
			versionFilterID = q.VersionID
		case "sport_overall":
			if strings.TrimSpace(q.SportType) == "" {
				return nil, fmt.Errorf("%w: sport_overall leaderboard requires sport type", ErrSegmentValidation)
			}
		default:
			return nil, fmt.Errorf("%w: unsupported checkpoint leaderboard scope", ErrSegmentValidation)
		}
	} else if scope != "sport_overall" {
		var err error
		groupID, err = db.resolveSegmentLeaderboardGroup(ctx, q.SegmentID, scope, q.VersionID, groupID)
		if err != nil {
			return nil, err
		}
		if groupID == "" {
			return []model.SegmentEffort{}, nil
		}
	} else if strings.TrimSpace(q.SportType) == "" {
		return nil, fmt.Errorf("%w: sport_overall leaderboard requires sport type", ErrSegmentValidation)
	}

	query := `
		WITH matching AS (
			SELECT se.id, se.segment_id, se.segment_version_id, se.activity_id,
			       se.user_id, se.started_at, se.elapsed_time_s,
			       se.is_reverse, se.sport_type, se.sub_type,
		       COALESCE(act.is_indoor, false) AS is_indoor,
		       COALESCE(act.is_virtual, false) AS is_virtual,
		       COALESCE(act.is_electric, false) AS is_electric,
		       se.gear_id,
			       se.avg_power_w, se.avg_heart_rate, se.avg_speed_ms,
			       COALESCE(se.leaderboard_equivalence_group_id::text, '') AS leaderboard_equivalence_group_id,
			       COALESCE(current_gv.group_id::text, se.leaderboard_equivalence_group_id::text, '') AS current_leaderboard_equivalence_group_id,
			       se.leaderboard_sport_type,
		       se.leaderboard_direction,
		       sv.version_number,
		       se.in_leaderboard, se.created_at,
		       s.name AS segment_name,
		       u.display_name,
		       g.name AS gear_name,
		       act.visibility,
		       ROW_NUMBER() OVER (
		           PARTITION BY COALESCE(rg.id::text, act.id::text), se.started_at, se.elapsed_time_s, se.is_reverse
		           ORDER BY
		               CASE
		                   WHEN rg.primary_activity_id IS NOT NULL AND act.id = rg.primary_activity_id THEN 0
		                   WHEN COALESCE(act.is_group_secondary, false) = false THEN 1
		                   ELSE 2
		               END,
		               se.created_at ASC,
		               se.id ASC
		       ) AS canonical_rank
		FROM segment_efforts se
		JOIN segments s ON s.id = se.segment_id
		JOIN segment_versions sv ON sv.id = se.segment_version_id
		JOIN users u ON u.id = se.user_id
		LEFT JOIN gear g ON g.id = se.gear_id AND g.user_id = se.user_id
			JOIN activities act ON act.id = se.activity_id
			LEFT JOIN segment_leaderboard_equivalence_group_versions current_gv
			       ON current_gv.segment_version_id = se.segment_version_id
			      AND current_gv.leaderboard_surface = 'default'
			      AND current_gv.retired_at IS NULL
			LEFT JOIN recording_group_members rgm ON rgm.activity_id = act.id
		LEFT JOIN recording_groups rg ON rg.id = rgm.group_id`

	args := []any{q.SegmentID}
	argIdx := 2

	query += `
		WHERE se.segment_id = $1 AND se.in_leaderboard = true
		  AND se.superseded_at IS NULL`

	if useVersionFilter {
		query += fmt.Sprintf(" AND se.segment_version_id = $%d", argIdx)
		args = append(args, versionFilterID)
		argIdx++
	} else if scope != "sport_overall" {
		query += fmt.Sprintf(" AND COALESCE(current_gv.group_id, se.leaderboard_equivalence_group_id) = $%d::uuid", argIdx)
		args = append(args, groupID)
		argIdx++
	}
	if courseKind == string(model.CourseKindCheckpointSequence) {
		query += " AND se.leaderboard_contract_key = 'checkpoint_sequence_v1'"
	} else {
		query += " AND se.leaderboard_contract_key = 'course_gps_v1'"
	}

	if q.SportType != "" {
		query += fmt.Sprintf(" AND se.leaderboard_sport_type = $%d", argIdx)
		args = append(args, q.SportType)
		argIdx++
	}
	if q.IsReverse != nil {
		query += fmt.Sprintf(" AND se.leaderboard_direction = $%d", argIdx)
		if *q.IsReverse {
			args = append(args, "reverse")
		} else {
			args = append(args, "forward")
		}
		argIdx++
	}
	if q.GearID != "" {
		query += fmt.Sprintf(" AND se.gear_id = $%d", argIdx)
		args = append(args, q.GearID)
		argIdx++
	}
	switch electricMode {
	case "exclude_electric":
		query += " AND COALESCE(act.is_electric, false) = false"
	case "only_electric":
		query += " AND COALESCE(act.is_electric, false) = true"
	}

	query += `
		),
		global_ranked AS (
			SELECT id, started_at,
			       ROW_NUMBER() OVER (
				           PARTITION BY current_leaderboard_equivalence_group_id, segment_id, leaderboard_sport_type, leaderboard_direction
			           ORDER BY elapsed_time_s ASC
			       ) AS rank_overall,
			       (ROW_NUMBER() OVER (
				           PARTITION BY current_leaderboard_equivalence_group_id, segment_id, leaderboard_sport_type, leaderboard_direction
			           ORDER BY elapsed_time_s ASC
			       ) = 1) AS is_segment_record
			FROM matching
			WHERE canonical_rank = 1 AND visibility = 'public'
		),
		personal_ranked AS (
			SELECT id, started_at,
			       ROW_NUMBER() OVER (
				           PARTITION BY current_leaderboard_equivalence_group_id, segment_id, leaderboard_sport_type, leaderboard_direction, user_id
			           ORDER BY elapsed_time_s ASC
			       ) AS personal_rank
			FROM matching
			WHERE canonical_rank = 1
		)
		SELECT id, segment_id, segment_version_id, activity_id,
		       user_id, started_at, elapsed_time_s,
		       is_reverse, sport_type, sub_type,
		       is_indoor, is_virtual, is_electric,
		       gear_id,
		       avg_power_w, avg_heart_rate, avg_speed_ms,
		       gr.rank_overall, COALESCE(pr.personal_rank = 1, false),
		       CASE WHEN pr.personal_rank <= 3 THEN pr.personal_rank::smallint ELSE NULL END,
		       COALESCE(gr.is_segment_record, false),
		       in_leaderboard, created_at,
		       segment_name,
		       display_name,
		       gear_name,
		       version_number,
		       leaderboard_equivalence_group_id
		FROM matching
		LEFT JOIN global_ranked gr USING (id, started_at)
		LEFT JOIN personal_ranked pr USING (id, started_at)
		WHERE canonical_rank = 1`

	if q.OnlyUserID != "" {
		query += fmt.Sprintf(" AND user_id = $%d", argIdx)
		args = append(args, q.OnlyUserID)
		argIdx++
	} else {
		query += " AND visibility = 'public'"
	}

	query += `
		ORDER BY elapsed_time_s ASC
		LIMIT 100`

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get leaderboard: %w", err)
	}
	defer rows.Close()

	var efforts []model.SegmentEffort
	for rows.Next() {
		var e model.SegmentEffort
		err := rows.Scan(
			&e.ID, &e.SegmentID, &e.SegmentVersionID, &e.ActivityID,
			&e.UserID, &e.StartedAt, &e.ElapsedTimeS,
			&e.IsReverse, &e.SportType, &e.SubType,
			&e.IsIndoor, &e.IsVirtual, &e.IsElectric,
			&e.GearID,
			&e.AvgPowerW, &e.AvgHeartRate, &e.AvgSpeedMs,
			&e.RankOverall, &e.IsPersonalBest, &e.PersonalRank, &e.IsSegmentRecord,
			&e.InLeaderboard, &e.CreatedAt,
			&e.SegmentName, &e.DisplayName, &e.GearName,
			&e.VersionNumber, &e.LeaderboardEquivalenceGroupID,
		)
		if err != nil {
			return nil, fmt.Errorf("scan effort: %w", err)
		}
		efforts = append(efforts, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get segment efforts: %w", err)
	}
	if efforts == nil {
		efforts = []model.SegmentEffort{}
	}
	return efforts, nil
}

func (db *DB) resolveSegmentLeaderboardGroup(ctx context.Context, segmentID, scope, versionID, groupID string) (string, error) {
	switch scope {
	case "current_version":
		err := db.Pool.QueryRow(ctx, `
			WITH active_version AS (
				SELECT id
				FROM segment_versions
					WHERE segment_id = $1
					  AND matching_contract_key = 'course_gps_v1'
					  AND matching_contract_version = 1
					  AND validity_basis <> 'rollback_retired'
					  AND (valid_from_at IS NULL OR valid_from_at <= NOW())
					  AND (valid_until_at IS NULL OR NOW() < valid_until_at)
				ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC,
				         version_number DESC
				LIMIT 1
			)
			SELECT gv.group_id
			FROM segment_leaderboard_equivalence_group_versions gv
			JOIN segment_leaderboard_equivalence_groups g ON g.id = gv.group_id
			WHERE gv.segment_version_id = (SELECT id FROM active_version)
			  AND gv.leaderboard_surface = 'default'
			  AND gv.retired_at IS NULL
			  AND g.segment_id = $1
		`, segmentID).Scan(&groupID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("resolve current leaderboard group: %w", err)
		}
		return groupID, nil
	case "version":
		if versionID == "" {
			return "", fmt.Errorf("%w: version leaderboard scope requires version id", ErrSegmentValidation)
		}
		err := db.Pool.QueryRow(ctx, `
			SELECT gv.group_id
			FROM segment_leaderboard_equivalence_group_versions gv
			JOIN segment_leaderboard_equivalence_groups g ON g.id = gv.group_id
			WHERE gv.segment_version_id = $2
			  AND gv.leaderboard_surface = 'default'
			  AND gv.retired_at IS NULL
			  AND g.segment_id = $1
		`, segmentID, versionID).Scan(&groupID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("resolve version leaderboard group: %w", err)
		}
		return groupID, nil
	case "equivalence_group":
		if groupID == "" {
			return "", fmt.Errorf("%w: equivalence_group leaderboard scope requires group id", ErrSegmentValidation)
		}
		var exists bool
		if err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM segment_leaderboard_equivalence_groups
				WHERE id = $2 AND segment_id = $1
			)
		`, segmentID, groupID).Scan(&exists); err != nil {
			return "", fmt.Errorf("verify leaderboard group: %w", err)
		}
		if !exists {
			return "", ErrSegmentNotFound
		}
		return groupID, nil
	default:
		return "", fmt.Errorf("%w: invalid leaderboard scope", ErrSegmentValidation)
	}
}

func (db *DB) GetActivitySegmentEfforts(ctx context.Context, activityID string) ([]model.SegmentEffort, error) {
	rows, err := db.Pool.Query(ctx, `
		WITH ranked AS (
		SELECT se.id, se.segment_id, se.segment_version_id, se.activity_id,
		       se.user_id, se.started_at, se.elapsed_time_s,
		       se.is_reverse, se.sport_type, se.sub_type,
		       COALESCE(a.is_indoor, false) AS is_indoor,
		       COALESCE(a.is_virtual, false) AS is_virtual,
		       COALESCE(a.is_electric, false) AS is_electric,
		       se.gear_id,
		       se.avg_power_w, se.avg_heart_rate, se.avg_speed_ms,
		       se.rank_overall, se.is_personal_best, se.personal_rank, se.is_segment_record,
		       se.in_leaderboard, se.created_at,
		       s.name AS segment_name,
		       NULL::text AS display_name,
		       g.name AS gear_name,
		       s.segment_type_meta,
		       s.course_kind,
		       cp.control_summary,
		       ROW_NUMBER() OVER (
		           PARTITION BY se.segment_id, se.started_at, se.elapsed_time_s, se.is_reverse
		           ORDER BY se.created_at ASC, se.id ASC
		       ) AS canonical_rank
		FROM segment_efforts se
		JOIN segments s ON s.id = se.segment_id
		LEFT JOIN checkpoint_course_projection cp
		       ON cp.segment_id = s.id
		      AND s.course_kind = 'checkpoint_sequence'
		JOIN activities a ON a.id = se.activity_id
		LEFT JOIN gear g ON g.id = se.gear_id AND g.user_id = se.user_id
		WHERE se.activity_id = $1
		  AND se.superseded_at IS NULL
		)
		SELECT id, segment_id, segment_version_id, activity_id,
		       user_id, started_at, elapsed_time_s,
		       is_reverse, sport_type, sub_type,
		       is_indoor, is_virtual, is_electric,
		       gear_id,
		       avg_power_w, avg_heart_rate, avg_speed_ms,
		       rank_overall, is_personal_best, personal_rank, is_segment_record,
		       in_leaderboard, created_at,
		       segment_name,
		       display_name,
		       gear_name,
		       segment_type_meta,
		       course_kind,
		       control_summary
		FROM ranked
		WHERE canonical_rank = 1
		ORDER BY started_at ASC`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get activity segment efforts: %w", err)
	}
	defer rows.Close()

	var efforts []model.SegmentEffort
	for rows.Next() {
		var e model.SegmentEffort
		var controlSummaryJSON []byte
		err := rows.Scan(
			&e.ID, &e.SegmentID, &e.SegmentVersionID, &e.ActivityID,
			&e.UserID, &e.StartedAt, &e.ElapsedTimeS,
			&e.IsReverse, &e.SportType, &e.SubType,
			&e.IsIndoor, &e.IsVirtual, &e.IsElectric,
			&e.GearID,
			&e.AvgPowerW, &e.AvgHeartRate, &e.AvgSpeedMs,
			&e.RankOverall, &e.IsPersonalBest, &e.PersonalRank, &e.IsSegmentRecord,
			&e.InLeaderboard, &e.CreatedAt,
			&e.SegmentName, &e.DisplayName, &e.GearName,
			&e.SegmentTypeMeta,
			&e.CourseKind, &controlSummaryJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("scan effort: %w", err)
		}
		if len(controlSummaryJSON) > 0 && string(controlSummaryJSON) != "null" {
			var summary model.CheckpointControlSummary
			if err := json.Unmarshal(controlSummaryJSON, &summary); err != nil {
				return nil, fmt.Errorf("decode effort checkpoint summary: %w", err)
			}
			e.ControlSummary = &summary
		}
		efforts = append(efforts, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get activity segment efforts: %w", err)
	}
	if efforts == nil {
		efforts = []model.SegmentEffort{}
	}
	return efforts, nil
}

func (db *DB) GetActivitySegmentEffortHistory(ctx context.Context, activityID, segmentID string) ([]model.SegmentEffort, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT se.id, se.segment_id, se.segment_version_id, sv.version_number, se.activity_id,
		       se.user_id, se.started_at, se.elapsed_time_s,
		       se.is_reverse, se.sport_type, se.sub_type,
		       COALESCE(a.is_indoor, false) AS is_indoor,
		       COALESCE(a.is_virtual, false) AS is_virtual,
		       COALESCE(a.is_electric, false) AS is_electric,
		       se.gear_id,
		       se.avg_power_w, se.avg_heart_rate, se.avg_speed_ms,
		       se.rank_overall, se.is_personal_best, se.personal_rank, se.is_segment_record,
		       se.in_leaderboard, se.created_at,
		       se.superseded_by_effort_id, se.superseded_at, se.superseded_reason,
		       se.superseded_by_recalculation_run_id, se.created_by_recalculation_run_id,
		       COALESCE(se.leaderboard_equivalence_group_id::text, '') AS leaderboard_equivalence_group_id,
		       s.name AS segment_name,
		       NULL::text AS display_name,
		       g.name AS gear_name
		FROM segment_efforts se
		JOIN segment_versions sv ON sv.id = se.segment_version_id
		JOIN segments s ON s.id = se.segment_id
		JOIN activities a ON a.id = se.activity_id
		LEFT JOIN gear g ON g.id = se.gear_id AND g.user_id = se.user_id
		WHERE se.activity_id = $1
		  AND se.segment_id = $2
		ORDER BY se.started_at ASC, se.created_at ASC, se.id ASC`, activityID, segmentID)
	if err != nil {
		return nil, fmt.Errorf("get activity segment effort history: %w", err)
	}
	defer rows.Close()

	var efforts []model.SegmentEffort
	for rows.Next() {
		var e model.SegmentEffort
		if err := rows.Scan(
			&e.ID, &e.SegmentID, &e.SegmentVersionID, &e.VersionNumber, &e.ActivityID,
			&e.UserID, &e.StartedAt, &e.ElapsedTimeS,
			&e.IsReverse, &e.SportType, &e.SubType,
			&e.IsIndoor, &e.IsVirtual, &e.IsElectric,
			&e.GearID,
			&e.AvgPowerW, &e.AvgHeartRate, &e.AvgSpeedMs,
			&e.RankOverall, &e.IsPersonalBest, &e.PersonalRank, &e.IsSegmentRecord,
			&e.InLeaderboard, &e.CreatedAt,
			&e.SupersededByEffortID, &e.SupersededAt, &e.SupersededReason,
			&e.SupersededByRecalculationRunID, &e.CreatedByRecalculationRunID,
			&e.LeaderboardEquivalenceGroupID,
			&e.SegmentName, &e.DisplayName, &e.GearName,
		); err != nil {
			return nil, fmt.Errorf("scan segment effort history: %w", err)
		}
		efforts = append(efforts, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get activity segment effort history: %w", err)
	}
	if efforts == nil {
		efforts = []model.SegmentEffort{}
	}
	return efforts, nil
}

func (db *DB) InsertSegmentEffort(ctx context.Context, effort *model.SegmentEffort) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			segment_id, segment_version_id, activity_id, user_id,
			started_at, elapsed_time_s, is_reverse,
			sport_type, sub_type, gear_id,
			avg_speed_ms, avg_heart_rate, avg_power_w,
			distance_m, elevation_gain_m,
			matching_contract_key, matching_contract_version,
			match_source_type, match_source_id, match_identity_key,
			occurrence_index, occurrence_hash, superseded_by_effort_id,
			superseded_at, superseded_reason, superseded_by_recalculation_run_id,
			created_by_recalculation_run_id, leaderboard_equivalence_group_id,
			leaderboard_sport_type, leaderboard_sub_type,
			leaderboard_equipment_class, leaderboard_assist_class,
			leaderboard_indoor_policy_state, leaderboard_virtual_policy_state,
			leaderboard_electric_policy_state, leaderboard_direction,
			leaderboard_version_policy_key,
			leaderboard_contract_key, leaderboard_contract_version,
			activity_trait_bucket_key, eligibility_policy_key,
			eligibility_policy_version, eligibility_source_hash,
			in_leaderboard
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10,
			$11, $12, $13,
			$14, $15,
			$16, $17,
			$18, $19, $20,
			$21, $22, $23,
			$24, $25, $26,
			$27, $28,
			$29, $30,
			$31, $32,
			$33, $34,
			$35, $36,
			$37,
			$38, $39,
			$40, $41,
			$42, $43,
			$44
		)
		ON CONFLICT DO NOTHING`,
		effort.SegmentID, effort.SegmentVersionID, effort.ActivityID, effort.UserID,
		effort.StartedAt, effort.ElapsedTimeS, effort.IsReverse, effort.SportType, effort.SubType,
		effort.GearID, effort.AvgSpeedMs, effort.AvgHeartRate, effort.AvgPowerW,
		effort.DistanceM, effort.ElevationGainM,
		nullableString(effort.MatchingContractKey), nullableInt(effort.MatchingContractVersion),
		nullableString(effort.MatchSourceType), nullableString(effort.MatchSourceID), nullableString(effort.MatchIdentityKey),
		effort.OccurrenceIndex, effort.OccurrenceHash, effort.SupersededByEffortID,
		effort.SupersededAt, effort.SupersededReason, effort.SupersededByRecalculationRunID,
		effort.CreatedByRecalculationRunID, nullableString(effort.LeaderboardEquivalenceGroupID),
		nullableString(effort.LeaderboardSportType), nullableString(effort.LeaderboardSubType),
		nullableString(effort.LeaderboardEquipmentClass), nullableString(effort.LeaderboardAssistClass),
		nullableString(effort.LeaderboardIndoorPolicyState), nullableString(effort.LeaderboardVirtualPolicyState),
		nullableString(effort.LeaderboardElectricPolicyState), nullableString(effort.LeaderboardDirection),
		nullableString(effort.LeaderboardVersionPolicyKey),
		nullableString(effort.LeaderboardContractKey), nullableInt(effort.LeaderboardContractVersion),
		nullableString(effort.ActivityTraitBucketKey), segmentEffortEligibilityPolicyKey(effort),
		segmentEffortEligibilityPolicyVersion(effort), nullableString(effort.EligibilitySourceHash),
		segmentEffortInLeaderboard(effort),
	)
	if err != nil {
		return fmt.Errorf("insert segment effort: %w", err)
	}
	return nil
}

func (db *DB) UpdateSegmentEffortRanks(ctx context.Context, segmentID string) error {
	return updateSegmentEffortRanks(ctx, db.Pool, segmentID)
}

func UpdateSegmentEffortRanksTx(ctx context.Context, tx pgx.Tx, segmentID string) error {
	return updateSegmentEffortRanks(ctx, tx, segmentID)
}

func updateSegmentEffortRanks(ctx context.Context, conn Querier, segmentID string) error {
	// Two CTEs:
	// global_ranked: ranks efforts from non-private activities only (for public leaderboard).
	// personal_ranked: ranks ALL of a user's efforts, including private activities.
	//   Intentional: a user should see their own personal best even if the activity is private.
	//   The global rank (rank_overall) will be NULL for private-activity efforts — that's correct.
	//
	// TimescaleDB note: both CTEs include started_at so the UPDATE WHERE clause can use the
	// composite PK (id, started_at) — required for efficient chunk pruning on the hypertable.
	_, err := conn.Exec(ctx, `
		WITH
			canonical AS (
				SELECT se.id, se.started_at, se.user_id,
				       se.leaderboard_equivalence_group_id,
				       COALESCE(current_gv.group_id, se.leaderboard_equivalence_group_id) AS current_leaderboard_equivalence_group_id,
				       se.leaderboard_sport_type, se.leaderboard_direction, se.elapsed_time_s,
				       a.visibility,
				       ROW_NUMBER() OVER (
				           PARTITION BY COALESCE(current_gv.group_id, se.leaderboard_equivalence_group_id), COALESCE(rg.id::text, a.id::text), se.started_at, se.elapsed_time_s, se.leaderboard_direction
			           ORDER BY
			               CASE
			                   WHEN rg.primary_activity_id IS NOT NULL AND a.id = rg.primary_activity_id THEN 0
			                   WHEN COALESCE(a.is_group_secondary, false) = false THEN 1
			                   ELSE 2
			               END,
			               se.created_at ASC,
			               se.id ASC
			       ) AS canonical_rank
				FROM segment_efforts se
				JOIN activities a ON a.id = se.activity_id
				LEFT JOIN segment_leaderboard_equivalence_group_versions current_gv
				       ON current_gv.segment_version_id = se.segment_version_id
				      AND current_gv.leaderboard_surface = 'default'
				      AND current_gv.retired_at IS NULL
				LEFT JOIN recording_group_members rgm ON rgm.activity_id = a.id
			LEFT JOIN recording_groups rg ON rg.id = rgm.group_id
			WHERE se.segment_id = $1
			  AND se.in_leaderboard = true
			  AND se.superseded_at IS NULL
		),
		global_ranked AS (
			SELECT c.id, c.started_at,
				       ROW_NUMBER() OVER (
				           PARTITION BY c.current_leaderboard_equivalence_group_id, c.leaderboard_sport_type, c.leaderboard_direction
			           ORDER BY c.elapsed_time_s ASC
			       ) AS rk,
				       (ROW_NUMBER() OVER (
				           PARTITION BY c.current_leaderboard_equivalence_group_id, c.leaderboard_sport_type, c.leaderboard_direction
			           ORDER BY c.elapsed_time_s ASC
			       ) = 1) AS is_record
			FROM canonical c
			WHERE c.canonical_rank = 1 AND c.visibility = 'public'
		),
		personal_ranked AS (
			SELECT id, started_at,
				       ROW_NUMBER() OVER (
				           PARTITION BY current_leaderboard_equivalence_group_id, leaderboard_sport_type, leaderboard_direction, user_id
			           ORDER BY elapsed_time_s ASC
			       ) AS user_rk
			FROM canonical
			WHERE canonical_rank = 1
		),
		rank_updates AS (
			SELECT c.id, c.started_at, c.canonical_rank, pr.user_rk, gr.rk, gr.is_record
			FROM canonical c
			LEFT JOIN personal_ranked pr ON pr.id = c.id AND pr.started_at = c.started_at
			LEFT JOIN global_ranked gr ON gr.id = c.id AND gr.started_at = c.started_at
		)
		UPDATE segment_efforts se
		SET rank_overall      = CASE WHEN ru.canonical_rank = 1 THEN ru.rk ELSE NULL END,
		    is_personal_best  = COALESCE(ru.canonical_rank = 1 AND ru.user_rk = 1, false),
		    personal_rank     = CASE WHEN ru.canonical_rank = 1 AND ru.user_rk <= 3 THEN ru.user_rk::smallint ELSE NULL END,
		    is_segment_record = COALESCE(ru.canonical_rank = 1 AND ru.is_record, false)
		FROM rank_updates ru
		WHERE se.id = ru.id AND se.started_at = ru.started_at`, segmentID)
	if err != nil {
		return fmt.Errorf("update effort ranks: %w", err)
	}

	// Update effort_count and athlete_count: public canonical efforts only.
	_, err = conn.Exec(ctx, `
		WITH canonical AS (
			SELECT se.user_id,
			       ROW_NUMBER() OVER (
			           PARTITION BY COALESCE(rg.id::text, a.id::text), se.started_at, se.elapsed_time_s, se.leaderboard_direction
			           ORDER BY
			               CASE
			                   WHEN rg.primary_activity_id IS NOT NULL AND a.id = rg.primary_activity_id THEN 0
			                   WHEN COALESCE(a.is_group_secondary, false) = false THEN 1
			                   ELSE 2
			               END,
			               se.created_at ASC,
			               se.id ASC
			       ) AS canonical_rank
			FROM segment_efforts se
			JOIN activities a ON a.id = se.activity_id AND a.visibility = 'public'
			LEFT JOIN recording_group_members rgm ON rgm.activity_id = a.id
			LEFT JOIN recording_groups rg ON rg.id = rgm.group_id
			WHERE se.segment_id = $1
			  AND se.superseded_at IS NULL
		)
		UPDATE segments SET
			effort_count = (
				SELECT COUNT(*) FROM canonical WHERE canonical_rank = 1
			),
			athlete_count = (
				SELECT COUNT(DISTINCT user_id) FROM canonical WHERE canonical_rank = 1
			)
		WHERE id = $1`, segmentID)
	if err != nil {
		return fmt.Errorf("update segment counts: %w", err)
	}

	return nil
}

// CandidateSegment holds info for spatial pre-filtered segment matching.
type CandidateSegment struct {
	SegmentID              string
	VersionID              string
	SportGroup             string
	DistanceM              float64
	SegmentType            string
	TypeMeta               map[string]any
	TrackWKB               []byte // Segment track as WKB for in-memory Hausdorff
	Bidirectional          bool
	ActivityTraitBucketKey string
	RankAllowed            bool
	CompletionAllowed      bool
	EligibilitySourceHash  string
}

type CheckpointCandidateSegment struct {
	CandidateSegment
	Controls []model.CheckpointControl
}

// GetSpatialCandidateSegments returns segments within 50m of the activity track,
// filtered by the course eligibility projection. Uses ST_DWithin with GIST
// index for fast spatial pre-filtering after indexed eligibility lookup.
func (db *DB) GetSpatialCandidateSegments(ctx context.Context, activityID, userID, activityTraitBucketKey string, activityDate time.Time) ([]CandidateSegment, error) {
	query := `
		SELECT s.id, sv.id, s.sport_group, COALESCE(s.distance_m, 0),
		       s.segment_type, COALESCE(s.segment_type_meta, '{}'),
		       ST_AsBinary(sv.track), s.bidirectional,
		       cep.activity_trait_bucket_key, cep.rank_allowed,
		       cep.completion_allowed, cep.source_hash
		FROM segments s
		` + activeCourseGPSV1VersionLateral("JOIN", "s.id", "$4", "id, track") + `
		JOIN target_course_eligibility_projection cep
		  ON cep.target_kind = 'segment'
		 AND cep.target_id = s.id
		 AND cep.target_version_id = sv.id
		 AND cep.activity_trait_bucket_key = $3
		 AND cep.match_allowed = true
		JOIN activity_tracks at ON at.activity_id = $1
		WHERE ` + visibleSegmentExportSQL("s", "$2") + `
		  AND at.track_full IS NOT NULL
		  AND sv.track IS NOT NULL
		  AND ST_DWithin(at.track_full::geography, sv.track::geography, 100)`
	rows, err := db.Pool.Query(ctx, query, activityID, userID, activityTraitBucketKey, activityDate)
	if err != nil {
		return nil, fmt.Errorf("get spatial candidate segments: %w", err)
	}
	defer rows.Close()

	var result []CandidateSegment
	for rows.Next() {
		var c CandidateSegment
		var rawMeta json.RawMessage
		if err := rows.Scan(&c.SegmentID, &c.VersionID, &c.SportGroup, &c.DistanceM,
			&c.SegmentType, &rawMeta, &c.TrackWKB, &c.Bidirectional,
			&c.ActivityTraitBucketKey, &c.RankAllowed, &c.CompletionAllowed,
			&c.EligibilitySourceHash); err != nil {
			return nil, fmt.Errorf("scan candidate segment: %w", err)
		}
		if len(rawMeta) > 0 && string(rawMeta) != "{}" {
			_ = json.Unmarshal(rawMeta, &c.TypeMeta)
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get spatial candidate segments: %w", err)
	}
	return result, nil
}

func ApplyCandidateEligibilityToEffort(e *model.SegmentEffort, candidate CandidateSegment) {
	e.ActivityTraitBucketKey = candidate.ActivityTraitBucketKey
	e.EligibilityPolicyKey = courseEligibilityPolicyKey
	e.EligibilityPolicyVersion = courseEligibilityPolicyVersion
	e.EligibilitySourceHash = candidate.EligibilitySourceHash
	e.InLeaderboard = candidate.RankAllowed
}

func (db *DB) GetCheckpointCandidateSegments(ctx context.Context, activityID, userID, activityTraitBucketKey string, activityDate time.Time) ([]CheckpointCandidateSegment, error) {
	query := `
		SELECT s.id, sv.id, s.sport_group, COALESCE(s.distance_m, 0),
		       s.segment_type, COALESCE(s.segment_type_meta, '{}'),
		       s.bidirectional,
		       cep.activity_trait_bucket_key, cep.rank_allowed,
		       cep.completion_allowed, cep.source_hash
		FROM segments s
		` + activeSegmentCourseVersionLateral("JOIN", "s.id", "s.course_kind", "$4", "id") + `
		JOIN checkpoint_course_projection cp ON cp.segment_id = s.id AND cp.active_segment_version_id = sv.id
		JOIN target_course_eligibility_projection cep
		  ON cep.target_kind = 'segment'
		 AND cep.target_id = s.id
		 AND cep.target_version_id = sv.id
		 AND cep.activity_trait_bucket_key = $3
		 AND cep.match_allowed = true
		JOIN activity_tracks at ON at.activity_id = $1
		WHERE s.course_kind = 'checkpoint_sequence'
		  AND ` + visibleSegmentExportSQL("s", "$2") + `
		  AND at.track_full IS NOT NULL
		  AND NOT EXISTS (
		      SELECT 1
		      FROM course_version_controls c
		      WHERE c.segment_version_id = sv.id
		        AND c.required = true
		        AND NOT (
		            (
		                c.geometry_shape IN ('point', 'circle')
		                AND ST_DWithin(
		                    at.track_full::geography,
		                    c.geom::geography,
		                    GREATEST(COALESCE(c.hit_radius_m, c.area_radius_m, 0), 1)
		                )
		            )
		            OR (
		                c.geometry_shape IN ('polygon', 'multipolygon')
		                AND ST_Intersects(at.track_full, c.geom)
		            )
		        )
		  )`
	rows, err := db.Pool.Query(ctx, query, activityID, userID, activityTraitBucketKey, activityDate)
	if err != nil {
		return nil, fmt.Errorf("get checkpoint candidate segments: %w", err)
	}
	defer rows.Close()

	var result []CheckpointCandidateSegment
	var versionIDs []string
	for rows.Next() {
		var c CheckpointCandidateSegment
		var rawMeta json.RawMessage
		if err := rows.Scan(&c.SegmentID, &c.VersionID, &c.SportGroup, &c.DistanceM,
			&c.SegmentType, &rawMeta, &c.Bidirectional,
			&c.ActivityTraitBucketKey, &c.RankAllowed, &c.CompletionAllowed,
			&c.EligibilitySourceHash); err != nil {
			return nil, fmt.Errorf("scan checkpoint candidate segment: %w", err)
		}
		if len(rawMeta) > 0 && string(rawMeta) != "{}" {
			_ = json.Unmarshal(rawMeta, &c.TypeMeta)
		}
		result = append(result, c)
		versionIDs = append(versionIDs, c.VersionID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get checkpoint candidate segments: %w", err)
	}
	controlsByVersionID, err := db.getCheckpointControlsByVersionIDs(ctx, versionIDs)
	if err != nil {
		return nil, err
	}
	for i := range result {
		controls := controlsByVersionID[result[i].VersionID]
		if controls == nil {
			controls = []model.CheckpointControl{}
		}
		result[i].Controls = controls
	}
	return result, nil
}

func (db *DB) getCheckpointControlsByVersionIDs(ctx context.Context, segmentVersionIDs []string) (map[string][]model.CheckpointControl, error) {
	controlsByVersionID := make(map[string][]model.CheckpointControl, len(segmentVersionIDs))
	if len(segmentVersionIDs) == 0 {
		return controlsByVersionID, nil
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT segment_version_id::text, id::text, sequence_index, role, primitive, geometry_shape, label,
		       ST_AsGeoJSON(geom), hit_radius_m, area_radius_m, required
		FROM course_version_controls
		WHERE segment_version_id = ANY($1)
		ORDER BY segment_version_id, sequence_index
	`, segmentVersionIDs)
	if err != nil {
		return nil, fmt.Errorf("batch load checkpoint controls: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var versionID string
		var control model.CheckpointControl
		var geometry string
		if err := rows.Scan(&versionID, &control.ID, &control.SequenceIndex, &control.Role, &control.Primitive,
			&control.GeometryShape, &control.Label, &geometry, &control.HitRadiusM,
			&control.AreaRadiusM, &control.Required); err != nil {
			return nil, fmt.Errorf("scan checkpoint control: %w", err)
		}
		control.Geometry = json.RawMessage(geometry)
		controlsByVersionID[versionID] = append(controlsByVersionID[versionID], control)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checkpoint controls: %w", err)
	}
	return controlsByVersionID, nil
}

func (db *DB) GetSegmentMatchActivityContext(ctx context.Context, activityID string) (SegmentMatchActivityContext, error) {
	var result SegmentMatchActivityContext
	err := db.Pool.QueryRow(ctx, `
		SELECT COALESCE(is_indoor, false),
		       COALESCE(is_virtual, false),
		       COALESCE(is_electric, false)
		FROM activities
		WHERE id = $1`, activityID).Scan(&result.IsIndoor, &result.IsVirtual, &result.IsElectric)
	if err != nil {
		return result, fmt.Errorf("get segment match activity context: %w", err)
	}
	return result, nil
}

// SegmentEndpoints holds start/end coordinates and length of a segment version.
type SegmentEndpoints struct {
	StartLat float64
	StartLng float64
	EndLat   float64
	EndLng   float64
	LengthM  float64
}

// GetSegmentEndpoints returns start/end point coordinates and geographic length of a segment version.
func (db *DB) GetSegmentEndpoints(ctx context.Context, versionID string) (*SegmentEndpoints, error) {
	var ep SegmentEndpoints
	err := db.Pool.QueryRow(ctx, `
		SELECT ST_Y(ST_StartPoint(track)), ST_X(ST_StartPoint(track)),
		       ST_Y(ST_EndPoint(track)), ST_X(ST_EndPoint(track)),
		       ST_Length(track::geography)
		FROM segment_versions
		WHERE id = $1 AND track IS NOT NULL`, versionID,
	).Scan(&ep.StartLat, &ep.StartLng, &ep.EndLat, &ep.EndLng, &ep.LengthM)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get segment endpoints: %w", err)
	}
	return &ep, nil
}

// GetSegmentEndpointsBatch returns endpoints for multiple segment versions in a single query.
func (db *DB) GetSegmentEndpointsBatch(ctx context.Context, versionIDs []string) (map[string]*SegmentEndpoints, error) {
	if len(versionIDs) == 0 {
		return map[string]*SegmentEndpoints{}, nil
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT id,
		       ST_Y(ST_StartPoint(track)), ST_X(ST_StartPoint(track)),
		       ST_Y(ST_EndPoint(track)), ST_X(ST_EndPoint(track)),
		       ST_Length(track::geography)
		FROM segment_versions
		WHERE id = ANY($1) AND track IS NOT NULL`, versionIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("get segment endpoints batch: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*SegmentEndpoints, len(versionIDs))
	for rows.Next() {
		var id string
		var ep SegmentEndpoints
		if err := rows.Scan(&id, &ep.StartLat, &ep.StartLng, &ep.EndLat, &ep.EndLng, &ep.LengthM); err != nil {
			return nil, fmt.Errorf("get segment endpoints batch scan: %w", err)
		}
		result[id] = &ep
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get segment endpoints batch rows: %w", err)
	}
	return result, nil
}

// GPSStreamPoint is a lightweight GPS point with timestamp for matching.
type GPSStreamPoint struct {
	Time      time.Time
	Lat       float64
	Lng       float64
	SpeedMs   *float64 // NEW: speed in m/s (nullable)
	HeartRate *int     // NEW: heart rate in bpm (nullable)
	PowerW    *int     // NEW: power in watts (nullable)
	AltitudeM *float64 // NEW: altitude in meters (nullable)
}

// GetActivityGPSStream returns all GPS stream points (time, lat, lng) for an activity,
// ordered chronologically. Only points with valid GPS coordinates are returned.
func (db *DB) GetActivityGPSStream(ctx context.Context, activityID string) ([]GPSStreamPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT "time", lat, lng, speed_ms, heart_rate, power_w, altitude_m
		FROM activity_streams
		WHERE activity_id = $1 AND lat IS NOT NULL AND lng IS NOT NULL
		ORDER BY "time" ASC`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get activity GPS stream: %w", err)
	}
	defer rows.Close()

	var points []GPSStreamPoint
	for rows.Next() {
		var p GPSStreamPoint
		if err := rows.Scan(&p.Time, &p.Lat, &p.Lng, &p.SpeedMs, &p.HeartRate, &p.PowerW, &p.AltitudeM); err != nil {
			return nil, fmt.Errorf("scan GPS stream point: %w", err)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get GPS stream points: %w", err)
	}
	return points, nil
}

// NOTE: GetSegmentEffortMetrics, GetSegmentEffortMetricsAndElevation, and
// GetSubTrackElevationGain have been replaced by in-memory computeEffortMetrics
// in handler/segment_matching.go (eliminates per-candidate DB round-trips).

// DeleteSegmentEffortsByActivity removes all segment efforts for a given activity.
func (db *DB) DeleteSegmentEffortsByActivity(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM segment_efforts WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete segment efforts by activity: %w", err)
	}
	return nil
}

// DeleteSegmentEffortsBySegment removes all efforts for a segment by a specific user.
func (db *DB) DeleteSegmentEffortsBySegment(ctx context.Context, segmentID, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM segment_efforts WHERE segment_id = $1 AND user_id = $2`, segmentID, userID)
	if err != nil {
		return fmt.Errorf("delete segment efforts by segment: %w", err)
	}
	return nil
}

// DeleteSegmentEffortsBySegmentForActivities removes efforts only for the
// activity set that will be recomputed. Historical efforts outside the current
// backfill candidate set remain attached to their original segment version.
func (db *DB) DeleteSegmentEffortsBySegmentForActivities(ctx context.Context, segmentID, userID string, activityIDs []string) error {
	if len(activityIDs) == 0 {
		return nil
	}
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM segment_efforts
		 WHERE segment_id = $1
		   AND user_id = $2
		   AND activity_id = ANY($3::uuid[])`,
		segmentID, userID, activityIDs)
	if err != nil {
		return fmt.Errorf("delete segment efforts by segment activities: %w", err)
	}
	return nil
}

// GetAffectedSegmentIDs returns all segment IDs that have efforts for a given activity.
func (db *DB) GetAffectedSegmentIDs(ctx context.Context, activityID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT DISTINCT segment_id
		 FROM segment_efforts
		 WHERE activity_id = $1
		   AND superseded_at IS NULL`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get affected segment IDs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan segment ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list segment IDs: %w", err)
	}
	return ids, nil
}

// InsertDetectedLaps batch-inserts detected laps for an activity.
func (db *DB) InsertDetectedLaps(ctx context.Context, activityID string, laps []model.DetectedLap) error {
	if len(laps) == 0 {
		return nil
	}
	for _, lap := range laps {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_detected_laps (
				activity_id, lap_number, start_time, end_time, elapsed_time_s,
				distance_m, avg_speed_ms, avg_heart_rate, avg_power_w, avg_cadence,
				total_ascent_m, total_descent_m, start_lat, start_lng
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			activityID, lap.LapNumber, lap.StartTime, lap.EndTime, lap.ElapsedTimeS,
			lap.DistanceM, lap.AvgSpeedMs, lap.AvgHeartRate, lap.AvgPowerW, lap.AvgCadence,
			lap.TotalAscentM, lap.TotalDescentM, lap.StartLat, lap.StartLng,
		)
		if err != nil {
			return fmt.Errorf("insert detected lap %d: %w", lap.LapNumber, err)
		}
	}
	return nil
}

// GetDetectedLaps returns all detected laps for an activity, ordered by lap number.
func (db *DB) GetDetectedLaps(ctx context.Context, activityID string) ([]model.DetectedLap, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, activity_id, lap_number, start_time, end_time, elapsed_time_s,
		       distance_m, avg_speed_ms, avg_heart_rate, avg_power_w, avg_cadence,
		       total_ascent_m, total_descent_m, start_lat, start_lng
		FROM activity_detected_laps
		WHERE activity_id = $1
		ORDER BY lap_number ASC`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get detected laps: %w", err)
	}
	defer rows.Close()

	var laps []model.DetectedLap
	for rows.Next() {
		var l model.DetectedLap
		if err := rows.Scan(
			&l.ID, &l.ActivityID, &l.LapNumber, &l.StartTime, &l.EndTime, &l.ElapsedTimeS,
			&l.DistanceM, &l.AvgSpeedMs, &l.AvgHeartRate, &l.AvgPowerW, &l.AvgCadence,
			&l.TotalAscentM, &l.TotalDescentM, &l.StartLat, &l.StartLng,
		); err != nil {
			return nil, fmt.Errorf("scan detected lap: %w", err)
		}
		laps = append(laps, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get detected laps: %w", err)
	}
	if laps == nil {
		laps = []model.DetectedLap{}
	}
	return laps, nil
}

// DeleteDetectedLaps removes all detected laps for an activity.
func (db *DB) DeleteDetectedLaps(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM activity_detected_laps WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete detected laps: %w", err)
	}
	return nil
}

// IsLapDetectionEnabled checks if lap detection is enabled for a user.
func (db *DB) IsLapDetectionEnabled(ctx context.Context, userID string) bool {
	var enabled bool
	err := db.Pool.QueryRow(ctx,
		`SELECT enable_lap_detection FROM user_enrichment_settings WHERE user_id = $1`, userID,
	).Scan(&enabled)
	if err != nil {
		return true // default enabled
	}
	return enabled
}

func sportGroupFromTuple(sportType, subType string) string {
	if group, ok := sport.SegmentProjectionGroupForTuple(sportType, subType); ok {
		return group
	}
	return "generic"
}

type liftSegmentImportCandidate struct {
	osmID   int64
	name    *string
	geojson string
}

// ImportLiftSegments creates segment_type='lift' segments from osm_lifts.
// Idempotent: skips lifts that already have a matching segment and repairs
// missing geometry versions for existing imported lift segments.
func (db *DB) ImportLiftSegments(ctx context.Context, userID string) (int, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin lift import: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('skadia_import_lift_segments'))`); err != nil {
		return 0, fmt.Errorf("lock lift import: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT l.osm_id, l.name, ST_AsGeoJSON(l.geom) AS geojson
		FROM osm_lifts l
		ORDER BY l.osm_id
	`)
	if err != nil {
		return 0, fmt.Errorf("query osm_lifts: %w", err)
	}
	candidates := make([]liftSegmentImportCandidate, 0)
	for rows.Next() {
		var c liftSegmentImportCandidate
		if err := rows.Scan(&c.osmID, &c.name, &c.geojson); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	count := 0
	for _, c := range candidates {
		segID, created, err := ensureImportedLiftSegment(ctx, tx, userID, c)
		if err != nil {
			return count, err
		}
		if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segID); err != nil {
			return count, err
		}
		if created {
			count++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return count, fmt.Errorf("commit lift import: %w", err)
	}
	return count, nil
}

func ensureImportedLiftSegment(ctx context.Context, tx pgx.Tx, userID string, c liftSegmentImportCandidate) (string, bool, error) {
	segID, created, err := findOrCreateImportedLiftSegment(ctx, tx, userID, c)
	if err != nil {
		return "", false, err
	}

	if _, err := tx.Exec(ctx, `
		WITH g AS (SELECT ST_SetSRID(ST_GeomFromGeoJSON($2), 4326) AS geom)
		INSERT INTO segment_versions (segment_id, version_number, track,
		                              start_point, end_point, valid_from, created_by)
		SELECT $1, 1, g.geom, ST_StartPoint(g.geom), ST_EndPoint(g.geom), CURRENT_DATE, $3
		FROM g
		WHERE NOT EXISTS (
			SELECT 1 FROM segment_versions WHERE segment_id = $1
		)
	`, segID, c.geojson, userID); err != nil {
		return "", false, fmt.Errorf("ensure lift segment version osm_id=%d: %w", c.osmID, err)
	}
	return segID, created, nil
}

func findOrCreateImportedLiftSegment(ctx context.Context, tx pgx.Tx, userID string, c liftSegmentImportCandidate) (string, bool, error) {
	var segID string
	err := tx.QueryRow(ctx, `
		SELECT s.id
		FROM segments s
		WHERE s.osm_lift_id = $1
		  AND s.segment_type = 'lift'
		ORDER BY EXISTS (
			SELECT 1 FROM segment_versions sv WHERE sv.segment_id = s.id
		) DESC, s.created_at ASC, s.id ASC
		LIMIT 1
	`, c.osmID).Scan(&segID)
	if err == nil {
		return segID, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("lookup lift segment osm_id=%d: %w", c.osmID, err)
	}

	segName := "Lift"
	if c.name != nil && *c.name != "" {
		segName = *c.name
	}
	if err := tx.QueryRow(ctx, `
			INSERT INTO segments (created_by, name, sport_group, segment_type,
			                      terrain_type, climb_type, elevation_kind,
			                      segment_type_meta, osm_lift_id, visibility)
			VALUES ($1, $2, 'generic', 'lift', 'lift', 'bergfahrt', 'uphill', '{}', $3, 'public')
			RETURNING id
		`, userID, segName, c.osmID).Scan(&segID); err != nil {
		return "", false, fmt.Errorf("insert lift segment osm_id=%d: %w", c.osmID, err)
	}
	return segID, true, nil
}

// ListActivityIDsForSegmentBackfill returns IDs of all GPS activities belonging
// to userID whose track passes near the segment version active at the activity
// start time. This mirrors GetSpatialCandidateSegments in the opposite direction:
// given a segment, find activities that may traverse the version they would have
// matched historically.
func (db *DB) ListActivityIDsForSegmentBackfill(ctx context.Context, userID, segmentID string) ([]string, error) {
	query := fmt.Sprintf(`
			SELECT at.activity_id
			FROM activity_tracks at
			JOIN activities a ON a.id = at.activity_id
			%s
			WHERE a.user_id = $1
			  AND at.track_full IS NOT NULL
			  AND sv.track IS NOT NULL
			  AND ST_DWithin(at.track_full::geography, sv.track::geography, 100)
			ORDER BY a.started_at, at.activity_id`,
		activeCourseGPSV1VersionLateral("JOIN", "$2", "a.started_at", "track"),
	)
	rows, err := db.Pool.Query(ctx, query,
		userID, segmentID)
	if err != nil {
		return nil, fmt.Errorf("list activity ids for segment backfill: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan activity id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list activity ids for segment backfill: %w", err)
	}
	return ids, nil
}

// ListActivityIDsForSegmentRecompute returns candidate activities plus activities
// that already have efforts for this segment. Including existing efforts lets
// callers clear stale matches via per-activity transactional replacement instead
// of deleting the whole segment upfront.
func (db *DB) ListActivityIDsForSegmentRecompute(ctx context.Context, userID, segmentID string) ([]string, error) {
	query := fmt.Sprintf(`
		SELECT DISTINCT activity_id
		FROM (
			SELECT at.activity_id
			FROM activity_tracks at
			JOIN activities a ON a.id = at.activity_id
			%s
			WHERE a.user_id = $1
			  AND at.track_full IS NOT NULL
			  AND sv.track IS NOT NULL
			  AND ST_DWithin(at.track_full::geography, sv.track::geography, 100)

			UNION

			SELECT se.activity_id
			FROM segment_efforts se
			JOIN activities a ON a.id = se.activity_id
			WHERE se.segment_id = $2
			  AND se.superseded_at IS NULL
			  AND a.user_id = $1
		) ids`,
		activeCourseGPSV1VersionLateral("JOIN", "$2", "a.started_at", "track"),
	)
	rows, err := db.Pool.Query(ctx, query,
		userID, segmentID)
	if err != nil {
		return nil, fmt.Errorf("list activity ids for segment recompute: %w", err)
	}
	return scanActivityIDs(rows)
}

// ListAllActivityIDsForSegmentBackfill returns activity IDs from ALL users
// whose track passes near the segment version active at each activity's start.
// Used when publishing a previously private segment.
func (db *DB) ListAllActivityIDsForSegmentBackfill(ctx context.Context, segmentID string) ([]string, error) {
	query := fmt.Sprintf(`
			SELECT at.activity_id
			FROM activity_tracks at
			JOIN activities a ON a.id = at.activity_id
			%s
			WHERE at.track_full IS NOT NULL
			  AND sv.track IS NOT NULL
			  AND ST_DWithin(at.track_full::geography, sv.track::geography, 100)
			ORDER BY a.started_at, at.activity_id`,
		activeCourseGPSV1VersionLateral("JOIN", "$1", "a.started_at", "track"),
	)
	rows, err := db.Pool.Query(ctx, query,
		segmentID)
	if err != nil {
		return nil, fmt.Errorf("list all activity ids for segment backfill: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan activity id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListAllActivityIDsForSegmentRecompute is the all-user variant used when a
// segment is published. It includes already-matched activities so stale efforts
// are cleared without a segment-wide pre-delete.
func (db *DB) ListAllActivityIDsForSegmentRecompute(ctx context.Context, segmentID string) ([]string, error) {
	query := fmt.Sprintf(`
		SELECT DISTINCT activity_id
		FROM (
			SELECT at.activity_id
			FROM activity_tracks at
			JOIN activities a ON a.id = at.activity_id
			%s
			WHERE at.track_full IS NOT NULL
			  AND sv.track IS NOT NULL
			  AND ST_DWithin(at.track_full::geography, sv.track::geography, 100)

			UNION

			SELECT activity_id
			FROM segment_efforts
			WHERE segment_id = $1
			  AND superseded_at IS NULL
		) ids`,
		activeCourseGPSV1VersionLateral("JOIN", "$1", "a.started_at", "track"),
	)
	rows, err := db.Pool.Query(ctx, query,
		segmentID)
	if err != nil {
		return nil, fmt.Errorf("list all activity ids for segment recompute: %w", err)
	}
	return scanActivityIDs(rows)
}

func scanActivityIDs(rows pgx.Rows) ([]string, error) {
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan activity id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list activity ids: %w", err)
	}
	return ids, nil
}

// DeleteAllSegmentEffortsBySegment removes ALL efforts for a segment across all users.
// Used when publishing a previously private segment (full recompute).
func (db *DB) DeleteAllSegmentEffortsBySegment(ctx context.Context, segmentID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM segment_efforts WHERE segment_id = $1`, segmentID)
	if err != nil {
		return fmt.Errorf("delete all segment efforts by segment: %w", err)
	}
	return nil
}

// DeleteAllSegmentEffortsBySegmentForActivities removes efforts for a segment
// only inside the activity set scheduled for publish recompute.
func (db *DB) DeleteAllSegmentEffortsBySegmentForActivities(ctx context.Context, segmentID string, activityIDs []string) error {
	if len(activityIDs) == 0 {
		return nil
	}
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM segment_efforts
		 WHERE segment_id = $1
		   AND activity_id = ANY($2::uuid[])`,
		segmentID, activityIDs)
	if err != nil {
		return fmt.Errorf("delete all segment efforts by segment activities: %w", err)
	}
	return nil
}

// InsertSegmentEffortsBatch inserts multiple segment efforts in a single multi-row INSERT.
func (db *DB) InsertSegmentEffortsBatch(ctx context.Context, efforts []*model.SegmentEffort) error {
	return insertSegmentEffortsBatch(ctx, db.Pool, efforts)
}

// InsertSegmentEffortsBatchTx inserts segment efforts inside an existing transaction.
// The caller is responsible for committing or rolling back the transaction.
// This is the transactional counterpart of InsertSegmentEffortsBatch, used by
// recomputeSegmentEffortsForActivity to atomically DELETE + re-INSERT efforts.
func InsertSegmentEffortsBatchTx(ctx context.Context, tx pgx.Tx, efforts []*model.SegmentEffort) error {
	return insertSegmentEffortsBatch(ctx, tx, efforts)
}

// LockSegmentEffortReplacementTx serializes delete+insert replacement for one
// activity's segment efforts. segment_efforts has UUID primary keys, so without
// this lock concurrent matcher runs for the same activity can interleave and
// recreate logical duplicates.
func LockSegmentEffortReplacementTx(ctx context.Context, tx pgx.Tx, activityID string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('segment_efforts:' || $1, 0))`, activityID); err != nil {
		return fmt.Errorf("lock segment effort replacement: %w", err)
	}
	return nil
}

// ReplaceSegmentEffortsForActivitySegments atomically replaces the matched efforts
// for one activity across the provided segment IDs. It keeps repeated matcher runs
// idempotent even though segment_efforts uses UUID primary keys.
func (db *DB) ReplaceSegmentEffortsForActivitySegments(ctx context.Context, activityID, userID string, segmentIDs []string, efforts []*model.SegmentEffort) error {
	if _, err := db.SupersedeSegmentEffortsForActivitySegments(ctx, activityID, userID, segmentIDs, efforts, nil, nil, nil); err != nil {
		return err
	}
	return nil
}

// insertSegmentEffortsBatch is the shared implementation for InsertSegmentEffortsBatch
// and InsertSegmentEffortsBatchTx. It accepts any Querier (pool or transaction).
func insertSegmentEffortsBatch(ctx context.Context, conn Querier, efforts []*model.SegmentEffort) error {
	if len(efforts) == 0 {
		return nil
	}

	const cols = 44
	valueParts := make([]string, 0, len(efforts))
	args := make([]any, 0, len(efforts)*cols)

	for i, e := range efforts {
		base := i * cols
		valueParts = append(valueParts, fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7,
			base+8, base+9, base+10, base+11, base+12, base+13, base+14, base+15,
			base+16, base+17, base+18, base+19, base+20, base+21, base+22,
			base+23, base+24, base+25, base+26, base+27, base+28, base+29,
			base+30, base+31, base+32, base+33, base+34, base+35, base+36,
			base+37, base+38, base+39, base+40, base+41, base+42, base+43,
			base+44,
		))
		args = append(args,
			e.SegmentID, e.SegmentVersionID, e.ActivityID, e.UserID,
			e.StartedAt, e.ElapsedTimeS, e.IsReverse,
			e.SportType, e.SubType, e.GearID,
			e.AvgSpeedMs, e.AvgHeartRate, e.AvgPowerW,
			e.DistanceM, e.ElevationGainM,
			nullableString(e.MatchingContractKey), nullableInt(e.MatchingContractVersion),
			nullableString(e.MatchSourceType), nullableString(e.MatchSourceID), nullableString(e.MatchIdentityKey),
			e.OccurrenceIndex, e.OccurrenceHash, e.SupersededByEffortID,
			e.SupersededAt, e.SupersededReason, e.SupersededByRecalculationRunID,
			e.CreatedByRecalculationRunID, nullableString(e.LeaderboardEquivalenceGroupID),
			nullableString(e.LeaderboardSportType), nullableString(e.LeaderboardSubType),
			nullableString(e.LeaderboardEquipmentClass), nullableString(e.LeaderboardAssistClass),
			nullableString(e.LeaderboardIndoorPolicyState), nullableString(e.LeaderboardVirtualPolicyState),
			nullableString(e.LeaderboardElectricPolicyState), nullableString(e.LeaderboardDirection),
			nullableString(e.LeaderboardVersionPolicyKey),
			nullableString(e.LeaderboardContractKey), nullableInt(e.LeaderboardContractVersion),
			nullableString(e.ActivityTraitBucketKey), segmentEffortEligibilityPolicyKey(e),
			segmentEffortEligibilityPolicyVersion(e), nullableString(e.EligibilitySourceHash),
			segmentEffortInLeaderboard(e),
		)
	}

	query := `INSERT INTO segment_efforts
		(segment_id, segment_version_id, activity_id, user_id,
		 started_at, elapsed_time_s, is_reverse,
		 sport_type, sub_type, gear_id,
		 avg_speed_ms, avg_heart_rate, avg_power_w,
		 distance_m, elevation_gain_m,
		 matching_contract_key, matching_contract_version,
		 match_source_type, match_source_id, match_identity_key,
		 occurrence_index, occurrence_hash, superseded_by_effort_id,
		 superseded_at, superseded_reason, superseded_by_recalculation_run_id,
		 created_by_recalculation_run_id, leaderboard_equivalence_group_id,
		 leaderboard_sport_type, leaderboard_sub_type,
		 leaderboard_equipment_class, leaderboard_assist_class,
		 leaderboard_indoor_policy_state, leaderboard_virtual_policy_state,
		 leaderboard_electric_policy_state, leaderboard_direction,
		 leaderboard_version_policy_key,
		 leaderboard_contract_key, leaderboard_contract_version,
		 activity_trait_bucket_key, eligibility_policy_key,
		 eligibility_policy_version, eligibility_source_hash,
		 in_leaderboard)
		VALUES ` + strings.Join(valueParts, ", ") + `
		ON CONFLICT DO NOTHING`

	_, err := conn.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("batch insert segment efforts: %w", err)
	}
	return nil
}

func segmentEffortInLeaderboard(e *model.SegmentEffort) bool {
	if !e.InLeaderboard {
		return false
	}
	if e.LeaderboardContractKey == CheckpointSequenceV1Key {
		return true
	}
	if e.ActivityTraitBucketKey != "" &&
		e.EligibilityPolicyKey == courseEligibilityPolicyKey &&
		e.EligibilityPolicyVersion == courseEligibilityPolicyVersion &&
		e.EligibilitySourceHash != "" {
		return true
	}
	return e.EligibilityPolicyKey == "legacy_leaderboard_bypass" && e.EligibilitySourceHash != ""
}

func segmentEffortEligibilityPolicyKey(e *model.SegmentEffort) string {
	if e.EligibilityPolicyKey == "" {
		return courseEligibilityPolicyKey
	}
	return e.EligibilityPolicyKey
}

func segmentEffortEligibilityPolicyVersion(e *model.SegmentEffort) int {
	if e.EligibilityPolicyVersion == 0 {
		return courseEligibilityPolicyVersion
	}
	return e.EligibilityPolicyVersion
}

// IsSegmentFavoriteByUser checks if a segment is favorited by a user.
func (db *DB) IsSegmentFavoriteByUser(ctx context.Context, userID, segmentID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM segment_favorites WHERE user_id = $1 AND segment_id = $2)`,
		userID, segmentID).Scan(&exists)
	return exists, err
}

// AddSegmentFavorite adds a favorite record for a segment.
func (db *DB) AddSegmentFavorite(ctx context.Context, userID, segmentID string) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_favorites (user_id, segment_id) VALUES ($1, $2)
         ON CONFLICT DO NOTHING`,
		userID, segmentID)
	return err
}

// RemoveSegmentFavorite removes a favorite record for a segment.
func (db *DB) RemoveSegmentFavorite(ctx context.Context, userID, segmentID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM segment_favorites WHERE user_id = $1 AND segment_id = $2`,
		userID, segmentID)
	return err
}

// GetSegmentPerformances retrieves all segment efforts for a specific segment from a given user.
// isReverse: nil = all directions, false = forward only, true = reverse only.
func (db *DB) GetSegmentPerformances(ctx context.Context, segmentID, userID string, isReverse *bool) ([]model.SegmentPerformance, error) {
	activityTitleSQL := ActivityDisplayTitleSQL("a")
	query := `
	WITH ranked AS (
	SELECT
		se.id,
		se.activity_id,
		se.elapsed_time_s,
		a.moving_time_s,
		a.started_at,
		a.sport_type,
		a.sub_type,
		` + activityTitleSQL + ` AS title,
		se.is_reverse,
		aga.gear_id,
		g.name AS gear_name,
		g.type AS gear_type,
		a.timezone,
		ROW_NUMBER() OVER (
			PARTITION BY
				COALESCE(rg.id::text, a.id::text),
				se.started_at,
				se.elapsed_time_s,
				se.is_reverse
			ORDER BY
				CASE
					WHEN rg.primary_activity_id IS NOT NULL AND a.id = rg.primary_activity_id THEN 0
					WHEN COALESCE(a.is_group_secondary, false) = false THEN 1
					ELSE 2
				END,
				se.created_at ASC,
				se.id ASC
		) AS rn
	FROM segment_efforts se
	JOIN activities a ON se.activity_id = a.id
	LEFT JOIN recording_group_members rgm ON rgm.activity_id = a.id
	LEFT JOIN recording_groups rg ON rg.id = rgm.group_id
	LEFT JOIN activity_gear_assignments aga ON aga.activity_id = a.id AND aga.role_key = 'primary' AND aga.user_id = a.user_id
	LEFT JOIN gear g ON g.id = aga.gear_id AND g.user_id = a.user_id
	WHERE se.segment_id = $1
	  AND a.user_id = $2
	  AND se.superseded_at IS NULL`

	args := []any{segmentID, userID}
	if isReverse != nil {
		args = append(args, *isReverse)
		query += fmt.Sprintf(" AND se.is_reverse = $%d", len(args))
	}
	query += `
	)
	SELECT id, activity_id, elapsed_time_s, moving_time_s, started_at,
	       sport_type, sub_type, title, is_reverse, gear_id, gear_name,
	       gear_type, timezone
	FROM ranked
	WHERE rn = 1
	ORDER BY started_at DESC`

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get segment performances: %w", err)
	}
	defer rows.Close()

	var performances []model.SegmentPerformance
	for rows.Next() {
		var perf model.SegmentPerformance
		if err := rows.Scan(&perf.SegmentEffortID, &perf.ActivityID, &perf.ElapsedTimeS,
			&perf.MovingTimeS, &perf.StartedAt, &perf.SportType, &perf.SubType, &perf.Title, &perf.IsReverse,
			&perf.GearID, &perf.GearName, &perf.GearType, &perf.Timezone); err != nil {
			return nil, fmt.Errorf("scan performance: %w", err)
		}
		performances = append(performances, perf)
	}

	if performances == nil {
		performances = []model.SegmentPerformance{}
	}

	return performances, rows.Err()
}

// GetUserSegmentPRs returns all segment efforts where the user achieved personal_rank 1, 2, or 3,
// one row per (segment, personal_rank), ordered by segment name then rank.
func (db *DB) GetUserSegmentPRs(ctx context.Context, userID string) ([]model.SegmentEffort, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT se.id, se.segment_id, se.segment_version_id, se.activity_id,
		       se.user_id, se.started_at, se.elapsed_time_s,
		       se.is_reverse, se.sport_type, se.sub_type,
		       COALESCE(a.is_indoor, false), COALESCE(a.is_virtual, false), COALESCE(a.is_electric, false),
		       se.gear_id,
		       se.avg_power_w, se.avg_heart_rate, se.avg_speed_ms,
		       se.rank_overall, se.is_personal_best, se.personal_rank, se.is_segment_record,
		       se.in_leaderboard, se.created_at,
		       s.name AS segment_name,
		       NULL::text AS display_name,
		       NULL::text AS gear_name
		FROM segment_efforts se
		JOIN segments s ON s.id = se.segment_id
		JOIN activities a ON a.id = se.activity_id
		WHERE se.user_id = $1
		  AND se.personal_rank IS NOT NULL
		  AND se.personal_rank <= 3
		  AND se.superseded_at IS NULL
		ORDER BY s.name ASC, se.personal_rank ASC`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("get user segment prs: %w", err)
	}
	defer rows.Close()

	var efforts []model.SegmentEffort
	for rows.Next() {
		var e model.SegmentEffort
		err := rows.Scan(
			&e.ID, &e.SegmentID, &e.SegmentVersionID, &e.ActivityID,
			&e.UserID, &e.StartedAt, &e.ElapsedTimeS,
			&e.IsReverse, &e.SportType, &e.SubType,
			&e.IsIndoor, &e.IsVirtual, &e.IsElectric,
			&e.GearID,
			&e.AvgPowerW, &e.AvgHeartRate, &e.AvgSpeedMs,
			&e.RankOverall, &e.IsPersonalBest, &e.PersonalRank, &e.IsSegmentRecord,
			&e.InLeaderboard, &e.CreatedAt,
			&e.SegmentName, &e.DisplayName, &e.GearName,
		)
		if err != nil {
			return nil, fmt.Errorf("scan segment pr: %w", err)
		}
		efforts = append(efforts, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get user segment prs: %w", err)
	}
	if efforts == nil {
		efforts = []model.SegmentEffort{}
	}
	return efforts, nil
}

// GlobalTopSegmentEffort wraps a SegmentEffort with the segment's current athlete count,
// so the frontend can render "#rank von N" labels.
type GlobalTopSegmentEffort struct {
	model.SegmentEffort
	AthleteCount int `json:"athlete_count"`
}

// GetUserGlobalTopSegmentEfforts returns all segment efforts where the user is ranked
// rank_overall <= 10 and the effort is in the leaderboard.
// Ordered by rank_overall ASC, then segment name ASC.
func (db *DB) GetUserGlobalTopSegmentEfforts(ctx context.Context, userID string) ([]GlobalTopSegmentEffort, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT se.id, se.segment_id, se.segment_version_id, se.activity_id,
		       se.user_id, se.started_at, se.elapsed_time_s,
		       se.is_reverse, se.sport_type, se.sub_type,
		       COALESCE(a.is_indoor, false), COALESCE(a.is_virtual, false), COALESCE(a.is_electric, false),
		       se.gear_id,
		       se.avg_power_w, se.avg_heart_rate, se.avg_speed_ms,
		       se.rank_overall, se.is_personal_best, se.personal_rank, se.is_segment_record,
		       se.in_leaderboard, se.created_at,
		       s.name AS segment_name,
		       NULL::text AS display_name,
		       NULL::text AS gear_name,
		       s.athlete_count
		FROM segment_efforts se
		JOIN segments s ON s.id = se.segment_id
		JOIN activities a ON a.id = se.activity_id
		WHERE se.user_id = $1
		  AND se.rank_overall IS NOT NULL
		  AND se.rank_overall <= 10
		  AND se.in_leaderboard = true
		  AND se.superseded_at IS NULL
		ORDER BY se.rank_overall ASC, s.name ASC`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("get user global top segment efforts: %w", err)
	}
	defer rows.Close()

	var efforts []GlobalTopSegmentEffort
	for rows.Next() {
		var e GlobalTopSegmentEffort
		err := rows.Scan(
			&e.ID, &e.SegmentID, &e.SegmentVersionID, &e.ActivityID,
			&e.UserID, &e.StartedAt, &e.ElapsedTimeS,
			&e.IsReverse, &e.SportType, &e.SubType,
			&e.IsIndoor, &e.IsVirtual, &e.IsElectric,
			&e.GearID,
			&e.AvgPowerW, &e.AvgHeartRate, &e.AvgSpeedMs,
			&e.RankOverall, &e.IsPersonalBest, &e.PersonalRank, &e.IsSegmentRecord,
			&e.InLeaderboard, &e.CreatedAt,
			&e.SegmentName, &e.DisplayName, &e.GearName,
			&e.AthleteCount,
		)
		if err != nil {
			return nil, fmt.Errorf("scan global top segment effort: %w", err)
		}
		efforts = append(efforts, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get user global top segment efforts: %w", err)
	}
	if efforts == nil {
		efforts = []GlobalTopSegmentEffort{}
	}
	return efforts, nil
}

func encodeCursor(t time.Time, id string) string {
	raw := t.Format(time.RFC3339Nano) + "|" + id
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// GetSegmentsForExport returns minimal segment data needed for GPX/GeoJSON export.
// Only returns segments the user can access.
func (db *DB) GetSegmentsForExport(ctx context.Context, ids []string, userID string, visibleAccessScopeIDs []string) ([]model.Segment, error) {
	result := make([]model.Segment, 0, len(ids))
	for _, id := range ids {
		seg, err := db.GetSegmentWithOptions(ctx, id, userID, false, GetSegmentOptions{
			IncludeGeoJSON:        true,
			IncludeControls:       true,
			VisibleAccessScopeIDs: visibleAccessScopeIDs,
		})
		if err != nil {
			return nil, err
		}
		if seg == nil || seg.TrackGeoJSON == nil {
			continue
		}
		result = append(result, *seg)
	}
	return result, nil
}

func decodeCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("invalid cursor format")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return t, parts[1], nil
}
