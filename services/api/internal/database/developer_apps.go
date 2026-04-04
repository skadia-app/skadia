package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// DeveloperAppRow is the DB projection of activity_developer_apps for a single activity.
type DeveloperAppRow struct {
	ID            string
	ApplicationID []byte
	AppName       *string
	AppCategory   *string
	SerialNumber  *string
	DeviceID      *string
}

// InsertDeveloperApps inserts developer app records for an activity.
// If apps is empty, it returns immediately. On conflict, app_name and app_category
// are updated from the registry so that re-imports or registry upgrades propagate
// the latest values. serial_number and device_id are intentionally excluded from
// the update — they come from other sources and must not be overwritten here.
func (db *DB) InsertDeveloperApps(ctx context.Context, activityID string, apps []model.DeveloperAppRecord) error {
	if len(apps) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, app := range apps {
		batch.Queue(`
			INSERT INTO activity_developer_apps (activity_id, application_id, app_name, app_category, serial_number)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (activity_id, application_id) DO UPDATE
			    SET app_name = EXCLUDED.app_name,
			        app_category = EXCLUDED.app_category`,
			activityID,
			app.ApplicationID,
			nilIfEmpty(app.AppName),
			nilIfEmpty(app.AppCategory),
			nilIfEmpty(app.SerialNumber),
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range apps {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert developer app: %w", err)
		}
	}
	return nil
}

// DeleteDeveloperApps removes all developer app records for an activity.
func (db *DB) DeleteDeveloperApps(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM activity_developer_apps WHERE activity_id = $1`,
		activityID,
	)
	if err != nil {
		return fmt.Errorf("delete developer apps: %w", err)
	}
	return nil
}

// SetDeveloperAppDeviceID links a developer app entry to a resolved hardware device.
func (db *DB) SetDeveloperAppDeviceID(ctx context.Context, activityID string, applicationID []byte, deviceID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activity_developer_apps SET device_id = $1 WHERE activity_id = $2 AND application_id = $3`,
		deviceID, activityID, applicationID,
	)
	if err != nil {
		return fmt.Errorf("set developer app device_id: %w", err)
	}
	return nil
}

// GetDeveloperAppsForActivity returns all developer app rows for a given activity,
// ordered by app_name.
func (db *DB) GetDeveloperAppsForActivity(ctx context.Context, activityID string) ([]DeveloperAppRow, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, application_id, app_name, app_category, serial_number, device_id
		FROM activity_developer_apps
		WHERE activity_id = $1
		ORDER BY app_name`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get developer apps for activity: %w", err)
	}
	defer rows.Close()

	var result []DeveloperAppRow
	for rows.Next() {
		var row DeveloperAppRow
		if err := rows.Scan(&row.ID, &row.ApplicationID, &row.AppName, &row.AppCategory, &row.SerialNumber, &row.DeviceID); err != nil {
			return nil, fmt.Errorf("scan developer app row: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// GetDeveloperAppsForGDPR returns developer app data for all activities owned by a
// given user, for use in GDPR data-export responses.
func (db *DB) GetDeveloperAppsForGDPR(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT ada.activity_id, encode(ada.application_id, 'hex') AS application_id,
		       ada.app_name, ada.app_category, ada.serial_number
		FROM activity_developer_apps ada
		JOIN activities a ON a.id = ada.activity_id
		WHERE a.user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get developer apps for GDPR: %w", err)
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var activityID, applicationID string
		var appName, appCategory, serialNumber *string
		if err := rows.Scan(&activityID, &applicationID, &appName, &appCategory, &serialNumber); err != nil {
			return nil, fmt.Errorf("scan developer app GDPR row: %w", err)
		}
		result = append(result, map[string]any{
			"activity_id":    activityID,
			"application_id": applicationID,
			"app_name":       appName,
			"app_category":   appCategory,
			"serial_number":  serialNumber,
		})
	}
	return result, rows.Err()
}
