package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// validEnrichmentCols is the allowlist of column names that may be passed to
// UpdateEnrichmentFields / UpdateEnrichmentFieldsSafe. Any column not in this
// set is skipped with a warning log so that a future caller passing user-
// controlled input cannot inject arbitrary SQL identifiers.
var validEnrichmentCols = map[string]bool{
	// Scalar metrics (from importer/enrich.go buildEnrichFields / buildReplaceFields)
	"avg_heart_rate":    true,
	"max_heart_rate":    true,
	"avg_power_w":       true,
	"max_power_w":       true,
	"avg_cadence":       true,
	"avg_temperature_c": true,
	"elevation_gain_m":  true,
	"elevation_loss_m":  true,
	"max_elevation_m":   true,
	"min_elevation_m":   true,
	"moving_time_s":     true,
	"tss":               true,
	"distance_m":        true,
	// Speed metrics (from editor.go recalcActivityMetricsFromStreams)
	"avg_speed_ms": true,
	"max_speed_ms": true,
	// Distance / source flags (from corrections.go)
	"distance_source": true,
	// Elevation source (from elevation.go, stage1.go)
	"elevation_source": true,
	// Stage-1 enrichment fields (from enrichment/stage1.go)
	"time_of_day":          true,
	"sunrise_at":           true,
	"sunset_at":            true,
	"trimp":                true,
	"variability_index":    true,
	"efficiency_factor":    true,
	"aerobic_decoupling":   true,
	"intensity":            true,
	"start_city":           true,
	"start_state":          true,
	"start_country":        true,
	"start_country_code":   true,
	"end_city":             true,
	"title_auto":           true,
	"tags":                 true,
	"sub_type":             true,
	"exclude_from_heatmap": true,
	"sport_type":           true,
	"parsed_sport_type":    true,
	"parsed_sub_type":      true,
	"is_indoor":            true,
	"is_electric":          true,
	"is_virtual":           true,
	"is_commute":           true,
	// Activity metadata (manual activity, recompute, importers)
	"title":           true,
	"title_is_manual": true,
	"description":     true,
	// Timezone from GPS start position (tzf library)
	"timezone": true,
}

// GetGeoTags returns all geo tags for an activity.
func (db *DB) GetGeoTags(ctx context.Context, activityID string) ([]model.GeoTag, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, activity_id, category, name, osm_id,
			lat, lng, ele, dist_m, duration_min, created_at,
			map_type, map_point_lat, map_point_lon,
			map_bbox_west, map_bbox_south, map_bbox_east, map_bbox_north,
			length_m, ele_m,
			ST_AsGeoJSON(map_section) AS map_section_geojson,
			source, admin_level
		FROM activity_geo_tags
		WHERE activity_id = $1
		ORDER BY COALESCE(dist_m, 0)`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get geo tags: %w", err)
	}
	defer rows.Close()

	var tags []model.GeoTag
	for rows.Next() {
		var t model.GeoTag
		if err := rows.Scan(
			&t.ID, &t.ActivityID, &t.Category, &t.Name, &t.OsmID,
			&t.Lat, &t.Lng, &t.Ele, &t.DistM, &t.DurationMin, &t.CreatedAt,
			&t.MapType, &t.MapPointLat, &t.MapPointLon,
			&t.MapBboxWest, &t.MapBboxSouth, &t.MapBboxEast, &t.MapBboxNorth,
			&t.LengthM, &t.EleM,
			&t.MapSection,
			&t.Source, &t.AdminLevel,
		); err != nil {
			return nil, fmt.Errorf("scan geo tag: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get geo tags: %w", err)
	}
	if tags == nil {
		tags = []model.GeoTag{}
	}
	return tags, nil
}

// UpdateEnrichmentFields updates enrichment-related fields on an activity.
// Always overwrites existing values — use UpdateEnrichmentFieldsSafe for lock-free fill-in semantics.
func (db *DB) UpdateEnrichmentFields(ctx context.Context, activityID string, fields map[string]any) error {
	return updateEnrichmentFieldsExec(ctx, db.Pool, activityID, fields)
}

// updateEnrichmentFieldsExec runs the UpdateEnrichmentFields SQL against any
// pgx querier (pool, tx, or the systemModifierTagSyncQuerier interface). This
// is the tx-aware path used by the WithSystemModifierTagSyncSuspended helper
// when a caller needs to write enrichment fields atomically alongside system
// modifier tag updates.
func updateEnrichmentFieldsExec(ctx context.Context, q systemModifierTagSyncQuerier, activityID string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}

	var sets []string
	var args []any
	args = append(args, activityID) // $1
	argIdx := 2

	for col, val := range fields {
		if !validEnrichmentCols[col] {
			slog.Warn("enrichment: unknown column skipped", "col", col)
			continue
		}
		sets = append(sets, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}
	if len(sets) == 0 {
		return nil
	}

	query := fmt.Sprintf("UPDATE activities SET %s WHERE id = $1", strings.Join(sets, ", "))
	if _, err := q.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("update enrichment fields: %w", err)
	}
	return nil
}

// UpdateEnrichmentFieldsSafe updates enrichment-related fields on an activity using
// COALESCE semantics: each column is only written if the existing DB value is NULL.
// This is safe for concurrent imports — parallel importers cannot overwrite each
// other's data and will never replace a real value with a zero/null.
func (db *DB) UpdateEnrichmentFieldsSafe(ctx context.Context, activityID string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}

	var sets []string
	var args []any
	args = append(args, activityID) // $1
	argIdx := 2

	for col, val := range fields {
		if !validEnrichmentCols[col] {
			slog.Warn("enrichment: unknown column skipped", "col", col)
			continue
		}
		// COALESCE(col, $N): keeps existing non-NULL value, writes $N only when NULL
		sets = append(sets, fmt.Sprintf("%s = COALESCE(%s, $%d)", col, col, argIdx))
		args = append(args, val)
		argIdx++
	}
	if len(sets) == 0 {
		return nil
	}

	query := fmt.Sprintf("UPDATE activities SET %s WHERE id = $1", strings.Join(sets, ", "))
	_, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update enrichment fields safe: %w", err)
	}
	return nil
}

// GetEnrichmentSettings returns enrichment settings for a user, creating defaults if missing.
func (db *DB) GetEnrichmentSettings(ctx context.Context, userID string) (*model.EnrichmentSettings, error) {
	var s model.EnrichmentSettings
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO user_enrichment_settings (user_id)
		VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET user_id = $1
		RETURNING user_id, enable_summit_detection, enable_weather_enrichment,
			enable_stop_detection, enable_ascent_detection,
			enable_auto_naming, enable_lap_detection,
			climb_min_gradient_pct, climb_min_elevation_m,
			round_trip_threshold_m, ocr_invoice_prompt, ocr_gear_prompt,
			enable_lift_detection, enable_settlement_detection, enable_border_detection,
			enable_water_detection, enable_forest_detection, enable_surface_detection,
			enable_interval_detection, enable_embeddings,
			photo_preview_size, terra_incognita_radius_m,
			route_duplicate_hausdorff_m, route_duplicate_length_diff_km,
			video_clip_frames, phash_threshold_bits,
			geocode_fallback_provider, geocode_fallback_url, geocode_fallback_timeout_s,
			summit_detection_radius_m, enable_difficulty_scales`,
		userID,
	).Scan(
		&s.UserID, &s.EnableSummitDetection, &s.EnableWeatherEnrichment,
		&s.EnableStopDetection, &s.EnableAscentDetection,
		&s.EnableAutoNaming, &s.EnableLapDetection, &s.ClimbMinGradientPct, &s.ClimbMinElevationM,
		&s.RoundTripThresholdM, &s.OCRInvoicePrompt, &s.OCRGearPrompt,
		&s.EnableLiftDetection, &s.EnableSettlementDetection, &s.EnableBorderDetection,
		&s.EnableWaterDetection, &s.EnableForestDetection, &s.EnableSurfaceDetection,
		&s.EnableIntervalDetection, &s.EnableEmbeddings,
		&s.PhotoPreviewSize, &s.TerraIncognitaRadiusM,
		&s.RouteDuplicateHausdorffM, &s.RouteDuplicateLengthDiffKm,
		&s.VideoClipFrames, &s.PhashThresholdBits,
		&s.GeocodeFallbackProvider, &s.GeocodeFallbackURL, &s.GeocodeFallbackTimeoutS,
		&s.SummitDetectionRadiusM, &s.EnableDifficultyScales,
	)
	if err != nil {
		return nil, fmt.Errorf("get enrichment settings: %w", err)
	}
	return &s, nil
}

// UpdateEnrichmentSettings updates enrichment settings for a user.
func (db *DB) UpdateEnrichmentSettings(ctx context.Context, userID string, s *model.EnrichmentSettings) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO user_enrichment_settings (
			user_id, enable_summit_detection, enable_weather_enrichment,
			enable_stop_detection, enable_ascent_detection,
			enable_auto_naming, enable_lap_detection,
			climb_min_gradient_pct, climb_min_elevation_m,
			round_trip_threshold_m, ocr_invoice_prompt, ocr_gear_prompt,
			enable_lift_detection, enable_settlement_detection, enable_border_detection,
			enable_water_detection, enable_forest_detection, enable_surface_detection,
			enable_interval_detection, enable_embeddings,
			photo_preview_size, terra_incognita_radius_m,
			route_duplicate_hausdorff_m, route_duplicate_length_diff_km,
			video_clip_frames, phash_threshold_bits,
			geocode_fallback_provider, geocode_fallback_url, geocode_fallback_timeout_s,
			summit_detection_radius_m, enable_difficulty_scales
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24, $25, $26,
			$27, $28, $29, $30, $31
		)
		ON CONFLICT (user_id) DO UPDATE SET
			enable_summit_detection = $2,
			enable_weather_enrichment = $3,
			enable_stop_detection = $4,
			enable_ascent_detection = $5,
			enable_auto_naming = $6,
			enable_lap_detection = $7,
			climb_min_gradient_pct = $8,
			climb_min_elevation_m = $9,
			round_trip_threshold_m = $10,
			ocr_invoice_prompt = $11,
			ocr_gear_prompt = $12,
			enable_lift_detection = $13,
			enable_settlement_detection = $14,
			enable_border_detection = $15,
			enable_water_detection = $16,
			enable_forest_detection = $17,
			enable_surface_detection = $18,
			enable_interval_detection = $19,
			enable_embeddings = $20,
			photo_preview_size = $21,
			terra_incognita_radius_m = $22,
			route_duplicate_hausdorff_m = $23,
			route_duplicate_length_diff_km = $24,
			video_clip_frames = $25,
			phash_threshold_bits = $26,
			geocode_fallback_provider = $27,
			geocode_fallback_url = $28,
			geocode_fallback_timeout_s = $29,
			summit_detection_radius_m = $30,
			enable_difficulty_scales = $31`,
		userID, s.EnableSummitDetection, s.EnableWeatherEnrichment,
		s.EnableStopDetection, s.EnableAscentDetection,
		s.EnableAutoNaming, s.EnableLapDetection, s.ClimbMinGradientPct, s.ClimbMinElevationM,
		s.RoundTripThresholdM, s.OCRInvoicePrompt, s.OCRGearPrompt,
		s.EnableLiftDetection, s.EnableSettlementDetection, s.EnableBorderDetection,
		s.EnableWaterDetection, s.EnableForestDetection, s.EnableSurfaceDetection,
		s.EnableIntervalDetection, s.EnableEmbeddings,
		s.PhotoPreviewSize, s.TerraIncognitaRadiusM,
		s.RouteDuplicateHausdorffM, s.RouteDuplicateLengthDiffKm,
		s.VideoClipFrames, s.PhashThresholdBits,
		s.GeocodeFallbackProvider, s.GeocodeFallbackURL, s.GeocodeFallbackTimeoutS,
		s.SummitDetectionRadiusM, s.EnableDifficultyScales,
	)
	if err != nil {
		return fmt.Errorf("update enrichment settings: %w", err)
	}
	return nil
}

// BackfillActivity holds the minimal data needed for backfill operations.
type BackfillActivity struct {
	ID        string
	Lat       float64
	Lng       float64
	EndLat    float64
	EndLng    float64
	StartedAt time.Time
	EndedAt   time.Time
	SportType string
	AvgHR     float64
	MaxHR     int
}

// ListActivitiesForBackfill returns activity data needed for enrichment backfill.
func (db *DB) ListActivitiesForBackfill(ctx context.Context, userID string) ([]BackfillActivity, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT a.id,
			COALESCE(ST_Y(a.start_location), 0) AS lat,
			COALESCE(ST_X(a.start_location), 0) AS lng,
			COALESCE(ST_Y(a.end_location), 0) AS end_lat,
			COALESCE(ST_X(a.end_location), 0) AS end_lng,
			a.started_at, COALESCE(a.ended_at, a.started_at), a.sport_type,
			COALESCE(a.avg_heart_rate, 0), COALESCE(a.max_heart_rate, 0)
		FROM activities a
		WHERE a.user_id = $1
		ORDER BY a.started_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list activities for backfill: %w", err)
	}
	defer rows.Close()

	var activities []BackfillActivity
	for rows.Next() {
		var a BackfillActivity
		if err := rows.Scan(
			&a.ID, &a.Lat, &a.Lng, &a.EndLat, &a.EndLng,
			&a.StartedAt, &a.EndedAt, &a.SportType, &a.AvgHR, &a.MaxHR,
		); err != nil {
			return nil, fmt.Errorf("scan backfill activity: %w", err)
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list activities for backfill: %w", err)
	}
	return activities, nil
}

// GetStreamPointNearTime returns the stream point with GPS data closest to the
// given time for the specified activity. Returns nil if no GPS points exist.
func (db *DB) GetStreamPointNearTime(ctx context.Context, activityID string, t time.Time) (*model.StreamPoint, error) {
	var sp model.StreamPoint
	err := db.Pool.QueryRow(ctx, `
		SELECT time, lat, lng FROM activity_streams
		WHERE activity_id = $1 AND lat IS NOT NULL
		ORDER BY ABS(EXTRACT(EPOCH FROM (time - $2::timestamptz)))
		LIMIT 1`,
		activityID, t,
	).Scan(&sp.Time, &sp.Lat, &sp.Lng)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get stream point near time: %w", err)
	}
	return &sp, nil
}

// GetActivityClimbs returns all detected climbs for an activity.
func (db *DB) GetActivityClimbs(ctx context.Context, activityID string) ([]model.Climb, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, climb_number, start_distance_m, end_distance_m, length_m,
				elevation_gain_m, avg_gradient_pct, max_gradient_pct,
				COALESCE(category, ''), sport_class
		 FROM activity_climbs
		 WHERE activity_id = $1
		 ORDER BY climb_number`,
		activityID)
	if err != nil {
		return nil, fmt.Errorf("get activity climbs: %w", err)
	}
	defer rows.Close()

	var climbs []model.Climb
	for rows.Next() {
		var c model.Climb
		if err := rows.Scan(&c.ID, &c.ClimbNumber, &c.StartDistanceM, &c.EndDistanceM,
			&c.LengthM, &c.ElevationGainM, &c.AvgGradientPct, &c.MaxGradientPct,
			&c.Category, &c.SportClass); err != nil {
			return nil, fmt.Errorf("scan activity climb: %w", err)
		}
		climbs = append(climbs, c)
	}
	if climbs == nil {
		climbs = []model.Climb{}
	}
	return climbs, rows.Err()
}

// GetActivityStops returns detected stops for an activity.
func (db *DB) GetActivityStops(ctx context.Context, activityID string) ([]model.ActivityStop, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, started_at, ended_at, duration_s,
		        ST_Y(location::geometry) AS lat, ST_X(location::geometry) AS lng,
		        name, poi_type, poi_name, poi_osm_id, poi_category, confidence
		 FROM activity_stops WHERE activity_id = $1 ORDER BY started_at`,
		activityID)
	if err != nil {
		return nil, fmt.Errorf("get activity stops: %w", err)
	}
	defer rows.Close()
	var stops []model.ActivityStop
	for rows.Next() {
		var s model.ActivityStop
		if err := rows.Scan(&s.ID, &s.StartedAt, &s.EndedAt, &s.DurationS,
			&s.Lat, &s.Lng, &s.Name, &s.PoiType, &s.PoiName, &s.PoiOsmID, &s.PoiCategory, &s.Confidence); err != nil {
			return nil, fmt.Errorf("scan activity stop: %w", err)
		}
		stops = append(stops, s)
	}
	return stops, rows.Err()
}

// GetEnrichmentModuleStatus returns per-module enrichment status for an activity.
// It checks enrichment_errors for failures and various data tables for presence.
func (db *DB) GetEnrichmentModuleStatus(ctx context.Context, activityID string) ([]model.EnrichmentModuleStatus, error) {
	// 1) Fetch errors from enrichment_errors
	errorRows, err := db.Pool.Query(ctx,
		`SELECT module, error_message FROM enrichment_errors WHERE activity_id = $1`,
		activityID)
	if err != nil {
		return nil, fmt.Errorf("get enrichment errors: %w", err)
	}
	defer errorRows.Close()

	errs := map[string]string{}
	for errorRows.Next() {
		var module, msg string
		if err := errorRows.Scan(&module, &msg); err != nil {
			return nil, fmt.Errorf("scan enrichment error: %w", err)
		}
		errs[module] = msg
	}

	// 2) Check data presence in one query
	var summitCount, settlementCount, waterCount, forestCount, borderCount int
	var stopCount, surfaceCount, climbCount, liftCount, intervalCount int
	var hasGastronomy bool
	err = db.Pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM activity_geo_tags WHERE activity_id = $1 AND category IN ('summit', 'pass')),
			(SELECT count(*) FROM activity_geo_tags WHERE activity_id = $1 AND category = 'settlement'),
			(SELECT count(*) FROM activity_geo_tags WHERE activity_id = $1 AND category IN ('still_water', 'flowing_water')),
			(SELECT count(*) FROM activity_geo_tags WHERE activity_id = $1 AND category = 'forest'),
			(SELECT count(*) FROM activity_geo_tags WHERE activity_id = $1 AND category = 'border'),
			(SELECT count(*) FROM activity_stops WHERE activity_id = $1),
			(SELECT count(*) FROM activity_enrichments WHERE activity_id = $1 AND category IN ('surface', 'highway')),
			(SELECT count(*) FROM activity_climbs WHERE activity_id = $1),
			(SELECT count(*) FROM activity_lift_candidates WHERE activity_id = $1),
			(SELECT count(*) FROM activity_intervals WHERE activity_id = $1),
			(SELECT count(*) FROM activity_geo_tags WHERE activity_id = $1 AND category = 'gastronomy') > 0
	`, activityID).Scan(
		&summitCount, &settlementCount, &waterCount, &forestCount, &borderCount,
		&stopCount, &surfaceCount, &climbCount, &liftCount, &intervalCount,
		&hasGastronomy,
	)
	if err != nil {
		return nil, fmt.Errorf("get enrichment data presence: %w", err)
	}

	modules := []struct {
		name    string
		hasData bool
		count   int
	}{
		{"summits", summitCount > 0, summitCount},
		{"lifts", liftCount > 0, liftCount},
		{"settlements", settlementCount > 0, settlementCount},
		{"borders", borderCount > 0, borderCount},
		{"water", waterCount > 0, waterCount},
		{"forests", forestCount > 0, forestCount},
		{"surfaces", surfaceCount > 0, surfaceCount},
		{"stops", stopCount > 0 || hasGastronomy, stopCount},
		{"intervals", intervalCount > 0, intervalCount},
		{"ascents", climbCount > 0, climbCount},
		{"embeddings", false, 0}, // skeleton — not yet implemented
	}

	result := make([]model.EnrichmentModuleStatus, 0, len(modules))
	for _, m := range modules {
		ms := model.EnrichmentModuleStatus{
			Name:  m.name,
			Count: m.count,
		}
		if errMsg, ok := errs[m.name]; ok {
			ms.Status = "error"
			ms.Error = errMsg
		} else if m.hasData {
			ms.Status = "completed"
		} else {
			ms.Status = "no_data"
		}
		result = append(result, ms)
	}

	return result, nil
}

// DeleteActivityGeoTags removes all geo tags for an activity (Stage 2 data).
func (db *DB) DeleteActivityGeoTags(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_geo_tags WHERE activity_id = $1`, activityID)
	return err
}

// DeleteActivityStops removes all stops for an activity (Stage 2 data).
func (db *DB) DeleteActivityStops(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_stops WHERE activity_id = $1`, activityID)
	return err
}

// DeleteActivityLiftCandidates removes all lift candidates for an activity (Stage 2 data).
func (db *DB) DeleteActivityLiftCandidates(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_lift_candidates WHERE activity_id = $1`, activityID)
	return err
}

// VisitedPOI represents a unique visited restaurant/hut.
type VisitedPOI struct {
	Name    string  `json:"name"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Count   int     `json:"count"`
	PoiType string  `json:"poi_type,omitempty"`
}

// GetVisitedPOIs returns all named stops the user has spent >= 900s at.
func (db *DB) GetVisitedPOIs(ctx context.Context, userID string) ([]VisitedPOI, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			s.name,
			AVG(ST_Y(s.location::geometry)) AS lat,
			AVG(ST_X(s.location::geometry)) AS lng,
			COUNT(*) AS count
		FROM activity_stops s
		JOIN activities a ON a.id = s.activity_id
		WHERE a.user_id = $1
		  AND s.name IS NOT NULL
		  AND s.duration_s >= 900
		GROUP BY s.name
		ORDER BY count DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get visited pois: %w", err)
	}
	defer rows.Close()
	var result []VisitedPOI
	for rows.Next() {
		var p VisitedPOI
		if err := rows.Scan(&p.Name, &p.Lat, &p.Lng, &p.Count); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// LiftCandidate represents an unconfirmed lift ride detected by ML.
type LiftCandidate struct {
	ID              string    `json:"id"`
	ActivityID      string    `json:"activity_id"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationS       int       `json:"duration_s"`
	AscentM         float64   `json:"ascent_m"`
	DescentM        *float64  `json:"descent_m,omitempty"`
	DistanceM       *float64  `json:"distance_m,omitempty"`
	Confidence      string    `json:"confidence"`
	Confirmed       bool      `json:"confirmed"`
	RejectedByUser  bool      `json:"rejected_by_user"`
	Source          string    `json:"source"`
	SegmentEffortID *string   `json:"segment_effort_id"`
}

type LiftCandidateFilter struct {
	Source          string
	IncludeRejected bool
}

func (db *DB) GetLiftCandidates(ctx context.Context, activityID, userID string, filter LiftCandidateFilter) ([]LiftCandidate, error) {
	query := `
		SELECT
			alc.id::text,
			alc.activity_id::text,
			alc.started_at,
			alc.ended_at,
			alc.duration_s,
			alc.ascent_m,
			alc.descent_m,
			alc.distance_m,
			alc.confidence,
			alc.confirmed,
			alc.rejected_by_user,
			alc.source,
			alc.segment_effort_id::text
		FROM activity_lift_candidates alc
		JOIN activities a ON a.id = alc.activity_id
		WHERE alc.activity_id = $1 AND a.user_id = $2
	`
	args := []any{activityID, userID}
	if filter.Source != "" {
		args = append(args, filter.Source)
		query += fmt.Sprintf(" AND alc.source = $%d", len(args))
	}
	if !filter.IncludeRejected {
		query += " AND NOT alc.rejected_by_user"
	}
	query += " ORDER BY alc.started_at"

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []LiftCandidate
	for rows.Next() {
		var c LiftCandidate
		if err := rows.Scan(&c.ID, &c.ActivityID, &c.StartedAt, &c.EndedAt,
			&c.DurationS, &c.AscentM, &c.DescentM, &c.DistanceM, &c.Confidence,
			&c.Confirmed, &c.RejectedByUser, &c.Source, &c.SegmentEffortID); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (db *DB) ConfirmLiftCandidate(ctx context.Context, userID, candidateID, activityID string) error {
	var returnedActivityID string
	err := db.Pool.QueryRow(ctx,
		`UPDATE activity_lift_candidates alc
		 SET confirmed = true
		 FROM activities a
		 WHERE alc.id = $1 AND alc.activity_id = $2
		   AND a.id = alc.activity_id AND a.user_id = $3
		 RETURNING alc.activity_id`,
		candidateID, activityID, userID,
	).Scan(&returnedActivityID)
	if err != nil {
		return err
	}
	return db.recomputeLiftStats(ctx, userID, returnedActivityID)
}

// RejectLiftCandidate soft-rejects a lift candidate and recomputes activity
// lift stats. The row is marked rejected_by_user=true rather than deleted so
// that it survives subsequent re-enrichment passes (Phase 4d: the idempotent
// DELETE in enrichStreamLifts now skips rows with rejected_by_user=true).
func (db *DB) RejectLiftCandidate(ctx context.Context, userID, candidateID, activityID string) error {
	var returnedActivityID string
	err := db.Pool.QueryRow(ctx,
		`UPDATE activity_lift_candidates alc
		 SET rejected_by_user = true
		 FROM activities a
		 WHERE alc.id = $1 AND alc.activity_id = $2
		   AND a.id = alc.activity_id AND a.user_id = $3
		 RETURNING alc.activity_id`,
		candidateID, activityID, userID,
	).Scan(&returnedActivityID)
	if err != nil {
		return err
	}
	return db.recomputeLiftStats(ctx, userID, returnedActivityID)
}

// recomputeLiftStats recomputes activities.lift_* columns. Phase 4c
// dedup contract: every lift counted exactly once across the two storage
// layers — segment_efforts (Stage 1/2/3 of the snap pipeline, auto-counted)
// + activity_lift_candidates (heuristic-only, user-confirmed).
//
// The dedup discriminator is `segment_effort_id IS NULL`: heuristic
// candidates that were also snapped to OSM in Stage 1/2 carry
// segment_effort_id pointing at the corresponding segment_efforts row, so
// the heuristic-only sub-query excludes them and they are counted only via
// the segment_efforts arm. Same goes for Stage 2 cable-hold merges (N audit
// candidates → 1 segment_effort) and Stage 3 method-A-only inserts
// (1 audit candidate → 1 segment_effort).
//
// Phase-4 critic F2 (Variante B): the segment_efforts arm filters out
// efforts whose audit-row has been soft-rejected by the user. The predicate
// is "NOT EXISTS a rejected audit-row referencing this effort" rather than
// a JOIN on segment_effort_id — Stage 2 cable-hold merges produce one
// canonical effort with N audit-rows, and a JOIN would N-fold the count
// of every kept merge. The NOT EXISTS form is also correct for
// segment_efforts that have no audit-row at all (pre-Phase-4c data): the
// sub-select returns zero rows, the predicate is true, the effort is
// counted normally.
type recomputeOpts struct {
	skipGearRefresh bool
}

type recomputeOpt func(*recomputeOpts)

func skipGearRefresh() recomputeOpt {
	return func(o *recomputeOpts) { o.skipGearRefresh = true }
}

func (db *DB) recomputeLiftStats(ctx context.Context, userID, activityID string, opts ...recomputeOpt) error {
	var cfg recomputeOpts
	for _, opt := range opts {
		opt(&cfg)
	}

	// Confirm-gate. lifts.AutoConfirmHeuristic is a package-level var (not a
	// const) so this branch stays reachable for static analysis even while the
	// default is false; flipping it to true would still require recompile +
	// redeploy + a /lifts/recalculate-all backfill across the corpus before
	// previously-unconfirmed candidates become visible in activities.lift_*.
	// See lifts/config.go for the future settings-key migration note.
	confirmedFilter := "AND confirmed = true"
	if lifts.AutoConfirmHeuristic {
		confirmedFilter = ""
	}

	query := fmt.Sprintf(`
		UPDATE activities SET
			lift_count = (
				SELECT COUNT(*) FROM segment_efforts se
				JOIN segments s ON s.id = se.segment_id
				WHERE se.activity_id = $1 AND s.segment_type = 'lift'
				  AND NOT EXISTS (
					SELECT 1 FROM activity_lift_candidates alc
					WHERE alc.segment_effort_id = se.id AND alc.rejected_by_user = true
				  )
			) + (
				SELECT COUNT(*) FROM activity_lift_candidates
				WHERE activity_id = $1 %s AND segment_effort_id IS NULL
				  AND NOT rejected_by_user
			),
			lift_duration_s = (
				SELECT COALESCE(SUM(se.elapsed_time_s), 0) FROM segment_efforts se
				JOIN segments s ON s.id = se.segment_id
				WHERE se.activity_id = $1 AND s.segment_type = 'lift'
				  AND NOT EXISTS (
					SELECT 1 FROM activity_lift_candidates alc
					WHERE alc.segment_effort_id = se.id AND alc.rejected_by_user = true
				  )
			) + (
				SELECT COALESCE(SUM(duration_s), 0) FROM activity_lift_candidates
				WHERE activity_id = $1 %s AND segment_effort_id IS NULL
				  AND NOT rejected_by_user
			),
			lift_ascent_m = (
				SELECT COALESCE(SUM(CASE WHEN NOT se.is_reverse THEN se.elevation_gain_m ELSE 0 END), 0)
				FROM segment_efforts se
				JOIN segments s ON s.id = se.segment_id
				WHERE se.activity_id = $1 AND s.segment_type = 'lift'
				  AND NOT EXISTS (
					SELECT 1 FROM activity_lift_candidates alc
					WHERE alc.segment_effort_id = se.id AND alc.rejected_by_user = true
				  )
			) + (
				SELECT COALESCE(SUM(ascent_m), 0) FROM activity_lift_candidates
				WHERE activity_id = $1 %s AND segment_effort_id IS NULL
				  AND NOT rejected_by_user
			),
			lift_descent_m = (
				SELECT COALESCE(SUM(CASE WHEN se.is_reverse THEN se.elevation_gain_m ELSE 0 END), 0)
				FROM segment_efforts se
				JOIN segments s ON s.id = se.segment_id
				WHERE se.activity_id = $1 AND s.segment_type = 'lift'
				  AND NOT EXISTS (
					SELECT 1 FROM activity_lift_candidates alc
					WHERE alc.segment_effort_id = se.id AND alc.rejected_by_user = true
				  )
			) + (
				SELECT COALESCE(SUM(descent_m), 0) FROM activity_lift_candidates
				WHERE activity_id = $1 %s AND segment_effort_id IS NULL
				  AND NOT rejected_by_user
			),
			lift_distance_m = (
				SELECT COALESCE(SUM(se.distance_m), 0) FROM segment_efforts se
				JOIN segments s ON s.id = se.segment_id
				WHERE se.activity_id = $1 AND s.segment_type = 'lift'
				  AND NOT EXISTS (
					SELECT 1 FROM activity_lift_candidates alc
					WHERE alc.segment_effort_id = se.id AND alc.rejected_by_user = true
				  )
			) + (
				SELECT COALESCE(SUM(distance_m), 0) FROM activity_lift_candidates
				WHERE activity_id = $1 %s AND segment_effort_id IS NULL
				  AND NOT rejected_by_user
			)
		WHERE id = $1 AND user_id = $2`, confirmedFilter, confirmedFilter, confirmedFilter, confirmedFilter, confirmedFilter)

	// F6 fix: UPDATE activities.lift_* and the dependent gear-stats refresh
	// must commit atomically. Pre-fix the UPDATE ran on db.Pool (auto-commit)
	// and refreshActivityGearStats opened a separate Tx — on gear-refresh
	// failure the lift stats stayed committed, gear totals went stale, and
	// the error was only slog.Warn'd while the caller received nil. Now both
	// statements share one Tx; on any failure both roll back and the caller
	// gets the error.
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("recompute lift stats: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // Rollback after Commit is a no-op

	tag, err := tx.Exec(ctx, query, activityID, userID)
	if err != nil {
		return fmt.Errorf("recompute lift stats: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// No matching activity (deleted or wrong user) — nothing to refresh,
		// nothing to commit. Rollback via defer.
		return nil
	}

	var gearIDs []string
	if !cfg.skipGearRefresh {
		gearIDs, err = db.refreshActivityGearStatsTx(ctx, tx, userID, activityID)
		if err != nil {
			return fmt.Errorf("recompute lift stats: refresh gear stats: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("recompute lift stats: commit: %w", err)
	}
	if len(gearIDs) > 0 {
		if err := db.EvaluateGearUsageRulesForGear(ctx, gearIDs); err != nil {
			slog.Warn("evaluate gear usage rules",
				"activity_id", activityID,
				"user_id", userID,
				"error", err)
		}
		slog.Debug("recomputeLiftStats: refreshed gear usage",
			"activity_id", activityID,
			"user_id", userID,
			"gear_count", len(gearIDs))
	}

	// Sanity probe runs after Commit: it is read-only and a probe-read failure
	// must not roll back a correct UPDATE. F9 fix: probe skips columns whose
	// raw counterpart is NULL to avoid spurious warnings on imports without
	// GPS / elevation data.
	db.warnIfLiftAggregatesExceedRaw(ctx, userID, activityID)
	return nil
}

// warnIfLiftAggregatesExceedRaw logs a warning when the recomputed lift_*
// aggregates exceed the corresponding raw activity totals — a signal that
// the dedup/recompute query produced an impossible result.
//
// F9 fix: distance_m, elevation_gain_m, elevation_loss_m on activities are
// nullable (see migration 000003 — DOUBLE PRECISION without NOT NULL).
// Activities imported without GPS / elevation data carry NULL for those
// columns. The pre-fix probe COALESCE'd them to 0 and then warned when
// any non-zero lift_* aggregate "exceeded 0", producing alert-fatigue on
// every recompute for those activities. The probe now reads raw fields as
// nullable *float64 and only compares pairs where the raw value is NOT
// NULL — a NULL raw value means "we don't know the activity total" and
// the probe abstains rather than firing a false positive.
//
// lift_count is omitted from the probe by design: there is no raw
// counterpart to compare against ("activity total ride count" is not a
// stored column).
func (db *DB) warnIfLiftAggregatesExceedRaw(ctx context.Context, userID, activityID string) {
	var distM, ascentM, lossM *float64
	var liftDistM, liftAscentM, liftDescentM float64
	err := db.Pool.QueryRow(ctx, `
		SELECT distance_m, COALESCE(lift_distance_m, 0),
		       elevation_gain_m, COALESCE(lift_ascent_m, 0),
		       elevation_loss_m, COALESCE(lift_descent_m, 0)
		FROM activities
		WHERE id = $1 AND user_id = $2`, activityID, userID).Scan(
		&distM, &liftDistM, &ascentM, &liftAscentM, &lossM, &liftDescentM,
	)
	if err != nil {
		slog.Warn("recomputeLiftStats: lift sanity read failed",
			"activity_id", activityID,
			"user_id", userID,
			"error", err)
		return
	}
	// Only flag pairs where the raw column is known. A NULL raw column
	// means the activity carries no recorded total for that axis (e.g.
	// no GPS → distance_m IS NULL); comparing a real lift aggregate to
	// an unknown raw is meaningless and would always trip the warning.
	distExceeds := distM != nil && liftDistM > *distM
	ascentExceeds := ascentM != nil && liftAscentM > *ascentM
	descentExceeds := lossM != nil && liftDescentM > *lossM
	if distExceeds || ascentExceeds || descentExceeds {
		slog.Warn("lift aggregates exceed activity raw - possible enrichment defect",
			"activity_id", activityID,
			"user_id", userID,
			"distance_m", distM,
			"lift_distance_m", liftDistM,
			"ascent_m", ascentM,
			"lift_ascent_m", liftAscentM,
			"loss_m", lossM,
			"lift_descent_m", liftDescentM)
	}
}

// refreshActivityGearStatsTx refreshes gear totals for the activity's gear
// inside the caller-supplied transaction. F6 fix: previously this function
// opened its own Tx and swallowed errors via slog.Warn — when the gear
// refresh failed the just-committed lift_* update was left orphaned with
// stale gear totals and the caller received nil. The Tx is now passed in
// so the lift_* UPDATE and the gear UPDATE commit (or roll back) together,
// and any failure is returned to the caller.
//
// "No gear assigned" is not an error: the activity simply has no
// activity_gear_assignments rows, nothing to refresh, return nil.
func (db *DB) refreshActivityGearStatsTx(ctx context.Context, tx pgx.Tx, userID, activityID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT aga.gear_id::text
		FROM activity_gear_assignments aga
		JOIN activities a ON a.id = aga.activity_id
		WHERE aga.activity_id = $1
		  AND aga.user_id = $2
		  AND a.user_id = $2`, activityID, userID)
	if err != nil {
		return nil, fmt.Errorf("read activity gear assignments: %w", err)
	}
	defer rows.Close()

	var gearIDs []string
	for rows.Next() {
		var gearID string
		if err := rows.Scan(&gearID); err != nil {
			return nil, fmt.Errorf("scan activity gear assignment: %w", err)
		}
		if err := db.refreshGearAndDependentUsageStatsOwnerVerifiedOn(ctx, tx, gearID); err != nil {
			return nil, fmt.Errorf("update gear usage stats (gear_id=%s): %w", gearID, err)
		}
		gearIDs = append(gearIDs, gearID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read activity gear assignments: %w", err)
	}
	return gearIDs, nil
}

// RecomputeLiftStats is the exported entry-point for the enrichment adapter.
func (db *DB) RecomputeLiftStats(ctx context.Context, userID, activityID string) error {
	return db.recomputeLiftStats(ctx, userID, activityID)
}

// HeuristicCandidateInput carries the fields needed for one heuristic lift
// candidate insert. Used by InsertHeuristicCandidateIfNotRejected.
//
// DescentM and DistanceM are stored as nullable DOUBLE PRECISION in the DB.
// F25 fix: passing zero stores 0 — NOT NULL — because the SQL binds the
// Go float64 value directly without a NULLIF wrapper. This matches the
// pre-Phase-4d insert path and is the intended behaviour: callers that
// genuinely have no descent/distance data should not pass this struct
// (or future callers can switch the fields to *float64 and add explicit
// nil-handling at the SQL site if NULL semantics are needed). A zero
// reading and an unknown reading are aggregated identically by the
// downstream recompute SUM, so the distinction is currently moot.
type HeuristicCandidateInput struct {
	StartedAt  time.Time
	EndedAt    time.Time
	DurationS  int
	AscentM    float64
	DescentM   float64
	DistanceM  float64
	Confidence string
}

// InsertHeuristicCandidateIfNotRejected inserts a heuristic candidate unless a
// rejected row already exists whose boundaries are within ±5 s of the new
// candidate's boundaries. The check uses per-boundary deltas rather than
// broad interval overlap — a candidate whose window merely overlaps a rejected
// one but has moved by more than 5 s on either edge is treated as a distinct
// detection and is not suppressed (Phase-4 critic F4).
//
// The check and insert are collapsed into a single INSERT ... WHERE NOT EXISTS
// statement so there is no window for a concurrent reject to land between
// them (Phase-4 critic F5).
//
// Returns true if a row was inserted, false if it was suppressed (matching
// rejected row) or already present (ON CONFLICT). Does not call
// recomputeLiftStats — callers are responsible for triggering recompute after
// all candidates for an activity have been processed.
//
// Asymmetric reject-filter (intentional, paired with IsOSMRejectedInWindow):
// the NOT EXISTS sub-query here checks `rejected_by_user = true` WITHOUT a
// source filter, so a rejected row of either source ('osm' or 'heuristic')
// blocks a fresh heuristic re-insert in the same window. IsOSMRejectedInWindow
// further scopes its check to source = 'osm', so a rejected heuristic does
// NOT block a later OSM snap of the same window — the OSM data import that
// fills in missing geometry is allowed to re-detect the lift correctly.
// Rationale: a user rejecting an OSM-snapped lift signals "the lift does
// not apply at all" (blocks both heuristic and OSM in window); a user
// rejecting a heuristic-only candidate signals "the heuristic was wrong"
// (blocks future heuristics, but a higher-quality OSM detection is allowed
// to override).
// Anchor: Phase-4d adversarial-review finding F14 on commit 9066dd2a9 in
// branch feature/lift-detection-go; user-decision recorded in that review's
// findings table.
func (db *DB) InsertHeuristicCandidateIfNotRejected(
	ctx context.Context,
	userID, activityID string,
	in HeuristicCandidateInput,
) (bool, error) {
	// Single atomic statement: insert only when no rejected row with boundaries
	// within ±5 s exists. The interval literals avoid pgx type-inference issues
	// with make_interval(secs => $N) that appeared in Phase 4d.
	tag, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, false, 'heuristic'
		WHERE EXISTS (SELECT 1 FROM activities a WHERE a.id = $1 AND a.user_id = $9)
		  AND NOT EXISTS (
			SELECT 1 FROM activity_lift_candidates rej
			WHERE rej.activity_id = $1
			  AND rej.rejected_by_user = true
			  AND ABS(EXTRACT(EPOCH FROM (rej.started_at - $2::timestamptz))) <= 5
			  AND ABS(EXTRACT(EPOCH FROM (rej.ended_at   - $3::timestamptz))) <= 5
		  )
		ON CONFLICT (activity_id, started_at) DO NOTHING
	`, activityID, in.StartedAt, in.EndedAt, in.DurationS,
		in.AscentM, in.DescentM, in.DistanceM, in.Confidence, userID)
	if err != nil {
		return false, fmt.Errorf("insert heuristic candidate: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// IsOSMRejectedInWindow reports whether a soft-rejected OSM audit-row
// covers the given [startedAt, endedAt] window with both per-boundary
// deltas ≤ 5 s. The OSM-snap pipeline calls this before inserting a
// fresh segment_effort + audit-row so a user's reject persists across
// re-enrichment passes (Phase-4 critic F2 Variante B).
//
// The boundary-delta tolerance mirrors InsertHeuristicCandidateIfNotRejected
// (Slot-2 F4): a re-detection that has shifted by more than 5 s on either
// edge is treated as a distinct lift and is not suppressed.
//
// Asymmetric reject-filter (intentional, paired with
// InsertHeuristicCandidateIfNotRejected): the check is scoped to
// source = 'osm', so a rejected heuristic candidate does NOT block a later
// OSM snap of the same window — the OSM data import that fills in missing
// lift geometry is allowed to override an earlier wrong heuristic. The
// reverse direction is asymmetric: see the corresponding block above
// InsertHeuristicCandidateIfNotRejected for the full rationale and the
// user-decision anchor (Phase-4d adversarial-review finding F14 on commit
// 9066dd2a9 in branch feature/lift-detection-go).
func (db *DB) IsOSMRejectedInWindow(ctx context.Context, activityID string, startedAt, endedAt time.Time) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity_lift_candidates
			WHERE activity_id = $1
			  AND source = 'osm'
			  AND rejected_by_user = true
			  AND ABS(EXTRACT(EPOCH FROM (started_at - $2::timestamptz))) <= 5
			  AND ABS(EXTRACT(EPOCH FROM (ended_at   - $3::timestamptz))) <= 5
		)`, activityID, startedAt, endedAt).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check rejected OSM window: %w", err)
	}
	return exists, nil
}
