package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
)

const (
	surfaceMatchToleranceM = 25.0   // max distance GPS point → nearest way
	surfaceResampleM       = 10.0   // resample track to uniform point spacing
	surfaceBBoxExpand      = 0.0006 // ST_Expand in degrees (~28m lon at 65°, ~67m lat)
	surfaceModeWindowSize  = 5      // sliding window for mode filter (5 points = 50m)
)

type matchedPoint struct {
	Idx          int
	Surface      *string
	Highway      *string
	OsmID        *int64
	DistM        *float64
	SurfaceClass string
	HighwayClass string
}

type SurfaceBreakdown struct {
	Type       string  `json:"type"`
	LengthM    float64 `json:"length_m"`
	Percentage float64 `json:"percentage"`
}

// enrichSurfacesSQL matches the activity track against osm_ways using a
// nearest-neighbour lateral join on resampled track points.
//
// The approach avoids the slow ST_Intersection + ST_Buffer pattern from the
// legacy EnrichSurfaces (17 min → ~100 ms) and produces per-point
// classifications that are then smoothed by a sliding-window mode filter
// before being aggregated into SurfaceBreakdown rows.
func enrichSurfacesSQL(ctx context.Context, db *database.DB, activityID string) error {
	rows, err := db.Pool.Query(ctx, `
		WITH route_pts AS (
		    SELECT (dp).path[1] AS idx, (dp).geom AS pt
		    FROM (
		        SELECT ST_DumpPoints(
		            ST_Segmentize(ST_Simplify(track_full, 0.00005)::geography, $2)::geometry
		        ) AS dp
		        FROM activity_tracks WHERE activity_id = $1
		    ) sub
		),
		matched AS (
		    SELECT rp.idx, w.surface, w.highway, w.osm_id,
		           ST_Distance(rp.pt::geography, w.geom::geography) AS dist_m
		    FROM route_pts rp
		    LEFT JOIN LATERAL (
		        SELECT w.surface, w.highway, w.osm_id, w.geom
		        FROM osm_ways w
		        WHERE w.geom && ST_Expand(rp.pt, $3)
		        ORDER BY w.geom <-> rp.pt
		        LIMIT 1
		    ) w ON true
		)
		SELECT idx, surface, highway, osm_id, dist_m FROM matched ORDER BY idx
	`, activityID, surfaceResampleM, surfaceBBoxExpand)
	if err != nil {
		return fmt.Errorf("surfaces_sql: query: %w", err)
	}
	defer rows.Close()

	var points []matchedPoint
	for rows.Next() {
		var p matchedPoint
		if err := rows.Scan(&p.Idx, &p.Surface, &p.Highway, &p.OsmID, &p.DistM); err != nil {
			return fmt.Errorf("surfaces_sql: scan: %w", err)
		}

		// Classify each point based on match distance and available tags.
		if p.DistM == nil || *p.DistM > surfaceMatchToleranceM {
			p.SurfaceClass = "offroad"
			p.HighwayClass = "offroad"
		} else if p.Surface == nil {
			p.SurfaceClass = "unknown"
			if p.Highway != nil {
				p.HighwayClass = *p.Highway
			} else {
				p.HighwayClass = "offroad"
			}
		} else {
			p.SurfaceClass = *p.Surface
			if p.Highway != nil {
				p.HighwayClass = *p.Highway
			} else {
				p.HighwayClass = "unknown"
			}
		}

		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("surfaces_sql: rows iteration: %w", err)
	}

	if len(points) == 0 {
		return nil
	}

	// Smooth per-point classifications with a sliding-window majority vote.
	points = modeFilter(points)

	// Aggregate into surface and highway breakdowns.
	surfaceBreakdowns := aggregateByField(points, func(p matchedPoint) string { return p.SurfaceClass })
	highwayBreakdowns := aggregateByField(points, func(p matchedPoint) string { return p.HighwayClass })

	return persistSurfaceResults(ctx, db, activityID, surfaceBreakdowns, highwayBreakdowns)
}

// modeFilter applies a sliding-window majority vote of size surfaceModeWindowSize
// over the OsmID field. For each point, if more than half the window points
// share a single OsmID different from the current point's OsmID, the current
// point adopts the classification of the majority OsmID representative.
// Offroad points (nil OsmID) are left unchanged.
func modeFilter(points []matchedPoint) []matchedPoint {
	result := make([]matchedPoint, len(points))
	copy(result, points)

	half := surfaceModeWindowSize / 2

	for i := range points {
		// Offroad points are not candidates for reassignment.
		if points[i].OsmID == nil {
			continue
		}

		// Determine window bounds.
		lo := max(0, i-surfaceModeWindowSize/2)
		hi := min(len(points), i+surfaceModeWindowSize/2+1)

		// Count OsmID occurrences in the original window (skip nil OsmIDs).
		counts := make(map[int64]int)
		firstIdx := make(map[int64]int) // index of first occurrence for representative lookup
		for j := lo; j < hi; j++ {
			if points[j].OsmID == nil {
				continue
			}
			id := *points[j].OsmID
			counts[id]++
			if _, seen := firstIdx[id]; !seen {
				firstIdx[id] = j
			}
		}

		// Find the majority OsmID (strictly more than half).
		var majorityID int64
		majorityCount := 0
		for id, cnt := range counts {
			if cnt > majorityCount {
				majorityCount = cnt
				majorityID = id
			}
		}

		if majorityCount > half && majorityID != *points[i].OsmID {
			// Adopt classification from the representative of the majority OsmID.
			rep := points[firstIdx[majorityID]]
			result[i].OsmID = rep.OsmID
			result[i].Surface = rep.Surface
			result[i].Highway = rep.Highway
			result[i].DistM = rep.DistM
			result[i].SurfaceClass = rep.SurfaceClass
			result[i].HighwayClass = rep.HighwayClass
		}
	}

	return result
}

// aggregateByField accumulates length (each point = surfaceResampleM metres)
// per unique value returned by fieldFn, computes percentages, and returns
// the results sorted descending by LengthM.
func aggregateByField(points []matchedPoint, fieldFn func(matchedPoint) string) []SurfaceBreakdown {
	totals := make(map[string]float64)
	for _, p := range points {
		totals[fieldFn(p)] += surfaceResampleM
	}

	totalAll := float64(len(points)) * surfaceResampleM

	breakdowns := make([]SurfaceBreakdown, 0, len(totals))
	for typ, lengthM := range totals {
		pct := 0.0
		if totalAll > 0 {
			pct = lengthM / totalAll * 100.0
		}
		breakdowns = append(breakdowns, SurfaceBreakdown{
			Type:       typ,
			LengthM:    lengthM,
			Percentage: pct,
		})
	}

	slices.SortFunc(breakdowns, func(a, b SurfaceBreakdown) int {
		if b.LengthM > a.LengthM {
			return 1
		}
		if b.LengthM < a.LengthM {
			return -1
		}
		return 0
	})

	return breakdowns
}

// persistSurfaceResults deletes existing activity_surfaces rows for the
// activity and re-inserts them via a pgx.Batch. Surface breakdowns are
// written with category='surface' (surface_type filled, path_type empty);
// highway breakdowns are written with category='highway' (surface_type
// empty, path_type filled).
func persistSurfaceResults(
	ctx context.Context,
	db *database.DB,
	activityID string,
	surfaces []SurfaceBreakdown,
	highways []SurfaceBreakdown,
) error {
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_surfaces WHERE activity_id = $1",
		activityID,
	); err != nil {
		return fmt.Errorf("surfaces_sql: delete existing rows: %w", err)
	}

	batch := &pgx.Batch{}

	for _, s := range surfaces {
		batch.Queue(
			`INSERT INTO activity_surfaces
			    (activity_id, category, surface_type, path_type, length_m, percentage)
			VALUES ($1, $2, $3, '', $4, $5)`,
			activityID, "surface", s.Type, s.LengthM, s.Percentage,
		)
	}

	for _, h := range highways {
		batch.Queue(
			`INSERT INTO activity_surfaces
			    (activity_id, category, surface_type, path_type, length_m, percentage)
			VALUES ($1, 'highway', '', $2, $3, $4)`,
			activityID, h.Type, h.LengthM, h.Percentage,
		)
	}

	if batch.Len() > 0 {
		if err := db.Pool.SendBatch(ctx, batch).Close(); err != nil {
			return fmt.Errorf("surfaces_sql: batch insert: %w", err)
		}
	}

	slog.Info("surfaces_sql: enrichment complete",
		"activity_id", activityID,
		"surface_types", len(surfaces),
		"highway_types", len(highways),
	)
	return nil
}
