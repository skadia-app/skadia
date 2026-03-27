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
		tileIDs, err = discoverActiveTiles(ctx, pool, bboxClause)
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
