package database

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

type DB struct {
	Pool          *pgxpool.Pool
	terraInflight singleflight.Group // deduplicates concurrent terra incognita computations per user
}

func New(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	slog.Info("database connected")
	return &DB{Pool: pool}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}

func (db *DB) CheckDuplicate(ctx context.Context, userID, fileHash string) (*model.Activity, error) {
	row := db.Pool.QueryRow(ctx,
		`SELECT id, sport_type, started_at FROM activities WHERE user_id = $1 AND file_hash = $2 LIMIT 1`,
		userID, fileHash,
	)
	var a model.Activity
	err := row.Scan(&a.ID, &a.SportType, &a.StartedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check duplicate: %w", err)
	}
	return &a, nil
}

func (db *DB) CheckTimeOverlap(ctx context.Context, userID string, startedAt, endedAt time.Time) (*model.Activity, error) {
	row := db.Pool.QueryRow(ctx,
		`SELECT id, sport_type, started_at FROM activities
		 WHERE user_id = $1
		   AND started_at < $3
		   AND ended_at > $2
		 LIMIT 1`,
		userID, startedAt, endedAt,
	)
	var a model.Activity
	err := row.Scan(&a.ID, &a.SportType, &a.StartedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check time overlap: %w", err)
	}
	return &a, nil
}

func (db *DB) InsertActivity(ctx context.Context, userID string, data *model.ActivityData) (*model.Activity, error) {
	var id string
	var createdAt, updatedAt time.Time
	var isEbike bool

	err := db.Pool.QueryRow(ctx, `
		INSERT INTO activities (
			user_id, file_hash, sport_type, sub_type,
			parsed_sport_type, parsed_sub_type,
			fit_sport_id, fit_sub_sport_id,
			started_at, ended_at, elapsed_time_s, moving_time_s,
			distance_m, elevation_gain_m, elevation_loss_m,
			max_elevation_m, min_elevation_m,
			avg_speed_ms, max_speed_ms,
			avg_heart_rate, max_heart_rate,
			avg_power_w, max_power_w, normalized_power_w,
			avg_cadence, avg_temperature_c,
			tss, trimp, is_indoor, is_ebike, has_gps,
			start_location, end_location,
			original_filename, original_file_format,
			source, source_device,
			visibility,
			sport_conflict_pending
		) VALUES (
			$1, $2, $3, $4,
			$3, $4,
			$5, $6,
			$7, $8, $9, $10,
			$11, $12, $13,
			$14, $15,
			$16, $17,
			$18, $19,
			$20, $21, $22,
			$23, $24,
			$25, $26, $27, $28, $29,
			CASE WHEN $30::float8 != 0 THEN ST_SetSRID(ST_MakePoint($31::float8, $30::float8), 4326) ELSE NULL END,
			CASE WHEN $32::float8 != 0 THEN ST_SetSRID(ST_MakePoint($33::float8, $32::float8), 4326) ELSE NULL END,
			$34, $35,
			$36, $37,
			$38,
			$39
		)
		RETURNING id, is_ebike, created_at, updated_at`,
		userID, nilIfEmpty(data.FileHash), data.SportType, nilIfEmpty(data.SubType),
		nilIfZeroI16(data.FitSportID), nilIfZeroI16(data.FitSubSportID),
		data.StartedAt, nilIfZeroTime(data.EndedAt), nilIfZeroInt(data.ElapsedTimeS), nilIfZeroInt(data.MovingTimeS),
		nilIfZeroF64(data.DistanceM), nilIfZeroF64(data.ElevationGainM), nilIfZeroF64(data.ElevationLossM),
		nilIfZeroF64(data.MaxElevationM), nilIfZeroF64(data.MinElevationM),
		nilIfZeroF64(data.AvgSpeedMS), nilIfZeroF64(data.MaxSpeedMS),
		nilIfZeroF64(data.AvgHeartRate), nilIfZeroInt(data.MaxHeartRate),
		nilIfZeroF64(data.AvgPowerW), nilIfZeroInt(data.MaxPowerW), nilIfZeroF64(data.NormalizedPowerW),
		nilIfZeroF64(data.AvgCadence), nilIfZeroF64(data.AvgTemperatureC),
		nilIfZeroF64(data.TSS), nilIfZeroF64(data.TRIMP), data.IsIndoor, data.IsEbike, data.HasGPS,
		data.StartLat, data.StartLng,
		data.EndLat, data.EndLng,
		nilIfEmpty(data.OriginalName), nilIfEmpty(data.FileFormat),
		nilIfEmpty(data.Source), nilIfEmpty(data.SourceDevice),
		visibilityOrDefault(data.Visibility),
		data.SportConflictPending,
	).Scan(&id, &isEbike, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert activity: %w", err)
	}

	return &model.Activity{
		ID:                   id,
		UserID:               userID,
		SportType:            data.SportType,
		SubType:              ptrString(data.SubType),
		Source:               ptrString(data.Source),
		SourceDevice:         ptrString(data.SourceDevice),
		StartedAt:            data.StartedAt,
		IsEbike:              isEbike,
		IsIndoor:             data.IsIndoor,
		HasGPS:               data.HasGPS,
		NeedsClassification:  data.SportType == "unknown",
		OriginalFilename:     ptrString(data.OriginalName),
		Visibility:           visibilityOrDefault(data.Visibility),
		EnrichmentStatus:     "pending",
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
		SportConflictPending: data.SportConflictPending,
	}, nil
}

func (db *DB) InsertStreams(ctx context.Context, activityID string, streams []model.StreamPoint) error {
	if len(streams) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, s := range streams {
		batch.Queue(`
			INSERT INTO activity_streams (
				time, activity_id,
				lat, lng, altitude_m, speed_ms, distance_m,
				heart_rate, power_w, cadence, temperature_c,
				ground_contact_time_ms, vertical_oscillation_mm, stance_time_percent, left_right_balance
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
			s.Time, activityID,
			s.Lat, s.Lng, s.AltitudeM, s.SpeedMS, s.DistanceM,
			s.HeartRate, s.PowerW, s.Cadence, s.TemperatureC,
			s.GroundContactTimeMs, s.VerticalOscillationMm, s.StanceTimePercent, s.LeftRightBalance,
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range streams {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert stream point: %w", err)
		}
	}
	return nil
}

// UpdateStreamSpeeds writes smoothed speed_ms values back to activity_streams.
// Uses a single UPDATE with unnest arrays for performance (~36k points in one query).
func (db *DB) UpdateStreamSpeeds(ctx context.Context, activityID string, streams []model.StreamPoint) error {
	times := make([]time.Time, 0, len(streams))
	speeds := make([]float64, 0, len(streams))
	for _, s := range streams {
		if s.SpeedMS == nil {
			continue
		}
		times = append(times, s.Time)
		speeds = append(speeds, *s.SpeedMS)
	}
	if len(times) == 0 {
		return nil
	}
	_, err := db.Pool.Exec(ctx, `
		UPDATE activity_streams AS a
		SET speed_ms = v.speed
		FROM unnest($1::timestamptz[], $2::float8[]) AS v(t, speed)
		WHERE a.activity_id = $3 AND a.time = v.t`,
		times, speeds, activityID,
	)
	if err != nil {
		return fmt.Errorf("update stream speeds: %w", err)
	}
	return nil
}

func (db *DB) InsertTrack(ctx context.Context, activityID string, trackPoints []model.TrackPoint) error {
	if len(trackPoints) < 2 {
		return nil
	}

	wkt := trackToWKT(trackPoints)

	_, err := db.Pool.Exec(ctx, `
		WITH g AS (SELECT ST_SetSRID(ST_GeomFromText($2), 4326) AS geom)
		INSERT INTO activity_tracks (activity_id, track_full, track_simplified, bbox, point_count)
		SELECT $1, g.geom, ST_Simplify(g.geom, 0.0001), ST_Envelope(g.geom), $3
		FROM g`,
		activityID, wkt, len(trackPoints),
	)
	if err != nil {
		return fmt.Errorf("insert track: %w", err)
	}
	return nil
}

func trackToWKT(points []model.TrackPoint) string {
	var sb strings.Builder
	sb.Grow(len(points) * 25)
	sb.WriteString("LINESTRING(")
	for i, p := range points {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(p.Lng, 'f', 6, 64)) // WKT: longitude first
		sb.WriteByte(' ')
		sb.WriteString(strconv.FormatFloat(p.Lat, 'f', 6, 64))
	}
	sb.WriteByte(')')
	return sb.String()
}

func (db *DB) InsertSensors(ctx context.Context, activityID string, sensors []model.SensorRecord) error {
	if len(sensors) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, s := range sensors {
		batch.Queue(`
			INSERT INTO activity_sensors (
				activity_id, device_index, serial_number, ant_device_number, source_type,
				antplus_device_type, all_device_types,
				manufacturer, manufacturer_id, garmin_product, garmin_product_id,
				product_name, software_version, hardware_version,
				battery_status, battery_voltage
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			activityID, s.DeviceIndex, s.SerialNumber, s.AntDeviceNumber, s.SourceType,
			nilIfEmpty(s.AntplusDeviceType), s.AllDeviceTypes,
			nilIfEmpty(s.Manufacturer), nilIfZeroI16(s.ManufacturerID),
			nilIfEmpty(s.GarminProduct), nilIfZeroI32(s.GarminProductID),
			nilIfEmpty(s.ProductName), nilIfZeroF64(s.SoftwareVersion),
			nilIfZeroI16(s.HardwareVersion),
			nilIfEmpty(s.BatteryStatus), nilIfZeroF64(s.BatteryVoltage),
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range sensors {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert sensor: %w", err)
		}
	}
	return nil
}

func (db *DB) UpdateActivityFileInfo(ctx context.Context, id, bucket, key, format, filename, metaKey string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE activities SET
			original_file_bucket = $2,
			original_file_key = $3,
			original_file_format = $4,
			original_filename = $5,
			sidecar_meta_key = $6,
			updated_at = NOW()
		WHERE id = $1`,
		id, bucket, key, format, filename, metaKey,
	)
	if err != nil {
		return fmt.Errorf("update file info: %w", err)
	}
	return nil
}

func (db *DB) GetActivity(ctx context.Context, id string) (*model.Activity, error) {
	var a model.Activity
	var hasTrack bool
	var sportConflictMeta []byte

	err := db.Pool.QueryRow(ctx, `
		SELECT
			a.id, a.user_id, a.file_hash, a.title, a.title_auto, a.description,
			a.sport_type, a.sub_type, a.fit_sport_id, a.fit_sub_sport_id, a.is_ebike,
			a.started_at, a.ended_at, a.elapsed_time_s, a.moving_time_s,
			a.distance_m, a.elevation_gain_m, a.elevation_loss_m,
			a.max_elevation_m, a.min_elevation_m,
			a.avg_speed_ms, a.max_speed_ms,
			a.avg_heart_rate, a.max_heart_rate,
			a.avg_power_w, a.max_power_w, a.normalized_power_w,
			a.avg_cadence, a.avg_temperature_c,
			a.min_temperature_c, a.max_temperature_c,
			a.tss, a.trimp, a.is_indoor,
			a.source, a.source_device,
			a.tags, a.visibility, a.enrichment_status, a.original_filename, a.original_file_format,
			a.start_city, a.end_city, a.start_state, a.start_country, a.start_country_code,
			a.is_round_trip, a.intensity, a.time_of_day, a.sunrise_at, a.sunset_at, a.gear_id,
			a.created_at, a.updated_at,
			COALESCE(t.point_count, 0) > 0,
			a.location_lat, a.location_lng,
			a.lift_count, a.lift_distance_m, a.lift_ascent_m, a.lift_descent_m, a.lift_duration_s,
			a.exclude_lift_subtraction,
			COALESCE(a.elevation_source, 'device'),
			COALESCE(a.distance_source, 'device'),
			a.parent_id,
			a.fusion_sources,
			a.garmin_activity_id, a.strava_activity_id, a.komoot_tour_id,
		a.sport_conflict_pending,
		iq.metadata AS sport_conflict_meta,
		a.recording_device_id,
		COALESCE(rd.custom_name, rd.product_name) AS recording_device_name
		FROM activities a
		LEFT JOIN activity_tracks t ON t.activity_id = a.id
		LEFT JOIN LATERAL (
			SELECT metadata FROM import_queue iq
			WHERE iq.queue_type = 'sport_conflict'
			  AND iq.status = 'pending'
			  AND (iq.metadata->>'activity_id') = a.id::text
			  AND iq.user_id = a.user_id
			LIMIT 1
		) iq ON true
		LEFT JOIN devices rd ON rd.id = a.recording_device_id
		WHERE a.id = $1`, id,
	).Scan(
		&a.ID, &a.UserID, &a.FileHash, &a.Title, &a.TitleAuto, &a.Description,
		&a.SportType, &a.SubType, &a.FitSportID, &a.FitSubSportID, &a.IsEbike,
		&a.StartedAt, &a.EndedAt, &a.ElapsedTimeS, &a.MovingTimeS,
		&a.DistanceM, &a.ElevationGainM, &a.ElevationLossM,
		&a.MaxElevationM, &a.MinElevationM,
		&a.AvgSpeedMS, &a.MaxSpeedMS,
		&a.AvgHeartRate, &a.MaxHeartRate,
		&a.AvgPowerW, &a.MaxPowerW, &a.NormalizedPowerW,
		&a.AvgCadence, &a.AvgTemperatureC,
		&a.MinTemperatureC, &a.MaxTemperatureC,
		&a.TSS, &a.TRIMP, &a.IsIndoor,
		&a.Source, &a.SourceDevice,
		&a.Tags, &a.Visibility, &a.EnrichmentStatus, &a.OriginalFilename, &a.OriginalFileFormat,
		&a.StartCity, &a.EndCity, &a.StartState, &a.StartCountry, &a.StartCountryCode,
		&a.IsRoundTrip, &a.Intensity, &a.TimeOfDay, &a.SunriseAt, &a.SunsetAt, &a.GearID,
		&a.CreatedAt, &a.UpdatedAt,
		&hasTrack,
		&a.LocationLat, &a.LocationLng,
		&a.LiftCount, &a.LiftDistanceM, &a.LiftAscentM, &a.LiftDescentM, &a.LiftDurationS,
		&a.ExcludeLiftSubtraction,
		&a.ElevationSource,
		&a.DistanceSource,
		&a.ParentID,
		&a.FusionSources,
		&a.GarminActivityID, &a.StravaActivityID, &a.KomootTourID,
		&a.SportConflictPending,
		&sportConflictMeta,
		&a.RecordingDeviceID,
		&a.RecordingDeviceName,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get activity: %w", err)
	}
	a.HasGPS = hasTrack
	a.NeedsClassification = a.SportType == "unknown"
	if a.Tags == nil {
		a.Tags = []string{}
	}
	if sportConflictMeta != nil && a.SportConflictPending {
		var meta model.SportConflictMeta
		if json.Unmarshal(sportConflictMeta, &meta) == nil && meta.FileSport != "" {
			a.SportConflictMeta = &meta
		}
	}
	return &a, nil
}

// GetActivityWithWeather returns an activity by ID, including its weather readings.
func (db *DB) GetActivityWithWeather(ctx context.Context, id string) (*model.Activity, error) {
	a, err := db.GetActivity(ctx, id)
	if err != nil || a == nil {
		return a, err
	}
	readings, err := db.GetWeatherReadings(ctx, id)
	if err != nil {
		return a, nil // best-effort: return activity without weather on error
	}
	a.WeatherReadings = readings
	return a, nil
}

// allowedSortColumns maps allowed sort_by parameter values to SQL column names.
var allowedSortColumns = map[string]string{
	"started_at":       "started_at",
	"distance_m":       "distance_m",
	"elevation_gain_m": "elevation_gain_m",
	"elapsed_time_s":   "elapsed_time_s",
}

// buildFilterSQL constructs dynamic WHERE conditions from ActivityFilters.
// startIdx is the first $N placeholder index to use for filter args.
// Returns the SQL fragment (e.g. "AND sport_type = ANY($2) AND ...") and args slice.
func buildFilterSQL(filters model.ActivityFilters, startIdx int) (string, []any) {
	var conds []string
	var args []any
	idx := startIdx

	if len(filters.SportTypes) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf("sport_type = ANY($%d)", idx))
		args = append(args, filters.SportTypes)
	}
	if len(filters.SubTypes) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf("sub_type = ANY($%d)", idx))
		args = append(args, filters.SubTypes)
	}
	if filters.DateFrom != "" {
		idx++
		conds = append(conds, fmt.Sprintf("started_at::date >= $%d::date", idx))
		args = append(args, filters.DateFrom)
	}
	if filters.DateTo != "" {
		idx++
		conds = append(conds, fmt.Sprintf("started_at::date <= $%d::date", idx))
		args = append(args, filters.DateTo)
	}
	if filters.IsIndoor != nil {
		idx++
		conds = append(conds, fmt.Sprintf("is_indoor = $%d", idx))
		args = append(args, *filters.IsIndoor)
	}
	if filters.IsEbike != nil {
		idx++
		conds = append(conds, fmt.Sprintf("is_ebike = $%d", idx))
		args = append(args, *filters.IsEbike)
	}
	if filters.HasGPS != nil {
		idx++
		conds = append(conds, fmt.Sprintf("has_gps = $%d", idx))
		args = append(args, *filters.HasGPS)
	}
	if filters.GearID != "" {
		idx++
		conds = append(conds, fmt.Sprintf("gear_id = $%d", idx))
		args = append(args, filters.GearID)
	}
	if filters.Search != "" {
		idx++
		conds = append(conds, fmt.Sprintf("(title ILIKE '%%' || $%d || '%%' OR title_auto ILIKE '%%' || $%d || '%%')", idx, idx))
		args = append(args, filters.Search)
	}
	if len(filters.IncludeTags) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf("id IN (SELECT activity_id FROM activity_tags WHERE tag_id = ANY($%d))", idx))
		args = append(args, filters.IncludeTags)
	}
	if len(filters.ExcludeTags) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf("id NOT IN (SELECT activity_id FROM activity_tags WHERE tag_id = ANY($%d))", idx))
		args = append(args, filters.ExcludeTags)
	}
	if len(filters.IncludeGear) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf("gear_id = ANY($%d)", idx))
		args = append(args, filters.IncludeGear)
	}
	if len(filters.ExcludeGear) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf("(gear_id IS NULL OR gear_id != ALL($%d))", idx))
		args = append(args, filters.ExcludeGear)
	}
	if len(filters.IncludeDevices) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf(
			"(recording_device_id = ANY($%d) OR id IN (SELECT activity_id FROM activity_sensors WHERE device_id = ANY($%d)))",
			idx, idx))
		args = append(args, filters.IncludeDevices)
	}
	if len(filters.ExcludeDevices) > 0 {
		idx++
		conds = append(conds, fmt.Sprintf(
			"(recording_device_id IS NULL OR recording_device_id != ALL($%d)) AND id NOT IN (SELECT activity_id FROM activity_sensors WHERE device_id = ANY($%d))",
			idx, idx))
		args = append(args, filters.ExcludeDevices)
	}

	if len(conds) == 0 {
		return "", nil
	}
	return "AND " + strings.Join(conds, " AND "), args
}

func (db *DB) ListActivities(ctx context.Context, userID string, cursor string, limit int, sortBy string, sortDir string, filters model.ActivityFilters) (*model.ActivityList, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Validate sort parameters
	sqlCol, ok := allowedSortColumns[sortBy]
	if !ok {
		sortBy = "started_at"
		sqlCol = "started_at"
	}
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "desc"
	}

	// Determine comparison operator for cursor-based pagination.
	// DESC order: we want rows "less than" the cursor value → use <
	// ASC order: we want rows "greater than" the cursor value → use >
	cmpOp := "<"
	if sortDir == "asc" {
		cmpOp = ">"
	}

	// Parse cursor: base64(sortValue|id)
	// For started_at: sortValue is RFC3339Nano timestamp
	// For numeric columns: sortValue is the string representation of the number
	var cursorSortVal string
	var cursorID string
	hasCursor := false

	if cursor != "" {
		decoded, err := base64.URLEncoding.DecodeString(cursor)
		if err == nil {
			parts := strings.SplitN(string(decoded), "|", 2)
			if len(parts) == 2 {
				cursorSortVal = parts[0]
				cursorID = parts[1]
				hasCursor = true
			}
		}
	}

	var rows pgx.Rows
	var err error

	// Count with filters: $1 = userID, filter args start at $1
	countFilterSQL, countFilterArgs := buildFilterSQL(filters, 1)
	countArgs := []any{userID}
	countArgs = append(countArgs, countFilterArgs...)

	var total int
	if err := db.Pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM activities WHERE user_id = $1 AND parent_id IS NULL %s`, countFilterSQL), countArgs...,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count activities: %w", err)
	}

	selectCols := `id, sport_type, sub_type, title, started_at, distance_m,
		elapsed_time_s, elevation_gain_m, elevation_loss_m, is_indoor, is_ebike, visibility,
		created_at, updated_at, sport_conflict_pending`

	// Build ORDER BY: primary sort column + tiebreaker id (same direction)
	orderClause := fmt.Sprintf("%s %s, id %s", sqlCol, strings.ToUpper(sortDir), strings.ToUpper(sortDir))

	// For nullable numeric columns, push NULLs to the end regardless of sort direction.
	// DESC → NULLS LAST (default for DESC, but be explicit)
	// ASC  → NULLS LAST (override default which is NULLS FIRST for ASC)
	if sqlCol != "started_at" {
		orderClause = fmt.Sprintf("%s %s NULLS LAST, id %s", sqlCol, strings.ToUpper(sortDir), strings.ToUpper(sortDir))
	}

	if hasCursor {
		if sortBy == "started_at" {
			// Parse cursor value as time
			cursorTime, parseErr := time.Parse(time.RFC3339Nano, cursorSortVal)
			if parseErr != nil {
				// Invalid cursor, fall through to no-cursor path
				hasCursor = false
			} else {
				// Base args: $1=userID, $2=cursorTime, $3=cursorID, then filters, then limit
				filterSQL, filterArgs := buildFilterSQL(filters, 3)
				allArgs := []any{userID, cursorTime, cursorID}
				allArgs = append(allArgs, filterArgs...)
				limitIdx := len(allArgs) + 1
				allArgs = append(allArgs, limit+1)

				query := fmt.Sprintf(`
					SELECT %s
					FROM activities
					WHERE user_id = $1
						AND parent_id IS NULL
						AND (%s, id) %s ($2, $3)
						%s
					ORDER BY %s
					LIMIT $%d`, selectCols, sqlCol, cmpOp, filterSQL, orderClause, limitIdx)
				rows, err = db.Pool.Query(ctx, query, allArgs...)
			}
		} else if cursorSortVal == "" {
			// NULL cursor: the sort column was NULL for the last item.
			// Base args: $1=userID, $2=cursorID, then filters, then limit
			filterSQL, filterArgs := buildFilterSQL(filters, 2)
			allArgs := []any{userID, cursorID}
			allArgs = append(allArgs, filterArgs...)
			limitIdx := len(allArgs) + 1
			allArgs = append(allArgs, limit+1)

			query := fmt.Sprintf(`
				SELECT %s
				FROM activities
				WHERE user_id = $1
					AND parent_id IS NULL
					AND %s IS NULL
					AND id %s $2
					%s
				ORDER BY %s
				LIMIT $%d`, selectCols, sqlCol, cmpOp, filterSQL, orderClause, limitIdx)
			rows, err = db.Pool.Query(ctx, query, allArgs...)
		} else {
			// Numeric column: parse as float64
			cursorNum, parseErr := strconv.ParseFloat(cursorSortVal, 64)
			if parseErr != nil {
				hasCursor = false
			} else {
				// Base args: $1=userID, $2=cursorNum, $3=cursorID, then filters, then limit
				filterSQL, filterArgs := buildFilterSQL(filters, 3)
				allArgs := []any{userID, cursorNum, cursorID}
				allArgs = append(allArgs, filterArgs...)
				limitIdx := len(allArgs) + 1
				allArgs = append(allArgs, limit+1)

				nullClause := fmt.Sprintf(`AND ((%s, id) %s ($2, $3) OR %s IS NULL)`, sqlCol, cmpOp, sqlCol)
				query := fmt.Sprintf(`
					SELECT %s
					FROM activities
					WHERE user_id = $1
						AND parent_id IS NULL
						%s
						%s
					ORDER BY %s
					LIMIT $%d`, selectCols, nullClause, filterSQL, orderClause, limitIdx)
				rows, err = db.Pool.Query(ctx, query, allArgs...)
			}
		}
	}

	if !hasCursor {
		// Base args: $1=userID, then filters, then limit
		filterSQL, filterArgs := buildFilterSQL(filters, 1)
		allArgs := []any{userID}
		allArgs = append(allArgs, filterArgs...)
		limitIdx := len(allArgs) + 1
		allArgs = append(allArgs, limit+1)

		query := fmt.Sprintf(`
			SELECT %s
			FROM activities
			WHERE user_id = $1
			AND parent_id IS NULL
			%s
			ORDER BY %s
			LIMIT $%d`, selectCols, filterSQL, orderClause, limitIdx)
		rows, err = db.Pool.Query(ctx, query, allArgs...)
	}

	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()

	var activities []model.Activity
	for rows.Next() {
		var a model.Activity
		if err := rows.Scan(
			&a.ID, &a.SportType, &a.SubType, &a.Title, &a.StartedAt, &a.DistanceM,
			&a.ElapsedTimeS, &a.ElevationGainM, &a.ElevationLossM, &a.IsIndoor, &a.IsEbike, &a.Visibility,
			&a.CreatedAt, &a.UpdatedAt, &a.SportConflictPending,
		); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		a.Tags = []string{}
		a.NeedsClassification = a.SportType == "unknown"
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}

	result := &model.ActivityList{
		Activities: activities,
		Total:      total,
	}

	if len(activities) > limit {
		result.Activities = activities[:limit]
		last := activities[limit-1]

		// Build cursor value based on sort column
		var sortVal string
		switch sortBy {
		case "started_at":
			sortVal = last.StartedAt.Format(time.RFC3339Nano)
		case "distance_m":
			if last.DistanceM != nil {
				sortVal = strconv.FormatFloat(*last.DistanceM, 'f', -1, 64)
			} else {
				sortVal = ""
			}
		case "elevation_gain_m":
			if last.ElevationGainM != nil {
				sortVal = strconv.FormatFloat(*last.ElevationGainM, 'f', -1, 64)
			} else {
				sortVal = ""
			}
		case "elapsed_time_s":
			if last.ElapsedTimeS != nil {
				sortVal = strconv.Itoa(*last.ElapsedTimeS)
			} else {
				sortVal = ""
			}
		}

		cursorStr := sortVal + "|" + last.ID
		result.NextCursor = base64.URLEncoding.EncodeToString([]byte(cursorStr))
	}

	if result.Activities == nil {
		result.Activities = []model.Activity{}
	}

	return result, nil
}

func (db *DB) UpdateActivity(ctx context.Context, id, userID string, update *model.ActivityUpdate) (*model.Activity, error) {
	var sets []string
	var args []any
	argIdx := 3
	args = append(args, id, userID)

	if update.Title != nil {
		sets = append(sets, fmt.Sprintf("title = $%d, title_is_manual = true", argIdx))
		args = append(args, *update.Title)
		argIdx++
	}
	if update.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *update.Description)
		argIdx++
	}
	if update.SportType != nil || update.SubType != nil {
		// Load parsed values to compare — sport_type_is_manual should only be
		// true when the manual value differs from the parser-detected value.
		var parsedSportType, parsedSubType *string
		_ = db.Pool.QueryRow(ctx,
			`SELECT parsed_sport_type, parsed_sub_type FROM activities WHERE id = $1`, id,
		).Scan(&parsedSportType, &parsedSubType)

		if update.SportType != nil {
			sets = append(sets, fmt.Sprintf("sport_type = $%d", argIdx))
			args = append(args, *update.SportType)
			argIdx++
		}
		if update.SubType != nil {
			sets = append(sets, fmt.Sprintf("sub_type = $%d", argIdx))
			args = append(args, *update.SubType)
			argIdx++
		}

		// Determine if the effective values differ from parsed values.
		newST := derefOr(update.SportType, derefOr(parsedSportType, ""))
		newSub := derefOr(update.SubType, derefOr(parsedSubType, ""))
		parsedST := derefOr(parsedSportType, "")
		parsedSub := derefOr(parsedSubType, "")
		isManual := newST != parsedST || newSub != parsedSub
		sets = append(sets, fmt.Sprintf("sport_type_is_manual = %t", isManual))
	}
	if update.Tags != nil {
		sets = append(sets, fmt.Sprintf("tags = $%d", argIdx))
		args = append(args, update.Tags)
		argIdx++
	}
	if update.Visibility != nil {
		sets = append(sets, fmt.Sprintf("visibility = $%d", argIdx))
		args = append(args, *update.Visibility)
		argIdx++
	}
	if update.HideMap != nil {
		sets = append(sets, fmt.Sprintf("hide_map = $%d", argIdx))
		args = append(args, *update.HideMap)
		argIdx++
	}
	if update.DataVisibility != nil {
		sets = append(sets, fmt.Sprintf("data_visibility = $%d", argIdx))
		args = append(args, update.DataVisibility)
		argIdx++
	}
	if update.GearID != nil {
		if string(update.GearID) == "null" {
			sets = append(sets, "gear_id = NULL")
		} else {
			var gearID string
			if err := json.Unmarshal(update.GearID, &gearID); err == nil && gearID != "" {
				sets = append(sets, fmt.Sprintf("gear_id = $%d", argIdx))
				args = append(args, gearID)
				argIdx++
			}
		}
	}
	if update.LocationLat != nil {
		sets = append(sets, fmt.Sprintf("location_lat = $%d", argIdx))
		args = append(args, *update.LocationLat)
		argIdx++
	}
	if update.LocationLng != nil {
		sets = append(sets, fmt.Sprintf("location_lng = $%d", argIdx))
		args = append(args, *update.LocationLng)
		argIdx++
	}
	if update.ExcludeLiftSubtraction != nil {
		sets = append(sets, fmt.Sprintf("exclude_lift_subtraction = $%d", argIdx))
		args = append(args, *update.ExcludeLiftSubtraction)
		argIdx++
	}

	if len(sets) == 0 {
		return db.GetActivity(ctx, id)
	}

	sets = append(sets, "updated_at = NOW()")

	query := fmt.Sprintf("UPDATE activities SET %s WHERE id = $1 AND user_id = $2", strings.Join(sets, ", "))
	_, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update activity: %w", err)
	}

	return db.GetActivity(ctx, id)
}

func (db *DB) DeleteActivity(ctx context.Context, userID, id string) error {
	// Clean up any pending sport conflict queue entries for this activity
	_ = db.ResolveImportQueueByActivity(ctx, userID, id)
	result, err := db.Pool.Exec(ctx, `DELETE FROM activities WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete activity: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("activity not found")
	}
	return nil
}

// DeleteActivities deletes multiple activities by ID that belong to the given user.
// Returns the number of rows deleted. Activities not owned by userID are silently ignored.
func (db *DB) DeleteActivities(ctx context.Context, userID string, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	// userID is $1; activity IDs start at $2
	args := make([]any, 0, len(ids)+1)
	args = append(args, userID)
	placeholders := make([]string, len(ids))
	for i, id := range ids {
		args = append(args, id)
		placeholders[i] = fmt.Sprintf("$%d", i+2)
	}
	// Clean up sport conflict queue entries for the deleted activities
	cleanArgs := make([]any, 0, len(ids)+1)
	cleanArgs = append(cleanArgs, userID)
	cleanPlaceholders := make([]string, len(ids))
	for i, id := range ids {
		cleanArgs = append(cleanArgs, id)
		cleanPlaceholders[i] = fmt.Sprintf("metadata->>'activity_id' = $%d", i+2)
	}
	cleanQuery := fmt.Sprintf(
		`UPDATE import_queue SET status = 'resolved', resolved_at = NOW()
		 WHERE user_id = $1 AND queue_type = 'sport_conflict' AND status = 'pending'
		 AND (%s)`,
		strings.Join(cleanPlaceholders, " OR "))
	if _, cleanErr := db.Pool.Exec(ctx, cleanQuery, cleanArgs...); cleanErr != nil {
		slog.Warn("delete activities: failed to clean sport conflict queue", "error", cleanErr)
	}

	query := fmt.Sprintf(`DELETE FROM activities WHERE user_id = $1 AND id IN (%s)`,
		strings.Join(placeholders, ", "))
	result, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete activities: %w", err)
	}
	return result.RowsAffected(), nil
}

func (db *DB) GetStreams(ctx context.Context, activityID string) ([]model.StreamResponse, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT time, lat, lng, altitude_m, speed_ms, distance_m,
			heart_rate, power_w, cadence, temperature_c,
			ground_contact_time_ms, vertical_oscillation_mm, stance_time_percent, left_right_balance
		FROM activity_streams
		WHERE activity_id = $1
		ORDER BY time ASC`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get streams: %w", err)
	}
	defer rows.Close()

	var streams []model.StreamResponse
	for rows.Next() {
		var s model.StreamResponse
		if err := rows.Scan(
			&s.Time, &s.Lat, &s.Lng, &s.AltitudeM, &s.SpeedMS, &s.DistanceM,
			&s.HeartRate, &s.PowerW, &s.Cadence, &s.TemperatureC,
			&s.GroundContactTimeMs, &s.VerticalOscillationMm, &s.StanceTimePercent, &s.LeftRightBalance,
		); err != nil {
			return nil, fmt.Errorf("scan stream: %w", err)
		}
		streams = append(streams, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get streams: %w", err)
	}
	if streams == nil {
		streams = []model.StreamResponse{}
	}
	return streams, nil
}

// GetStreamPoints loads stream data for an activity as StreamPoint slices,
// which is the type required by activityutil.CalcMovingTime.
func (db *DB) GetStreamPoints(ctx context.Context, activityID string) ([]model.StreamPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT time, lat, lng, altitude_m, speed_ms, distance_m,
			heart_rate, power_w, cadence, temperature_c
		FROM activity_streams
		WHERE activity_id = $1
		ORDER BY time ASC`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get stream points: %w", err)
	}
	defer rows.Close()

	var streams []model.StreamPoint
	for rows.Next() {
		var s model.StreamPoint
		if err := rows.Scan(
			&s.Time, &s.Lat, &s.Lng, &s.AltitudeM, &s.SpeedMS, &s.DistanceM,
			&s.HeartRate, &s.PowerW, &s.Cadence, &s.TemperatureC,
		); err != nil {
			return nil, fmt.Errorf("scan stream point: %w", err)
		}
		streams = append(streams, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get stream points: %w", err)
	}
	return streams, nil
}

// ListActivityIDsForUser returns all activity IDs belonging to the given user.
func (db *DB) ListActivityIDsForUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id FROM activities WHERE user_id = $1 ORDER BY started_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list activity ids: %w", err)
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
		return nil, fmt.Errorf("list activity ids: %w", err)
	}
	return ids, nil
}

// UpdateMovingTime sets moving_time_s for a single activity, scoped by user_id
// to prevent cross-user mutations.
func (db *DB) UpdateMovingTime(ctx context.Context, activityID, userID string, movingTimeS int) error {
	result, err := db.Pool.Exec(ctx,
		`UPDATE activities SET moving_time_s = $1, updated_at = NOW() WHERE id = $2 AND user_id = $3`,
		movingTimeS, activityID, userID,
	)
	if err != nil {
		return fmt.Errorf("update moving time: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("activity not found or access denied")
	}
	return nil
}

func (db *DB) GetSensors(ctx context.Context, activityID string) ([]model.SensorResponse, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, device_index, source_type, antplus_device_type, all_device_types,
			manufacturer, product_name, serial_number, ant_device_number,
			software_version, battery_status, device_id
		FROM activity_sensors
		WHERE activity_id = $1
		ORDER BY device_index`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get sensors: %w", err)
	}
	defer rows.Close()

	var sensors []model.SensorResponse
	for rows.Next() {
		var s model.SensorResponse
		var primaryType, manufacturer, productName, batteryStatus *string
		var allTypes []string
		var swVersion *float64

		if err := rows.Scan(
			&s.ID, &s.DeviceIndex, &s.SourceType, &primaryType, &allTypes,
			&manufacturer, &productName, &s.SerialNumber, &s.AntDeviceNumber,
			&swVersion, &batteryStatus, &s.DeviceID,
		); err != nil {
			return nil, fmt.Errorf("scan sensor: %w", err)
		}

		if primaryType != nil {
			s.PrimaryType = *primaryType
			s.PrimaryTypeLabel = sensorTypeLabel(s.PrimaryType)
		}
		if manufacturer != nil {
			s.Manufacturer = *manufacturer
		}
		if productName != nil {
			s.ProductName = *productName

		}
		if batteryStatus != nil {
			s.BatteryStatus = *batteryStatus
		}
		s.AllTypes = allTypes
		if s.AllTypes == nil {
			s.AllTypes = []string{}
		}
		if swVersion != nil {
			s.SoftwareVersion = fmt.Sprintf("%.1f", *swVersion)
		}

		sensors = append(sensors, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get sensors: %w", err)
	}
	if sensors == nil {
		sensors = []model.SensorResponse{}
	}
	return sensors, nil
}

var sensorLabels = map[string]string{
	"heart_rate":                 "Herzfrequenzsensor",
	"bike_speed":                 "Geschwindigkeitssensor",
	"bike_cadence":               "Trittfrequenzsensor",
	"bike_speed_cadence":         "Geschwindigkeits-/Trittfrequenzsensor",
	"bike_power":                 "Leistungsmesser",
	"stride_speed_distance":      "Laufpod / Running Dynamics",
	"radar":                      "Rückwärtsradar",
	"shift":                      "Schaltung (Di2 / eTap)",
	"environment_sensor_legacy":  "Temperatursensor",
	"multi_sport_speed_distance": "Multi-Sport Sensor",
}

func sensorTypeLabel(deviceType string) string {
	if label, ok := sensorLabels[deviceType]; ok {
		return label
	}
	return deviceType
}

// ReprocessInfo holds the data needed to reprocess an activity from its original file.
type ReprocessInfo struct {
	FileKey           string
	FileFormat        string
	SportTypeIsManual bool
	OriginalFilename  string
}

// GetActivityReprocessInfo loads the fields needed to re-parse an activity from S3.
func (db *DB) GetActivityReprocessInfo(ctx context.Context, activityID string) (*ReprocessInfo, error) {
	var info ReprocessInfo
	var key, format, filename *string
	err := db.Pool.QueryRow(ctx,
		`SELECT original_file_key, original_file_format, COALESCE(sport_type_is_manual, false), original_filename
		 FROM activities WHERE id = $1`,
		activityID,
	).Scan(&key, &format, &info.SportTypeIsManual, &filename)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get reprocess info: %w", err)
	}
	if key != nil {
		info.FileKey = *key
	}
	if format != nil {
		info.FileFormat = *format
	}
	if filename != nil {
		info.OriginalFilename = *filename
	}
	return &info, nil
}

// CountStreamChannels returns the number of distinct non-NULL sensor channels
// present in the activity_streams for the given activity (0–6).
func (db *DB) CountStreamChannels(ctx context.Context, activityID string) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `
		SELECT
			(CASE WHEN COUNT(*) FILTER (WHERE heart_rate IS NOT NULL) > 0 THEN 1 ELSE 0 END) +
			(CASE WHEN COUNT(*) FILTER (WHERE cadence IS NOT NULL) > 0 THEN 1 ELSE 0 END) +
			(CASE WHEN COUNT(*) FILTER (WHERE power_w IS NOT NULL) > 0 THEN 1 ELSE 0 END) +
			(CASE WHEN COUNT(*) FILTER (WHERE temperature_c IS NOT NULL) > 0 THEN 1 ELSE 0 END) +
			(CASE WHEN COUNT(*) FILTER (WHERE altitude_m IS NOT NULL) > 0 THEN 1 ELSE 0 END) +
			(CASE WHEN COUNT(*) FILTER (WHERE speed_ms IS NOT NULL) > 0 THEN 1 ELSE 0 END)
		FROM activity_streams WHERE activity_id = $1`, activityID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count stream channels: %w", err)
	}
	return count, nil
}

// DeleteStreams removes all stream points for an activity.
func (db *DB) DeleteStreams(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_streams WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete streams: %w", err)
	}
	return nil
}

// DeleteTrack removes the track geometry for an activity.
func (db *DB) DeleteTrack(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_tracks WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete track: %w", err)
	}
	return nil
}

// DeleteSensors removes all sensor records for an activity.
func (db *DB) DeleteSensors(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_sensors WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete sensors: %w", err)
	}
	return nil
}

// UpdateActivityMetrics updates all parser-derived fields on an activity.
// If skipSportType is true, sport_type and sub_type are NOT overwritten (manual override).
func (db *DB) UpdateActivityMetrics(ctx context.Context, activityID string, data *model.ActivityData, skipSportType bool) error {
	args := []any{activityID} // $1
	argIdx := 2

	idx := func() string {
		s := fmt.Sprintf("$%d", argIdx)
		argIdx++
		return s
	}

	var sets []string

	// Always update the parsed reference values so sport_type_is_manual
	// can be correctly determined on future PATCH requests.
	sets = append(sets, fmt.Sprintf("parsed_sport_type = %s", idx()))
	args = append(args, data.SportType)
	sets = append(sets, fmt.Sprintf("parsed_sub_type = %s", idx()))
	args = append(args, nilIfEmpty(data.SubType))

	if !skipSportType {
		sets = append(sets, fmt.Sprintf("sport_type = %s", idx()))
		args = append(args, data.SportType)
		sets = append(sets, fmt.Sprintf("sub_type = %s", idx()))
		args = append(args, nilIfEmpty(data.SubType))
		sets = append(sets, fmt.Sprintf("fit_sport_id = %s", idx()))
		args = append(args, nilIfZeroI16(data.FitSportID))
		sets = append(sets, fmt.Sprintf("fit_sub_sport_id = %s", idx()))
		args = append(args, nilIfZeroI16(data.FitSubSportID))
	}

	// Time fields
	sets = append(sets, fmt.Sprintf("started_at = %s", idx()))
	args = append(args, data.StartedAt)
	sets = append(sets, fmt.Sprintf("ended_at = %s", idx()))
	args = append(args, nilIfZeroTime(data.EndedAt))
	sets = append(sets, fmt.Sprintf("elapsed_time_s = %s", idx()))
	args = append(args, nilIfZeroInt(data.ElapsedTimeS))
	sets = append(sets, fmt.Sprintf("moving_time_s = %s", idx()))
	args = append(args, nilIfZeroInt(data.MovingTimeS))

	// Distance / elevation
	sets = append(sets, fmt.Sprintf("distance_m = %s", idx()))
	args = append(args, nilIfZeroF64(data.DistanceM))
	sets = append(sets, fmt.Sprintf("elevation_gain_m = %s", idx()))
	args = append(args, nilIfZeroF64(data.ElevationGainM))
	sets = append(sets, fmt.Sprintf("elevation_loss_m = %s", idx()))
	args = append(args, nilIfZeroF64(data.ElevationLossM))
	sets = append(sets, fmt.Sprintf("max_elevation_m = %s", idx()))
	args = append(args, nilIfZeroF64(data.MaxElevationM))
	sets = append(sets, fmt.Sprintf("min_elevation_m = %s", idx()))
	args = append(args, nilIfZeroF64(data.MinElevationM))

	// Speed
	sets = append(sets, fmt.Sprintf("avg_speed_ms = %s", idx()))
	args = append(args, nilIfZeroF64(data.AvgSpeedMS))
	sets = append(sets, fmt.Sprintf("max_speed_ms = %s", idx()))
	args = append(args, nilIfZeroF64(data.MaxSpeedMS))

	// Heart rate
	sets = append(sets, fmt.Sprintf("avg_heart_rate = %s", idx()))
	args = append(args, nilIfZeroF64(data.AvgHeartRate))
	sets = append(sets, fmt.Sprintf("max_heart_rate = %s", idx()))
	args = append(args, nilIfZeroInt(data.MaxHeartRate))

	// Power
	sets = append(sets, fmt.Sprintf("avg_power_w = %s", idx()))
	args = append(args, nilIfZeroF64(data.AvgPowerW))
	sets = append(sets, fmt.Sprintf("max_power_w = %s", idx()))
	args = append(args, nilIfZeroInt(data.MaxPowerW))
	sets = append(sets, fmt.Sprintf("normalized_power_w = %s", idx()))
	args = append(args, nilIfZeroF64(data.NormalizedPowerW))

	// Cadence / temperature
	sets = append(sets, fmt.Sprintf("avg_cadence = %s", idx()))
	args = append(args, nilIfZeroF64(data.AvgCadence))
	sets = append(sets, fmt.Sprintf("avg_temperature_c = %s", idx()))
	args = append(args, nilIfZeroF64(data.AvgTemperatureC))

	// Training load
	sets = append(sets, fmt.Sprintf("tss = %s", idx()))
	args = append(args, nilIfZeroF64(data.TSS))
	sets = append(sets, fmt.Sprintf("trimp = %s", idx()))
	args = append(args, nilIfZeroF64(data.TRIMP))
	sets = append(sets, fmt.Sprintf("is_indoor = %s", idx()))
	args = append(args, data.IsIndoor)
	sets = append(sets, fmt.Sprintf("is_ebike = %s", idx()))
	args = append(args, data.IsEbike)

	// Location (start)
	latIdx := idx()
	lngIdx := idx()
	sets = append(sets, fmt.Sprintf(
		"start_location = CASE WHEN %s::float8 != 0 THEN ST_SetSRID(ST_MakePoint(%s::float8, %s::float8), 4326) ELSE NULL END",
		latIdx, lngIdx, latIdx))
	args = append(args, data.StartLat, data.StartLng)

	// Location (end)
	latIdx = idx()
	lngIdx = idx()
	sets = append(sets, fmt.Sprintf(
		"end_location = CASE WHEN %s::float8 != 0 THEN ST_SetSRID(ST_MakePoint(%s::float8, %s::float8), 4326) ELSE NULL END",
		latIdx, lngIdx, latIdx))
	args = append(args, data.EndLat, data.EndLng)

	// Source
	sets = append(sets, fmt.Sprintf("source = %s", idx()))
	args = append(args, nilIfEmpty(data.Source))
	sets = append(sets, fmt.Sprintf("source_device = %s", idx()))
	args = append(args, nilIfEmpty(data.SourceDevice))

	// Always reset enrichment and timestamp
	sets = append(sets, "enrichment_status = 'pending'", "updated_at = NOW()")

	query := fmt.Sprintf("UPDATE activities SET %s WHERE id = $1", strings.Join(sets, ", "))
	_, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update activity metrics: %w", err)
	}
	return nil
}

// ReprocessFilter defines optional filters for batch reprocessing.
type ReprocessFilter struct {
	SportTypes []string   `json:"sport_types,omitempty"`
	Format     string     `json:"format,omitempty"`
	Before     *time.Time `json:"before,omitempty"`
	After      *time.Time `json:"after,omitempty"`
	IDs        []string   `json:"ids,omitempty"`
	Scope      []string   `json:"scope,omitempty"` // streams, sensors, metrics, sport_type
}

// ListActivityIDsFiltered returns activity IDs matching the given filter criteria.
func (db *DB) ListActivityIDsFiltered(ctx context.Context, userID string, filter *ReprocessFilter) ([]string, error) {
	if filter == nil {
		return db.ListActivityIDsForUser(ctx, userID)
	}

	// If explicit IDs are given, use only those (scoped to user).
	if len(filter.IDs) > 0 {
		rows, err := db.Pool.Query(ctx,
			`SELECT id FROM activities WHERE user_id = $1 AND id = ANY($2) ORDER BY started_at DESC`,
			userID, filter.IDs,
		)
		if err != nil {
			return nil, fmt.Errorf("list filtered ids: %w", err)
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, fmt.Errorf("scan id: %w", err)
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("list filtered ids: %w", err)
		}
		return ids, nil
	}

	// Build dynamic WHERE clause.
	where := []string{"user_id = $1"}
	args := []any{userID}
	argIdx := 2

	if len(filter.SportTypes) > 0 {
		where = append(where, fmt.Sprintf("sport_type = ANY($%d)", argIdx))
		args = append(args, filter.SportTypes)
		argIdx++
	}
	if filter.Format != "" {
		where = append(where, fmt.Sprintf("original_file_format = $%d", argIdx))
		args = append(args, filter.Format)
		argIdx++
	}
	if filter.Before != nil {
		where = append(where, fmt.Sprintf("started_at < $%d", argIdx))
		args = append(args, *filter.Before)
		argIdx++
	}
	if filter.After != nil {
		where = append(where, fmt.Sprintf("started_at >= $%d", argIdx))
		args = append(args, *filter.After)
		argIdx++
	}

	query := fmt.Sprintf(
		`SELECT id FROM activities WHERE %s ORDER BY started_at DESC`,
		strings.Join(where, " AND "),
	)

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list filtered ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list filtered ids: %w", err)
	}
	return ids, nil
}

// Helper functions

func derefOr(p *string, fallback string) string {
	if p != nil {
		return *p
	}
	return fallback
}

// visibilityOrDefault returns the given visibility if non-empty, otherwise "private".
func visibilityOrDefault(v string) string {
	if v != "" {
		return v
	}
	return "private"
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nilIfZeroF64(f float64) *float64 {
	if f == 0 {
		return nil
	}
	return &f
}

func nilIfZeroInt(i int) *int {
	if i == 0 {
		return nil
	}
	return &i
}

func nilIfZeroI16(i int16) *int16 {
	if i == 0 {
		return nil
	}
	return &i
}

func nilIfZeroI32(i int32) *int32 {
	if i == 0 {
		return nil
	}
	return &i
}

func nilIfZeroTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ── Shadow File System ──────────────────────────────────────────────────────

// GetUserFileNamingSchema returns the file naming schema for a user.
// Returns empty string if not set.
func (db *DB) GetUserFileNamingSchema(ctx context.Context, userID string) (string, error) {
	var schema *string
	err := db.Pool.QueryRow(ctx,
		`SELECT file_naming_schema FROM users WHERE id = $1`,
		userID,
	).Scan(&schema)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get file naming schema: %w", err)
	}
	if schema == nil {
		return "", nil
	}
	return *schema, nil
}

// UpdateUserFileNamingSchema updates the file naming schema for a user.
func (db *DB) UpdateUserFileNamingSchema(ctx context.Context, userID, schema string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET file_naming_schema = $2, updated_at = NOW() WHERE id = $1`,
		userID, schema,
	)
	if err != nil {
		return fmt.Errorf("update file naming schema: %w", err)
	}
	return nil
}

// GetUserHotfolderEnabled returns the user's hotfolder_enabled preference.
func (db *DB) GetUserHotfolderEnabled(ctx context.Context, userID string) (bool, error) {
	var enabled bool
	err := db.Pool.QueryRow(ctx,
		`SELECT hotfolder_enabled FROM users WHERE id = $1`,
		userID,
	).Scan(&enabled)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get hotfolder enabled: %w", err)
	}
	return enabled, nil
}

// GetUsersWithHotfolderEnabled returns IDs of all users that have hotfolder_enabled = true.
func (db *DB) GetUsersWithHotfolderEnabled(ctx context.Context) ([]string, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id FROM users WHERE hotfolder_enabled = true`)
	if err != nil {
		return nil, fmt.Errorf("get users with hotfolder enabled: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan user id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UpdateUserHotfolderEnabled persists the hotfolder_enabled toggle for a user.
func (db *DB) UpdateUserHotfolderEnabled(ctx context.Context, userID string, enabled bool) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET hotfolder_enabled = $2, updated_at = NOW() WHERE id = $1`,
		userID, enabled,
	)
	if err != nil {
		return fmt.Errorf("update hotfolder enabled: %w", err)
	}
	return nil
}

// UpdateActivityLocalFilePath sets the local_file_path for an activity.
func (db *DB) UpdateActivityLocalFilePath(ctx context.Context, activityID, path string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activities SET local_file_path = $2, updated_at = NOW() WHERE id = $1`,
		activityID, path,
	)
	if err != nil {
		return fmt.Errorf("update local file path: %w", err)
	}
	return nil
}

// UpdateActivityLocalFilePathByOldPath updates local_file_path from oldPath to newPath
// for activities belonging to the given user.
func (db *DB) UpdateActivityLocalFilePathByOldPath(ctx context.Context, userID, oldPath, newPath string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activities SET local_file_path = $3, updated_at = NOW()
		 WHERE user_id = $1 AND local_file_path = $2`,
		userID, oldPath, newPath,
	)
	if err != nil {
		return fmt.Errorf("update local file path by old path: %w", err)
	}
	return nil
}

// ListActivitiesWithLocalPath returns activities that have a local_file_path set.
func (db *DB) ListActivitiesWithLocalPath(ctx context.Context, userID string) ([]model.ActivityWithPath, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, started_at, sport_type, sub_type, title, start_city, distance_m, elapsed_time_s,
			original_file_format, local_file_path
		FROM activities
		WHERE user_id = $1 AND local_file_path IS NOT NULL
		ORDER BY started_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list activities with local path: %w", err)
	}
	defer rows.Close()

	var activities []model.ActivityWithPath
	for rows.Next() {
		var a model.ActivityWithPath
		if err := rows.Scan(
			&a.ID, &a.StartedAt, &a.SportType, &a.SubType, &a.Title, &a.City, &a.DistanceM,
			&a.ElapsedTimeS, &a.OriginalFileFormat, &a.LocalFilePath,
		); err != nil {
			return nil, fmt.Errorf("scan activity with path: %w", err)
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list activities with local path: %w", err)
	}
	return activities, nil
}

// ── Heatmap ─────────────────────────────────────────────────────────────────

// InsertHeatmapLine copies the track geometry into heatmap_lines.
// Uses COALESCE(track_public, track_full) so privacy zones are respected.
// Segments longer than 2000 m (GPS teleportation jumps) are treated as group
// boundaries: each contiguous group of points is stored as a separate row so
// that straight "teleportation lines" never appear in the heatmap.
func (db *DB) InsertHeatmapLine(ctx context.Context, userID, activityID, sportType string, subType *string, startedAt time.Time) error {
	_, err := db.Pool.Exec(ctx, `
		WITH pts AS (
		  SELECT (ST_DumpPoints(COALESCE(track_public, track_full))).path[1] AS idx,
		         (ST_DumpPoints(COALESCE(track_public, track_full))).geom  AS pt
		  FROM activity_tracks
		  WHERE activity_id = $2
		    AND track_full IS NOT NULL
		),
		with_dist AS (
		  SELECT idx, pt,
		    COALESCE(
		      ST_Distance(pt::geography, LAG(pt) OVER (ORDER BY idx)::geography),
		      0
		    ) AS prev_dist
		  FROM pts
		),
		grouped AS (
		  SELECT idx, pt,
		    SUM(CASE WHEN prev_dist > 2000 THEN 1 ELSE 0 END)
		      OVER (ORDER BY idx) AS grp
		  FROM with_dist
		),
		line_groups AS (
		  SELECT grp, ST_MakeLine(pt ORDER BY idx) AS geom
		  FROM grouped
		  GROUP BY grp
		  HAVING COUNT(*) >= 2
		)
		INSERT INTO heatmap_lines (user_id, activity_id, sport_type, sub_type, geom, geom_3857, activity_date)
		SELECT $1, $2, $3, $5, geom, ST_Transform(geom, 3857), $4::date
		FROM line_groups
		WHERE geom IS NOT NULL`,
		userID, activityID, sportType, startedAt, subType,
	)
	if err != nil {
		return fmt.Errorf("insert heatmap line: %w", err)
	}
	return nil
}

// DeleteHeatmapLines removes heatmap entries for an activity.
func (db *DB) DeleteHeatmapLines(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM heatmap_lines WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete heatmap lines: %w", err)
	}
	return nil
}

// BackfillHeatmap populates heatmap_lines for all existing activities of a user.
// Returns the number of rows inserted.
// Segments longer than 2000 m are treated as GPS teleportation jumps and used
// as group boundaries; each contiguous group becomes a separate heatmap row.
func (db *DB) BackfillHeatmap(ctx context.Context, userID string) (int, error) {
	result, err := db.Pool.Exec(ctx, `
		WITH activities_to_fill AS (
		  SELECT a.id AS activity_id, a.user_id, a.sport_type, a.sub_type, a.started_at
		  FROM activities a
		  JOIN activity_tracks t ON t.activity_id = a.id
		  WHERE a.user_id = $1
		    AND t.track_full IS NOT NULL
		    AND NOT EXISTS (
		        SELECT 1 FROM heatmap_lines h WHERE h.activity_id = a.id
		    )
		),
		pts AS (
		  SELECT af.activity_id, af.user_id, af.sport_type, af.sub_type, af.started_at,
		         (ST_DumpPoints(COALESCE(t.track_public, t.track_full))).path[1] AS idx,
		         (ST_DumpPoints(COALESCE(t.track_public, t.track_full))).geom  AS pt
		  FROM activities_to_fill af
		  JOIN activity_tracks t ON t.activity_id = af.activity_id
		),
		with_dist AS (
		  SELECT activity_id, user_id, sport_type, sub_type, started_at, idx, pt,
		    COALESCE(
		      ST_Distance(pt::geography,
		        LAG(pt) OVER (PARTITION BY activity_id ORDER BY idx)::geography),
		      0
		    ) AS prev_dist
		  FROM pts
		),
		grouped AS (
		  SELECT activity_id, user_id, sport_type, sub_type, started_at, idx, pt,
		    SUM(CASE WHEN prev_dist > 2000 THEN 1 ELSE 0 END)
		      OVER (PARTITION BY activity_id ORDER BY idx) AS grp
		  FROM with_dist
		),
		line_groups AS (
		  SELECT activity_id, user_id, sport_type, sub_type, started_at, grp,
		         ST_MakeLine(pt ORDER BY idx) AS geom
		  FROM grouped
		  GROUP BY activity_id, user_id, sport_type, sub_type, started_at, grp
		  HAVING COUNT(*) >= 2
		)
		INSERT INTO heatmap_lines (user_id, activity_id, sport_type, sub_type, geom, geom_3857, activity_date)
		SELECT user_id, activity_id, sport_type, sub_type, geom, ST_Transform(geom, 3857), started_at::date
		FROM line_groups
		WHERE geom IS NOT NULL`,
		userID,
	)
	if err != nil {
		return 0, fmt.Errorf("backfill heatmap: %w", err)
	}
	return int(result.RowsAffected()), nil
}

// ── Heatmap Points ──────────────────────────────────────────────────────────

// InsertHeatmapPoints generates evenly-spaced points from an activity's track
// using ST_Segmentize + ST_DumpPoints and inserts them into heatmap_points.
// Points that are more than 2000 m from their predecessor (GPS teleportation
// jumps) are excluded so that no phantom points appear in gap areas.
// Each contiguous group of valid points is segmentized independently so that
// the sampling interval stays consistent within each continuous section.
func (db *DB) InsertHeatmapPoints(ctx context.Context, userID, activityID, sportType string, subType *string, startedAt time.Time, intervalM int) error {
	_, err := db.Pool.Exec(ctx, `
		WITH pts AS (
		  SELECT (ST_DumpPoints(COALESCE(track_public, track_full))).path[1] AS idx,
		         (ST_DumpPoints(COALESCE(track_public, track_full))).geom  AS pt
		  FROM activity_tracks
		  WHERE activity_id = $2
		    AND track_full IS NOT NULL
		),
		with_dist AS (
		  SELECT idx, pt,
		    COALESCE(
		      ST_Distance(pt::geography, LAG(pt) OVER (ORDER BY idx)::geography),
		      0
		    ) AS prev_dist
		  FROM pts
		),
		grouped AS (
		  SELECT idx, pt,
		    SUM(CASE WHEN prev_dist > 2000 THEN 1 ELSE 0 END)
		      OVER (ORDER BY idx) AS grp
		  FROM with_dist
		),
		line_groups AS (
		  SELECT grp, ST_MakeLine(pt ORDER BY idx) AS geom
		  FROM grouped
		  GROUP BY grp
		  HAVING COUNT(*) >= 2
		)
		INSERT INTO heatmap_points (user_id, activity_id, sport_type, sub_type, activity_date, geom)
		SELECT $1, $2, $3, $6, $4::date,
		       (ST_DumpPoints(ST_Segmentize(geom::geography, $5)::geometry)).geom
		FROM line_groups
		WHERE geom IS NOT NULL`,
		userID, activityID, sportType, startedAt, intervalM, subType,
	)
	if err != nil {
		return fmt.Errorf("insert heatmap points: %w", err)
	}
	return nil
}

// DeleteHeatmapPoints removes all heatmap points for an activity.
func (db *DB) DeleteHeatmapPoints(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM heatmap_points WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete heatmap points: %w", err)
	}
	return nil
}

// BackfillHeatmapPoints generates heatmap points for all activities of a user.
// Segments longer than 2000 m (GPS teleportation jumps) are used as group
// boundaries; each contiguous group is segmentized independently.
func (db *DB) BackfillHeatmapPoints(ctx context.Context, userID string, intervalM int) (int, error) {
	result, err := db.Pool.Exec(ctx, `
		WITH activities_to_fill AS (
		  SELECT a.id AS activity_id, a.user_id, a.sport_type, a.sub_type, a.started_at
		  FROM activities a
		  JOIN activity_tracks t ON t.activity_id = a.id
		  WHERE a.user_id = $1
		    AND t.track_full IS NOT NULL
		    AND NOT EXISTS (
		        SELECT 1 FROM heatmap_points h WHERE h.activity_id = a.id
		    )
		),
		pts AS (
		  SELECT af.activity_id, af.user_id, af.sport_type, af.sub_type, af.started_at,
		         (ST_DumpPoints(COALESCE(t.track_public, t.track_full))).path[1] AS idx,
		         (ST_DumpPoints(COALESCE(t.track_public, t.track_full))).geom  AS pt
		  FROM activities_to_fill af
		  JOIN activity_tracks t ON t.activity_id = af.activity_id
		),
		with_dist AS (
		  SELECT activity_id, user_id, sport_type, sub_type, started_at, idx, pt,
		    COALESCE(
		      ST_Distance(pt::geography,
		        LAG(pt) OVER (PARTITION BY activity_id ORDER BY idx)::geography),
		      0
		    ) AS prev_dist
		  FROM pts
		),
		grouped AS (
		  SELECT activity_id, user_id, sport_type, sub_type, started_at, idx, pt,
		    SUM(CASE WHEN prev_dist > 2000 THEN 1 ELSE 0 END)
		      OVER (PARTITION BY activity_id ORDER BY idx) AS grp
		  FROM with_dist
		),
		line_groups AS (
		  SELECT activity_id, user_id, sport_type, sub_type, started_at, grp,
		         ST_MakeLine(pt ORDER BY idx) AS geom
		  FROM grouped
		  GROUP BY activity_id, user_id, sport_type, sub_type, started_at, grp
		  HAVING COUNT(*) >= 2
		)
		INSERT INTO heatmap_points (user_id, activity_id, sport_type, sub_type, activity_date, geom)
		SELECT user_id, activity_id, sport_type, sub_type, started_at::date,
		       (ST_DumpPoints(ST_Segmentize(geom::geography, $2)::geometry)).geom
		FROM line_groups
		WHERE geom IS NOT NULL`,
		userID, intervalM,
	)
	if err != nil {
		return 0, fmt.Errorf("backfill heatmap points: %w", err)
	}
	return int(result.RowsAffected()), nil
}

// GetHeatmapSettings returns all heatmap-related settings for a user.
func (db *DB) GetHeatmapSettings(ctx context.Context, userID string) (*model.HeatmapSettings, error) {
	var s model.HeatmapSettings
	err := db.Pool.QueryRow(ctx, `
		SELECT COALESCE(heatmap_sample_interval_m, 20),
		       COALESCE(teleport_threshold_m, 2000),
		       COALESCE(teleport_filter_map, true),
		       COALESCE(teleport_filter_activity, true)
		FROM users WHERE id = $1`, userID,
	).Scan(&s.SamplingIntervalM, &s.TeleportThresholdM, &s.TeleportFilterMap, &s.TeleportFilterActivity)
	if err != nil {
		// Defaults falls User nicht gefunden
		return &model.HeatmapSettings{SamplingIntervalM: 20, TeleportThresholdM: 2000, TeleportFilterMap: true, TeleportFilterActivity: true}, nil
	}
	return &s, nil
}

// UpdateHeatmapSettings writes all heatmap-related settings for a user.
func (db *DB) UpdateHeatmapSettings(ctx context.Context, userID string, s *model.HeatmapSettings) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE users SET
		    heatmap_sample_interval_m = $2,
		    teleport_threshold_m = $3,
		    teleport_filter_map = $4,
		    teleport_filter_activity = $5
		WHERE id = $1`,
		userID, s.SamplingIntervalM, s.TeleportThresholdM, s.TeleportFilterMap, s.TeleportFilterActivity,
	)
	return err
}

// GetUserHeatmapInterval is a compatibility wrapper for BackfillHeatmapPoints.
func (db *DB) GetUserHeatmapInterval(ctx context.Context, userID string) (int, error) {
	s, err := db.GetHeatmapSettings(ctx, userID)
	if err != nil {
		return 20, nil
	}
	return s.SamplingIntervalM, nil
}

// UpdateUserHeatmapInterval is a compatibility wrapper (updates only the sampling interval).
func (db *DB) UpdateUserHeatmapInterval(ctx context.Context, userID string, intervalM int) error {
	s, err := db.GetHeatmapSettings(ctx, userID)
	if err != nil {
		s = &model.HeatmapSettings{SamplingIntervalM: intervalM, TeleportThresholdM: 2000, TeleportFilterMap: true, TeleportFilterActivity: true}
	} else {
		s.SamplingIntervalM = intervalM
	}
	return db.UpdateHeatmapSettings(ctx, userID, s)
}

// ── Privacy Zones ───────────────────────────────────────────────────────────

// ListPrivacyZones returns all private zones for a user.
func (db *DB) ListPrivacyZones(ctx context.Context, userID string) ([]model.PrivacyZone, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, name,
			ST_Y(center) AS lat, ST_X(center) AS lng,
			radius_m, color, is_active, created_at,
			zone_type,
			CASE WHEN polygon IS NOT NULL THEN ST_AsGeoJSON(polygon) ELSE NULL END AS polygon_geojson
		FROM user_privacy_zones
		WHERE user_id = $1
		ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list privacy zones: %w", err)
	}
	defer rows.Close()

	var zones []model.PrivacyZone
	for rows.Next() {
		var z model.PrivacyZone
		if err := rows.Scan(&z.ID, &z.UserID, &z.Name, &z.Lat, &z.Lng, &z.RadiusM, &z.Color, &z.IsActive, &z.CreatedAt, &z.ZoneType, &z.Polygon); err != nil {
			return nil, fmt.Errorf("scan privacy zone: %w", err)
		}
		zones = append(zones, z)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list privacy zones: %w", err)
	}
	if zones == nil {
		zones = []model.PrivacyZone{}
	}
	return zones, nil
}

// CreatePrivacyZone creates a new private zone.
// zoneType is 'circle' or 'polygon'. polygonGeoJSON is the GeoJSON string for polygon zones (nil for circles).
func (db *DB) CreatePrivacyZone(ctx context.Context, userID, name string, lat, lng float64, radiusM int, color string, zoneType string, polygonGeoJSON *string) (*model.PrivacyZone, error) {
	var z model.PrivacyZone
	var err error
	if zoneType == "polygon" && polygonGeoJSON != nil {
		err = db.Pool.QueryRow(ctx, `
			INSERT INTO user_privacy_zones (user_id, name, center, radius_m, color, zone_type, polygon)
			VALUES ($1, $2, ST_SetSRID(ST_MakePoint($4, $3), 4326), $5, $6, 'polygon', ST_GeomFromGeoJSON($7))
			RETURNING id, user_id, name,
				ST_Y(center) AS lat, ST_X(center) AS lng,
				radius_m, color, is_active, created_at,
				zone_type,
				ST_AsGeoJSON(polygon) AS polygon_geojson`,
			userID, name, lat, lng, radiusM, color, *polygonGeoJSON,
		).Scan(&z.ID, &z.UserID, &z.Name, &z.Lat, &z.Lng, &z.RadiusM, &z.Color, &z.IsActive, &z.CreatedAt, &z.ZoneType, &z.Polygon)
	} else {
		err = db.Pool.QueryRow(ctx, `
			INSERT INTO user_privacy_zones (user_id, name, center, radius_m, color, zone_type)
			VALUES ($1, $2, ST_SetSRID(ST_MakePoint($4, $3), 4326), $5, $6, 'circle')
			RETURNING id, user_id, name,
				ST_Y(center) AS lat, ST_X(center) AS lng,
				radius_m, color, is_active, created_at,
				zone_type,
				NULL AS polygon_geojson`,
			userID, name, lat, lng, radiusM, color,
		).Scan(&z.ID, &z.UserID, &z.Name, &z.Lat, &z.Lng, &z.RadiusM, &z.Color, &z.IsActive, &z.CreatedAt, &z.ZoneType, &z.Polygon)
	}
	if err != nil {
		return nil, fmt.Errorf("create privacy zone: %w", err)
	}
	return &z, nil
}

// UpdatePrivacyZone updates an existing privacy zone.
func (db *DB) UpdatePrivacyZone(ctx context.Context, zoneID, userID string, update *model.PrivacyZoneUpdate) (*model.PrivacyZone, error) {
	var sets []string
	var args []any
	argIdx := 3
	args = append(args, zoneID, userID) // $1 = zoneID, $2 = userID

	if update.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *update.Name)
		argIdx++
	}
	if update.Lat != nil && update.Lng != nil {
		sets = append(sets, fmt.Sprintf("center = ST_SetSRID(ST_MakePoint($%d, $%d), 4326)", argIdx+1, argIdx))
		args = append(args, *update.Lat, *update.Lng)
		argIdx += 2
	}
	if update.RadiusM != nil {
		sets = append(sets, fmt.Sprintf("radius_m = $%d", argIdx))
		args = append(args, *update.RadiusM)
		argIdx++
	}
	if update.Color != nil {
		sets = append(sets, fmt.Sprintf("color = $%d", argIdx))
		args = append(args, *update.Color)
		argIdx++
	}
	if update.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *update.IsActive)
		argIdx++
	}
	if update.ZoneType != nil {
		sets = append(sets, fmt.Sprintf("zone_type = $%d", argIdx))
		args = append(args, *update.ZoneType)
		argIdx++
	}
	if update.Polygon != nil {
		sets = append(sets, fmt.Sprintf("polygon = ST_GeomFromGeoJSON($%d)", argIdx))
		args = append(args, *update.Polygon)
		argIdx++
	}

	if len(sets) == 0 {
		return db.getPrivacyZone(ctx, zoneID, userID)
	}

	query := fmt.Sprintf(`
		UPDATE user_privacy_zones SET %s
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, name,
			ST_Y(center) AS lat, ST_X(center) AS lng,
			radius_m, color, is_active, created_at,
			zone_type,
			CASE WHEN polygon IS NOT NULL THEN ST_AsGeoJSON(polygon) ELSE NULL END AS polygon_geojson`,
		strings.Join(sets, ", "),
	)

	var z model.PrivacyZone
	err := db.Pool.QueryRow(ctx, query, args...).Scan(
		&z.ID, &z.UserID, &z.Name, &z.Lat, &z.Lng, &z.RadiusM, &z.Color, &z.IsActive, &z.CreatedAt, &z.ZoneType, &z.Polygon,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update privacy zone: %w", err)
	}
	return &z, nil
}

func (db *DB) getPrivacyZone(ctx context.Context, zoneID, userID string) (*model.PrivacyZone, error) {
	var z model.PrivacyZone
	err := db.Pool.QueryRow(ctx, `
		SELECT id, user_id, name,
			ST_Y(center) AS lat, ST_X(center) AS lng,
			radius_m, color, is_active, created_at,
			zone_type,
			CASE WHEN polygon IS NOT NULL THEN ST_AsGeoJSON(polygon) ELSE NULL END AS polygon_geojson
		FROM user_privacy_zones
		WHERE id = $1 AND user_id = $2`,
		zoneID, userID,
	).Scan(&z.ID, &z.UserID, &z.Name, &z.Lat, &z.Lng, &z.RadiusM, &z.Color, &z.IsActive, &z.CreatedAt, &z.ZoneType, &z.Polygon)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get privacy zone: %w", err)
	}
	return &z, nil
}

// DeletePrivacyZone deletes a privacy zone.
func (db *DB) DeletePrivacyZone(ctx context.Context, zoneID, userID string) error {
	result, err := db.Pool.Exec(ctx,
		`DELETE FROM user_privacy_zones WHERE id = $1 AND user_id = $2`,
		zoneID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete privacy zone: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("privacy zone not found")
	}
	return nil
}

// ── Track Public (Privacy) ──────────────────────────────────────────────────

// ComputeTrackPublic calculates the public track by removing privacy zone areas.
// Uses PostGIS ST_Difference to subtract buffered privacy zones from the full track.
func (db *DB) ComputeTrackPublic(ctx context.Context, activityID, userID string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE activity_tracks SET track_public = (
			SELECT ST_Difference(
				t.track_full,
				COALESCE(
					(SELECT ST_Union(
						CASE
							WHEN z.zone_type = 'polygon' AND z.polygon IS NOT NULL
								THEN z.polygon
							ELSE ST_Buffer(z.center::geography, z.radius_m)::geometry
						END
					)
					FROM user_privacy_zones z
					WHERE z.user_id = $2 AND z.is_active = true),
					ST_GeomFromText('GEOMETRYCOLLECTION EMPTY', 4326)
				)
			)
			FROM activity_tracks t
			WHERE t.activity_id = $1
		)
		WHERE activity_id = $1`,
		activityID, userID,
	)
	if err != nil {
		return fmt.Errorf("compute track public: %w", err)
	}
	return nil
}

// RecomputeAllTrackPublic recalculates track_public for all activities of a user.
// Also refreshes heatmap_lines to use the updated track_public.
func (db *DB) RecomputeAllTrackPublic(ctx context.Context, userID string) error {
	// Step 1: Recompute track_public for all user activities
	_, err := db.Pool.Exec(ctx, `
		UPDATE activity_tracks SET track_public = (
			SELECT ST_Difference(
				activity_tracks.track_full,
				COALESCE(
					(SELECT ST_Union(
						CASE
							WHEN z.zone_type = 'polygon' AND z.polygon IS NOT NULL
								THEN z.polygon
							ELSE ST_Buffer(z.center::geography, z.radius_m)::geometry
						END
					)
					FROM user_privacy_zones z
					WHERE z.user_id = $1 AND z.is_active = true),
					ST_GeomFromText('GEOMETRYCOLLECTION EMPTY', 4326)
				)
			)
		)
		WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("recompute all track public: %w", err)
	}

	// Step 2: Refresh heatmap_lines with updated geometries
	_, err = db.Pool.Exec(ctx, `
		UPDATE heatmap_lines h SET geom = COALESCE(t.track_public, t.track_full)
		FROM activity_tracks t
		WHERE t.activity_id = h.activity_id
		  AND h.user_id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("refresh heatmap lines: %w", err)
	}

	return nil
}

// ── Enrichment ──────────────────────────────────────────────────────────────

// GetGeoTags returns all geo tags for an activity.
func (db *DB) GetGeoTags(ctx context.Context, activityID string) ([]model.GeoTag, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, activity_id, category, name, osm_id,
			lat, lng, ele, dist_m, duration_min, created_at,
			map_type, map_point_lat, map_point_lon,
			map_bbox_west, map_bbox_south, map_bbox_east, map_bbox_north,
			length_m, ele_m,
			ST_AsGeoJSON(map_section) AS map_section_geojson
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

// ── Tags ────────────────────────────────────────────────────────────────────

// ListTags returns all tags for a user, ordered by activity_count DESC, name ASC.
func (db *DB) ListTags(ctx context.Context, userID string) ([]model.Tag, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, name, color, text_color, category, is_system,
			is_auto, conditions, condition_groups, conditions_logic, description, activity_count, created_at,
			lat, lng, cover_image_url, cover_image_thumb_url, has_automation
		FROM tags
		WHERE user_id = $1
		ORDER BY activity_count DESC, name ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Name, &t.Color, &t.TextColor,
			&t.Category, &t.IsSystem, &t.IsAuto, &t.Conditions,
			&t.ConditionGroups, &t.ConditionsLogic,
			&t.Description, &t.ActivityCount, &t.CreatedAt,
			&t.Lat, &t.Lng, &t.CoverImageURL, &t.CoverImageThumbURL, &t.HasAutomation,
		); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	if tags == nil {
		tags = []model.Tag{}
	}

	// Enrich with zone data in a single query.
	zones, err := db.getTagZonesByUserID(ctx, userID)
	if err != nil {
		// Non-fatal: log and continue without zone data
		slog.Warn("list tags: failed to load zones", "error", err)
	} else {
		for i := range tags {
			if z, ok := zones[tags[i].ID]; ok {
				tags[i].Zone = z
			}
		}
	}

	// Append orphan tags: tag names in activities.tags TEXT[] with no tags table entry
	orphanRows, orphanErr := db.Pool.Query(ctx, `
		SELECT unnested.name, COUNT(DISTINCT a.id) AS activity_count
		FROM activities a
		CROSS JOIN LATERAL unnest(a.tags) AS unnested(name)
		WHERE a.user_id = $1
		  AND unnested.name != ''
		  AND NOT EXISTS (
		      SELECT 1 FROM tags t
		      WHERE t.user_id = $1 AND lower(t.name) = lower(unnested.name)
		  )
		GROUP BY unnested.name
		ORDER BY activity_count DESC, unnested.name ASC`,
		userID,
	)
	if orphanErr == nil {
		for orphanRows.Next() {
			var name string
			var count int
			if scanErr := orphanRows.Scan(&name, &count); scanErr == nil {
				tags = append(tags, model.Tag{
					ID:            "",
					UserID:        userID,
					Name:          name,
					Color:         "#9CA3AF",
					TextColor:     "#FFFFFF",
					Category:      "orphan",
					ActivityCount: count,
				})
			}
		}
		orphanRows.Close()
	}

	return tags, nil
}

// GetTagByID returns a single tag by ID, scoped to the given user.
func (db *DB) GetTagByID(ctx context.Context, tagID, userID string) (*model.Tag, error) {
	var t model.Tag
	err := db.Pool.QueryRow(ctx, `
		SELECT id, user_id, name, color, text_color, category, is_system,
			is_auto, conditions, condition_groups, conditions_logic, description, activity_count, created_at,
			lat, lng, cover_image_url, cover_image_thumb_url, has_automation
		FROM tags
		WHERE id = $1 AND user_id = $2`, tagID, userID,
	).Scan(
		&t.ID, &t.UserID, &t.Name, &t.Color, &t.TextColor,
		&t.Category, &t.IsSystem, &t.IsAuto, &t.Conditions,
		&t.ConditionGroups, &t.ConditionsLogic,
		&t.Description, &t.ActivityCount, &t.CreatedAt,
		&t.Lat, &t.Lng, &t.CoverImageURL, &t.CoverImageThumbURL, &t.HasAutomation,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tag by id: %w", err)
	}
	return &t, nil
}

// GetTagActivityIDs returns the IDs of all activities associated with the given tag.
func (db *DB) GetTagActivityIDs(ctx context.Context, tagID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT activity_id FROM activity_tags WHERE tag_id = $1`, tagID,
	)
	if err != nil {
		return nil, fmt.Errorf("get tag activity ids: %w", err)
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
		return nil, fmt.Errorf("get tag activity ids: %w", err)
	}
	return ids, nil
}

// CreateTag creates a new tag for a user.
func (db *DB) CreateTag(ctx context.Context, userID, name, color, textColor string, isAuto bool, conditions, conditionGroups []byte, conditionsLogic string, description *string, lat, lng *float64) (*model.Tag, error) {
	if conditionsLogic == "" {
		conditionsLogic = "and"
	}
	category := "custom"
	if lat != nil && lng != nil {
		category = "standort"
	}
	var t model.Tag
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, is_auto, conditions, condition_groups, conditions_logic, description, lat, lng, category)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, user_id, name, color, text_color, category, is_system,
			is_auto, conditions, condition_groups, conditions_logic, description, activity_count, created_at,
			lat, lng, cover_image_url, cover_image_thumb_url, has_automation`,
		userID, name, color, textColor, isAuto, conditions, conditionGroups, conditionsLogic, description, lat, lng, category,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.Color, &t.TextColor,
		&t.Category, &t.IsSystem, &t.IsAuto, &t.Conditions,
		&t.ConditionGroups, &t.ConditionsLogic,
		&t.Description, &t.ActivityCount, &t.CreatedAt,
		&t.Lat, &t.Lng, &t.CoverImageURL, &t.CoverImageThumbURL, &t.HasAutomation)
	if err != nil {
		return nil, fmt.Errorf("create tag: %w", err)
	}
	return &t, nil
}

// UpdateTag updates an existing tag.
// lat and lng are treated as optional nullable fields:
//   - locationPresent=false → lat/lng columns not touched
//   - locationPresent=true, lat/lng=nil → SET lat=NULL, lng=NULL
//   - locationPresent=true, lat/lng=non-nil → SET lat=*lat, lng=*lng
func (db *DB) UpdateTag(ctx context.Context, tagID, userID, name, color, textColor string, isAuto *bool, conditions, conditionGroups *[]byte, conditionsLogic *string, description *string, lat, lng *float64, locationPresent bool, coverImageURL *string, coverImageThumbURL *string) (*model.Tag, error) {
	var sets []string
	var args []any
	args = append(args, tagID, userID) // $1 = tagID, $2 = userID
	argIdx := 3

	if name != "" {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, name)
		argIdx++
	}
	if color != "" {
		sets = append(sets, fmt.Sprintf("color = $%d", argIdx))
		args = append(args, color)
		argIdx++
	}
	if textColor != "" {
		sets = append(sets, fmt.Sprintf("text_color = $%d", argIdx))
		args = append(args, textColor)
		argIdx++
	}
	if isAuto != nil {
		sets = append(sets, fmt.Sprintf("is_auto = $%d", argIdx))
		args = append(args, *isAuto)
		argIdx++
	}
	if conditions != nil {
		sets = append(sets, fmt.Sprintf("conditions = $%d", argIdx))
		args = append(args, *conditions)
		argIdx++
	}
	if conditionGroups != nil {
		sets = append(sets, fmt.Sprintf("condition_groups = $%d", argIdx))
		args = append(args, *conditionGroups)
		argIdx++
	}
	if conditionsLogic != nil {
		sets = append(sets, fmt.Sprintf("conditions_logic = $%d", argIdx))
		args = append(args, *conditionsLogic)
		argIdx++
	}
	if description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *description)
		argIdx++
	}
	if locationPresent {
		// lat and lng are either both NULL or both a value — update both together.
		sets = append(sets, fmt.Sprintf("lat = $%d", argIdx))
		args = append(args, lat) // pgx treats nil *float64 as SQL NULL
		argIdx++
		sets = append(sets, fmt.Sprintf("lng = $%d", argIdx))
		args = append(args, lng)
		argIdx++
		// Auto-set category based on location
		if lat != nil {
			sets = append(sets, fmt.Sprintf("category = $%d", argIdx))
			args = append(args, "standort")
			argIdx++
		} else {
			// lat removed → revert standort back to custom (only if it was standort)
			sets = append(sets, "category = CASE WHEN category = 'standort' THEN 'custom' ELSE category END")
		}
	}
	if coverImageURL != nil {
		sets = append(sets, fmt.Sprintf("cover_image_url = $%d", argIdx))
		args = append(args, *coverImageURL)
		argIdx++
	}
	if coverImageThumbURL != nil {
		sets = append(sets, fmt.Sprintf("cover_image_thumb_url = $%d", argIdx))
		args = append(args, *coverImageThumbURL)
		argIdx++
	}

	tagCols := `id, user_id, name, color, text_color, category, is_system,
		is_auto, conditions, condition_groups, conditions_logic, description, activity_count, created_at,
		lat, lng, cover_image_url, cover_image_thumb_url, has_automation`

	scanTag := func(row interface{ Scan(...any) error }) (*model.Tag, error) {
		var t model.Tag
		err := row.Scan(
			&t.ID, &t.UserID, &t.Name, &t.Color, &t.TextColor,
			&t.Category, &t.IsSystem, &t.IsAuto, &t.Conditions,
			&t.ConditionGroups, &t.ConditionsLogic,
			&t.Description, &t.ActivityCount, &t.CreatedAt,
			&t.Lat, &t.Lng, &t.CoverImageURL, &t.CoverImageThumbURL, &t.HasAutomation,
		)
		return &t, err
	}

	if len(sets) == 0 {
		t, err := scanTag(db.Pool.QueryRow(ctx,
			fmt.Sprintf(`SELECT %s FROM tags WHERE id = $1 AND user_id = $2`, tagCols),
			tagID, userID,
		))
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get tag: %w", err)
		}
		return t, nil
	}

	query := fmt.Sprintf(`
		UPDATE tags SET %s
		WHERE id = $1 AND user_id = $2
		RETURNING %s`,
		strings.Join(sets, ", "), tagCols,
	)

	t, err := scanTag(db.Pool.QueryRow(ctx, query, args...))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update tag: %w", err)
	}
	return t, nil
}

// ClearTagCoverImages setzt cover_image_url und cover_image_thumb_url auf NULL.
func (db *DB) ClearTagCoverImages(ctx context.Context, tagID, userID string) (*model.Tag, error) {
	tagCols := `id, user_id, name, color, text_color, category, is_system,
		is_auto, conditions, condition_groups, conditions_logic, description, activity_count, created_at,
		lat, lng, cover_image_url, cover_image_thumb_url, has_automation`
	var t model.Tag
	err := db.Pool.QueryRow(ctx,
		fmt.Sprintf(`UPDATE tags
                     SET cover_image_url = NULL, cover_image_thumb_url = NULL
                     WHERE id = $1 AND user_id = $2
                     RETURNING %s`, tagCols),
		tagID, userID,
	).Scan(
		&t.ID, &t.UserID, &t.Name, &t.Color, &t.TextColor,
		&t.Category, &t.IsSystem, &t.IsAuto, &t.Conditions,
		&t.ConditionGroups, &t.ConditionsLogic,
		&t.Description, &t.ActivityCount, &t.CreatedAt,
		&t.Lat, &t.Lng, &t.CoverImageURL, &t.CoverImageThumbURL, &t.HasAutomation,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clear tag cover images: %w", err)
	}
	return &t, nil
}

// DeleteTag deletes a tag.
func (db *DB) DeleteTag(ctx context.Context, tagID, userID string) error {
	result, err := db.Pool.Exec(ctx,
		`DELETE FROM tags WHERE id = $1 AND user_id = $2`,
		tagID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("tag not found")
	}
	return nil
}

// GetTagsByActivity returns all tags assigned to an activity via the activity_tags junction table.
func (db *DB) GetTagsByActivity(ctx context.Context, activityID string) ([]model.Tag, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT t.id, t.user_id, t.name, t.color, t.text_color, t.category, t.is_system,
			t.is_auto, t.conditions, t.condition_groups, t.conditions_logic, t.description, t.activity_count, t.created_at,
			t.lat, t.lng, t.cover_image_url, t.cover_image_thumb_url, t.has_automation
		FROM tags t
		JOIN activity_tags at ON at.tag_id = t.id
		WHERE at.activity_id = $1
		ORDER BY t.name ASC`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get tags by activity: %w", err)
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Name, &t.Color, &t.TextColor,
			&t.Category, &t.IsSystem, &t.IsAuto, &t.Conditions,
			&t.ConditionGroups, &t.ConditionsLogic,
			&t.Description, &t.ActivityCount, &t.CreatedAt,
			&t.Lat, &t.Lng, &t.CoverImageURL, &t.CoverImageThumbURL, &t.HasAutomation,
		); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get tags by activity: %w", err)
	}
	if tags == nil {
		tags = []model.Tag{}
	}
	return tags, nil
}

// GetTagActivities returns a lightweight list of activities that have the given tag.
func (db *DB) GetTagActivities(ctx context.Context, tagID, userID string) ([]model.TagActivity, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT a.id, a.title, a.title_auto, a.started_at, a.sport_type, a.distance_m
		FROM activities a
		JOIN activity_tags at_join ON at_join.activity_id = a.id
		WHERE at_join.tag_id = $1 AND a.user_id = $2
		ORDER BY a.started_at DESC
		LIMIT 50
	`, tagID, userID)
	if err != nil {
		return nil, fmt.Errorf("get tag activities: %w", err)
	}
	defer rows.Close()
	var activities []model.TagActivity
	for rows.Next() {
		var a model.TagActivity
		if err := rows.Scan(&a.ID, &a.Title, &a.TitleAuto, &a.StartedAt, &a.SportType, &a.DistanceM); err != nil {
			return nil, fmt.Errorf("scan tag activity: %w", err)
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get tag activities: %w", err)
	}
	if activities == nil {
		activities = []model.TagActivity{}
	}
	return activities, nil
}

// SeedSystemDefaults creates the default system tags and automation rules for a new user.
func (db *DB) SeedSystemDefaults(ctx context.Context, userID string) error {
	nachtDesc := "Automatisch bei Aktivitaeten die in die astronomische Nacht fallen"
	frostDesc := "Automatisch bei Temperaturen unter 0 Grad C"

	// System tags (is_auto=false — now driven by automation rules, not the old autotag engine)
	seeds := []struct {
		name, color, textColor string
		desc                   string
	}{
		{"Nacht", "#1E293B", "#E2E8F0", nachtDesc},
		{"Frost", "#38BDF8", "#0C4A6E", frostDesc},
	}
	for _, s := range seeds {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, is_system, is_auto, description)
			VALUES ($1, $2, $3, $4, true, false, $5)
			ON CONFLICT (user_id, name) DO NOTHING`,
			userID, s.name, s.color, s.textColor, s.desc,
		); err != nil {
			return fmt.Errorf("seed %s tag: %w", s.name, err)
		}
	}

	// Rundtour — rule-managed (set_field + add_tag via automation rule)
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, is_system, is_auto, description)
		VALUES ($1, 'Rundtour', '#10B981', '#FFFFFF', true, false, 'Automatisch bei Aktivitaeten die als Rundtour erkannt wurden (Sync mit is_round_trip)')
		ON CONFLICT (user_id, name) DO UPDATE SET is_auto = false, conditions = NULL, condition_groups = NULL, conditions_logic = NULL`,
		userID,
	); err != nil {
		return fmt.Errorf("seed Rundtour tag: %w", err)
	}

	// Weather system tags
	weatherTags := []struct{ name, color string }{
		{"sonnig", "#FCD34D"},
		{"bewölkt", "#9CA3AF"},
		{"Regen", "#3B82F6"},
		{"Schnee", "#F3F4F6"},
		{"Nebel", "#D1D5DB"},
		{"Gewitter", "#7C3AED"},
	}
	weatherDesc := "Automatisch erkannt aus Wetterdaten"
	for _, w := range weatherTags {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description)
			VALUES ($1, $2, $3, $4, 'weather', true, false, $5)
			ON CONFLICT (user_id, name) DO NOTHING`,
			userID, w.name, w.color, contrastTextColor(w.color), weatherDesc,
		); err != nil {
			return fmt.Errorf("seed weather tag %s: %w", w.name, err)
		}
	}

	// E-Bike system tag (no conditions — synced via automation rule with is_ebike column).
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, is_system, is_auto, description)
		VALUES ($1, 'E-Bike', '#16A34A', '#FFFFFF', true, false, 'Automatisch bei E-Bike-Aktivitäten (Sync mit is_ebike)')
		ON CONFLICT (user_id, name) DO NOTHING`,
		userID,
	); err != nil {
		return fmt.Errorf("seed E-Bike tag: %w", err)
	}

	// Migrate existing tags from activities.tags TEXT[] to tags table
	// This ensures all previously used tags appear in the Tags overview
	if err := db.migrateExistingTagsToTable(ctx, userID); err != nil {
		slog.Warn("seed: migrate existing tags failed (non-fatal)", "user_id", userID, "error", err)
	}

	// System automation rules for new users
	// (existing users get them via migration 000176)
	type tagRule struct {
		tagName    string
		systemType string
		priority   int
		conditions string
	}
	tagRules := []tagRule{
		{"Rundtour", "round_trip", -99, `[{"field":"start_end_distance_m","op":"lte","value":500}]`},
		{"Frost", "frost", -98, `[{"field":"min_temperature_c","op":"lte","value":0}]`},
		{"Nacht", "nacht", -97, `[{"field":"sun_elevation","op":"lte","value":-6}]`},
	}
	for _, tr := range tagRules {
		var tagID string
		err := db.Pool.QueryRow(ctx,
			`SELECT id FROM tags WHERE user_id = $1 AND name = $2 AND is_system = true`,
			userID, tr.tagName,
		).Scan(&tagID)
		if err != nil {
			continue
		}
		var actions string
		switch tr.tagName {
		case "Rundtour":
			actions = fmt.Sprintf(`[{"action":"add_tag","params":{"tag_id":"%s"}},{"action":"set_field","params":{"field":"is_round_trip","value":true}}]`, tagID)
		default:
			actions = fmt.Sprintf(`[{"action":"add_tag","params":{"tag_id":"%s"}}]`, tagID)
		}
		_, _ = db.Pool.Exec(ctx, `
			INSERT INTO automation_rules (user_id, name, is_system, system_type, is_active, priority, conditions, conditions_logic, actions)
			VALUES ($1, $2, true, $3, true, $4, $5::jsonb, 'and', $6::jsonb)
			ON CONFLICT (user_id, system_type) WHERE is_system = true DO NOTHING`,
			userID, tr.tagName, tr.systemType, tr.priority, tr.conditions, actions)
	}
	// E-Bike rule (no conditions/actions — handled by import pipeline)
	_, _ = db.Pool.Exec(ctx, `
		INSERT INTO automation_rules (user_id, name, is_system, system_type, is_active, priority, conditions, conditions_logic, actions)
		VALUES ($1, 'E-Bike', true, 'ebike_sync', true, -100, '[]'::jsonb, 'and', '[]'::jsonb)
		ON CONFLICT (user_id, system_type) WHERE is_system = true DO NOTHING`,
		userID)

	return nil
}

// migrateExistingTagsToTable scans all activities.tags TEXT[] and creates missing entries in tags table.
// This ensures all previously-used tag names appear in the Tags overview.
func (db *DB) migrateExistingTagsToTable(ctx context.Context, userID string) error {
	// Collect all unique tag names from activities.tags TEXT[] for this user
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT UNNEST(tags) AS tag_name
		FROM activities
		WHERE user_id = $1 AND tags IS NOT NULL AND array_length(tags, 1) > 0
		ORDER BY tag_name
	`, userID)
	if err != nil {
		return fmt.Errorf("query existing tags: %w", err)
	}
	defer rows.Close()

	var tagNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan tag name: %w", err)
		}
		if name != "" {
			tagNames = append(tagNames, name)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate tag names: %w", err)
	}

	if len(tagNames) == 0 {
		return nil // No tags to migrate
	}

	// Color palette for auto-assigned colors to existing tags
	colorPalette := []struct{ color, text string }{
		{"#f97316", "#ffffff"}, // orange
		{"#3b82f6", "#ffffff"}, // blue
		{"#10b981", "#ffffff"}, // green
		{"#8b5cf6", "#ffffff"}, // purple
		{"#f43f5e", "#ffffff"}, // rose
		{"#06b6d4", "#ffffff"}, // cyan
		{"#eab308", "#1a1a1a"}, // yellow
		{"#6b7280", "#ffffff"}, // gray
	}

	// Insert each tag if it doesn't exist
	for i, tagName := range tagNames {
		// Assign color from palette (cycle if needed)
		color := colorPalette[i%len(colorPalette)].color
		textColor := colorPalette[i%len(colorPalette)].text

		// Determine if this is a known auto-tag
		isAuto := false
		category := "custom"
		if tagName == "Nacht" || tagName == "Frost" {
			isAuto = true
		}
		// Rundtour is rule-managed (system rule round_trip), so isAuto stays false
		if tagName == "sonnig" || tagName == "bewölkt" || tagName == "Regen" ||
			tagName == "Schnee" || tagName == "Nebel" || tagName == "Gewitter" {
			isAuto = false // Weather tags are not marked as auto (system flag)
			category = "weather"
		}

		_, err := db.Pool.Exec(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, category, is_auto, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW())
			ON CONFLICT (user_id, name) DO NOTHING
		`, userID, tagName, color, textColor, category, isAuto)
		if err != nil {
			slog.Warn("migrate tag to table", "user_id", userID, "tag_name", tagName, "error", err)
			// Continue with other tags instead of failing
		}
	}

	// Step 2: For EACH tag, find all activities with that tag in activities.tags TEXT[]
	// and create activity_tags junction entries.
	for _, tagName := range tagNames {
		// Get the tag ID that was just created/upserted
		var tagID string
		if err := db.Pool.QueryRow(ctx,
			"SELECT id FROM tags WHERE user_id = $1 AND name = $2",
			userID, tagName).Scan(&tagID); err != nil {
			slog.Warn("migrate tags: get tag id failed", "user_id", userID, "tag_name", tagName, "error", err)
			continue // Skip if tag not found
		}

		// Find all activities that have this tag in their tags TEXT[] array
		rows, err := db.Pool.Query(ctx,
			"SELECT id FROM activities WHERE user_id = $1 AND $2 = ANY(tags)",
			userID, tagName)
		if err != nil {
			slog.Warn("migrate tags: query activities failed", "user_id", userID, "tag_name", tagName, "error", err)
			continue
		}

		// For each activity, insert into activity_tags (skip duplicates via ON CONFLICT)
		for rows.Next() {
			var activityID string
			if err := rows.Scan(&activityID); err != nil {
				slog.Warn("migrate tags: scan activity id failed", "user_id", userID, "tag_name", tagName, "error", err)
				continue
			}

			if _, execErr := db.Pool.Exec(ctx,
				"INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES ($1, $2, 'migration') ON CONFLICT DO NOTHING",
				activityID, tagID); execErr != nil {
				slog.Warn("migrate tags: insert activity_tag failed", "activity_id", activityID, "tag_id", tagID, "error", execErr)
				// Continue with other activities
			}
		}
		rows.Close()
	}

	// Step 3: Update activity_count for all migrated tags
	if _, err := db.Pool.Exec(ctx, `
		UPDATE tags
		SET activity_count = (
			SELECT COUNT(DISTINCT activity_id)
			FROM activity_tags
			WHERE tag_id = tags.id
		)
		WHERE user_id = $1 AND category IN ('custom', 'weather')
	`, userID); err != nil {
		slog.Warn("migrate tags: update activity counts failed", "user_id", userID, "error", err)
		// Non-fatal, continue
	}

	return nil
}

// MergeTag merges sourceTagID into targetTagID in a single transaction:
//  1. All activity_tags rows pointing to sourceTagID → replaced by targetTagID (skip
//     if the activity already has targetTagID to avoid duplicate-key violation).
//  2. activities.tags TEXT[] cache: remove source tag name, add target tag name if
//     not already present.
//  3. The source tag is deleted.
//
// Returns the surviving (target) tag on success.
func (db *DB) MergeTag(ctx context.Context, targetTagID, sourceTagID, userID string) (*model.Tag, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// 1. Resolve names of both tags and verify ownership.
	var targetName, sourceName string
	if err := tx.QueryRow(ctx,
		`SELECT name FROM tags WHERE id = $1 AND user_id = $2`,
		targetTagID, userID,
	).Scan(&targetName); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("target tag not found")
		}
		return nil, fmt.Errorf("get target tag: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT name FROM tags WHERE id = $1 AND user_id = $2`,
		sourceTagID, userID,
	).Scan(&sourceName); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("source tag not found")
		}
		return nil, fmt.Errorf("get source tag: %w", err)
	}
	if targetTagID == sourceTagID {
		return nil, fmt.Errorf("target and source tag must differ")
	}

	// Block merge if source is a system tag
	var sourceIsSystem bool
	if err := tx.QueryRow(ctx,
		`SELECT is_system FROM tags WHERE id = $1 AND user_id = $2`,
		sourceTagID, userID,
	).Scan(&sourceIsSystem); err != nil {
		return nil, fmt.Errorf("check source system: %w", err)
	}
	if sourceIsSystem {
		return nil, fmt.Errorf("source_is_system")
	}

	// Block merge if source tag is referenced by an automation rule
	var hasRule bool
	tx.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM automation_rules
			WHERE user_id = $1 AND actions::text LIKE '%' || $2::text || '%'
		)`,
		userID, sourceTagID,
	).Scan(&hasRule) //nolint:errcheck
	if hasRule {
		return nil, fmt.Errorf("source_has_rule")
	}

	// 2. Move activity_tags rows: for activities that have source but not target,
	//    update the tag_id. For activities that already have both, just delete the
	//    source row (to avoid a unique constraint violation).
	_, err = tx.Exec(ctx, `
		UPDATE activity_tags
		SET tag_id = $1
		WHERE tag_id = $2
		  AND activity_id NOT IN (
		      SELECT activity_id FROM activity_tags WHERE tag_id = $1
		  )`,
		targetTagID, sourceTagID,
	)
	if err != nil {
		return nil, fmt.Errorf("migrate activity_tags: %w", err)
	}
	// Delete any remaining rows that still reference sourceTagID (those activities
	// already had targetTagID so the UPDATE above skipped them).
	_, err = tx.Exec(ctx,
		`DELETE FROM activity_tags WHERE tag_id = $1`,
		sourceTagID,
	)
	if err != nil {
		return nil, fmt.Errorf("delete orphan activity_tags: %w", err)
	}

	// Update activity_count for targetTagID (it now includes activities from sourceTagID)
	_, err = tx.Exec(ctx,
		`UPDATE tags
		 SET activity_count = (SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = $1)
		 WHERE id = $1`,
		targetTagID)
	if err != nil {
		return nil, fmt.Errorf("merge tags: update target count: %w", err)
	}

	// 3. Sync the denormalised activities.tags TEXT[] cache.
	//    Remove sourceName, add targetName (if not already present).
	_, err = tx.Exec(ctx, `
		UPDATE activities
		SET tags = array_append(
		              array_remove(tags, $1::text),
		              $2::text
		          ),
		    updated_at = NOW()
		WHERE user_id = $3
		  AND $1 = ANY(tags)
		  AND NOT ($2 = ANY(tags))`,
		sourceName, targetName, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("update activities.tags (add target): %w", err)
	}
	// For activities that already had both names: just remove sourceName.
	_, err = tx.Exec(ctx, `
		UPDATE activities
		SET tags = array_remove(tags, $1::text),
		    updated_at = NOW()
		WHERE user_id = $2
		  AND $1 = ANY(tags)`,
		sourceName, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("update activities.tags (remove source): %w", err)
	}

	// 4a. Migrate activity_tag_removals from source to target.
	_, err = tx.Exec(ctx, `
		UPDATE activity_tag_removals SET tag_id = $1
		WHERE tag_id = $2
		  AND activity_id NOT IN (SELECT activity_id FROM activity_tag_removals WHERE tag_id = $1)`,
		targetTagID, sourceTagID)
	if err != nil {
		return nil, fmt.Errorf("migrate removals: %w", err)
	}
	_, err = tx.Exec(ctx,
		`DELETE FROM activity_tag_removals WHERE tag_id = $1`, sourceTagID)
	if err != nil {
		return nil, fmt.Errorf("delete stale removals: %w", err)
	}

	// 4b. Delete the source tag.
	_, err = tx.Exec(ctx,
		`DELETE FROM tags WHERE id = $1 AND user_id = $2`,
		sourceTagID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("delete source tag: %w", err)
	}

	// 5. Return the surviving target tag (with refreshed activity_count).
	tagCols := `id, user_id, name, color, text_color, category, is_system,
		is_auto, conditions, condition_groups, conditions_logic, description, activity_count, created_at,
		lat, lng, cover_image_url, cover_image_thumb_url, has_automation`
	var t model.Tag
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM tags WHERE id = $1`, tagCols),
		targetTagID,
	).Scan(
		&t.ID, &t.UserID, &t.Name, &t.Color, &t.TextColor,
		&t.Category, &t.IsSystem, &t.IsAuto, &t.Conditions,
		&t.ConditionGroups, &t.ConditionsLogic,
		&t.Description, &t.ActivityCount, &t.CreatedAt,
		&t.Lat, &t.Lng, &t.CoverImageURL, &t.CoverImageThumbURL, &t.HasAutomation,
	)
	if err != nil {
		return nil, fmt.Errorf("get merged tag: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit merge: %w", err)
	}
	return &t, nil
}

// UpdateEnrichmentFields updates enrichment-related fields on an activity.
// Always overwrites existing values — use UpdateEnrichmentFieldsSafe for lock-free fill-in semantics.
func (db *DB) UpdateEnrichmentFields(ctx context.Context, activityID string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}

	var sets []string
	var args []any
	args = append(args, activityID) // $1
	argIdx := 2

	for col, val := range fields {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}

	sets = append(sets, "updated_at = NOW()")

	query := fmt.Sprintf("UPDATE activities SET %s WHERE id = $1", strings.Join(sets, ", "))
	_, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update enrichment fields: %w", err)
	}
	return nil
}

// UpdateEnrichmentFieldsSafe updates enrichment-related fields on an activity using
// COALESCE semantics: each column is only written if the existing DB value is NULL.
// This is safe for concurrent imports — parallel importers cannot overwrite each
// other's data and will never replace a real value with a zero/null.
// updated_at is always refreshed regardless of COALESCE.
func (db *DB) UpdateEnrichmentFieldsSafe(ctx context.Context, activityID string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}

	var sets []string
	var args []any
	args = append(args, activityID) // $1
	argIdx := 2

	for col, val := range fields {
		// COALESCE(col, $N): keeps existing non-NULL value, writes $N only when NULL
		sets = append(sets, fmt.Sprintf("%s = COALESCE(%s, $%d)", col, col, argIdx))
		args = append(args, val)
		argIdx++
	}

	sets = append(sets, "updated_at = NOW()")

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
			enable_stop_detection, enable_climb_detection, enable_commute_detection,
			enable_auto_naming, enable_lap_detection,
			climb_min_gradient_pct, climb_min_elevation_m,
			round_trip_threshold_m, ocr_invoice_prompt, ocr_gear_prompt,
			enable_peaks, enable_lifts, enable_settlements, enable_commute,
			enable_borders, enable_water, enable_forests, enable_surfaces,
			enable_stops, enable_intervals, enable_ascents, enable_embeddings,
			photo_preview_size, terra_incognita_radius_m,
			route_duplicate_hausdorff_m, route_duplicate_length_diff_km,
			video_clip_frames, phash_threshold_bits`,
		userID,
	).Scan(
		&s.UserID, &s.EnableSummitDetection, &s.EnableWeatherEnrichment,
		&s.EnableStopDetection, &s.EnableClimbDetection, &s.EnableCommuteDetection,
		&s.EnableAutoNaming, &s.EnableLapDetection, &s.ClimbMinGradientPct, &s.ClimbMinElevationM,
		&s.RoundTripThresholdM, &s.OCRInvoicePrompt, &s.OCRGearPrompt,
		&s.EnablePeaks, &s.EnableLifts, &s.EnableSettlements, &s.EnableCommute,
		&s.EnableBorders, &s.EnableWater, &s.EnableForests, &s.EnableSurfaces,
		&s.EnableStops, &s.EnableIntervals, &s.EnableAscents, &s.EnableEmbeddings,
		&s.PhotoPreviewSize, &s.TerraIncognitaRadiusM,
		&s.RouteDuplicateHausdorffM, &s.RouteDuplicateLengthDiffKm,
		&s.VideoClipFrames, &s.PhashThresholdBits,
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
			enable_stop_detection, enable_climb_detection, enable_commute_detection,
			enable_auto_naming, enable_lap_detection,
			climb_min_gradient_pct, climb_min_elevation_m,
			round_trip_threshold_m, ocr_invoice_prompt, ocr_gear_prompt,
			enable_peaks, enable_lifts, enable_settlements, enable_commute,
			enable_borders, enable_water, enable_forests, enable_surfaces,
			enable_stops, enable_intervals, enable_ascents, enable_embeddings,
			photo_preview_size, terra_incognita_radius_m,
			route_duplicate_hausdorff_m, route_duplicate_length_diff_km,
			video_clip_frames, phash_threshold_bits
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25,
			$26, $27, $28, $29, $30, $31
		)
		ON CONFLICT (user_id) DO UPDATE SET
			enable_summit_detection = $2,
			enable_weather_enrichment = $3,
			enable_stop_detection = $4,
			enable_climb_detection = $5,
			enable_commute_detection = $6,
			enable_auto_naming = $7,
			enable_lap_detection = $8,
			climb_min_gradient_pct = $9,
			climb_min_elevation_m = $10,
			round_trip_threshold_m = $11,
			ocr_invoice_prompt = $12,
			ocr_gear_prompt = $13,
			enable_peaks = $14,
			enable_lifts = $15,
			enable_settlements = $16,
			enable_commute = $17,
			enable_borders = $18,
			enable_water = $19,
			enable_forests = $20,
			enable_surfaces = $21,
			enable_stops = $22,
			enable_intervals = $23,
			enable_ascents = $24,
			enable_embeddings = $25,
			photo_preview_size = $26,
			terra_incognita_radius_m = $27,
			route_duplicate_hausdorff_m = $28,
			route_duplicate_length_diff_km = $29,
			video_clip_frames = $30,
			phash_threshold_bits = $31`,
		userID, s.EnableSummitDetection, s.EnableWeatherEnrichment,
		s.EnableStopDetection, s.EnableClimbDetection, s.EnableCommuteDetection,
		s.EnableAutoNaming, s.EnableLapDetection, s.ClimbMinGradientPct, s.ClimbMinElevationM,
		s.RoundTripThresholdM, s.OCRInvoicePrompt, s.OCRGearPrompt,
		s.EnablePeaks, s.EnableLifts, s.EnableSettlements, s.EnableCommute,
		s.EnableBorders, s.EnableWater, s.EnableForests, s.EnableSurfaces,
		s.EnableStops, s.EnableIntervals, s.EnableAscents, s.EnableEmbeddings,
		s.PhotoPreviewSize, s.TerraIncognitaRadiusM,
		s.RouteDuplicateHausdorffM, s.RouteDuplicateLengthDiffKm,
		s.VideoClipFrames, s.PhashThresholdBits,
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
	if err == pgx.ErrNoRows {
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
				elevation_gain_m, avg_gradient_pct, max_gradient_pct, COALESCE(category, '')
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
			&c.LengthM, &c.ElevationGainM, &c.AvgGradientPct, &c.MaxGradientPct, &c.Category); err != nil {
			return nil, fmt.Errorf("scan activity climb: %w", err)
		}
		climbs = append(climbs, c)
	}
	if climbs == nil {
		climbs = []model.Climb{}
	}
	return climbs, rows.Err()
}

// DeleteOrphanTag removes a tag name from all activities.tags TEXT[] for a user.
// Used to clean up orphan tag names that have no corresponding tags table entry.
func (db *DB) DeleteOrphanTag(ctx context.Context, userID, tagName string) (int64, error) {
	result, err := db.Pool.Exec(ctx, `
		UPDATE activities
		SET tags = array_remove(tags, $2), updated_at = NOW()
		WHERE user_id = $1 AND $2 = ANY(tags)`,
		userID, tagName,
	)
	if err != nil {
		return 0, fmt.Errorf("delete orphan tag: %w", err)
	}
	return result.RowsAffected(), nil
}

// GetActivityStops returns detected stops for an activity.
func (db *DB) GetActivityStops(ctx context.Context, activityID string) ([]model.ActivityStop, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, started_at, ended_at, duration_s,
		        ST_Y(location::geometry) AS lat, ST_X(location::geometry) AS lng,
		        name, poi_type, poi_name, poi_osm_id
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
			&s.Lat, &s.Lng, &s.Name, &s.PoiType, &s.PoiName, &s.PoiOsmID); err != nil {
			return nil, fmt.Errorf("scan activity stop: %w", err)
		}
		stops = append(stops, s)
	}
	return stops, rows.Err()
}

// GetActivitySurfaces returns detected surfaces for an activity.
func (db *DB) GetActivitySurfaces(ctx context.Context, activityID string) ([]model.ActivitySurface, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, surface_type, path_type, length_m, percentage
		 FROM activity_surfaces WHERE activity_id = $1 ORDER BY percentage DESC`,
		activityID)
	if err != nil {
		return nil, fmt.Errorf("get activity surfaces: %w", err)
	}
	defer rows.Close()
	var surfaces []model.ActivitySurface
	for rows.Next() {
		var s model.ActivitySurface
		if err := rows.Scan(&s.ID, &s.SurfaceType, &s.PathType, &s.LengthM, &s.Percentage); err != nil {
			return nil, fmt.Errorf("scan activity surface: %w", err)
		}
		surfaces = append(surfaces, s)
	}
	return surfaces, rows.Err()
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

	errors := map[string]string{}
	for errorRows.Next() {
		var module, msg string
		if err := errorRows.Scan(&module, &msg); err != nil {
			return nil, fmt.Errorf("scan enrichment error: %w", err)
		}
		errors[module] = msg
	}

	// 2) Check data presence in one query
	var peakCount, settlementCount, waterCount, forestCount, borderCount int
	var stopCount, surfaceCount, climbCount, liftCount, intervalCount int
	var hasCommuteClusters, hasGastronomy bool
	err = db.Pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM activity_geo_tags WHERE activity_id = $1 AND category = 'peak'),
			(SELECT count(*) FROM activity_geo_tags WHERE activity_id = $1 AND category = 'settlement'),
			(SELECT count(*) FROM activity_geo_tags WHERE activity_id = $1 AND category IN ('still_water', 'flowing_water')),
			(SELECT count(*) FROM activity_geo_tags WHERE activity_id = $1 AND category = 'forest'),
			(SELECT count(*) FROM activity_geo_tags WHERE activity_id = $1 AND category = 'border'),
			(SELECT count(*) FROM activity_stops WHERE activity_id = $1),
			(SELECT count(*) FROM activity_surfaces WHERE activity_id = $1),
			(SELECT count(*) FROM activity_climbs WHERE activity_id = $1),
			(SELECT count(*) FROM activity_lift_candidates WHERE activity_id = $1),
			(SELECT count(*) FROM activity_intervals WHERE activity_id = $1),
			EXISTS(SELECT 1 FROM commute_clusters WHERE user_id = (SELECT user_id FROM activities WHERE id = $1)),
			(SELECT count(*) FROM activity_geo_tags WHERE activity_id = $1 AND category = 'gastronomy') > 0
	`, activityID).Scan(
		&peakCount, &settlementCount, &waterCount, &forestCount, &borderCount,
		&stopCount, &surfaceCount, &climbCount, &liftCount, &intervalCount,
		&hasCommuteClusters, &hasGastronomy,
	)
	if err != nil {
		return nil, fmt.Errorf("get enrichment data presence: %w", err)
	}

	modules := []struct {
		name    string
		hasData bool
		count   int
	}{
		{"peaks", peakCount > 0, peakCount},
		{"lifts", liftCount > 0, liftCount},
		{"settlements", settlementCount > 0, settlementCount},
		{"commute", hasCommuteClusters, 0},
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
		if errMsg, ok := errors[m.name]; ok {
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

// DeleteActivitySurfaces removes all surface data for an activity (Stage 2 data).
func (db *DB) DeleteActivitySurfaces(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_surfaces WHERE activity_id = $1`, activityID)
	return err
}

// ── Import Settings ──────────────────────────────────────────────────────────

// GetImportSettings lädt die Import-Einstellungen eines Nutzers.
// Bei Fehler oder fehlenden Einstellungen: sichere Defaults zurückgeben.
func (db *DB) GetImportSettings(ctx context.Context, userID string) (*model.ImportSettings, error) {
	var s model.ImportSettings
	err := db.Pool.QueryRow(ctx,
		`SELECT COALESCE(import_conflict_strategy, 'enrich_missing'),
		        COALESCE(gps_duplicate_threshold, 75),
		        COALESCE(fusion_time_window_min, 30)
		 FROM users WHERE id = $1`, userID,
	).Scan(&s.ConflictStrategy, &s.GPSDuplicateThreshold, &s.FusionTimeWindowMin)
	if err != nil {
		return &model.ImportSettings{ConflictStrategy: "enrich_missing", GPSDuplicateThreshold: 75, FusionTimeWindowMin: 30}, nil
	}
	return &s, nil
}

// UpdateImportSettings speichert die Import-Einstellungen eines Nutzers.
func (db *DB) UpdateImportSettings(ctx context.Context, userID string, s *model.ImportSettings) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET import_conflict_strategy = $2, gps_duplicate_threshold = $3, fusion_time_window_min = $4 WHERE id = $1`,
		userID, s.ConflictStrategy, s.GPSDuplicateThreshold, s.FusionTimeWindowMin,
	)
	return err
}

// FindActivitiesByTimeRange findet Aktivitäten in einem Zeitfenster mit passendem Sport.
// Wird für Tier-1 Temporal-Matching im DuplicateDetector verwendet.
// Sport-Filter wird nur angewendet wenn sportType bekannt ist (nicht "unknown"/"generic"/leer),
// damit plattformübergreifende Duplikate erkannt werden (z.B. Garmin "cycling" vs Komoot "unknown").
func (db *DB) FindActivitiesByTimeRange(ctx context.Context, userID string, from, to time.Time, sportType string) ([]model.Activity, error) {
	knownSport := sportType != "" && sportType != "unknown" && sportType != "generic"

	var query string
	var args []any
	if knownSport {
		query = `SELECT id, sport_type, started_at, distance_m,
		                avg_heart_rate, max_heart_rate, avg_power_w, max_power_w,
		                avg_cadence, avg_temperature_c, elevation_gain_m
		         FROM activities
		         WHERE user_id = $1 AND started_at BETWEEN $2 AND $3 AND sport_type = $4
		         LIMIT 5`
		args = []any{userID, from, to, sportType}
	} else {
		query = `SELECT id, sport_type, started_at, distance_m,
		                avg_heart_rate, max_heart_rate, avg_power_w, max_power_w,
		                avg_cadence, avg_temperature_c, elevation_gain_m
		         FROM activities
		         WHERE user_id = $1 AND started_at BETWEEN $2 AND $3
		         LIMIT 5`
		args = []any{userID, from, to}
	}

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Activity
	for rows.Next() {
		var a model.Activity
		if err := rows.Scan(
			&a.ID, &a.SportType, &a.StartedAt, &a.DistanceM,
			&a.AvgHeartRate, &a.MaxHeartRate, &a.AvgPowerW, &a.MaxPowerW,
			&a.AvgCadence, &a.AvgTemperatureC, &a.ElevationGainM,
		); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
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
	ID         string    `json:"id"`
	ActivityID string    `json:"activity_id"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationS  int       `json:"duration_s"`
	AscentM    float64   `json:"ascent_m"`
	DescentM   *float64  `json:"descent_m,omitempty"`
	DistanceM  *float64  `json:"distance_m,omitempty"`
	Confidence string    `json:"confidence"`
	Confirmed  bool      `json:"confirmed"`
}

func (db *DB) GetLiftCandidates(ctx context.Context, activityID string) ([]LiftCandidate, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed
		FROM activity_lift_candidates
		WHERE activity_id = $1
		ORDER BY started_at
	`, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []LiftCandidate
	for rows.Next() {
		var c LiftCandidate
		if err := rows.Scan(&c.ID, &c.ActivityID, &c.StartedAt, &c.EndedAt,
			&c.DurationS, &c.AscentM, &c.DescentM, &c.DistanceM, &c.Confidence, &c.Confirmed); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (db *DB) ConfirmLiftCandidate(ctx context.Context, candidateID string) error {
	var activityID string
	err := db.Pool.QueryRow(ctx,
		`UPDATE activity_lift_candidates SET confirmed = true WHERE id = $1 RETURNING activity_id`,
		candidateID,
	).Scan(&activityID)
	if err != nil {
		return err
	}
	return db.recomputeLiftStats(ctx, activityID)
}

// RejectLiftCandidate deletes a lift candidate and recomputes activity lift stats.
func (db *DB) RejectLiftCandidate(ctx context.Context, candidateID string) error {
	var activityID string
	err := db.Pool.QueryRow(ctx,
		`DELETE FROM activity_lift_candidates WHERE id = $1 RETURNING activity_id`,
		candidateID,
	).Scan(&activityID)
	if err != nil {
		return err
	}
	return db.recomputeLiftStats(ctx, activityID)
}

// recomputeLiftStats recomputes activities.lift_* columns from both sources:
//   - segment_efforts with segment_type='lift' (Method A: OSM-matched, auto-confirmed)
//   - activity_lift_candidates where confirmed=true (Method B: heuristic, user-confirmed)
func (db *DB) recomputeLiftStats(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE activities SET
			lift_count = (
				SELECT COUNT(*) FROM segment_efforts se
				JOIN segments s ON s.id = se.segment_id
				WHERE se.activity_id = $1 AND s.segment_type = 'lift'
			) + (
				SELECT COUNT(*) FROM activity_lift_candidates
				WHERE activity_id = $1 AND confirmed = true
			),
			lift_duration_s = (
				SELECT COALESCE(SUM(se.elapsed_time_s), 0) FROM segment_efforts se
				JOIN segments s ON s.id = se.segment_id
				WHERE se.activity_id = $1 AND s.segment_type = 'lift'
			) + (
				SELECT COALESCE(SUM(duration_s), 0) FROM activity_lift_candidates
				WHERE activity_id = $1 AND confirmed = true
			),
			lift_ascent_m = (
				SELECT COALESCE(SUM(CASE WHEN NOT se.is_reverse THEN se.elevation_gain_m ELSE 0 END), 0)
				FROM segment_efforts se
				JOIN segments s ON s.id = se.segment_id
				WHERE se.activity_id = $1 AND s.segment_type = 'lift'
			) + (
				SELECT COALESCE(SUM(ascent_m), 0) FROM activity_lift_candidates
				WHERE activity_id = $1 AND confirmed = true
			),
			lift_descent_m = (
				SELECT COALESCE(SUM(CASE WHEN se.is_reverse THEN se.elevation_gain_m ELSE 0 END), 0)
				FROM segment_efforts se
				JOIN segments s ON s.id = se.segment_id
				WHERE se.activity_id = $1 AND s.segment_type = 'lift'
			) + (
				SELECT COALESCE(SUM(descent_m), 0) FROM activity_lift_candidates
				WHERE activity_id = $1 AND confirmed = true
			),
			lift_distance_m = (
				SELECT COALESCE(SUM(se.distance_m), 0) FROM segment_efforts se
				JOIN segments s ON s.id = se.segment_id
				WHERE se.activity_id = $1 AND s.segment_type = 'lift'
			) + (
				SELECT COALESCE(SUM(distance_m), 0) FROM activity_lift_candidates
				WHERE activity_id = $1 AND confirmed = true
			),
			updated_at = NOW()
		WHERE id = $1`, activityID)
	return err
}

// GetTagIDsByNamesAndCategory returns a map of tag names to tag IDs
// for tags matching the given user, category, and names (case-insensitive).
func (db *DB) GetTagIDsByNamesAndCategory(ctx context.Context, userID, category string, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return make(map[string]string), nil
	}

	rows, err := db.Pool.Query(ctx,
		`SELECT id, name FROM tags WHERE user_id = $1 AND category = $2 AND name ILIKE ANY($3)`,
		userID, category, names)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		result[name] = id
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return result, nil
}

// AssignActivitiesToTag links multiple activities to a tag (manual assignment).
// Inserts into activity_tags junction table and syncs activities.tags TEXT[] cache.
func (db *DB) AssignActivitiesToTag(ctx context.Context, tagID, userID string, activityIDs []string) error {
	if len(activityIDs) == 0 {
		return nil
	}
	// Resolve tag name for updating the denormalized TEXT[] cache.
	var tagName string
	if err := db.Pool.QueryRow(ctx,
		`SELECT name FROM tags WHERE id = $1 AND user_id = $2`, tagID, userID,
	).Scan(&tagName); err != nil {
		return fmt.Errorf("assign activities to tag: resolve tag name: %w", err)
	}
	for _, actID := range activityIDs {
		if err := db.LinkActivityTag(ctx, actID, tagID, "manual"); err != nil {
			return fmt.Errorf("assign activities to tag: link %s: %w", actID, err)
		}
		// Keep denormalized TEXT[] cache in sync.
		// COALESCE guards against NULL tags array: NOT ($2 = ANY(NULL)) → NULL → FALSE, skips update.
		_, _ = db.Pool.Exec(ctx,
			`UPDATE activities SET tags = array_append(COALESCE(tags, '{}'), $2), updated_at = NOW()
			 WHERE id = $1 AND user_id = $3 AND NOT ($2 = ANY(COALESCE(tags, '{}')))`,
			actID, tagName, userID)
	}
	return nil
}

// UnassignActivitiesFromTag removes multiple activities from a tag.
// Deletes from activity_tags junction table and syncs activities.tags TEXT[] cache.
func (db *DB) UnassignActivitiesFromTag(ctx context.Context, tagID, userID string, activityIDs []string) error {
	if len(activityIDs) == 0 {
		return nil
	}
	// Resolve tag name for updating the denormalized TEXT[] cache.
	var tagName string
	if err := db.Pool.QueryRow(ctx,
		`SELECT name FROM tags WHERE id = $1 AND user_id = $2`, tagID, userID,
	).Scan(&tagName); err != nil {
		return fmt.Errorf("unassign activities from tag: resolve tag name: %w", err)
	}
	for _, actID := range activityIDs {
		_, err := db.Pool.Exec(ctx,
			`DELETE FROM activity_tags WHERE activity_id = $1 AND tag_id = $2`,
			actID, tagID)
		if err != nil {
			return fmt.Errorf("unassign activities from tag: delete %s: %w", actID, err)
		}
		// Keep denormalized TEXT[] cache in sync.
		_, _ = db.Pool.Exec(ctx,
			`UPDATE activities SET tags = array_remove(tags, $2), updated_at = NOW()
			 WHERE id = $1 AND user_id = $3`,
			actID, tagName, userID)
	}

	// Update activity_count from junction table
	_, err := db.Pool.Exec(ctx,
		`UPDATE tags
		 SET activity_count = (SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = $1)
		 WHERE id = $1`,
		tagID)
	if err != nil {
		return fmt.Errorf("unassign activities from tag: update count: %w", err)
	}
	return nil
}

// LinkActivityTag links a tag to an activity with the given assignment type.
// If the link already exists, it's silently ignored (ON CONFLICT DO NOTHING).
// Also updates the tag's activity_count denormalized field.
func (db *DB) LinkActivityTag(ctx context.Context, activityID, tagID, assignedBy string) error {
	// Insert link with ON CONFLICT DO NOTHING
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES ($1, $2, $3)
		 ON CONFLICT (activity_id, tag_id) DO NOTHING`,
		activityID, tagID, assignedBy)
	if err != nil {
		return fmt.Errorf("link activity tag: %w", err)
	}

	// Update activity_count from junction table
	_, err = db.Pool.Exec(ctx,
		`UPDATE tags
		 SET activity_count = (SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = $1)
		 WHERE id = $1`,
		tagID)
	if err != nil {
		return fmt.Errorf("link activity tag: update count: %w", err)
	}
	return nil
}

// RecordTagRemoval records that a user explicitly removed a system tag from an activity.
// This prevents enrichment from re-adding the tag later.
func (db *DB) RecordTagRemoval(ctx context.Context, activityID, tagID string) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO activity_tag_removals (activity_id, tag_id)
		 VALUES ($1, $2)
		 ON CONFLICT (activity_id, tag_id) DO NOTHING`,
		activityID, tagID)
	return err
}

// ClearTagRemoval deletes a removal record, allowing enrichment to re-add the tag.
// Called when a user manually re-assigns a system tag to an activity.
func (db *DB) ClearTagRemoval(ctx context.Context, activityID, tagID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM activity_tag_removals WHERE activity_id = $1 AND tag_id = $2`,
		activityID, tagID)
	return err
}

// IsTagRemovalRecorded checks whether a user has explicitly removed a system tag from an activity.
func (db *DB) IsTagRemovalRecorded(ctx context.Context, activityID, tagID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM activity_tag_removals WHERE activity_id = $1 AND tag_id = $2)`,
		activityID, tagID).Scan(&exists)
	return exists, err
}

// contrastTextColor returns a readable text color (#1a1a1a or #ffffff) for a given
// hex background color using the YIQ luminance formula, matching the frontend logic.
func contrastTextColor(hex string) string {
	if len(hex) != 7 || hex[0] != '#' {
		return "#ffffff"
	}
	r, errR := strconv.ParseInt(hex[1:3], 16, 32)
	g, errG := strconv.ParseInt(hex[3:5], 16, 32)
	b, errB := strconv.ParseInt(hex[5:7], 16, 32)
	if errR != nil || errG != nil || errB != nil {
		return "#ffffff"
	}
	luminance := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 255
	if luminance > 0.5 {
		return "#1a1a1a"
	}
	return "#ffffff"
}
