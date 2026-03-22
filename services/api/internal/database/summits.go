package database

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SummitVisit represents a single visited summit with aggregated visit data.
type SummitVisit struct {
	OsmID        *int64
	Name         string
	ElevationM   *float64
	VisitCount   int
	FirstVisit   time.Time
	LastVisit    time.Time
	Lat          *float64
	Lng          *float64
	CountryCodes []string // all country codes where this summit was visited from
}

// SummitCollectionFilter holds optional filter parameters for GetSummitCollection.
type SummitCollectionFilter struct {
	Countries []string // ISO-alpha-2 codes; empty = all
	MinElevM  *float64
	MaxElevM  *float64
	Year      *int
}

// GetSummitCollection returns all summits visited by the given user,
// grouped by osm_id with visit statistics and optional filters.
func (db *DB) GetSummitCollection(ctx context.Context, userID string, f SummitCollectionFilter) ([]SummitVisit, error) {
	args := []any{userID}
	idx := 2

	var filters []string

	if len(f.Countries) > 0 {
		filters = append(filters, fmt.Sprintf("a.start_country_code = ANY($%d)", idx))
		args = append(args, f.Countries)
		idx++
	}
	if f.MinElevM != nil {
		filters = append(filters, fmt.Sprintf("g.ele_m >= $%d", idx))
		args = append(args, *f.MinElevM)
		idx++
	}
	if f.MaxElevM != nil {
		filters = append(filters, fmt.Sprintf("g.ele_m <= $%d", idx))
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
		WITH base AS (
			SELECT
				g.osm_id,
				COALESCE(g.name, 'Unbekannter Gipfel') AS name,
				g.ele_m,
				COUNT(DISTINCT g.activity_id) AS visit_count,
				MIN(a.started_at) AS first_visit,
				MAX(a.started_at) AS last_visit,
				MIN(p.geom) AS summit_geom
			FROM activity_geo_tags g
			JOIN activities a ON g.activity_id = a.id
			LEFT JOIN osm_pois p ON g.osm_id = p.osm_id AND p.category IN ('summit', 'pass')
			WHERE a.user_id = $1 AND g.category IN ('summit', 'pass')%s
			GROUP BY g.osm_id, g.name, g.ele_m
		)
		SELECT
			b.osm_id,
			b.name,
			b.ele_m,
			b.visit_count,
			b.first_visit,
			b.last_visit,
			ST_Y(ST_Centroid(b.summit_geom)) AS lat,
			ST_X(ST_Centroid(b.summit_geom)) AS lng,
			array_agg(DISTINCT ob.country_code) FILTER (WHERE ob.country_code IS NOT NULL AND ob.country_code <> '') AS country_codes
		FROM base b
		LEFT JOIN osm_boundaries ob ON ob.admin_level = 2 AND ob.boundary_type = 'administrative'
			AND ST_Within(b.summit_geom, ob.geom)
		GROUP BY b.osm_id, b.name, b.ele_m, b.visit_count, b.first_visit, b.last_visit, b.summit_geom
		ORDER BY b.ele_m DESC NULLS LAST
	`, filterSQL)

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("summit collection query: %w", err)
	}
	defer rows.Close()

	var summits []SummitVisit
	for rows.Next() {
		var sv SummitVisit
		if err := rows.Scan(
			&sv.OsmID, &sv.Name, &sv.ElevationM, &sv.VisitCount,
			&sv.FirstVisit, &sv.LastVisit, &sv.Lat, &sv.Lng,
			&sv.CountryCodes,
		); err != nil {
			return nil, fmt.Errorf("summit collection scan: %w", err)
		}
		summits = append(summits, sv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("summit collection iteration: %w", err)
	}
	return summits, nil
}

// SummitActivity is a single activity that visited a summit.
type SummitActivity struct {
	ActivityID     string
	Title          string
	StartedAt      time.Time
	SportType      string
	DistanceM      *float64
	ElevationGainM *float64
}

// GetSummitActivities returns all activities that visited a given summit (by osm_id).
// Falls back to name-match when osmID is nil.
func (db *DB) GetSummitActivities(ctx context.Context, userID string, osmID *int64, name string) ([]SummitActivity, error) {
	var query string
	var args []any

	if osmID != nil {
		query = `
			SELECT a.id, COALESCE(a.title, ''), a.started_at,
			       COALESCE(a.sport_type, ''), a.distance_m, a.elevation_gain_m
			FROM activity_geo_tags g
			JOIN activities a ON g.activity_id = a.id
			WHERE a.user_id = $1
			  AND g.osm_id = $2
			  AND g.category IN ('summit', 'pass')
			ORDER BY a.started_at DESC
		`
		args = []any{userID, *osmID}
	} else {
		query = `
			SELECT a.id, COALESCE(a.title, ''), a.started_at,
			       COALESCE(a.sport_type, ''), a.distance_m, a.elevation_gain_m
			FROM activity_geo_tags g
			JOIN activities a ON g.activity_id = a.id
			WHERE a.user_id = $1
			  AND g.name = $2
			  AND g.category IN ('summit', 'pass')
			ORDER BY a.started_at DESC
		`
		args = []any{userID, name}
	}

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("summit activities query: %w", err)
	}
	defer rows.Close()

	var acts []SummitActivity
	for rows.Next() {
		var sa SummitActivity
		if err := rows.Scan(&sa.ActivityID, &sa.Title, &sa.StartedAt,
			&sa.SportType, &sa.DistanceM, &sa.ElevationGainM); err != nil {
			return nil, fmt.Errorf("summit activities scan: %w", err)
		}
		acts = append(acts, sa)
	}
	return acts, rows.Err()
}
