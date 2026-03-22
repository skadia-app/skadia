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
		filters = append(filters, fmt.Sprintf("a.started_at >= $%d AND a.started_at < $%d", idx, idx+1))
		args = append(args, time.Date(*f.Year, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(*f.Year+1, 1, 1, 0, 0, 0, 0, time.UTC))
		idx += 2
	}

	filterSQL := ""
	if len(filters) > 0 {
		filterSQL = " AND " + strings.Join(filters, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT
			g.osm_id,
			COALESCE(g.name, 'Unbekannter Gipfel') AS name,
			MAX(g.ele_m) AS ele_m,
			COUNT(DISTINCT g.activity_id) AS visit_count,
			MIN(a.started_at) AS first_visit,
			MAX(a.started_at) AS last_visit,
			(SELECT ST_Y(p2.geom) FROM osm_pois p2 WHERE p2.osm_id = g.osm_id AND p2.category IN ('summit','pass') LIMIT 1) AS lat,
			(SELECT ST_X(p2.geom) FROM osm_pois p2 WHERE p2.osm_id = g.osm_id AND p2.category IN ('summit','pass') LIMIT 1) AS lng,
			array_agg(DISTINCT a.start_country_code) FILTER (WHERE a.start_country_code IS NOT NULL AND a.start_country_code <> '') AS country_codes
		FROM activity_geo_tags g
		JOIN activities a ON g.activity_id = a.id
		WHERE a.user_id = $1 AND g.category IN ('summit', 'pass') AND g.osm_id IS NOT NULL%s
		GROUP BY g.osm_id, g.name
		ORDER BY ele_m DESC NULLS LAST
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
