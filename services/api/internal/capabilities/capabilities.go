package capabilities

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	KeyDeploymentProfile           = "deployment_profile"
	KeyStorageMode                 = "storage_mode"
	KeyStorageBackend              = "storage_backend"
	KeyLibraryPaths                = "library_paths"
	KeyLibrarySchemas              = "library_schemas"
	KeyLibrarySync                 = "library_sync"
	KeyImportPathEnabled           = "import_path_enabled"
	KeyDirectUploadLifecycle       = "direct_upload_lifecycle"
	KeyRoutePhotoMedia             = "route_photo_media"
	KeyGDPRArtifacts               = "gdpr_artifacts"
	KeyOSMImport                   = "osm_import"
	KeyOSMEnrichment               = "osm_enrichment"
	KeyBRouter                     = "brouter"
	KeyBRouterSegmentDownload      = "brouter_segment_download"
	KeyBRouterSegmentBuild         = "brouter_segment_build"
	KeyBadRouter                   = "badrouter"
	KeyRouting                     = "routing"
	KeyRoutingTiles                = "routing_tiles"
	KeyDEMCorrection               = "dem_correction"
	KeyCoverage                    = "coverage"
	KeyCoverageTiles               = "coverage_tiles"
	KeySystemLayersVector          = "system_layers_vector"
	KeySystemLayersCoverage        = "system_layers_coverage"
	KeyOfflineMap                  = "offline_map"
	KeyGeoPOIEnrichment            = "geo_poi_enrichment"
	KeySurfaceEnrichment           = "surface_enrichment"
	KeyActivityTrackSummaryGeo     = "activity_track_summary_geo"
	KeyActivityTrackSummarySurface = "activity_track_summary_surface"
	KeySegments                    = "segments"
	KeySegmentRecompute            = "segment_recompute"
	KeySegmentPublish              = "segment_publish"
	KeySegmentOSMLiftImport        = "segment_osm_lift_import"
	KeySegmentEnrichmentSuggest    = "segment_enrichment_suggest"
	KeyOSMLiftSnap                 = "osm_lift_snap"
	KeyIsochrone                   = "isochrone"
	KeyRouteSuggestion             = "route_suggestion"
	KeyLocalPOISearch              = "local_poi_search"
	KeyMLService                   = "ml_service"
	KeyDocumentML                  = "document_ml"
	KeyPhotoEmbeddings             = "photo_embeddings"
	KeyPhotoEmbeddingStorageAccess = "photo_embedding_storage_access"
	KeyAIAgent                     = "ai_agent"

	ReasonCoreProfile        = "core_profile"
	ReasonMissingArtifacts   = "missing_artifacts"
	ReasonMissingProfiles    = "missing_profiles"
	ReasonMissingService     = "missing_service"
	ReasonStorageUnavailable = "storage_unavailable"
	ReasonS3Storage          = "s3_storage"
	ReasonNotConfigured      = "not_configured"
)

var canonicalKeys = []string{
	KeyDeploymentProfile,
	KeyStorageMode,
	KeyStorageBackend,
	KeyLibraryPaths,
	KeyLibrarySchemas,
	KeyLibrarySync,
	KeyImportPathEnabled,
	KeyDirectUploadLifecycle,
	KeyRoutePhotoMedia,
	KeyGDPRArtifacts,
	KeyOSMImport,
	KeyOSMEnrichment,
	KeyBRouter,
	KeyBRouterSegmentDownload,
	KeyBRouterSegmentBuild,
	KeyBadRouter,
	KeyRouting,
	KeyRoutingTiles,
	KeyDEMCorrection,
	KeyCoverage,
	KeyCoverageTiles,
	KeySystemLayersVector,
	KeySystemLayersCoverage,
	KeyOfflineMap,
	KeyGeoPOIEnrichment,
	KeySurfaceEnrichment,
	KeyActivityTrackSummaryGeo,
	KeyActivityTrackSummarySurface,
	KeySegments,
	KeySegmentRecompute,
	KeySegmentPublish,
	KeySegmentOSMLiftImport,
	KeySegmentEnrichmentSuggest,
	KeyOSMLiftSnap,
	KeyIsochrone,
	KeyRouteSuggestion,
	KeyLocalPOISearch,
	KeyMLService,
	KeyDocumentML,
	KeyPhotoEmbeddings,
	KeyPhotoEmbeddingStorageAccess,
	KeyAIAgent,
}

var validKeys = func() map[string]struct{} {
	out := make(map[string]struct{}, len(canonicalKeys))
	for _, key := range canonicalKeys {
		out[key] = struct{}{}
	}
	return out
}()

// FeatureState is the canonical capability contract. configured says the
// deployment profile/storage mode intends to expose the feature; available says
// the runtime artifacts or services needed for it are present right now.
type FeatureState struct {
	Configured bool   `json:"configured"`
	Available  bool   `json:"available"`
	Reason     string `json:"reason,omitempty"`
}

// Capabilities describes deployment, storage, feature states, and legacy
// boolean fields kept for older frontend callers.
type Capabilities struct {
	DeploymentProfile       string                  `json:"deployment_profile"`
	DeploymentProfileSource string                  `json:"deployment_profile_source"`
	StorageMode             string                  `json:"storage_mode"`
	StorageBackend          string                  `json:"storage_backend"`
	Features                map[string]FeatureState `json:"features"`

	Routing                     bool `json:"routing"`
	BRouter                     bool `json:"brouter"`
	BadRouter                   bool `json:"badrouter"`
	OfflineMap                  bool `json:"offline_map"`
	SurfaceEnrich               bool `json:"surface_enrichment"`
	DEMCorrection               bool `json:"dem_correction"`
	Isochrone                   bool `json:"isochrone"`
	RouteSuggestion             bool `json:"route_suggestion"`
	MLService                   bool `json:"ml_service"`
	ImportPathEnabled           bool `json:"import_path_enabled"`
	Coverage                    bool `json:"coverage"`
	CoverageTiles               bool `json:"coverage_tiles"`
	PhotoEmbeddings             bool `json:"photo_embeddings"`
	PhotoEmbeddingStorageAccess bool `json:"photo_embedding_storage_access"`
}

// RuntimeProbeState contains runtime artifact/service checks before profile and
// storage policy are applied.
type RuntimeProbeState struct {
	SurfaceEnrich      bool
	BRouter            bool
	BadRouter          bool
	BadRouterTiles     bool
	BadRouterProfiles  bool
	DEMCorrection      bool
	OfflineMap         bool
	Coverage           bool
	CoverageTiles      bool
	MLService          bool
	MLLocalStorageRead bool
	MLS3StorageRead    bool
}

type DetectInput struct {
	DeploymentProfile           string
	DeploymentProfileSrc        string
	StorageMode                 string
	StorageBackend              string
	BRouterURL                  string
	TileDir                     string
	PMTilesDir                  string
	CoverageDir                 string
	BadRouterProfilesKnown      bool
	BadRouterProfilesConfigured bool
	BadRouterProfilesAvailable  bool
	BadRouterProfilesReason     string
	ImportAllowedDirs           []string
	ImportPathEnabledHint       bool
	MLProbeVersion              uint64
}

func (in DetectInput) normalized() DetectInput {
	if in.DeploymentProfile == "" {
		in.DeploymentProfile = "full"
	}
	if in.DeploymentProfileSrc == "" {
		in.DeploymentProfileSrc = "implicit_legacy"
	}
	if in.StorageBackend == "" {
		in.StorageBackend = "local"
	}
	if in.StorageMode == "" {
		if in.StorageBackend == "s3" {
			in.StorageMode = "s3-external"
		} else {
			in.StorageMode = "local"
		}
	}
	return in
}

func (in DetectInput) CacheKey() string {
	in = in.normalized()
	parts := []string{
		"profile=" + in.DeploymentProfile,
		"profile_source=" + in.DeploymentProfileSrc,
		"storage_mode=" + in.StorageMode,
		"storage_backend=" + in.StorageBackend,
		"brouter_url=" + in.BRouterURL,
		"tile_dir=" + in.TileDir,
		"pmtiles_dir=" + in.PMTilesDir,
		"coverage_dir=" + in.CoverageDir,
		"badrouter_profiles_known=" + strconvFormatBool(in.BadRouterProfilesKnown),
		"badrouter_profiles_configured=" + strconvFormatBool(in.BadRouterProfilesConfigured),
		"badrouter_profiles_available=" + strconvFormatBool(in.BadRouterProfilesAvailable),
		"badrouter_profiles_reason=" + in.BadRouterProfilesReason,
		"ml_probe=" + strconvFormatUint(in.MLProbeVersion),
	}
	dirs := append([]string(nil), in.ImportAllowedDirs...)
	sort.Strings(dirs)
	parts = append(parts, "import_dirs="+strings.Join(dirs, "\x1f"))
	if in.ImportPathEnabledHint {
		parts = append(parts, "import_path_hint=true")
	}
	return strings.Join(parts, "\x1e")
}

func strconvFormatUint(v uint64) string {
	if v == 0 {
		return "0"
	}
	const digits = "0123456789"
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = digits[v%10]
		v /= 10
	}
	return string(buf[i:])
}

func strconvFormatBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func ValidKey(key string) bool {
	_, ok := validKeys[key]
	return ok
}

func ValidKeysSorted() []string {
	out := append([]string(nil), canonicalKeys...)
	sort.Strings(out)
	return out
}

func (c *Capabilities) ApplyLegacyBooleans() {
	c.Routing = c.FeatureEnabled(KeyRouting)
	c.BRouter = c.FeatureEnabled(KeyBRouter)
	c.BadRouter = c.FeatureEnabled(KeyBadRouter)
	c.OfflineMap = c.FeatureEnabled(KeyOfflineMap)
	c.SurfaceEnrich = c.FeatureEnabled(KeySurfaceEnrichment)
	c.DEMCorrection = c.FeatureEnabled(KeyDEMCorrection)
	c.Isochrone = c.FeatureEnabled(KeyIsochrone)
	c.RouteSuggestion = c.FeatureEnabled(KeyRouteSuggestion)
	c.MLService = c.FeatureEnabled(KeyMLService)
	c.ImportPathEnabled = c.FeatureEnabled(KeyImportPathEnabled)
	c.Coverage = c.FeatureEnabled(KeyCoverage)
	c.CoverageTiles = c.FeatureEnabled(KeyCoverageTiles)
	c.PhotoEmbeddings = c.FeatureEnabled(KeyPhotoEmbeddings)
	c.PhotoEmbeddingStorageAccess = c.FeatureEnabled(KeyPhotoEmbeddingStorageAccess)
}

func (c Capabilities) FeatureEnabled(key string) bool {
	state, ok := c.Features[key]
	return ok && state.Configured && state.Available
}

const cacheTTL = 60 * time.Second

var (
	cacheMu sync.Mutex
	cache   = map[string]cachedCapabilities{}
)

type cachedCapabilities struct {
	caps    Capabilities
	expires time.Time
}

func InvalidateCache() {
	cacheMu.Lock()
	cache = map[string]cachedCapabilities{}
	cacheMu.Unlock()
}

func Detect(ctx context.Context, pool *pgxpool.Pool, input DetectInput) Capabilities {
	input = input.normalized()
	key := input.CacheKey()
	cacheMu.Lock()
	if cached, ok := cache[key]; ok && time.Now().Before(cached.expires) {
		caps := cached.caps
		cacheMu.Unlock()
		return caps
	}
	cacheMu.Unlock()

	runtime := probeRuntime(ctx, pool, input)
	caps := BuildFromRuntime(input, runtime)

	cacheMu.Lock()
	cache[key] = cachedCapabilities{caps: caps, expires: time.Now().Add(cacheTTL)}
	cacheMu.Unlock()
	return caps
}

func probeRuntime(ctx context.Context, pool *pgxpool.Pool, input DetectInput) RuntimeProbeState {
	var runtime RuntimeProbeState
	if pool != nil {
		var hasWays bool
		if err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM osm_ways WHERE highway IS NOT NULL LIMIT 1)").Scan(&hasWays); err == nil && hasWays {
			runtime.SurfaceEnrich = true
		}
		var hasDEM bool
		if err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM dem.raster_tiles LIMIT 1)").Scan(&hasDEM); err == nil && hasDEM {
			runtime.DEMCorrection = true
		}
	}

	if input.BRouterURL != "" {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(input.BRouterURL)
		if err == nil {
			resp.Body.Close()
			if brouterProbeStatusUsable(resp.StatusCode) {
				runtime.BRouter = true
			} else {
				slog.Warn("BRouter probe returned unusable status", "status", resp.StatusCode)
			}
		} else {
			slog.Warn("BRouter not reachable", "error", err)
		}
	}
	if hasFileWithSuffix(input.PMTilesDir, ".pmtiles") {
		runtime.OfflineMap = true
	}
	if hasBadRouterTileFile(input.TileDir) {
		runtime.BadRouterTiles = true
	}
	if !input.BadRouterProfilesKnown || input.BadRouterProfilesAvailable {
		runtime.BadRouterProfiles = true
	}
	if runtime.BadRouterTiles && runtime.BadRouterProfiles {
		runtime.BadRouter = true
	}
	if input.CoverageDir != "" && hasFileWithSuffixRecursive(input.CoverageDir, ".bin", 2) {
		runtime.Coverage = true
		runtime.CoverageTiles = true
	}
	runtime.MLService = MLServiceAvailable()
	runtime.MLLocalStorageRead = MLLocalStorageReadAvailable()
	runtime.MLS3StorageRead = MLS3StorageReadAvailable()
	return runtime
}

func brouterProbeStatusUsable(status int) bool {
	return (status >= http.StatusOK && status < http.StatusMultipleChoices) || status == http.StatusNotFound
}

func hasFileWithSuffix(dir, suffix string) bool {
	if dir == "" {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			return true
		}
	}
	return false
}

func hasBadRouterTileFile(dir string) bool {
	if dir == "" {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "tile_") || !strings.HasSuffix(name, ".bin") {
			continue
		}
		info, err := e.Info()
		if err == nil && info.Size() > 0 {
			return true
		}
	}
	return false
}

func hasFileWithSuffixRecursive(dir, suffix string, maxDepth int) bool {
	if dir == "" || maxDepth < 0 {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			if hasFileWithSuffixRecursive(filepath.Join(dir, e.Name()), suffix, maxDepth-1) {
				return true
			}
			continue
		}
		if strings.HasSuffix(e.Name(), suffix) {
			return true
		}
	}
	return false
}

func BuildFromRuntime(input DetectInput, runtime RuntimeProbeState) Capabilities {
	input = input.normalized()
	features := make(map[string]FeatureState, len(canonicalKeys))
	set := func(key string, configured, available bool, reason string) {
		if configured && available {
			reason = ""
		}
		features[key] = FeatureState{Configured: configured, Available: configured && available, Reason: reason}
	}
	localStorage := input.StorageBackend == "local" && input.StorageMode == "local"
	core := input.DeploymentProfile == "core"
	importPathConfigured := localStorage && (input.ImportPathEnabledHint || importPathEnabled(input.ImportAllowedDirs))
	badRouterTilesAvailable := runtime.BadRouterTiles || runtime.BadRouter
	badRouterProfilesAvailable := runtime.BadRouterProfiles || runtime.BadRouter
	badRouterAvailable := runtime.BadRouter || (badRouterTilesAvailable && badRouterProfilesAvailable)
	badRouterReason := ReasonMissingProfiles
	if !badRouterProfilesAvailable {
		badRouterReason = ReasonMissingProfiles
		if input.BadRouterProfilesReason != "" {
			badRouterReason = input.BadRouterProfilesReason
		}
	} else if !badRouterTilesAvailable {
		badRouterReason = ReasonMissingArtifacts
	}
	routingAvailable := runtime.BRouter || (!core && badRouterAvailable)
	routingReason := reasonMissing(routingAvailable, ReasonMissingService)
	if !runtime.BRouter && !core {
		routingReason = badRouterReason
	}

	set(KeyDeploymentProfile, true, true, "")
	set(KeyStorageMode, true, true, "")
	set(KeyStorageBackend, true, true, "")
	set(KeyLibraryPaths, localStorage, localStorage, reasonIf(!localStorage, ReasonS3Storage))
	set(KeyLibrarySchemas, localStorage, localStorage, reasonIf(!localStorage, ReasonS3Storage))
	set(KeyLibrarySync, localStorage, localStorage, reasonIf(!localStorage, ReasonS3Storage))
	set(KeyImportPathEnabled, importPathConfigured, importPathConfigured, reasonIf(!localStorage, ReasonS3Storage))
	set(KeyDirectUploadLifecycle, true, true, "")
	set(KeyRoutePhotoMedia, true, true, "")
	set(KeyGDPRArtifacts, true, true, "")
	set(KeyBRouter, true, runtime.BRouter, reasonMissing(runtime.BRouter, ReasonMissingService))
	set(KeyBRouterSegmentDownload, true, true, "")
	set(KeyRouting, true, routingAvailable, routingReason)
	set(KeySegments, true, true, "")
	set(KeyAIAgent, true, true, "")

	mlStorageReadAvailable := false
	switch input.StorageBackend {
	case "local":
		mlStorageReadAvailable = runtime.MLLocalStorageRead
	case "s3":
		mlStorageReadAvailable = runtime.MLS3StorageRead
	}
	mlStorageReadReason := ReasonStorageUnavailable
	if !runtime.MLService {
		mlStorageReadReason = ReasonMissingService
	}
	set(KeyPhotoEmbeddingStorageAccess, true, runtime.MLService && mlStorageReadAvailable, mlStorageReadReason)

	fullOnly := []struct {
		key       string
		available bool
		reason    string
	}{
		{KeyOSMImport, true, ""},
		{KeyOSMEnrichment, runtime.SurfaceEnrich, ReasonMissingArtifacts},
		{KeyBRouterSegmentBuild, true, ""},
		{KeyBadRouter, badRouterAvailable, badRouterReason},
		{KeyRoutingTiles, badRouterTilesAvailable, ReasonMissingArtifacts},
		{KeyDEMCorrection, runtime.DEMCorrection, ReasonMissingArtifacts},
		{KeyCoverage, runtime.Coverage, ReasonMissingArtifacts},
		{KeyCoverageTiles, runtime.CoverageTiles, ReasonMissingArtifacts},
		{KeySystemLayersVector, runtime.SurfaceEnrich, ReasonMissingArtifacts},
		{KeySystemLayersCoverage, runtime.Coverage, ReasonMissingArtifacts},
		{KeyOfflineMap, runtime.OfflineMap, ReasonMissingArtifacts},
		{KeyGeoPOIEnrichment, runtime.SurfaceEnrich, ReasonMissingArtifacts},
		{KeySurfaceEnrichment, runtime.SurfaceEnrich, ReasonMissingArtifacts},
		{KeyActivityTrackSummaryGeo, runtime.SurfaceEnrich, ReasonMissingArtifacts},
		{KeyActivityTrackSummarySurface, runtime.SurfaceEnrich, ReasonMissingArtifacts},
		{KeySegmentRecompute, runtime.SurfaceEnrich, ReasonMissingArtifacts},
		{KeySegmentPublish, runtime.SurfaceEnrich, ReasonMissingArtifacts},
		{KeySegmentOSMLiftImport, runtime.SurfaceEnrich, ReasonMissingArtifacts},
		{KeySegmentEnrichmentSuggest, runtime.SurfaceEnrich && badRouterAvailable, ReasonMissingArtifacts},
		{KeyOSMLiftSnap, runtime.SurfaceEnrich, ReasonMissingArtifacts},
		{KeyIsochrone, badRouterAvailable, badRouterReason},
		{KeyRouteSuggestion, badRouterAvailable, badRouterReason},
		{KeyLocalPOISearch, runtime.SurfaceEnrich, ReasonMissingArtifacts},
		{KeyMLService, runtime.MLService, ReasonMissingService},
		{KeyDocumentML, runtime.MLService && mlStorageReadAvailable, mlStorageReadReason},
		{KeyPhotoEmbeddings, runtime.MLService && mlStorageReadAvailable, mlStorageReadReason},
	}
	for _, item := range fullOnly {
		if core {
			set(item.key, false, false, ReasonCoreProfile)
		} else {
			set(item.key, true, item.available, item.reason)
		}
	}

	caps := Capabilities{
		DeploymentProfile:       input.DeploymentProfile,
		DeploymentProfileSource: input.DeploymentProfileSrc,
		StorageMode:             input.StorageMode,
		StorageBackend:          input.StorageBackend,
		Features:                features,
	}
	caps.ApplyLegacyBooleans()
	return caps
}

func reasonIf(cond bool, reason string) string {
	if cond {
		return reason
	}
	return ""
}

func reasonMissing(available bool, reason string) string {
	if available {
		return ""
	}
	return reason
}

func importPathEnabled(importAllowedDirs []string) bool {
	tmpDir := os.TempDir()
	for _, dir := range importAllowedDirs {
		if dir != "" && filepath.Clean(dir) != filepath.Clean(tmpDir) {
			return true
		}
	}
	return false
}
