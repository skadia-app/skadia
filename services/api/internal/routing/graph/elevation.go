package graph

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SampleElevation queries DEM elevation for all nodes via batch SQL.
// Returns elevations in decimeters (0.1m precision).
func SampleElevation(ctx context.Context, pool *pgxpool.Pool,
	lats, lons []int32, isBridge []bool, progressFn func(done, total int)) ([]int32, error) {

	const batchSize = 10000
	n := len(lats)
	elevations := make([]int32, n)

	for i := 0; i < n; i += batchSize {
		end := i + batchSize
		if end > n {
			end = n
		}

		batchLats := make([]float64, end-i)
		batchLons := make([]float64, end-i)
		for j := i; j < end; j++ {
			batchLats[j-i] = float64(lats[j]) / 1e7
			batchLons[j-i] = float64(lons[j]) / 1e7
		}

		rows, err := pool.Query(ctx, `
			SELECT COALESCE(ROUND(dem.get_elevation(
				ST_SetSRID(ST_MakePoint(lon, lat), 4326)
			) * 10)::int, 0)
			FROM unnest($1::float8[], $2::float8[]) WITH ORDINALITY AS t(lon, lat, ord)
			ORDER BY ord`,
			batchLons, batchLats)
		if err != nil {
			return nil, fmt.Errorf("elevation batch %d-%d: %w", i, end, err)
		}

		j := i
		for rows.Next() {
			var ele int32
			if err := rows.Scan(&ele); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan elevation: %w", err)
			}
			elevations[j] = ele
			j++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("elevation rows: %w", err)
		}

		if progressFn != nil {
			progressFn(end, n)
		}
	}

	InterpolateMissing(elevations, isBridge)

	return elevations, nil
}

// InterpolateMissing fills gaps in elevation data for bridge/tunnel segments.
func InterpolateMissing(elevations []int32, isBridge []bool) {
	n := len(elevations)
	if n == 0 {
		return
	}

	i := 0
	for i < n {
		if !isBridge[i] || elevations[i] != 0 {
			i++
			continue
		}

		start := i
		for i < n && isBridge[i] && elevations[i] == 0 {
			i++
		}
		end := i

		var leftVal, rightVal int32
		if start > 0 {
			leftVal = elevations[start-1]
		}
		if end < n {
			rightVal = elevations[end]
		}

		if start == 0 && end < n {
			leftVal = rightVal
		} else if end == n && start > 0 {
			rightVal = leftVal
		} else if start == 0 && end == n {
			continue
		}

		for j := start; j < end; j++ {
			if start == 0 {
				elevations[j] = leftVal
			} else {
				pos := j - start + 1
				frac := float64(pos) / float64(end-start+1)
				elevations[j] = leftVal + int32(float64(rightVal-leftVal)*frac)
			}
		}
	}

	interpolated := 0
	for i, b := range isBridge {
		if b && elevations[i] != 0 {
			interpolated++
		}
	}
	if interpolated > 0 {
		slog.Info("interpolated bridge/tunnel elevations", "count", interpolated)
	}
}
