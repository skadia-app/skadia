package coverage

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	graphpkg "github.com/skadia-app/skadia/services/api/internal/routing/graph"

	"github.com/jackc/pgx/v5/pgxpool"
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
	sem := make(chan struct{}, opts.Workers)

	type result struct {
		tileID uint32
		err    error
	}
	results := make(chan result, len(tileIDs))

	for _, tid := range tileIDs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
		}

		go func(tileID uint32) {
			defer func() { <-sem }()

			tg, ti, err := buildGridsForTile(ctx, pool, tileID, noiseCfg)
			if err != nil {
				results <- result{tileID, err}
				return
			}
			defer tg.Release()

			bbox := graphpkg.TileBBox(tileID)
			core := tileBBoxToBound(bbox)

			err = WriteTileGrids(tg, ti, opts.BaseDir, core, opts.Layer)
			if ti != nil {
				ti.release()
			}
			if err != nil {
				results <- result{tileID, err}
				return
			}

			n := done.Add(1)
			if progressFn != nil {
				progressFn(n, total)
			}
			results <- result{tileID, nil}
		}(tid)
	}

	var firstErr error
	for range tileIDs {
		r := <-results
		if r.err != nil && firstErr == nil {
			slog.Error("build-coverage: tile failed", "tileID", r.tileID, "error", r.err)
			firstErr = r.err
		}
	}

	if firstErr != nil {
		return firstErr
	}

	if err := WriteBuildTimestamp(opts.BaseDir); err != nil {
		return fmt.Errorf("write timestamp: %w", err)
	}

	slog.Info("build-coverage: complete", "tiles", total)
	return nil
}
