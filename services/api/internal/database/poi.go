package database

import (
	"context"
	"fmt"
)

// POIRow is a generic point-of-interest result from OSM tables.
type POIRow struct {
	Name  string
	Type  string
	Ele   *float64
	Lat   float64
	Lng   float64
	OsmID int64
}

// SearchPeaks searches osm_peaks by name using pg_trgm similarity.
func (db *DB) SearchPeaks(ctx context.Context, query string, limit int) ([]POIRow, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT name,
			COALESCE(type, 'peak') AS type,
			ele,
			ST_Y(geom) AS lat, ST_X(geom) AS lng,
			osm_id
		FROM osm_peaks
		WHERE name IS NOT NULL AND name % $1
		ORDER BY similarity(name, $1) DESC
		LIMIT $2
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search peaks: %w", err)
	}
	defer rows.Close()

	var results []POIRow
	for rows.Next() {
		var r POIRow
		if err := rows.Scan(&r.Name, &r.Type, &r.Ele, &r.Lat, &r.Lng, &r.OsmID); err != nil {
			return nil, fmt.Errorf("scan peak: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search peaks rows: %w", err)
	}
	return results, nil
}

// SearchAmenities searches osm_amenities by name using pg_trgm similarity.
// The types parameter is currently unused for filtering (all amenity types are searched).
func (db *DB) SearchAmenities(ctx context.Context, query, types string, limit int) ([]POIRow, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT name,
			COALESCE(subtype, type, 'amenity') AS type,
			NULL::double precision AS ele,
			ST_Y(geom) AS lat, ST_X(geom) AS lng,
			osm_id
		FROM osm_amenities
		WHERE name IS NOT NULL AND name % $1
		ORDER BY similarity(name, $1) DESC
		LIMIT $2
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search amenities: %w", err)
	}
	defer rows.Close()

	var results []POIRow
	for rows.Next() {
		var r POIRow
		if err := rows.Scan(&r.Name, &r.Type, &r.Ele, &r.Lat, &r.Lng, &r.OsmID); err != nil {
			return nil, fmt.Errorf("scan amenity: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search amenities rows: %w", err)
	}
	return results, nil
}
