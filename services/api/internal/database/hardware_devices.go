package database

import (
	"errors"
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// hwDeviceSelectCols are the columns returned by hardware device queries (without activity_count).
const hwDeviceSelectCols = `d.id, d.user_id, d.type, d.manufacturer, d.manufacturer_id,
	d.product_name, d.product_id, d.serial_number, d.ant_device_number,
	d.custom_name, d.device_types, d.source_type,
	d.first_seen_at, d.last_seen_at, d.created_at`

// DeviceActivitySummary is a compact activity summary returned by ListDeviceActivities.
type DeviceActivitySummary struct {
	ID        string    `json:"id"`
	StartedAt time.Time `json:"started_at"`
	Title     *string   `json:"title,omitempty"`
	SportType string    `json:"sport_type"`
	DistanceM *float64  `json:"distance_m,omitempty"`
}

// scanHWDevice scans a hardware device row (activity_count is expected as the last column).
func scanHWDevice(row pgx.Row) (*model.Device, error) {
	var d model.Device
	err := row.Scan(
		&d.ID, &d.UserID, &d.Type, &d.Manufacturer, &d.ManufacturerID,
		&d.ProductName, &d.ProductID, &d.SerialNumber, &d.AntDeviceNumber,
		&d.CustomName, &d.DeviceTypes, &d.SourceType,
		&d.FirstSeenAt, &d.LastSeenAt, &d.CreatedAt,
		&d.ActivityCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if d.DeviceTypes == nil {
		d.DeviceTypes = []string{}
	}
	return &d, nil
}

// UpsertHWDevice performs a SELECT-first upsert of a hardware device record.
// Lookup priority: serial_number → ant_device_number → product_name fallback.
// Returns the device ID.
func (db *DB) UpsertHWDevice(ctx context.Context, d model.DeviceUpsert) (string, error) {
	var existingID string

	seenAt := d.SeenAt
	if seenAt.IsZero() {
		seenAt = time.Now()
	}

	// 1. Try to find existing device.
	if d.SerialNumber != nil {
		// Garmin-Familie (garmin/dynastream/dynastream_oem/tacx/ManufacturerInvalid) matcht
		// seriennummern-übergreifend, da dasselbe Gerät verschiedene manufacturer_id-Werte
		// melden kann. Andere Hersteller (COROS, Apple etc.) erfordern exakten manufacturer_id-Match,
		// damit verschiedene Hersteller mit gleicher Serial nicht kollisionieren.
		garminFamily := []int{1, 13, 14, 89, 65535}
		isGarminFamily := false
		for _, gm := range garminFamily {
			if d.ManufacturerID == gm {
				isGarminFamily = true
				break
			}
		}

		if isGarminFamily {
			// Garmin family: match serial across garmin/dynastream/invalid manufacturer IDs
			err := db.Pool.QueryRow(ctx,
				`SELECT id FROM devices
				 WHERE user_id = $1 AND type = $2 AND serial_number = $3
				   AND COALESCE(manufacturer_id, 0) IN (0, 1, 13, 14, 89, 65535)`,
				d.UserID, d.Type, *d.SerialNumber,
			).Scan(&existingID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return "", fmt.Errorf("upsert hw device (garmin serial lookup): %w", err)
			}
		} else {
			// Non-Garmin: require exact manufacturer_id match to prevent cross-manufacturer collisions
			err := db.Pool.QueryRow(ctx,
				`SELECT id FROM devices
				 WHERE user_id = $1 AND type = $2 AND serial_number = $3
				   AND manufacturer_id = $4`,
				d.UserID, d.Type, *d.SerialNumber, d.ManufacturerID,
			).Scan(&existingID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return "", fmt.Errorf("upsert hw device (serial lookup): %w", err)
			}
		}
	} else if d.AntDeviceNumber != nil {
		err := db.Pool.QueryRow(ctx,
			`SELECT id FROM devices
			 WHERE user_id = $1 AND type = $2 AND manufacturer_id = $3
			   AND ant_device_number = $4 AND serial_number IS NULL`,
			d.UserID, d.Type, d.ManufacturerID, *d.AntDeviceNumber,
		).Scan(&existingID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("upsert hw device (ant lookup): %w", err)
		}
	} else {
		err := db.Pool.QueryRow(ctx,
			`SELECT id FROM devices
			 WHERE user_id = $1 AND type = $2
			   AND COALESCE(manufacturer_id, 0) = COALESCE($3, 0)
			   AND product_name = $4
			   AND serial_number IS NULL AND ant_device_number IS NULL`,
			d.UserID, d.Type, d.ManufacturerID, d.ProductName,
		).Scan(&existingID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("upsert hw device (name lookup): %w", err)
		}
	}

	if existingID != "" {
		// 2. Update last_seen_at and optionally product_name/device_types/source_type.
		_, err := db.Pool.Exec(ctx,
			`UPDATE devices SET
				last_seen_at  = GREATEST(last_seen_at, $2),
				first_seen_at = LEAST(first_seen_at, $2),
				product_name  = CASE
				WHEN NULLIF($3, '') IS NULL THEN product_name
				WHEN $3 NOT LIKE 'GarminProduct%' THEN $3
				WHEN product_name IS NULL OR product_name LIKE 'GarminProduct%' THEN $3
				ELSE product_name
				END,
				device_types  = CASE WHEN array_length($4::text[], 1) > 0 THEN $4 ELSE device_types END,
				source_type   = COALESCE(NULLIF($5, ''), source_type)
			 WHERE id = $1`,
			existingID, seenAt, d.ProductName, d.DeviceTypes, d.SourceType,
		)
		if err != nil {
			return "", fmt.Errorf("upsert hw device (update): %w", err)
		}
		return existingID, nil
	}

	// 3. INSERT — partial index ON CONFLICT guards against race conditions.
	// PostgreSQL partial unique indexes require ON CONFLICT (cols) WHERE predicate form.
	var newID string
	if d.SerialNumber != nil {
		// idx_devices_serial: (user_id, type, serial_number) WHERE serial_number IS NOT NULL
		err := db.Pool.QueryRow(ctx,
			`INSERT INTO devices (
				user_id, type, manufacturer, manufacturer_id, product_name, product_id,
				serial_number, ant_device_number, device_types, source_type,
				first_seen_at, last_seen_at
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)
			 ON CONFLICT (user_id, type, serial_number) WHERE serial_number IS NOT NULL
			 DO UPDATE SET
				last_seen_at  = GREATEST(devices.last_seen_at, EXCLUDED.last_seen_at),
				first_seen_at = LEAST(devices.first_seen_at, EXCLUDED.first_seen_at),
				product_name  = CASE
					WHEN NULLIF(EXCLUDED.product_name, '') IS NULL THEN devices.product_name
					WHEN EXCLUDED.product_name NOT LIKE 'GarminProduct%' THEN EXCLUDED.product_name
					WHEN devices.product_name IS NULL OR devices.product_name LIKE 'GarminProduct%' THEN EXCLUDED.product_name
					ELSE devices.product_name
					END,
				device_types  = CASE WHEN array_length(EXCLUDED.device_types, 1) > 0 THEN EXCLUDED.device_types ELSE devices.device_types END,
				source_type   = COALESCE(NULLIF(EXCLUDED.source_type, ''), devices.source_type)
			 RETURNING id`,
			d.UserID, d.Type, d.Manufacturer, d.ManufacturerID, d.ProductName, d.ProductID,
			*d.SerialNumber, d.AntDeviceNumber, d.DeviceTypes, d.SourceType, seenAt,
		).Scan(&newID)
		if err != nil {
			return "", fmt.Errorf("upsert hw device (insert serial): %w", err)
		}
	} else if d.AntDeviceNumber != nil {
		// idx_devices_ant: (user_id, type, manufacturer_id, ant_device_number) WHERE ant_device_number IS NOT NULL AND serial_number IS NULL
		err := db.Pool.QueryRow(ctx,
			`INSERT INTO devices (
				user_id, type, manufacturer, manufacturer_id, product_name, product_id,
				serial_number, ant_device_number, device_types, source_type,
				first_seen_at, last_seen_at
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)
			 ON CONFLICT (user_id, type, manufacturer_id, ant_device_number) WHERE ant_device_number IS NOT NULL AND serial_number IS NULL
			 DO UPDATE SET
				last_seen_at  = GREATEST(devices.last_seen_at, EXCLUDED.last_seen_at),
				first_seen_at = LEAST(devices.first_seen_at, EXCLUDED.first_seen_at),
				product_name  = CASE
					WHEN NULLIF(EXCLUDED.product_name, '') IS NULL THEN devices.product_name
					WHEN EXCLUDED.product_name NOT LIKE 'GarminProduct%' THEN EXCLUDED.product_name
					WHEN devices.product_name IS NULL OR devices.product_name LIKE 'GarminProduct%' THEN EXCLUDED.product_name
					ELSE devices.product_name
					END,
				device_types  = CASE WHEN array_length(EXCLUDED.device_types, 1) > 0 THEN EXCLUDED.device_types ELSE devices.device_types END,
				source_type   = COALESCE(NULLIF(EXCLUDED.source_type, ''), devices.source_type)
			 RETURNING id`,
			d.UserID, d.Type, d.Manufacturer, d.ManufacturerID, d.ProductName, d.ProductID,
			nil, *d.AntDeviceNumber, d.DeviceTypes, d.SourceType, seenAt,
		).Scan(&newID)
		if err != nil {
			return "", fmt.Errorf("upsert hw device (insert ant): %w", err)
		}
	} else {
		// idx_devices_name: (user_id, type, COALESCE(manufacturer_id,0), product_name) WHERE serial_number IS NULL AND ant_device_number IS NULL
		// Note: COALESCE in index requires passing the expression — use DO NOTHING to avoid re-insert races.
		// For the DO UPDATE case we handle duplicates as DO NOTHING + re-read by product_name.
		// Simpler: plain INSERT, ignore duplicate, re-read on conflict.
		var insertErr error
		insertErr = db.Pool.QueryRow(ctx,
			`INSERT INTO devices (
				user_id, type, manufacturer, manufacturer_id, product_name, product_id,
				serial_number, ant_device_number, device_types, source_type,
				first_seen_at, last_seen_at
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)
			 ON CONFLICT (user_id, type, COALESCE(manufacturer_id, 0), product_name) WHERE serial_number IS NULL AND ant_device_number IS NULL
			 DO UPDATE SET
				last_seen_at  = GREATEST(devices.last_seen_at, EXCLUDED.last_seen_at),
				first_seen_at = LEAST(devices.first_seen_at, EXCLUDED.first_seen_at),
				device_types  = CASE WHEN array_length(EXCLUDED.device_types, 1) > 0 THEN EXCLUDED.device_types ELSE devices.device_types END,
				source_type   = COALESCE(NULLIF(EXCLUDED.source_type, ''), devices.source_type)
			 RETURNING id`,
			d.UserID, d.Type, d.Manufacturer, d.ManufacturerID, d.ProductName, d.ProductID,
			nil, nil, d.DeviceTypes, d.SourceType, seenAt,
		).Scan(&newID)
		if insertErr != nil {
			return "", fmt.Errorf("upsert hw device (insert name): %w", insertErr)
		}
	}
	return newID, nil
}

// GetHWDevice returns a single hardware device by ID, including activity_count.
// Returns nil, nil if not found.
func (db *DB) GetHWDevice(ctx context.Context, id string) (*model.Device, error) {
	d, err := scanHWDevice(db.Pool.QueryRow(ctx,
		`SELECT `+hwDeviceSelectCols+`,
			CASE d.type
				WHEN 'recorder' THEN (SELECT COUNT(*) FROM activities WHERE recording_device_id = d.id)
				WHEN 'sensor'   THEN (SELECT COUNT(DISTINCT activity_id) FROM activity_sensors WHERE device_id = d.id)
				ELSE 0
			END AS activity_count
		 FROM devices d
		 WHERE d.id = $1`, id,
	))
	if err != nil {
		return nil, fmt.Errorf("get hw device: %w", err)
	}
	return d, nil
}

// ListHWDevices returns all hardware devices for a user, optionally filtered by type.
// Uses aggregated CTEs for performance. Results are ordered by last_seen_at DESC.
func (db *DB) ListHWDevices(ctx context.Context, userID, deviceType string) ([]model.Device, error) {
	query := `
		WITH recorder_counts AS (
			SELECT recording_device_id AS device_id, COUNT(*) AS c
			FROM activities WHERE recording_device_id IS NOT NULL
			GROUP BY recording_device_id
		),
		sensor_counts AS (
			SELECT device_id, COUNT(DISTINCT activity_id) AS c
			FROM activity_sensors WHERE device_id IS NOT NULL
			GROUP BY device_id
		)
		SELECT ` + hwDeviceSelectCols + `,
			CASE d.type
				WHEN 'recorder' THEN COALESCE(rc.c, 0)
				WHEN 'sensor'   THEN COALESCE(sc.c, 0)
				ELSE 0
			END AS activity_count
		FROM devices d
		LEFT JOIN recorder_counts rc ON rc.device_id = d.id
		LEFT JOIN sensor_counts   sc ON sc.device_id = d.id
		WHERE d.user_id = $1`

	var args []any
	args = append(args, userID)

	if deviceType != "" {
		query += ` AND d.type = $2`
		args = append(args, deviceType)
	}
	query += ` ORDER BY d.last_seen_at DESC`

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list hw devices: %w", err)
	}
	defer rows.Close()

	var items []model.Device
	for rows.Next() {
		var d model.Device
		if err := rows.Scan(
			&d.ID, &d.UserID, &d.Type, &d.Manufacturer, &d.ManufacturerID,
			&d.ProductName, &d.ProductID, &d.SerialNumber, &d.AntDeviceNumber,
			&d.CustomName, &d.DeviceTypes, &d.SourceType,
			&d.FirstSeenAt, &d.LastSeenAt, &d.CreatedAt,
			&d.ActivityCount,
		); err != nil {
			return nil, fmt.Errorf("scan hw device: %w", err)
		}
		if d.DeviceTypes == nil {
			d.DeviceTypes = []string{}
		}
		items = append(items, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list hw devices: %w", err)
	}
	if items == nil {
		items = []model.Device{}
	}
	return items, nil
}

// UpdateHWDeviceCustomName sets a custom display name for a hardware device.
// Passing an empty string clears the custom name.
// Returns the updated device (with activity_count), or nil if not found.
func (db *DB) UpdateHWDeviceCustomName(ctx context.Context, id, customName string) (*model.Device, error) {
	var customNameArg *string
	if customName != "" {
		customNameArg = &customName
	}
	tag, err := db.Pool.Exec(ctx,
		`UPDATE devices SET custom_name = $1 WHERE id = $2`,
		customNameArg, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update hw device custom name: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}
	return db.GetHWDevice(ctx, id)
}

// MergeHWDevices merges sourceID into targetID within a transaction.
// Both devices must belong to userID. All activity and sensor references are
// redirected to the target, first_seen_at is updated to the earliest value,
// and the source device is deleted.
func (db *DB) MergeHWDevices(ctx context.Context, userID, sourceID, targetID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Ownership check — both devices must belong to the user.
	var count int
	if err = tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM devices WHERE id = ANY($1::uuid[]) AND user_id = $2`,
		[]string{sourceID, targetID}, userID,
	).Scan(&count); err != nil {
		return fmt.Errorf("merge hw devices (ownership check): %w", err)
	}
	if count != 2 {
		return fmt.Errorf("merge hw devices: one or both devices not found or not owned by user")
	}

	// 2. Reassign sensor records.
	if _, err = tx.Exec(ctx,
		`UPDATE activity_sensors SET device_id = $1 WHERE device_id = $2`,
		targetID, sourceID,
	); err != nil {
		return fmt.Errorf("merge hw devices (reassign sensors): %w", err)
	}

	// 3. Reassign recording device references.
	if _, err = tx.Exec(ctx,
		`UPDATE activities SET recording_device_id = $1 WHERE recording_device_id = $2`,
		targetID, sourceID,
	); err != nil {
		return fmt.Errorf("merge hw devices (reassign activities): %w", err)
	}

	// 4. Update target's first_seen_at to the earliest of the two.
	if _, err = tx.Exec(ctx,
		`UPDATE devices SET first_seen_at = LEAST(first_seen_at,
			(SELECT first_seen_at FROM devices WHERE id = $2))
		 WHERE id = $1`,
		targetID, sourceID,
	); err != nil {
		return fmt.Errorf("merge hw devices (update first_seen_at): %w", err)
	}

	// 5. Delete source device.
	if _, err = tx.Exec(ctx,
		`DELETE FROM devices WHERE id = $1`,
		sourceID,
	); err != nil {
		return fmt.Errorf("merge hw devices (delete source): %w", err)
	}

	return tx.Commit(ctx)
}

// ListDeviceActivities returns paginated activities for a hardware device.
// For recorder devices it queries activities.recording_device_id;
// for sensor devices it queries via activity_sensors.device_id.
// afterCursor is an ISO-8601 / RFC3339 timestamp for keyset pagination (started_at <).
func (db *DB) ListDeviceActivities(ctx context.Context, deviceID string, limit int, afterCursor string) ([]DeviceActivitySummary, error) {
	if limit <= 0 {
		limit = 20
	}

	// Determine device type.
	var deviceType string
	err := db.Pool.QueryRow(ctx, `SELECT type FROM devices WHERE id = $1`, deviceID).Scan(&deviceType)
	if errors.Is(err, pgx.ErrNoRows) {
		return []DeviceActivitySummary{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list device activities (type lookup): %w", err)
	}

	var rows pgx.Rows

	if deviceType == "recorder" {
		if afterCursor != "" {
			rows, err = db.Pool.Query(ctx,
				`SELECT id::text, started_at, title, sport_type, distance_m
				 FROM activities
				 WHERE recording_device_id = $1 AND started_at < $2::timestamptz
				 ORDER BY started_at DESC LIMIT $3`,
				deviceID, afterCursor, limit,
			)
		} else {
			rows, err = db.Pool.Query(ctx,
				`SELECT id::text, started_at, title, sport_type, distance_m
				 FROM activities
				 WHERE recording_device_id = $1
				 ORDER BY started_at DESC LIMIT $2`,
				deviceID, limit,
			)
		}
	} else {
		// sensor: look up via activity_sensors join
		if afterCursor != "" {
			rows, err = db.Pool.Query(ctx,
				`SELECT a.id::text, a.started_at, a.title, a.sport_type, a.distance_m
				 FROM activities a
				 INNER JOIN (
					SELECT DISTINCT activity_id FROM activity_sensors WHERE device_id = $1
				 ) s ON s.activity_id = a.id
				 WHERE a.started_at < $2::timestamptz
				 ORDER BY a.started_at DESC LIMIT $3`,
				deviceID, afterCursor, limit,
			)
		} else {
			rows, err = db.Pool.Query(ctx,
				`SELECT a.id::text, a.started_at, a.title, a.sport_type, a.distance_m
				 FROM activities a
				 INNER JOIN (
					SELECT DISTINCT activity_id FROM activity_sensors WHERE device_id = $1
				 ) s ON s.activity_id = a.id
				 ORDER BY a.started_at DESC LIMIT $2`,
				deviceID, limit,
			)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("list device activities: %w", err)
	}
	defer rows.Close()

	var items []DeviceActivitySummary
	for rows.Next() {
		var s DeviceActivitySummary
		if err := rows.Scan(&s.ID, &s.StartedAt, &s.Title, &s.SportType, &s.DistanceM); err != nil {
			return nil, fmt.Errorf("scan device activity: %w", err)
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list device activities: %w", err)
	}
	if items == nil {
		items = []DeviceActivitySummary{}
	}
	return items, nil
}

// SetRecordingDeviceID assigns a recording device to an activity.
func (db *DB) SetRecordingDeviceID(ctx context.Context, activityID, deviceID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activities SET recording_device_id = $1 WHERE id = $2`,
		deviceID, activityID,
	)
	if err != nil {
		return fmt.Errorf("set recording device id: %w", err)
	}
	return nil
}

// SetSensorDeviceID links an activity_sensors row to a hardware device.
// Matching priority: serial_number → ant_device_number → manufacturer_id+product_name.
// This intentionally uses field-based matching, NOT array index / device_index.
func (db *DB) SetSensorDeviceID(ctx context.Context, activityID string, sr model.SensorRecord, deviceID string) error {
	var err error

	if sr.SerialNumber != nil {
		_, err = db.Pool.Exec(ctx,
			`UPDATE activity_sensors SET device_id = $1
			 WHERE activity_id = $2 AND serial_number = $3`,
			deviceID, activityID, *sr.SerialNumber,
		)
	} else if sr.AntDeviceNumber != nil {
		_, err = db.Pool.Exec(ctx,
			`UPDATE activity_sensors SET device_id = $1
			 WHERE activity_id = $2 AND ant_device_number = $3 AND serial_number IS NULL`,
			deviceID, activityID, *sr.AntDeviceNumber,
		)
	} else {
		_, err = db.Pool.Exec(ctx,
			`UPDATE activity_sensors SET device_id = $1
			 WHERE activity_id = $2
			   AND manufacturer_id = $3
			   AND product_name = $4
			   AND serial_number IS NULL AND ant_device_number IS NULL`,
			deviceID, activityID, sr.ManufacturerID, sr.ProductName,
		)
	}

	if err != nil {
		return fmt.Errorf("set sensor device id: %w", err)
	}
	return nil
}
