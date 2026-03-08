package database

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PeakVisit represents a single visited peak with aggregated visit data.
type PeakVisit struct {
	OsmID        *int64
	Name         string
	ElevationM   *float64
	VisitCount   int
	FirstVisit   time.Time
	LastVisit    time.Time
	Lat          *float64
	Lng          *float64
	CountryCodes []string // all country codes where this peak was visited from
}

// PeakCollectionFilter holds optional filter parameters for GetPeakCollection.
type PeakCollectionFilter struct {
	Countries   []string // ISO-alpha-2 codes; empty = all
	MinElevM    *float64
	MaxElevM    *float64
	Year        *int
}

// GetPeakCollection returns all peaks visited by the given user,
// grouped by osm_id with visit statistics and optional filters.
func (db *DB) GetPeakCollection(ctx context.Context, userID string, f PeakCollectionFilter) ([]PeakVisit, error) {
	args := []any{userID}
	idx := 2

	var filters []string

	if len(f.Countries) > 0 {
		filters = append(filters, fmt.Sprintf("a.start_country_code = ANY($%d)", idx))
		args = append(args, f.Countries)
		idx++
	}
	if f.MinElevM != nil {
		filters = append(filters, fmt.Sprintf("g.elevation_m >= $%d", idx))
		args = append(args, *f.MinElevM)
		idx++
	}
	if f.MaxElevM != nil {
		filters = append(filters, fmt.Sprintf("g.elevation_m <= $%d", idx))
		args = append(args, *f.MaxElevM)
		idx++
	}
	if f.Year != nil {
		filters = append(filters, fmt.Sprintf("EXTRACT(YEAR FROM a.started_at) = $%d", idx))
		args = append(args, *f.Year)
		idx++
	}

	filterSQL := ""
	if len(filters) > 0 {
		filterSQL = " AND " + strings.Join(filters, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT
			g.osm_id,
			COALESCE(g.name, 'Unbekannter Gipfel') AS name,
			g.elevation_m,
			COUNT(DISTINCT g.activity_id) AS visit_count,
			MIN(a.started_at) AS first_visit,
			MAX(a.started_at) AS last_visit,
			ST_Y(p.geom) AS lat,
			ST_X(p.geom) AS lng,
			array_agg(DISTINCT a.start_country_code) FILTER (WHERE a.start_country_code IS NOT NULL AND a.start_country_code <> '') AS country_codes
		FROM activity_geo_tags g
		JOIN activities a ON g.activity_id = a.id
		LEFT JOIN osm_peaks p ON g.osm_id = p.osm_id
		WHERE g.user_id = $1 AND g.category = 'peak'%s
		GROUP BY g.osm_id, g.name, g.elevation_m, p.geom
		ORDER BY g.elevation_m DESC NULLS LAST
	`, filterSQL)

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("peak collection query: %w", err)
	}
	defer rows.Close()

	var peaks []PeakVisit
	for rows.Next() {
		var pv PeakVisit
		if err := rows.Scan(
			&pv.OsmID, &pv.Name, &pv.ElevationM, &pv.VisitCount,
			&pv.FirstVisit, &pv.LastVisit, &pv.Lat, &pv.Lng,
			&pv.CountryCodes,
		); err != nil {
			return nil, fmt.Errorf("peak collection scan: %w", err)
		}
		peaks = append(peaks, pv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("peak collection iteration: %w", err)
	}
	return peaks, nil
}
