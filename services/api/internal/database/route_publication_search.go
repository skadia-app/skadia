package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

type PublishedCourseFilters struct {
	SportType         string
	SubType           string
	SACMaxRank        *int
	MTBMaxRank        *int
	ViaFerrataMaxRank *int
	MinDistanceM      *float64
	MaxDistanceM      *float64
	MinElevationGainM *float64
	MaxElevationGainM *float64
	HasVariants       *bool
	VisibilityMode    string
}

func (db *DB) ListPublishedRouteCourses(ctx context.Context, userID string, limit, offset int, filter PublishedCourseFilters) ([]model.PublishedRouteCourse, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	args := []any{userID, limit, offset}
	where := []string{
		"p.subject_kind = 'published_course'",
		"rcp.publication_state = 'published'",
		"p.visibility = s.visibility",
		"p.access_scope_id IS NOT DISTINCT FROM s.access_scope_id",
	}
	switch strings.TrimSpace(filter.VisibilityMode) {
	case "", "visible_to_me":
		where = append(where, visibleSegmentAccessSQL("s", "$1", "false"))
	case "public", "public_only":
		where = append(where, "s.visibility = 'public'")
	case "owner", "owner_private":
		where = append(where, "s.created_by = $1")
	case "shared":
		where = append(where, "s.created_by = $1 AND s.visibility = 'shared'")
	case "shared_visible_to_me":
		where = append(where, sharedSegmentAccessSQL("s", "$1", "false"))
	default:
		return nil, fmt.Errorf("%w: invalid published-course visibility mode", ErrRoutePublicationValidation)
	}
	if filter.SportType != "" {
		args = append(args, filter.SportType)
		where = append(where, fmt.Sprintf("p.sport_type = $%d", len(args)))
	}
	if filter.SubType != "" {
		args = append(args, filter.SubType)
		where = append(where, fmt.Sprintf("p.sub_type = $%d", len(args)))
	}
	appendIntMax := func(column string, value *int) {
		if value == nil {
			return
		}
		args = append(args, *value)
		where = append(where, fmt.Sprintf("(%s IS NOT NULL AND %s <= $%d)", column, column, len(args)))
	}
	appendFloatMin := func(column string, value *float64) {
		if value == nil {
			return
		}
		args = append(args, *value)
		where = append(where, fmt.Sprintf("%s >= $%d", column, len(args)))
	}
	appendFloatMax := func(column string, value *float64) {
		if value == nil {
			return
		}
		args = append(args, *value)
		where = append(where, fmt.Sprintf("%s <= $%d", column, len(args)))
	}
	appendIntMax("p.sac_max_rank", filter.SACMaxRank)
	appendIntMax("p.mtb_max_rank", filter.MTBMaxRank)
	appendIntMax("p.via_ferrata_max_rank", filter.ViaFerrataMaxRank)
	appendFloatMin("p.distance_m", filter.MinDistanceM)
	appendFloatMax("p.distance_m", filter.MaxDistanceM)
	appendFloatMin("p.elevation_gain_m", filter.MinElevationGainM)
	appendFloatMax("p.elevation_gain_m", filter.MaxElevationGainM)
	if filter.HasVariants != nil {
		if *filter.HasVariants {
			where = append(where, "variants.has_variants = true")
		} else {
			where = append(where, "variants.has_variants = false")
		}
	}

	query := fmt.Sprintf(`
		SELECT rcp.id, rcp.route_id, rcp.route_snapshot_id,
		       rcp.course_segment_id, rcp.course_segment_version_id,
		       prs.route_name,
		       s.visibility, s.access_scope_id,
		       p.sport_type, p.sub_type,
		       p.is_electric, p.is_indoor, p.is_virtual, p.is_commute,
		       p.distance_m, p.elevation_gain_m, p.elevation_loss_m,
		       p.sac_max_rank, p.mtb_max_rank, p.via_ferrata_max_rank,
		       p.primary_surface, p.primary_way_type,
		       variants.has_variants,
		       rcp.created_at,
		       current_version.id,
		       COALESCE(current_version.id = rcp.course_segment_version_id, false)
		FROM route_course_profile_projection p
		JOIN route_course_publications rcp ON rcp.id = p.publication_id
		JOIN planned_route_snapshots prs ON prs.id = rcp.route_snapshot_id
		JOIN segments s ON s.id = rcp.course_segment_id
		LEFT JOIN LATERAL (
			SELECT sv.id
			FROM segment_versions sv
			WHERE sv.segment_id = s.id
			  AND sv.matching_contract_key = 'course_gps_v1'
			  AND sv.matching_contract_version = 1
			  AND sv.validity_basis <> 'rollback_retired'
			  AND (sv.valid_from_at IS NULL OR sv.valid_from_at <= NOW())
			  AND (sv.valid_until_at IS NULL OR NOW() < sv.valid_until_at)
			ORDER BY sv.version_number DESC
			LIMIT 1
		) current_version ON true
		LEFT JOIN LATERAL (
			SELECT EXISTS (
				SELECT 1
				FROM route_course_relationships rcr
				JOIN segments related_s ON related_s.id = CASE
					WHEN rcr.source_course_segment_id = rcp.course_segment_id THEN rcr.target_course_segment_id
					ELSE rcr.source_course_segment_id
				END
				WHERE (
					rcr.source_course_segment_id = rcp.course_segment_id
					OR rcr.target_course_segment_id = rcp.course_segment_id
				)
				  AND `+visibleSegmentAccessSQL("related_s", "$1", "false")+`
			) AS has_variants
		) variants ON true
		WHERE %s
		ORDER BY rcp.created_at DESC, rcp.id DESC
		LIMIT $2 OFFSET $3
	`, strings.Join(where, " AND "))
	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list published route courses: %w", err)
	}
	defer rows.Close()

	courses := []model.PublishedRouteCourse{}
	for rows.Next() {
		var course model.PublishedRouteCourse
		if err := rows.Scan(
			&course.PublicationID, &course.RouteID, &course.RouteSnapshotID,
			&course.CourseSegmentID, &course.CourseSegmentVersionID,
			&course.Name,
			&course.Visibility, &course.AccessScopeID,
			&course.SportType, &course.SubType,
			&course.IsElectric, &course.IsIndoor, &course.IsVirtual, &course.IsCommute,
			&course.DistanceM, &course.ElevationGainM, &course.ElevationLossM,
			&course.SACMaxRank, &course.MTBMaxRank, &course.ViaFerrataMaxRank,
			&course.PrimarySurface, &course.PrimaryWayType,
			&course.HasVariants,
			&course.PublishedAt,
			&course.CurrentSegmentVersionID,
			&course.CurrentSegmentVersionValid,
		); err != nil {
			return nil, fmt.Errorf("scan published route course: %w", err)
		}
		courses = append(courses, course)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list published route courses: %w", err)
	}
	return courses, nil
}
