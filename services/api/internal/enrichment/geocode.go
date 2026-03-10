package enrichment

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	osmBoundariesExists bool
	osmBoundariesOnce   sync.Once
)

// GeoResult holds reverse geocoding results.
type GeoResult struct {
	City        string
	State       string
	Country     string
	CountryCode string
}

// ReverseGeocode finds city/state/country for a point using OSM boundaries.
// Returns empty result if no OSM data is available.
func ReverseGeocode(ctx context.Context, pool *pgxpool.Pool, lat, lng float64) (*GeoResult, error) {
	// Check if osm_boundaries table exists — cached via sync.Once to avoid
	// per-request information_schema queries.
	osmBoundariesOnce.Do(func() {
		var exists bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = 'osm_boundaries'
			)`,
		).Scan(&exists)
		if err == nil {
			osmBoundariesExists = exists
		}
	})
	if !osmBoundariesExists {
		return nil, nil
	}

	result := &GeoResult{}

	// Query for city (admin_level 8), state (admin_level 4), country (admin_level 2)
	rows, err := pool.Query(ctx, `
		SELECT admin_level, name, COALESCE(country_code, '') AS country_code
		FROM osm_boundaries
		WHERE ST_Contains(geom, ST_SetSRID(ST_MakePoint($1, $2), 4326))
		  AND admin_level IN (2, 4, 6, 8)
		ORDER BY admin_level DESC`,
		lng, lat,
	)
	if err != nil {
		slog.Debug("reverse geocode query failed", "error", err)
		return nil, nil
	}
	defer rows.Close()

	for rows.Next() {
		var level int
		var name, countryCode string
		if err := rows.Scan(&level, &name, &countryCode); err != nil {
			continue
		}
		switch level {
		case 8:
			result.City = name
		case 6:
			// District/Kreis — use as city fallback
			if result.City == "" {
				result.City = name
			}
		case 4:
			result.State = name
		case 2:
			result.Country = name
			if countryCode != "" {
				result.CountryCode = countryCode
			}
		}
	}

	if result.City == "" && result.State == "" && result.Country == "" {
		return nil, nil
	}

	return result, nil
}

// ReverseGeocodeSimple is a lightweight version that returns only the city name.
func ReverseGeocodeSimple(ctx context.Context, pool *pgxpool.Pool, lat, lng float64) string {
	geo, err := ReverseGeocode(ctx, pool, lat, lng)
	if err != nil || geo == nil {
		return ""
	}
	return geo.City
}

