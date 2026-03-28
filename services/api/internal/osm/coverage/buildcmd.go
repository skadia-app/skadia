package coverage

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	graphpkg "github.com/skadia-app/skadia/services/api/internal/routing/graph"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

// BuildCoverageOptions configures the build-coverage command.
type BuildCoverageOptions struct {
	BaseDir    string
	Layer      string
	Region     string    // region name (e.g. austria, germany, switzerland) — informational only
	RegionBBox [4]float64 // [minLat, minLon, maxLat, maxLon]; used only when Region != ""
	TileLat    int
	TileLon    int
	SingleTile bool
	Workers    int
}

// RunBuildCoverage builds coverage binary files for all (or filtered) tiles.
// If opts.Region is non-empty, opts.RegionBBox must be set to the region bounding box
// [minLat, minLon, maxLat, maxLon] by the caller (to avoid import cycles).
func RunBuildCoverage(ctx context.Context, pool *pgxpool.Pool, opts BuildCoverageOptions,
	progressFn func(current, total int64)) error {

	if opts.Workers <= 0 {
		opts.Workers = 4
	}

	var tileIDs []uint32
	if opts.SingleTile {
		tileIDs = []uint32{graphpkg.TileIDFromCoord(float64(opts.TileLat)+0.5, float64(opts.TileLon)+0.5)}
	} else {
		bboxClause := ""
		if opts.Region != "" {
			bbox := opts.RegionBBox
			bboxClause = fmt.Sprintf(
				" AND ST_Intersects(geom, ST_MakeEnvelope(%f, %f, %f, %f, 4326))",
				bbox[1], bbox[0], bbox[3], bbox[2])
		}

		var err error
		tileIDs, err = discoverCoverageTiles(ctx, pool, bboxClause)
		if err != nil {
			return fmt.Errorf("discover tiles: %w", err)
		}
	}

	total := int64(len(tileIDs))
	if progressFn != nil {
		progressFn(0, total)
	}
	slog.Info("build-coverage: discovered tiles", "count", total)

	noiseCfg := DefaultNoiseSettings()
	var done atomic.Int64
	var firstErr atomic.Value // stores first error for logging

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Workers)

	for _, tid := range tileIDs {
		g.Go(func() error {
			tg, ti, err := buildGridsForTile(gctx, pool, tid, noiseCfg)
			if err != nil {
				if firstErr.CompareAndSwap(nil, err) {
					slog.Error("build-coverage: tile failed", "tileID", tid, "error", err)
				}
				return err
			}
			defer tg.Release()

			bbox := graphpkg.TileBBox(tid)
			core := tileBBoxToBound(bbox)

			err = WriteTileGrids(tg, ti, opts.BaseDir, core, opts.Layer)
			if ti != nil {
				ti.release()
			}
			if err != nil {
				return err
			}

			n := done.Add(1)
			if progressFn != nil {
				progressFn(n, total)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	if err := WriteBuildTimestamp(opts.BaseDir); err != nil {
		return fmt.Errorf("write timestamp: %w", err)
	}

	slog.Info("build-coverage: complete", "tiles", total)
	return nil
}

// discoverCoverageTiles finds all 1°×1° tiles that have ANY OSM data relevant
// for coverage grids. Unlike discoverActiveTiles (which only checks osm_ways),
// this queries all tables used by the coverage builder: forests, waters,
// waterways, landuse, boundaries, pois, railways, and ways.
func discoverCoverageTiles(ctx context.Context, pool *pgxpool.Pool, bboxClause string) ([]uint32, error) {
	type tableSpec struct {
		table string
		where string // extra WHERE conditions (appended with AND)
	}
	tables := []tableSpec{
		{"osm_ways", "highway IS NOT NULL"},
		{"osm_forests", ""},
		{"osm_waters", ""},
		{"osm_waterways", ""},
		{"osm_landuse", ""},
		{"osm_boundaries", ""},
		{"osm_pois", ""},
		{"osm_railways", ""},
	}

	var unions []string
	for _, t := range tables {
		extra := ""
		if t.where != "" {
			extra = " AND " + t.where
		}
		where := "WHERE geom IS NOT NULL" + extra + bboxClause
		// Centroid — covers point/polygon features
		unions = append(unions, fmt.Sprintf(
			"SELECT DISTINCT %s AS tid FROM %s %s",
			tileExprPoint("ST_PointOnSurface(geom)"), t.table, where))
		// SW corner of bounding box — catches start of linear features
		unions = append(unions, fmt.Sprintf(
			"SELECT DISTINCT %s AS tid FROM %s %s",
			tileExprPoint("ST_MakePoint(ST_XMin(geom), ST_YMin(geom))"), t.table, where))
		// NE corner of bounding box — catches end of linear features
		unions = append(unions, fmt.Sprintf(
			"SELECT DISTINCT %s AS tid FROM %s %s",
			tileExprPoint("ST_MakePoint(ST_XMax(geom), ST_YMax(geom))"), t.table, where))
	}

	q := "SELECT DISTINCT tid FROM (" + joinSQL(unions) + ") sub"
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tiles []uint32
	for rows.Next() {
		var tid int
		if err := rows.Scan(&tid); err != nil {
			return nil, err
		}
		tiles = append(tiles, uint32(tid))
	}
	return tiles, rows.Err()
}

func joinSQL(parts []string) string {
	result := parts[0]
	for _, p := range parts[1:] {
		result += " UNION ALL " + p
	}
	return result
}

// tileExprPoint builds the SQL expression that converts a point expression
// into a 1°×1° tile ID using the formula (floor(lat)+90)*360 + (floor(lon)+180).
func tileExprPoint(pointExpr string) string {
	return fmt.Sprintf("((floor(ST_Y(%s))::int + 90) * 360 + (floor(ST_X(%s))::int + 180))::int",
		pointExpr, pointExpr)
}
