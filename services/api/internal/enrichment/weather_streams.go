package enrichment

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SummitPoint returns lat, lng, altitude and timestamp of the highest GPS point.
func SummitPoint(ctx context.Context, pool *pgxpool.Pool, activityID string) (lat, lng, altM float64, at time.Time, ok bool) {
	err := pool.QueryRow(ctx, `
		SELECT lat, lng, altitude_m, time
		FROM activity_streams
		WHERE activity_id = $1
		  AND lat IS NOT NULL AND lng IS NOT NULL AND altitude_m IS NOT NULL
		ORDER BY altitude_m DESC
		LIMIT 1`, activityID,
	).Scan(&lat, &lng, &altM, &at)
	if err != nil {
		return 0, 0, 0, time.Time{}, false
	}
	return lat, lng, altM, at, true
}

// MidpointGPS returns lat, lng and timestamp of the temporal midpoint, plus mean altitude.
func MidpointGPS(ctx context.Context, pool *pgxpool.Pool, activityID string) (lat, lng, meanAltM float64, at time.Time, ok bool) {
	var (
		midLat  *float64
		midLng  *float64
		midTime *time.Time
		meanAlt *float64
		total   int
	)
	err := pool.QueryRow(ctx, `
		WITH gps AS (
			SELECT lat, lng, time, altitude_m,
			       ROW_NUMBER() OVER (ORDER BY time) - 1 AS rn,
			       COUNT(*)    OVER ()                   AS cnt
			FROM activity_streams
			WHERE activity_id = $1
			  AND lat IS NOT NULL AND lng IS NOT NULL
		)
		SELECT
			MAX(lat)        FILTER (WHERE rn = cnt / 2) AS mid_lat,
			MAX(lng)        FILTER (WHERE rn = cnt / 2) AS mid_lng,
			MAX(time)       FILTER (WHERE rn = cnt / 2) AS mid_time,
			AVG(altitude_m)                             AS mean_alt,
			MAX(cnt)                                    AS total
		FROM gps`, activityID,
	).Scan(&midLat, &midLng, &midTime, &meanAlt, &total)

	if err != nil || total < 2 || midLat == nil || midLng == nil || midTime == nil {
		return 0, 0, 0, time.Time{}, false
	}
	if meanAlt != nil {
		meanAltM = *meanAlt
	}
	return *midLat, *midLng, meanAltM, *midTime, true
}
