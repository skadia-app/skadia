package graph

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/skadia-app/skadia/services/api/internal/dem"
)

// TileBuildResult holds the outcome of building a single tile graph.
type TileBuildResult struct {
	TileID    uint32
	NodeCount uint32
	EdgeCount uint32
	FilePath  string
	Duration  time.Duration
}

// BuildTile constructs and serializes the graph for a single 1x1-degree tile.
// The tile is built with an expanded bounding box (BorderBuffer) to ensure
// border nodes overlap with neighboring tiles for cross-tile routing.
// A per-tile dem.Reader is created from hgtDirs and closed after use.
func BuildTile(ctx context.Context, pool *pgxpool.Pool, tileID uint32,
	outputDir string, hgtDirs []string,
	progressFn func(step string, pct int)) (*TileBuildResult, error) {

	start := time.Now()
	tilePath := TileFilePath(outputDir, tileID)
	bbox := TileBBoxExpanded(tileID, BorderBuffer)

	// Create DEM reader for this tile's bounding box.
	var demReader dem.Reader
	if len(hgtDirs) > 0 {
		var err error
		demReader, err = dem.NewReaderForTile(hgtDirs, tileID)
		if err != nil {
			return nil, fmt.Errorf("DEM reader for tile %d: %w", tileID, err)
		}
		defer demReader.Close()
	}

	g, err := Build(ctx, pool, "", &bbox, demReader, progressFn)
	if err != nil {
		return nil, fmt.Errorf("build tile %d: %w", tileID, err)
	}

	// borderDist must equal BorderBuffer so that every node in the overlap zone
	// between adjacent tiles is marked as a border node. A smaller value would
	// leave nodes in the 500m-2.2km band unmarked, breaking cross-tile matching.
	const borderDist = BorderBuffer
	borderIDs := IdentifyBorderNodes(tileID, g.NodeLat, g.NodeLon, borderDist)
	borderIdx := BuildBorderNodeIndex(borderIDs, g.NodeLat, g.NodeLon)

	g.IsBorder = make([]bool, g.NodeCount)
	for _, id := range borderIDs {
		g.IsBorder[id] = true
	}

	g.TileID = tileID
	g.BorderIndex = borderIdx

	if err := Serialize(g, tilePath); err != nil {
		return nil, fmt.Errorf("serialize tile %d: %w", tileID, err)
	}

	slog.Info("tile built",
		"tile_id", tileID,
		"nodes", g.NodeCount,
		"edges", g.EdgeCount,
		"border_nodes", len(borderIDs),
		"path", tilePath,
		"duration", time.Since(start),
	)

	return &TileBuildResult{
		TileID:    tileID,
		NodeCount: g.NodeCount,
		EdgeCount: g.EdgeCount,
		FilePath:  tilePath,
		Duration:  time.Since(start),
	}, nil
}

// DiscoverTileIDs queries the database for all 1x1-degree tiles that contain
// routing-eligible OSM ways. Returns tile IDs sorted ascending.
func DiscoverTileIDs(ctx context.Context, pool *pgxpool.Pool) ([]uint32, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT
			CAST((lat + 90) * 360 + (lon + 180) AS INTEGER) AS tile_id
		FROM osm_ways,
		     generate_series(FLOOR(ST_YMin(geom))::int, FLOOR(ST_YMax(geom))::int) AS lat,
		     generate_series(FLOOR(ST_XMin(geom))::int, FLOOR(ST_XMax(geom))::int) AS lon
		WHERE highway IS NOT NULL AND highway != 'construction'
		  AND (tags->>'area') IS DISTINCT FROM 'yes'
		ORDER BY tile_id`)
	if err != nil {
		return nil, fmt.Errorf("discover tiles: %w", err)
	}
	defer rows.Close()

	var ids []uint32
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan tile id: %w", err)
		}
		ids = append(ids, uint32(id))
	}
	return ids, rows.Err()
}

// BuildAllTiles discovers all tiles with OSM data and builds them in parallel
// using errgroup. Already-existing tile files are skipped. Memory is freed
// after each tile via GC + FreeOSMemory to stay within RAM limits.
//
// workers controls the degree of parallelism. If workers <= 0, it defaults to
// min(max(NumCPU/4, 1), 2).
func BuildAllTiles(ctx context.Context, pool *pgxpool.Pool, outputDir string,
	hgtDirs []string, workers int, filterBBox [4]float64, force bool,
	progressFn func(tileID uint32, step string, pct int)) ([]TileBuildResult, error) {

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", outputDir, err)
	}

	tileIDs, err := DiscoverTileIDs(ctx, pool)
	if err != nil {
		return nil, err
	}
	slog.Info("discovered tiles", "count", len(tileIDs))

	// Filter tiles by region BBox if provided.
	// filterBBox uses [minLon, minLat, maxLon, maxLat] convention (geographic order).
	// NOTE: TileBBox() returns [latMin, lonMin, latMax, lonMax] — different order!
	if filterBBox != [4]float64{} {
		var filtered []uint32
		minLon, minLat, maxLon, maxLat := filterBBox[0], filterBBox[1], filterBBox[2], filterBBox[3]
		for _, id := range tileIDs {
			tb := TileBBox(id) // [latMin, lonMin, latMax, lonMax]
			tLatMin, tLonMin, tLatMax, tLonMax := tb[0], tb[1], tb[2], tb[3]
			if tLonMax >= minLon && tLonMin <= maxLon && tLatMax >= minLat && tLatMin <= maxLat {
				filtered = append(filtered, id)
			}
		}
		slog.Info("tile build: filtered by region BBox", "total", len(tileIDs), "filtered", len(filtered))
		tileIDs = filtered
	}

	// Report total tile count for progress tracking.
	if progressFn != nil {
		progressFn(0, "discovering", len(tileIDs))
	}

	// Default worker count.
	if workers <= 0 {
		workers = max(runtime.NumCPU()/4, 1)
		if workers > 2 {
			workers = 2
		}
	}
	slog.Info("tile build parallelism", "workers", workers)

	var mu sync.Mutex
	var results []TileBuildResult

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for i, tileID := range tileIDs {
		tilePath := TileFilePath(outputDir, tileID)

		if info, err := os.Stat(tilePath); err == nil && info.Size() > 0 {
			if !force {
				slog.Info("tile already exists, skipping", "tile_id", tileID, "path", tilePath)
				if progressFn != nil {
					progressFn(tileID, fmt.Sprintf("tile %d/%d", i+1, len(tileIDs)), 100)
				}
				continue
			}
			os.Remove(tilePath)
		}

		g.Go(func() error {
			slog.Info("building tile", "tile_id", tileID, "index", i+1, "total", len(tileIDs))
			result, err := BuildTile(gCtx, pool, tileID, outputDir, hgtDirs,
				func(step string, pct int) {
					if progressFn != nil {
						progressFn(tileID, fmt.Sprintf("tile %d/%d", i+1, len(tileIDs)), pct)
					}
				})
			if err != nil {
				return fmt.Errorf("tile %d: %w", tileID, err)
			}

			mu.Lock()
			results = append(results, *result)
			mu.Unlock()

			runtime.GC()
			debug.FreeOSMemory()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return results, err
	}

	return results, nil
}
