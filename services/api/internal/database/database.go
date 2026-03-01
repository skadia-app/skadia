package database

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

type DB struct {
	Pool *pgxpool.Pool
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

func (db *DB) InsertActivity(ctx context.Context, userID string, data *model.ActivityData) (*model.Activity, error) {
	var id string
	var createdAt, updatedAt time.Time
	var isEbike bool

	err := db.Pool.QueryRow(ctx, `
		INSERT INTO activities (
			user_id, file_hash, sport_type, sub_type,
			fit_sport_id, fit_sub_sport_id,
			started_at, ended_at, elapsed_time_s, moving_time_s,
			distance_m, elevation_gain_m, elevation_loss_m,
			max_elevation_m, min_elevation_m,
			avg_speed_ms, max_speed_ms,
			avg_heart_rate, max_heart_rate,
			avg_power_w, max_power_w, normalized_power_w,
			avg_cadence, avg_temperature_c,
			tss, trimp, is_indoor,
			start_location, end_location,
			original_filename, original_file_format,
			source, source_device
		) VALUES (
			$1, $2, $3, $4,
			$5, $6,
			$7, $8, $9, $10,
			$11, $12, $13,
			$14, $15,
			$16, $17,
			$18, $19,
			$20, $21, $22,
			$23, $24,
			$25, $26, $27,
			CASE WHEN $28::float8 != 0 THEN ST_SetSRID(ST_MakePoint($29::float8, $28::float8), 4326) ELSE NULL END,
			CASE WHEN $30::float8 != 0 THEN ST_SetSRID(ST_MakePoint($31::float8, $30::float8), 4326) ELSE NULL END,
			$32, $33,
			$34, $35
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
		nilIfZeroF64(data.TSS), nilIfZeroF64(data.TRIMP), data.IsIndoor,
		data.StartLat, data.StartLng,
		data.EndLat, data.EndLng,
		nilIfEmpty(data.OriginalName), nilIfEmpty(data.FileFormat),
		nilIfEmpty(data.Source), nilIfEmpty(data.SourceDevice),
	).Scan(&id, &isEbike, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert activity: %w", err)
	}

	return &model.Activity{
		ID:                  id,
		UserID:              userID,
		SportType:           data.SportType,
		SubType:             ptrString(data.SubType),
		Source:              ptrString(data.Source),
		SourceDevice:        ptrString(data.SourceDevice),
		StartedAt:           data.StartedAt,
		IsEbike:             isEbike,
		IsIndoor:            data.IsIndoor,
		HasGPS:              data.HasGPS,
		NeedsClassification: data.SportType == "unknown",
		OriginalFilename:    ptrString(data.OriginalName),
		Visibility:          "private",
		EnrichmentStatus:    "pending",
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
	}, nil
}

func (db *DB) InsertStreams(ctx context.Context, activityID string, streams []model.StreamPoint) error {
	if len(streams) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, s := range streams {
		batch.Queue(`
			INSERT INTO activity_streams (time, activity_id, lat, lng, altitude_m, speed_ms, distance_m, heart_rate, power_w, cadence, temperature_c)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			s.Time, activityID,
			s.Lat, s.Lng, s.AltitudeM, s.SpeedMS, s.DistanceM,
			s.HeartRate, s.PowerW, s.Cadence, s.TemperatureC,
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

func (db *DB) InsertTrack(ctx context.Context, activityID string, trackPoints []model.TrackPoint) error {
	if len(trackPoints) < 2 {
		return nil
	}

	wkt := trackToWKT(trackPoints)

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_tracks (activity_id, track_full, track_simplified, bbox, point_count)
		VALUES ($1,
			ST_SetSRID(ST_GeomFromText($2), 4326),
			ST_Simplify(ST_SetSRID(ST_GeomFromText($2), 4326), 0.0001),
			ST_Envelope(ST_SetSRID(ST_GeomFromText($2), 4326)),
			$3
		)`,
		activityID, wkt, len(trackPoints),
	)
	if err != nil {
		return fmt.Errorf("insert track: %w", err)
	}
	return nil
}

func trackToWKT(points []model.TrackPoint) string {
	parts := make([]string, len(points))
	for i, p := range points {
		parts[i] = fmt.Sprintf("%f %f", p.Lng, p.Lat) // WKT: longitude first
	}
	return "LINESTRING(" + strings.Join(parts, ",") + ")"
}

func (db *DB) InsertSensors(ctx context.Context, activityID string, sensors []model.SensorRecord) error {
	if len(sensors) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, s := range sensors {
		batch.Queue(`
			INSERT INTO activity_sensors (
				activity_id, device_index, serial_number, source_type,
				antplus_device_type, all_device_types,
				manufacturer, manufacturer_id, garmin_product, garmin_product_id,
				product_name, software_version, hardware_version,
				battery_status, battery_voltage
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
			activityID, s.DeviceIndex, s.SerialNumber, s.SourceType,
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
			a.tss, a.trimp, a.is_indoor,
			a.source, a.source_device,
			a.tags, a.visibility, a.enrichment_status, a.original_filename,
			a.created_at, a.updated_at,
			COALESCE(t.point_count, 0) > 0
		FROM activities a
		LEFT JOIN activity_tracks t ON t.activity_id = a.id
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
		&a.TSS, &a.TRIMP, &a.IsIndoor,
		&a.Source, &a.SourceDevice,
		&a.Tags, &a.Visibility, &a.EnrichmentStatus, &a.OriginalFilename,
		&a.CreatedAt, &a.UpdatedAt,
		&hasTrack,
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
	return &a, nil
}

func (db *DB) ListActivities(ctx context.Context, userID string, cursor string, limit int) (*model.ActivityList, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var cursorTime time.Time
	var cursorID string
	hasCursor := false

	if cursor != "" {
		decoded, err := base64.URLEncoding.DecodeString(cursor)
		if err == nil {
			parts := strings.SplitN(string(decoded), "|", 2)
			if len(parts) == 2 {
				t, err := time.Parse(time.RFC3339Nano, parts[0])
				if err == nil {
					cursorTime = t
					cursorID = parts[1]
					hasCursor = true
				}
			}
		}
	}

	var rows pgx.Rows
	var err error

	if hasCursor {
		rows, err = db.Pool.Query(ctx, `
			SELECT id, sport_type, sub_type, title, started_at, distance_m,
				elapsed_time_s, elevation_gain_m, is_indoor, is_ebike, visibility,
				created_at, updated_at
			FROM activities
			WHERE user_id = $1
				AND (started_at, id) < ($2, $3)
			ORDER BY started_at DESC, id DESC
			LIMIT $4`,
			userID, cursorTime, cursorID, limit+1,
		)
	} else {
		rows, err = db.Pool.Query(ctx, `
			SELECT id, sport_type, sub_type, title, started_at, distance_m,
				elapsed_time_s, elevation_gain_m, is_indoor, is_ebike, visibility,
				created_at, updated_at
			FROM activities
			WHERE user_id = $1
			ORDER BY started_at DESC, id DESC
			LIMIT $2`,
			userID, limit+1,
		)
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
			&a.ElapsedTimeS, &a.ElevationGainM, &a.IsIndoor, &a.IsEbike, &a.Visibility,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		a.Tags = []string{}
		a.NeedsClassification = a.SportType == "unknown"
		activities = append(activities, a)
	}

	result := &model.ActivityList{
		Activities: activities,
	}

	if len(activities) > limit {
		result.Activities = activities[:limit]
		last := activities[limit-1]
		cursorStr := last.StartedAt.Format(time.RFC3339Nano) + "|" + last.ID
		result.NextCursor = base64.URLEncoding.EncodeToString([]byte(cursorStr))
	}

	if result.Activities == nil {
		result.Activities = []model.Activity{}
	}

	return result, nil
}

func (db *DB) UpdateActivity(ctx context.Context, id string, update *model.ActivityUpdate) (*model.Activity, error) {
	var sets []string
	var args []any
	argIdx := 2
	args = append(args, id)

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

	if len(sets) == 0 {
		return db.GetActivity(ctx, id)
	}

	sets = append(sets, "updated_at = NOW()")

	query := fmt.Sprintf("UPDATE activities SET %s WHERE id = $1", strings.Join(sets, ", "))
	_, err := db.Pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update activity: %w", err)
	}

	return db.GetActivity(ctx, id)
}

func (db *DB) DeleteActivity(ctx context.Context, id string) error {
	result, err := db.Pool.Exec(ctx, `DELETE FROM activities WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete activity: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("activity not found")
	}
	return nil
}

func (db *DB) GetStreams(ctx context.Context, activityID string) ([]model.StreamResponse, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT time, lat, lng, altitude_m, speed_ms, distance_m,
			heart_rate, power_w, cadence, temperature_c
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
		); err != nil {
			return nil, fmt.Errorf("scan stream: %w", err)
		}
		streams = append(streams, s)
	}
	if streams == nil {
		streams = []model.StreamResponse{}
	}
	return streams, nil
}

func (db *DB) GetSensors(ctx context.Context, activityID string) ([]model.SensorResponse, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, device_index, source_type, antplus_device_type, all_device_types,
			manufacturer, product_name, serial_number, software_version, battery_status
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
		var primaryType *string
		var allTypes []string
		var swVersion *float64

		if err := rows.Scan(
			&s.ID, &s.DeviceIndex, &s.SourceType, &primaryType, &allTypes,
			&s.Manufacturer, &s.ProductName, &s.SerialNumber, &swVersion, &s.BatteryStatus,
		); err != nil {
			return nil, fmt.Errorf("scan sensor: %w", err)
		}

		if primaryType != nil {
			s.PrimaryType = *primaryType
			s.PrimaryTypeLabel = sensorTypeLabel(s.PrimaryType)
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
	if sensors == nil {
		sensors = []model.SensorResponse{}
	}
	return sensors, nil
}

var sensorLabels = map[string]string{
	"heart_rate":             "Herzfrequenzsensor",
	"bike_speed":             "Geschwindigkeitssensor",
	"bike_cadence":           "Trittfrequenzsensor",
	"bike_speed_cadence":     "Geschwindigkeits-/Trittfrequenzsensor",
	"bike_power":             "Leistungsmesser",
	"stride_speed_distance":  "Laufpod / Running Dynamics",
	"radar":                  "Rückwärtsradar",
	"shift":                  "Schaltung (Di2 / eTap)",
	"environment_sensor_legacy": "Temperatursensor",
	"multi_sport_speed_distance": "Multi-Sport Sensor",
}

func sensorTypeLabel(deviceType string) string {
	if label, ok := sensorLabels[deviceType]; ok {
		return label
	}
	return deviceType
}

// Helper functions

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
