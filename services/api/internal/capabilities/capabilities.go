package capabilities

import (
	"context"
	"log/slog"
	"net/http"
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
}

// Detect probes the database and BRouter to determine which features are
// available at runtime. All checks are best-effort — failures are logged
// and treated as "not available".
func Detect(ctx context.Context, pool *pgxpool.Pool, brouterURL string) Capabilities {
	caps := Capabilities{}

	// pgRouting extension present?
	var pgr int
	if err := pool.QueryRow(ctx,
		"SELECT 1 FROM pg_extension WHERE extname='pgrouting'").Scan(&pgr); err == nil {
		// Trail topology built (source column populated by pgr_createTopology)?
		var trailCount int
		if err := pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM osm_ways WHERE source IS NOT NULL").Scan(&trailCount); err == nil && trailCount > 0 {
			caps.Routing = true
			caps.Isochrone = true
			caps.SurfaceEnrich = true
		}
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

	// OfflineMap is available whenever routing is available (osm_ways imported).
	caps.OfflineMap = caps.Routing

	// Route suggestion requires full routing stack
	caps.RouteSuggestion = caps.Routing && caps.DEMCorrection && caps.SurfaceEnrich

	return caps
}
