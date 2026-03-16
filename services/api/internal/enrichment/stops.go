package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

const (
	stopSpeedThresholdMS   = 0.5  // m/s below which a point is considered stationary
	stopMinDurationS       = 60   // minimum stop duration in seconds
	stopPOIRadiusM         = 50.0 // radius in meters for POI matching
	stopGastronomyMinDurS  = 600  // minimum stop duration for a gastronomy geo tag
)

// gastronomyTypes is the set of OSM poi subtypes that qualify as gastronomic stops.
var gastronomyTypes = map[string]bool{
	// amenity
	"restaurant": true,
	"cafe":       true,
	"biergarten": true,
	"fast_food":  true,
	"ice_cream":  true,
	"bar":        true,
	"pub":        true,
	"food_court": true,
	// tourism
	"alpine_hut":  true,
	"guest_house": true,
	"hostel":      true,
	"hotel":       true,
	// shop
	"kiosk":   true,
	"bakery":  true,
	"butcher": true,
}

// streamPoint holds a single row from activity_streams.
type streamPoint struct {
	T       time.Time
	Lat     float64
	Lng     float64
	SpeedMS float64
}

// detectedStop holds the computed stop metadata before POI lookup.
type detectedStop struct {
	StartedAt  time.Time
	EndedAt    time.Time
	DurationS  int
	MidLat     float64
	MidLng     float64
}

// enrichStreamStops is the Go port of services/ml/enrichment/stops.py.
//
// Algorithm:
//  1. Load GPS+speed stream for the activity.
//  2. Detect consecutive points with speed_ms < 0.5 lasting >= 60 s.
//  3. Use the midpoint of each stop cluster as the representative location.
//  4. Batch POI lookup via LATERAL on osm_pois (gastronomy/accommodation).
//  5. DELETE existing activity_stops + gastronomy geo tags, then INSERT fresh results.
//  6. If duration >= 600 s AND poi_type is a gastronomy type AND poi_name is set,
//     also INSERT an activity_geo_tags row with category='gastronomy'.
func enrichStreamStops(ctx context.Context, db *database.DB, _ string, activityID string) error {
	// 1. Load stream rows.
	rows, err := db.Pool.Query(ctx, `
		SELECT time, lat, lng, COALESCE(speed_ms, 0)
		FROM activity_streams
		WHERE activity_id = $1
		  AND lat IS NOT NULL AND lng IS NOT NULL
		ORDER BY time
	`, activityID)
	if err != nil {
		return fmt.Errorf("stops: query stream: %w", err)
	}

	var points []streamPoint
	for rows.Next() {
		var p streamPoint
		if err := rows.Scan(&p.T, &p.Lat, &p.Lng, &p.SpeedMS); err != nil {
			rows.Close()
			return fmt.Errorf("stops: scan stream: %w", err)
		}
		points = append(points, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("stops: stream rows: %w", err)
	}

	if len(points) == 0 {
		return nil
	}

	// 2. Detect stops.
	stops := detectStops(points)
	if len(stops) == 0 {
		return nil
	}

	// 3. Delete existing data.
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_stops WHERE activity_id = $1",
		activityID,
	); err != nil {
		return fmt.Errorf("stops: delete existing stops: %w", err)
	}
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category = 'gastronomy'",
		activityID,
	); err != nil {
		return fmt.Errorf("stops: delete existing gastronomy tags: %w", err)
	}

	// 4. Batch POI lookup via LATERAL.
	// Build unnest-ready slices (1-based ordinality matches stops index + 1).
	lats := make([]float64, len(stops))
	lngs := make([]float64, len(stops))
	for i, s := range stops {
		lats[i] = s.MidLat
		lngs[i] = s.MidLng
	}

	type poiResult struct {
		OsmID    *int64
		Name     *string
		TagValue *string // subtype e.g. "restaurant"
	}
	poiByIdx := make(map[int]poiResult, len(stops))

	poiRows, err := db.Pool.Query(ctx, `
		WITH stops AS (
			SELECT ordinality AS idx,
			       ST_SetSRID(ST_MakePoint(lng, lat), 4326) AS geom
			FROM unnest($1::float8[], $2::float8[])
			WITH ORDINALITY AS t(lat, lng)
		)
		SELECT s.idx, a.osm_id, a.name, a.subtype
		FROM stops s
		LEFT JOIN LATERAL (
			SELECT a.osm_id, a.name, a.subtype
			FROM osm_pois a
			WHERE ST_DWithin(s.geom::geography, a.geom::geography, $3)
			  AND a.category IN ('gastronomy', 'accommodation')
			ORDER BY ST_Distance(s.geom::geography, a.geom::geography)
			LIMIT 1
		) a ON true
		ORDER BY s.idx
	`, lats, lngs, stopPOIRadiusM)
	if err != nil {
		return fmt.Errorf("stops: poi lookup: %w", err)
	}
	defer poiRows.Close()

	for poiRows.Next() {
		var idx int
		var pr poiResult
		if err := poiRows.Scan(&idx, &pr.OsmID, &pr.Name, &pr.TagValue); err != nil {
			return fmt.Errorf("stops: poi scan: %w", err)
		}
		poiByIdx[idx] = pr
	}
	if err := poiRows.Err(); err != nil {
		return fmt.Errorf("stops: poi rows: %w", err)
	}

	// 5 + 6. Insert stops and gastronomy geo tags.
	gastronomyCount := 0
	for i, stop := range stops {
		poi := poiByIdx[i+1] // ordinality is 1-based

		var poiType, poiName *string
		var poiOsmID *int64
		if poi.TagValue != nil && *poi.TagValue != "" {
			poiType = poi.TagValue
		}
		if poi.Name != nil && *poi.Name != "" {
			poiName = poi.Name
		}
		if poi.OsmID != nil {
			poiOsmID = poi.OsmID
		}

		// Insert into activity_stops.
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_stops
			    (activity_id, started_at, ended_at, duration_s,
			     location, poi_type, poi_name, poi_osm_id)
			VALUES ($1, $2, $3, $4,
			        ST_SetSRID(ST_MakePoint($6, $5), 4326),
			        $7, $8, $9)
		`, activityID, stop.StartedAt, stop.EndedAt, stop.DurationS,
			stop.MidLat, stop.MidLng,
			poiType, poiName, poiOsmID,
		); err != nil {
			slog.Warn("stops: insert stop failed",
				"activity_id", activityID,
				"started_at", stop.StartedAt,
				"error", err,
			)
		}

		// Insert gastronomy geo tag if criteria are met.
		if stop.DurationS >= stopGastronomyMinDurS &&
			poiType != nil && gastronomyTypes[*poiType] &&
			poiName != nil {

			metaBytes, _ := json.Marshal(map[string]string{"poi_type": *poiType})
			durationMin := stop.DurationS / 60

			if _, err := db.Pool.Exec(ctx, `
				INSERT INTO activity_geo_tags
				    (activity_id, category, name, osm_id, map_type,
				     map_point_lat, map_point_lon, duration_min, meta)
				VALUES ($1, 'gastronomy', $2, $3, 'point', $4, $5, $6, $7)
				ON CONFLICT DO NOTHING
			`, activityID, *poiName, poiOsmID,
				stop.MidLat, stop.MidLng, durationMin, metaBytes,
			); err != nil {
				slog.Warn("stops: insert gastronomy tag failed",
					"activity_id", activityID,
					"poi_name", *poiName,
					"error", err,
				)
			} else {
				gastronomyCount++
			}
		}
	}

	slog.Info("stops: enrichment complete",
		"activity_id", activityID,
		"stops", len(stops),
		"gastronomy_tags", gastronomyCount,
	)
	return nil
}

// detectStops scans the stream and returns all stops of at least stopMinDurationS.
// A stop is a run of consecutive points with speed_ms < stopSpeedThresholdMS.
// The representative location is the midpoint of the cluster.
func detectStops(points []streamPoint) []detectedStop {
	var stops []detectedStop

	inStop := false
	startIdx := 0

	flush := func(endIdx int) {
		cluster := points[startIdx : endIdx+1]
		durationS := cluster[len(cluster)-1].T.Sub(cluster[0].T).Seconds()
		if durationS >= float64(stopMinDurationS) {
			mid := len(cluster) / 2
			stops = append(stops, detectedStop{
				StartedAt: cluster[0].T,
				EndedAt:   cluster[len(cluster)-1].T,
				DurationS: int(durationS),
				MidLat:    cluster[mid].Lat,
				MidLng:    cluster[mid].Lng,
			})
		}
	}

	for i, p := range points {
		if p.SpeedMS < stopSpeedThresholdMS {
			if !inStop {
				inStop = true
				startIdx = i
			}
		} else {
			if inStop {
				flush(i - 1)
				inStop = false
			}
		}
	}
	// Handle stop that extends to end of activity.
	if inStop {
		flush(len(points) - 1)
	}

	return stops
}
