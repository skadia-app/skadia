package capabilities

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Capabilities describes which routing and geo features are available
// based on the current infrastructure (Lite vs. Full mode).
type Capabilities struct {
	Routing         bool `json:"routing"`
	BRouter         bool `json:"brouter"`
	BadRouter       bool `json:"badrouter"`
	OfflineMap      bool `json:"offline_map"`
	SurfaceEnrich   bool `json:"surface_enrichment"`
	DEMCorrection   bool `json:"dem_correction"`
	Isochrone       bool `json:"isochrone"`
	RouteSuggestion bool `json:"route_suggestion"`
	MLService       bool `json:"ml_service"`
	// ImportPathEnabled is true when IMPORT_ALLOWED_DIRS is configured, allowing
	// users to specify local filesystem paths for Garmin/Strava imports instead
	// of being limited to ZIP uploads. False in default/cloud deployments.
	ImportPathEnabled bool `json:"import_path_enabled"`
}

const cacheTTL = 60 * time.Second

var (
	cacheMu      sync.Mutex
	cachedCaps   Capabilities
	cacheExpires time.Time
)

// InvalidateCache resets the capabilities cache so the next Detect call
// performs a fresh probe. Call this after significant infrastructure changes
// such as tile rebuilds or coverage builds.
func InvalidateCache() {
	cacheMu.Lock()
	cacheExpires = time.Time{}
	cacheMu.Unlock()
}

// Detect probes the database and BRouter to determine which features are
// available at runtime. All checks are best-effort — failures are logged
// and treated as "not available". Results are cached for 60 seconds.
//
// tileDir is the routing-tiles directory (BadRouter .bin files).
// pmTilesDir is the directory scanned for offline PMTiles map files.
func Detect(ctx context.Context, pool *pgxpool.Pool, brouterURL, tileDir, pmTilesDir string, importAllowedDirs []string) Capabilities {
	cacheMu.Lock()
	if time.Now().Before(cacheExpires) {
		cached := cachedCaps
		cacheMu.Unlock()
		return cached
	}
	cacheMu.Unlock()

	caps := Capabilities{}

	// Surface enrichment: check if osm_ways has data
	var hasWays bool
	if err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM osm_ways WHERE highway IS NOT NULL LIMIT 1)").Scan(&hasWays); err == nil && hasWays {
		caps.SurfaceEnrich = true
	}

	// BRouter reachable? Just check if the service responds — no routing request
	// with hardcoded coordinates, which would fail for non-European regions.
	if brouterURL != "" {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(brouterURL)
		if err == nil {
			resp.Body.Close()
			caps.BRouter = true
		} else {
			slog.Warn("BRouter not reachable", "error", err)
		}
	}

	// DEM raster tiles available?
	var hasDEM bool
	if err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM dem.raster_tiles LIMIT 1)").Scan(&hasDEM); err == nil && hasDEM {
		caps.DEMCorrection = true
	}

	// OfflineMap: PMTiles files present in configured tiles directory?
	if entries, err := os.ReadDir(pmTilesDir); err == nil {
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
					caps.BadRouter = true
					caps.Isochrone = true
					caps.RouteSuggestion = true
					break
				}
			}
		}
	}

	// Routing = at least one engine available
	caps.Routing = caps.BRouter || caps.BadRouter

	// ML Service availability (cached from background probe)
	caps.MLService = MLServiceAvailable()

	// ImportPathEnabled: true if at least one allowed import dir is configured
	// beyond /tmp (which is the secure default for multi-user deployments).
	tmpDir := os.TempDir()
	for _, dir := range importAllowedDirs {
		if dir != "" && filepath.Clean(dir) != filepath.Clean(tmpDir) {
			caps.ImportPathEnabled = true
			break
		}
	}

	cacheMu.Lock()
	cachedCaps = caps
	cacheExpires = time.Now().Add(cacheTTL)
	cacheMu.Unlock()

	return caps
}
