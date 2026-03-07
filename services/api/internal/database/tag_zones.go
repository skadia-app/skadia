package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// scanTagZone scans a tag_zone row. Expects columns:
//   id, tag_id, zone_type,
//   ST_Y(center), ST_X(center),   -- lat, lng  (NULL for polygon zones)
//   radius_m,
//   ST_AsGeoJSON(polygon)          -- NULL for circle zones
//   created_at, updated_at
func scanTagZone(row interface{ Scan(...any) error }) (*model.TagZone, error) {
	var z model.TagZone
	var lat, lng *float64
	var polygon *string
	err := row.Scan(
		&z.ID, &z.TagID, &z.ZoneType,
		&lat, &lng,
		&z.RadiusM,
		&polygon,
		&z.CreatedAt, &z.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if lat != nil && lng != nil {
		center := [2]float64{*lng, *lat}
		z.Center = &center
	}
	z.Polygon = polygon
	return &z, nil
}

const tagZoneCols = `
	tz.id, tz.tag_id, tz.zone_type,
	ST_Y(tz.center) AS lat, ST_X(tz.center) AS lng,
	tz.radius_m,
	CASE WHEN tz.polygon IS NOT NULL THEN ST_AsGeoJSON(tz.polygon) ELSE NULL END AS polygon_geojson,
	tz.created_at, tz.updated_at`

// GetTagZone returns the zone for the given tag, or nil if no zone exists.
// Ownership is verified: the tag must belong to userID.
func (db *DB) GetTagZone(ctx context.Context, tagID, userID string) (*model.TagZone, error) {
	row := db.Pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s
		FROM tag_zones tz
		JOIN tags t ON t.id = tz.tag_id
		WHERE tz.tag_id = $1 AND t.user_id = $2`, tagZoneCols),
		tagID, userID,
	)
	z, err := scanTagZone(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tag zone: %w", err)
	}
	return z, nil
}

// getTagZonesByTagIDs returns zones keyed by tag_id for a set of tags.
// Used to enrich ListTags responses.
func (db *DB) getTagZonesByUserID(ctx context.Context, userID string) (map[string]*model.TagZone, error) {
	rows, err := db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT %s
		FROM tag_zones tz
		JOIN tags t ON t.id = tz.tag_id
		WHERE t.user_id = $1`, tagZoneCols),
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list tag zones: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*model.TagZone)
	for rows.Next() {
		z, err := scanTagZone(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tag zone: %w", err)
		}
		result[z.TagID] = z
	}
	return result, nil
}

// UpsertTagZone inserts or updates the zone for a tag (ON CONFLICT tag_id DO UPDATE).
// The tag must belong to userID (ownership verified before calling handlers).
func (db *DB) UpsertTagZone(ctx context.Context, tagID, userID string, zone model.TagZone) (*model.TagZone, error) {
	// Verify ownership
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM tags WHERE id = $1 AND user_id = $2)`,
		tagID, userID,
	).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("upsert tag zone: verify ownership: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("tag not found")
	}

	var z *model.TagZone
	if zone.ZoneType == "polygon" && zone.Polygon != nil {
		row := db.Pool.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO tag_zones (tag_id, zone_type, polygon, updated_at)
			VALUES ($1, 'polygon', ST_GeomFromGeoJSON($2), NOW())
			ON CONFLICT (tag_id) DO UPDATE SET
				zone_type  = EXCLUDED.zone_type,
				center     = NULL,
				radius_m   = NULL,
				polygon    = EXCLUDED.polygon,
				updated_at = NOW()
			RETURNING %s`, tagZoneCols),
			tagID, *zone.Polygon,
		)
		z, err = scanTagZone(row)
	} else {
		// circle: requires center coordinates
		if zone.Center == nil || zone.RadiusM == nil {
			return nil, fmt.Errorf("circle zone requires center and radius_m")
		}
		lng := zone.Center[0]
		lat := zone.Center[1]
		row := db.Pool.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO tag_zones (tag_id, zone_type, center, radius_m, updated_at)
			VALUES ($1, 'circle', ST_SetSRID(ST_MakePoint($2, $3), 4326), $4, NOW())
			ON CONFLICT (tag_id) DO UPDATE SET
				zone_type  = EXCLUDED.zone_type,
				center     = EXCLUDED.center,
				radius_m   = EXCLUDED.radius_m,
				polygon    = NULL,
				updated_at = NOW()
			RETURNING %s`, tagZoneCols),
			tagID, lng, lat, *zone.RadiusM,
		)
		z, err = scanTagZone(row)
	}
	if err != nil {
		return nil, fmt.Errorf("upsert tag zone: %w", err)
	}
	return z, nil
}

// DeleteTagZone deletes the zone for a tag. The tag must belong to userID.
func (db *DB) DeleteTagZone(ctx context.Context, tagID, userID string) error {
	result, err := db.Pool.Exec(ctx, `
		DELETE FROM tag_zones
		WHERE tag_id = $1
		  AND EXISTS (SELECT 1 FROM tags WHERE id = $1 AND user_id = $2)`,
		tagID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete tag zone: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("tag zone not found")
	}
	return nil
}
