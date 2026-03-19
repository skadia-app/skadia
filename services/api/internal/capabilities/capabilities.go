package capabilities

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Capabilities describes which routing and geo features are available
// based on the current infrastructure (Lite vs. Full mode).
type Capabilities struct {
	Routing         bool `json:"routing"`
	OfflineMap      bool `json:"offline_map"`
	SurfaceEnrich   bool `json:"surface_enrichment"`
	DEMCorrection   bool `json:"dem_correction"`
	Isochrone       bool `json:"isochrone"`
	RouteSuggestion bool `json:"route_suggestion"`
	MLService       bool `json:"ml_service"`
}

// Detect probes the database and BRouter to determine which features are
// available at runtime. All checks are best-effort — failures are logged
// and treated as "not available".
func Detect(ctx context.Context, pool *pgxpool.Pool, brouterURL, tileDir string) Capabilities {
	caps := Capabilities{}

	// Routing capability: check if osm_ways has data (CSR graph builder reads from osm_ways directly).
	// pgr_createTopology check removed — pgRouting 4.0+ removed it, and BadRouter's CSR graph
	// builder computes topology in Go (routing/graph/topology.go), not via pgRouting.
	var wayCount int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM osm_ways WHERE highway IS NOT NULL").Scan(&wayCount); err == nil && wayCount > 0 {
		caps.Routing = true
		caps.SurfaceEnrich = true
	}

	// BRouter reachable?
	if brouterURL != "" {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(brouterURL + "/brouter?lonlats=11.39,47.27|11.40,47.28&profile=trekking&format=geojson")
		if err == nil {
			resp.Body.Close()
		} else {
			slog.Warn("BRouter not reachable", "error", err)
		}
	}

	// DEM raster tiles available?
	var demCount int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM dem.raster_tiles").Scan(&demCount); err == nil && demCount > 0 {
		caps.DEMCorrection = true
	}

	// OfflineMap: PMTiles files present in /data/tiles/?
	tilesDir := "/data/tiles"
	if entries, err := os.ReadDir(tilesDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".pmtiles") {
				caps.OfflineMap = true
				break
			}
		}
	}

	// Check if routing tiles exist (BadRouter tiled reader)
	if tileDir != "" {
		if entries, err := os.ReadDir(tileDir); err == nil {
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".bin") {
					caps.Isochrone = true
					caps.RouteSuggestion = true
					break
				}
			}
		}
	}

	// ML Service availability (cached from background probe)
	caps.MLService = MLServiceAvailable()

	return caps
}
