package fusion

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StreamPoint repräsentiert einen GPS-Punkt mit Zeitstempel
type StreamPoint struct {
	Time      time.Time
	Lat       *float64
	Lng       *float64
	AltitudeM *float64
	SpeedMS   *float64
	DistanceM *float64
}

// StitchConfig konfiguriert den Stitching-Algorithmus
type StitchConfig struct {
	PrimarySource string
	ExtendTrack   bool
}

// StitchGPSTracks führt GPS-Streams aus mehreren Quellen zu einem Track zusammen.
//
// Algorithmus:
// 1. Alle Quell-Streams laden und nach Zeit sortieren
// 2. Unified Timeline aus allen einzigartigen Zeitpunkten erstellen
// 3. Für jeden Zeitpunkt: Primary-Source bevorzugen, sonst Fallback
// 4. Ergebnis per UPSERT in Target-Stream schreiben
func StitchGPSTracks(
	ctx context.Context,
	pool *pgxpool.Pool,
	targetID string,
	sourceIDs []string,
	config *StitchConfig,
) error {
	allIDs := make([]string, 0, len(sourceIDs)+1)
	allIDs = append(allIDs, targetID)
	allIDs = append(allIDs, sourceIDs...)

	// 1. Alle Streams laden
	streams := make(map[string][]StreamPoint, len(allIDs))
	for _, id := range allIDs {
		points, err := loadGPSStream(ctx, pool, id)
		if err != nil {
			return fmt.Errorf("load stream %s: %w", id, err)
		}
		streams[id] = points
	}

	// 2. Unified Timeline: alle einzigartigen Zeitpunkte mit GPS-Daten
	timeSet := make(map[time.Time]struct{})
	for _, points := range streams {
		for _, p := range points {
			if p.Lat != nil && p.Lng != nil {
				timeSet[p.Time] = struct{}{}
			}
		}
	}

	times := make([]time.Time, 0, len(timeSet))
	for t := range timeSet {
		times = append(times, t)
	}
	sortTimes(times)

	// 3. Index-Maps für schnellen Lookup
	indices := make(map[string]map[time.Time]int, len(allIDs))
	for id, points := range streams {
		idx := make(map[time.Time]int, len(points))
		for i, p := range points {
			idx[p.Time] = i
		}
		indices[id] = idx
	}

	primaryID := targetID
	if config != nil && config.PrimarySource != "" {
		primaryID = config.PrimarySource
	}

	// 4. Für jeden Zeitpunkt die beste Quelle wählen
	result := make([]StreamPoint, 0, len(times))
	for _, t := range times {
		var bestPoint *StreamPoint

		// Primary-Source zuerst
		if idx, ok := indices[primaryID]; ok {
			if i, found := idx[t]; found {
				p := streams[primaryID][i]
				if p.Lat != nil && p.Lng != nil {
					cp := p
					bestPoint = &cp
				}
			}
		}

		// Fallback: andere Quellen
		if bestPoint == nil {
			for _, id := range allIDs {
				if id == primaryID {
					continue
				}
				if idx, ok := indices[id]; ok {
					if i, found := idx[t]; found {
						p := streams[id][i]
						if p.Lat != nil && p.Lng != nil {
							cp := p
							bestPoint = &cp
							break
						}
					}
				}
			}
		}

		if bestPoint != nil {
			bestPoint.Time = t
			result = append(result, *bestPoint)
		}
	}

	// 5. Ergebnis in Target-Stream schreiben
	return writeStitchedStream(ctx, pool, targetID, result)
}

func loadGPSStream(ctx context.Context, pool *pgxpool.Pool, activityID string) ([]StreamPoint, error) {
	rows, err := pool.Query(ctx, `
		SELECT time, lat, lng, altitude_m, speed_ms, distance_m
		FROM activity_streams
		WHERE activity_id = $1
		ORDER BY time ASC
	`, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []StreamPoint
	for rows.Next() {
		var p StreamPoint
		if err := rows.Scan(&p.Time, &p.Lat, &p.Lng, &p.AltitudeM, &p.SpeedMS, &p.DistanceM); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

func writeStitchedStream(ctx context.Context, pool *pgxpool.Pool, targetID string, points []StreamPoint) error {
	const upsertSQL = `
		INSERT INTO activity_streams (activity_id, time, lat, lng, altitude_m, speed_ms, distance_m)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (activity_id, time) DO UPDATE
		SET lat = EXCLUDED.lat, lng = EXCLUDED.lng,
			altitude_m = COALESCE(EXCLUDED.altitude_m, activity_streams.altitude_m),
			speed_ms = COALESCE(EXCLUDED.speed_ms, activity_streams.speed_ms),
			distance_m = COALESCE(EXCLUDED.distance_m, activity_streams.distance_m)`

	batch := &pgx.Batch{}
	for _, p := range points {
		if p.Lat == nil || p.Lng == nil {
			continue
		}
		batch.Queue(upsertSQL, targetID, p.Time, p.Lat, p.Lng, p.AltitudeM, p.SpeedMS, p.DistanceM)
	}
	if batch.Len() > 0 {
		br := pool.SendBatch(ctx, batch)
		for i := 0; i < batch.Len(); i++ {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return fmt.Errorf("upsert stream point batch: %w", err)
			}
		}
		if err := br.Close(); err != nil {
			return fmt.Errorf("batch close: %w", err)
		}
	}

	// Distanz neu berechnen (kumulativ aus GPS-Punkten)
	_, err := pool.Exec(ctx, `
		WITH ordered AS (
			SELECT time, lat, lng,
				   LAG(lat) OVER (ORDER BY time) AS prev_lat,
				   LAG(lng) OVER (ORDER BY time) AS prev_lng
			FROM activity_streams
			WHERE activity_id = $1 AND lat IS NOT NULL AND lng IS NOT NULL
			ORDER BY time
		),
		distances AS (
			SELECT time,
				   SUM(CASE WHEN prev_lat IS NOT NULL THEN
					   ST_Distance(
						   ST_SetSRID(ST_MakePoint(lng, lat), 4326)::geography,
						   ST_SetSRID(ST_MakePoint(prev_lng, prev_lat), 4326)::geography
					   )
				   ELSE 0 END) OVER (ORDER BY time) AS cumulative_m
			FROM ordered
		)
		UPDATE activity_streams s
		SET distance_m = d.cumulative_m
		FROM distances d
		WHERE s.activity_id = $1 AND s.time = d.time
	`, targetID)
	if err != nil {
		return fmt.Errorf("recalculate distance: %w", err)
	}

	// Gesamtdistanz auf Activity aktualisieren
	_, err = pool.Exec(ctx, `
		UPDATE activities SET
			distance_m = (SELECT MAX(distance_m) FROM activity_streams WHERE activity_id = $1 AND distance_m IS NOT NULL),
			has_edits = true
		WHERE id = $1
	`, targetID)
	return err
}

func sortTimes(times []time.Time) {
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
}
