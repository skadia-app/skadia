package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/auth"
	"github.com/skadia-app/skadia/services/api/internal/bestefforts"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/dem"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/osm/coverage"
	"github.com/skadia-app/skadia/services/api/internal/pmc"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

var errProbeNotConfigured = errors.New("not configured")

var (
	probeDBPing = func(ctx context.Context, h *Handler) error {
		if h == nil || h.DB == nil || h.DB.Pool == nil {
			return errProbeNotConfigured
		}
		return h.DB.Ping(ctx)
	}
	probeS3Ping = func(ctx context.Context, h *Handler) error {
		if h == nil {
			return nil
		}
		if usfs, ok := h.ShadowFS.(*shadowfs.UnifiedShadowFS); ok && usfs.S3Configured() {
			return usfs.PingS3(ctx)
		}
		return nil
	}
)

type Handler struct {
	DB          *database.DB
	Confirm     *agent.ConfirmationManager
	Agent       *agent.Agent
	LLMResolver LLMRoleResolver

	// Segregated publisher interfaces. Production wires all six fields to the
	// same *event.Publisher; tests can substitute a FakeInvalidator (or other
	// fake) per-method. Each may be nil when NATS is not configured — call
	// sites must guard with a nil-check before publishing.
	AnalyticsInvalidator      event.AnalyticsInvalidator
	ActivityUploadedPublisher event.ActivityUploadedPublisher
	Stage2ReadyPublisher      event.Stage2ReadyPublisher
	DocumentUploadedPublisher event.DocumentUploadedPublisher
	PhotoEmbedPublisher       event.PhotoEmbedPublisher
	EventConn                 event.EventConn

	ShadowFS           shadowfs.ShadowFS            // nil when not yet initialized
	ShadowFSConfig     *shadowfs.ShadowFSConfig     // nil when shadow FS is not configured
	WatcherManager     *shadowfs.WatcherManager     // nil when per-user hotfolder is not configured
	Config             *HandlerConfig               // LLM/LiteLLM configuration
	ImportService      *importer.Service            // shared post-processing pipeline
	AIBrokerHTTPClient *http.Client                 // optional test override; production uses SSRF-protected client
	GraphManager       *routing.GraphManager        // nil when routing graph is not configured
	CoverageBaseDir    string                       // base directory for FCOV coverage files
	CoverageReader     *coverage.DiskCoverageReader // nil when coverage binaries are not available
	PNGCache           *coverage.PNGCache           // LRU cache for rendered PNG tiles

	DEMSvc dem.Servicer // lazy mmap'd HGT service; nil when not configured

	PMC          *pmc.Service         // TSS/PMC recomputation service
	BestEfforts  *bestefforts.Service // best effort computation service
	recomputeJob recomputeJobState    // in-memory state for async recompute-all

	Settings *settings.SettingsStore // server settings store (DB-backed, cached)
	Mailer   *auth.Mailer            // for admin test-email endpoint

	// TxFactory is an optional override for transaction creation. When nil,
	// beginTx falls through to DB.Pool.Begin().
	//
	// Thread-safety: this field is NOT thread-safe. Production code never
	// writes it after Handler construction. Tests that set TxFactory must
	// use a dedicated Handler instance (not a shared one) to avoid races
	// with parallel goroutines. See activity_set_virtual_test.go for the
	// expected pattern.
	TxFactory TxFactory

	// Stage1Deps overrides the external dependencies (WeatherFetcher,
	// ReverseGeocoder) used by RunStage1 in CreateManualActivity. When nil,
	// production defaults are used (real HTTP weather + PostGIS geocoder).
	// Set only in integration tests to avoid network calls.
	Stage1Deps *enrichment.Stage1Deps
}

type HandlerConfig struct {
	DeploymentProfile           string
	DeploymentProfileSource     string
	StorageMode                 string
	LiteLLMConfigPath           string
	LiteLLMInternalURL          string
	LLMEncryptionKey            string
	LLMBaseURL                  string
	LLMAPIKey                   string
	LLMModel                    string
	LLMContextWindow            int
	MLServiceURL                string        // e.g. http://ml:8000
	MLCallbackSecret            string        // ML_CALLBACK_SECRET env var
	MLLLMBrokerToken            string        // ML_LLM_BROKER_TOKEN or ML_CALLBACK_SECRET fallback
	BRouterURL                  string        // e.g. http://brouter:17777
	KomootEncryptionKey         string        // 32-byte hex for AES-256-GCM Komoot password encryption
	OAuthEncryptionKey          string        // 32-byte hex for OAuth token encryption
	RoutingGraphPath            string        // path to binary graph file
	RoutingTimeout              time.Duration // request timeout for route calculations (default 30s)
	BadRouterProfilesKnown      bool
	BadRouterProfilesConfigured bool
	BadRouterProfilesAvailable  bool
	BadRouterProfilesReason     string
	MaxUploadSize               int64 // maximum upload size in bytes
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
	LibraryBasePath   string // e.g. /data/library (local backend only)
	StorageBackend    string // "local" or "s3"
	KeepVersions      int    // 0=delete, 1=keep last, -1=keep all
	SyncEnabled       bool
	LocalBE           shadowfs.StorageBackend // nil when no local backend
	S3BE              shadowfs.StorageBackend // nil when no S3 backend
	BucketFn          func(string) string     // fileType → S3 bucket
	FrontendStaticDir string                  // directory serving production frontend assets
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

	if err := probeDBPing(ctx, h); err != nil {
		dbState := publicProbeDependencyValue("db", err)
		if errors.Is(err, errProbeNotConfigured) {
			dbState = "not configured"
		}
		httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"db":     dbState,
		})
		return
	}

	// NATS check: h.EventConn is nil when NATS is not configured or unavailable.
	if h.EventConn != nil && !h.EventConn.IsConnected() {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"nats":   "disconnected",
		})
		return
	}

	if err := probeS3Ping(ctx, h); err != nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"s3":     publicProbeDependencyValue("s3", err),
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// Healthz returns a human-readable status overview. Always 200 — use for dashboards, not probes.
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := map[string]string{"status": "ok", "db": "ok", "nats": "ok"}

	if err := probeDBPing(ctx, h); errors.Is(err, errProbeNotConfigured) {
		status["status"] = "degraded"
		status["db"] = "not configured"
	} else if err != nil {
		status["status"] = "degraded"
		status["db"] = publicProbeDependencyValue("db", err)
	}
	if h.EventConn == nil {
		status["nats"] = "not configured"
	} else if !h.EventConn.IsConnected() {
		status["status"] = "degraded"
		status["nats"] = "disconnected"
	}

	if err := probeS3Ping(ctx, h); err != nil {
		status["status"] = "degraded"
		status["s3"] = publicProbeDependencyValue("s3", err)
	} else if usfs, ok := h.ShadowFS.(*shadowfs.UnifiedShadowFS); ok && usfs.S3Configured() {
		status["s3"] = "ok"
	} else {
		status["s3"] = "not configured"
	}

	// DEM service health — nil when not configured (e.g. in tests).
	demHealth := map[string]any{}
	if h.DEMSvc != nil {
		dh := h.DEMSvc.Health()
		if len(dh.CorruptCells) > 0 {
			status["status"] = "degraded"
			demHealth["corrupt_cells"] = dh.CorruptCells
		}
		if dh.Closed {
			demHealth["status"] = "closed"
		} else {
			demHealth["status"] = "ok"
		}
	} else {
		demHealth["status"] = "not configured"
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"status": status["status"],
		"db":     status["db"],
		"nats":   status["nats"],
		"s3":     status["s3"],
		"dem":    demHealth,
	})
}

func publicProbeDependencyValue(_ string, _ error) string {
	return "unreachable"
}

// importAllowedDirs returns the configured import allowed directories,
// or nil if Config is not initialised (e.g. in tests).
func (h *Handler) importAllowedDirs() []string {
	if h.Config == nil {
		return nil
	}
	return h.Config.ImportAllowedDirs
}
