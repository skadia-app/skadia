package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

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
	if errors.Is(err, pgx.ErrNoRows) {
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
	if errors.Is(err, pgx.ErrNoRows) {
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
