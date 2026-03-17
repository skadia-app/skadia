package graph

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
func BuildTile(ctx context.Context, pool *pgxpool.Pool, tileID uint32,
	outputDir string,
	progressFn func(step string, pct int)) (*TileBuildResult, error) {

	start := time.Now()
	tilePath := TileFilePath(outputDir, tileID)
	bbox := TileBBoxExpanded(tileID, BorderBuffer)

	g, err := Build(ctx, pool, "", &bbox, progressFn)
	if err != nil {
		return nil, fmt.Errorf("build tile %d: %w", tileID, err)
	}

	const borderDist = 0.0045
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

// BuildAllTiles discovers all tiles with OSM data and builds each one
// sequentially. Already-existing tile files are skipped. Memory is freed
// between tiles via GC + FreeOSMemory to stay within RAM limits.
func BuildAllTiles(ctx context.Context, pool *pgxpool.Pool, outputDir string,
	progressFn func(tileID uint32, step string, pct int)) ([]TileBuildResult, error) {

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", outputDir, err)
	}

	tileIDs, err := DiscoverTileIDs(ctx, pool)
	if err != nil {
		return nil, err
	}
	slog.Info("discovered tiles", "count", len(tileIDs))

	var results []TileBuildResult
	for i, tileID := range tileIDs {
		tilePath := TileFilePath(outputDir, tileID)
		if info, err := os.Stat(tilePath); err == nil && info.Size() > 0 {
			slog.Info("tile already exists, skipping", "tile_id", tileID, "path", tilePath)
			continue
		}
		slog.Info("building tile", "tile_id", tileID, "index", i+1, "total", len(tileIDs))
		result, err := BuildTile(ctx, pool, tileID, outputDir, func(step string, pct int) {
			if progressFn != nil {
				progressFn(tileID, step, pct)
			}
		})
		if err != nil {
			return results, fmt.Errorf("tile %d: %w", tileID, err)
		}
		results = append(results, *result)
		runtime.GC()
		debug.FreeOSMemory()
	}

	return results, nil
}
