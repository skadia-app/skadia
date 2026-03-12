package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
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

// SearchPeaks searches osm_pois (category='peak' or 'pass') by name using pg_trgm similarity.
func (db *DB) SearchPeaks(ctx context.Context, query string, limit int) ([]POIRow, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT name,
			COALESCE(type, 'peak') AS type,
			ele,
			ST_Y(geom) AS lat, ST_X(geom) AS lng,
			osm_id
		FROM osm_pois
		WHERE category IN ('peak', 'pass') AND name IS NOT NULL AND name % $1
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

// SearchAmenities searches osm_pois (non-peak/pass/settlement categories) by name using pg_trgm similarity.
// When types is non-empty (comma-separated), results are filtered to matching type values.
func (db *DB) SearchAmenities(ctx context.Context, query, types string, limit int) ([]POIRow, error) {
	var rows pgx.Rows
	var err error

	if types != "" {
		typeList := strings.Split(types, ",")
		rows, err = db.Pool.Query(ctx, `
			SELECT name,
				COALESCE(type, 'amenity') AS type,
				NULL::double precision AS ele,
				ST_Y(geom) AS lat, ST_X(geom) AS lng,
				osm_id
			FROM osm_pois
			WHERE category NOT IN ('peak', 'pass', 'settlement')
			  AND name IS NOT NULL AND name % $1 AND type = ANY($3)
			ORDER BY similarity(name, $1) DESC
			LIMIT $2
		`, query, limit, typeList)
	} else {
		rows, err = db.Pool.Query(ctx, `
			SELECT name,
				COALESCE(type, 'amenity') AS type,
				NULL::double precision AS ele,
				ST_Y(geom) AS lat, ST_X(geom) AS lng,
				osm_id
			FROM osm_pois
			WHERE category NOT IN ('peak', 'pass', 'settlement')
			  AND name IS NOT NULL AND name % $1
			ORDER BY similarity(name, $1) DESC
			LIMIT $2
		`, query, limit)
	}
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
