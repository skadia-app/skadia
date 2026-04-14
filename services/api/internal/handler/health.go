package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/auth"
	"github.com/skadia-app/skadia/services/api/internal/bestefforts"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/osm/coverage"
	"github.com/skadia-app/skadia/services/api/internal/pmc"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

type Handler struct {
	DB             *database.DB
	Events         *event.Publisher
	ShadowFS       shadowfs.ShadowFS        // nil when not yet initialized
	ShadowFSConfig *shadowfs.ShadowFSConfig // nil when shadow FS is not configured
	WatcherManager *shadowfs.WatcherManager // nil when per-user hotfolder is not configured
	Config         *HandlerConfig           // LLM/LiteLLM configuration
	ImportService  *importer.Service        // shared post-processing pipeline
	GraphManager   *routing.GraphManager    // nil when routing graph is not configured
	CoverageBaseDir string                       // base directory for FCOV coverage files
	CoverageReader  *coverage.DiskCoverageReader // nil when coverage binaries are not available
	PNGCache        *coverage.PNGCache           // LRU cache for rendered PNG tiles

	PMC          *pmc.Service          // TSS/PMC recomputation service
	BestEfforts  *bestefforts.Service  // best effort computation service
	recomputeJob recomputeJobState     // in-memory state for async recompute-all

	Settings *settings.SettingsStore // server settings store (DB-backed, cached)
	Mailer   *auth.Mailer            // for admin test-email endpoint
}

type HandlerConfig struct {
	LiteLLMConfigPath   string
	LiteLLMInternalURL  string
	LLMEncryptionKey    string
	MLServiceURL        string        // e.g. http://ml:8000
	MLCallbackSecret    string        // ML_CALLBACK_SECRET env var
	BRouterURL          string        // e.g. http://brouter:17777
	KomootEncryptionKey string        // 32-byte hex for AES-256-GCM Komoot password encryption
	RoutingGraphPath    string        // path to binary graph file
	RoutingTimeout      time.Duration // request timeout for route calculations (default 30s)
	MaxUploadSize       int64         // maximum upload size in bytes
	// Data directories
	OSMDataDir         string // directory for OSM PBF files (default /data/osm)
	DEMDataDir         string // directory for DEM/HGT files (default /data/dem)
	BRouterSegmentsDir string // directory for BRouter segment files (default /data/brouter/segments)
	BRouterProfilesDir string // directory for BRouter profile files (default /data/brouter/profiles)
	TilesDir           string // directory for offline PMTiles map files (default /data/tiles)
	// ImportAllowedDirs lists extra directories beyond /tmp that are trusted for
	// source-path imports (Garmin, Strava). Populated from IMPORT_ALLOWED_DIRS.
	ImportAllowedDirs []string

	// ShadowFS — used by rename handlers and admin dashboard
	LibraryBasePath string // e.g. /data/library (local backend only)
	StorageBackend  string // "local" or "s3"
	KeepVersions    int    // 0=delete, 1=keep last, -1=keep all
	SyncEnabled     bool
	LocalBE         shadowfs.StorageBackend // nil when no local backend
	S3BE            shadowfs.StorageBackend // nil when no S3 backend
	BucketFn        func(string) string     // fileType → S3 bucket
}

// Livez returns 200 if the process is alive. Does NOT check external dependencies.
// Used as K8s livenessProbe — checking DB here would cause restart cascades.
func (h *Handler) Livez(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readyz returns 200 if the service can handle traffic, 503 otherwise.
// Checks DB connectivity and NATS (if configured).
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// DB check — nil DB means not wired (tests) OR configuration error.
	// Either way, service is not ready to serve traffic.
	if h.DB == nil || h.DB.Pool == nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"db":     "not configured",
		})
		return
	}
	if err := h.DB.Ping(ctx); err != nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"db":     err.Error(),
		})
		return
	}

	// NATS check: h.Events is nil when NATS is not configured or unavailable.
	if h.Events != nil && !h.Events.IsConnected() {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"nats":   "disconnected",
		})
		return
	}

	// S3 check: when ShadowFS is configured with S3, verify the bucket is reachable.
	if usfs, ok := h.ShadowFS.(*shadowfs.UnifiedShadowFS); ok && usfs.S3Configured() {
		if err := usfs.PingS3(ctx); err != nil {
			httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "not ready",
				"s3":     err.Error(),
			})
			return
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// Healthz returns a human-readable status overview. Always 200 — use for dashboards, not probes.
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := map[string]string{"status": "ok", "db": "ok", "nats": "ok"}

	if h.DB == nil || h.DB.Pool == nil {
		status["status"] = "degraded"
		status["db"] = "not configured"
	} else if err := h.DB.Ping(ctx); err != nil {
		status["status"] = "degraded"
		status["db"] = err.Error()
	}
	if h.Events == nil {
		status["nats"] = "not configured"
	} else if !h.Events.IsConnected() {
		status["status"] = "degraded"
		status["nats"] = "disconnected"
	}

	if usfs, ok := h.ShadowFS.(*shadowfs.UnifiedShadowFS); ok && usfs.S3Configured() {
		if err := usfs.PingS3(ctx); err != nil {
			status["status"] = "degraded"
			status["s3"] = err.Error()
		} else {
			status["s3"] = "ok"
		}
	} else {
		status["s3"] = "not configured"
	}

	httputil.WriteJSON(w, http.StatusOK, status)
}

// importAllowedDirs returns the configured import allowed directories,
// or nil if Config is not initialised (e.g. in tests).
func (h *Handler) importAllowedDirs() []string {
	if h.Config == nil {
		return nil
	}
	return h.Config.ImportAllowedDirs
}
