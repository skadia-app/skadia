package database

import (
	"context"
	"fmt"
	"time"
)

// PeakVisit represents a single visited peak with aggregated visit data.
type PeakVisit struct {
	OsmID      *int64
	Name       string
	ElevationM *float64
	VisitCount int
	FirstVisit time.Time
	LastVisit  time.Time
	Lat        *float64
	Lng        *float64
}

// GetPeakCollection returns all peaks visited by the given user,
// grouped by osm_id with visit statistics.
func (db *DB) GetPeakCollection(ctx context.Context, userID string) ([]PeakVisit, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			g.osm_id,
			g.name,
			g.elevation_m,
			COUNT(DISTINCT g.activity_id) AS visit_count,
			MIN(a.started_at) AS first_visit,
			MAX(a.started_at) AS last_visit,
			ST_Y(p.geom) AS lat,
			ST_X(p.geom) AS lng
		FROM activity_geo_tags g
		JOIN activities a ON g.activity_id = a.id
		LEFT JOIN osm_peaks p ON g.osm_id = p.osm_id
		WHERE g.user_id = $1 AND g.category = 'peak'
		GROUP BY g.osm_id, g.name, g.elevation_m, p.geom
		ORDER BY g.elevation_m DESC NULLS LAST
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("peak collection query: %w", err)
	}
	defer rows.Close()

	var peaks []PeakVisit
	for rows.Next() {
		var p PeakVisit
		if err := rows.Scan(&p.OsmID, &p.Name, &p.ElevationM, &p.VisitCount, &p.FirstVisit, &p.LastVisit, &p.Lat, &p.Lng); err != nil {
			return nil, fmt.Errorf("peak collection scan: %w", err)
		}
		peaks = append(peaks, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("peak collection iteration: %w", err)
	}
	return peaks, nil
}
