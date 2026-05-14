package capabilities

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFeatureStateLegacyBooleansRequireConfiguredAndAvailable(t *testing.T) {
	caps := Capabilities{
		Features: map[string]FeatureState{
			KeyBadRouter:       {Configured: true, Available: false, Reason: ReasonMissingProfiles},
			KeyBRouter:         {Configured: true, Available: true},
			KeyRouteSuggestion: {Configured: false, Available: false, Reason: ReasonCoreProfile},
		},
	}
	caps.ApplyLegacyBooleans()

	if caps.BadRouter {
		t.Fatal("BadRouter legacy bool true for configured-but-unavailable feature")
	}
	if !caps.BRouter {
		t.Fatal("BRouter legacy bool false for configured and available feature")
	}
	if caps.RouteSuggestion {
		t.Fatal("RouteSuggestion legacy bool true for unconfigured feature")
	}
}

func TestCapabilityCacheKeySeparatesProfileAndStorageMode(t *testing.T) {
	a := DetectInput{
		DeploymentProfile: "core",
		StorageMode:       "s3-local-garage",
		StorageBackend:    "s3",
		BRouterURL:        "http://brouter:17777",
		TileDir:           "/data/routing/tiles",
		PMTilesDir:        "/data/tiles",
		ImportAllowedDirs: []string{"/srv/imports"},
		MLProbeVersion:    3,
	}
	b := a
	b.DeploymentProfile = "full"
	if a.CacheKey() == b.CacheKey() {
		t.Fatal("cache key did not separate core and full profiles")
	}
	b = a
	b.StorageMode = "s3-external"
	if a.CacheKey() == b.CacheKey() {
		t.Fatal("cache key did not separate local Garage and external S3")
	}
	b = a
	b.StorageBackend = "local"
	if a.CacheKey() == b.CacheKey() {
		t.Fatal("cache key did not separate storage backend")
	}
}

func TestBuildFeatureStatesCoreBlocksFullOnlyCapabilities(t *testing.T) {
	runtime := RuntimeProbeState{
		BRouter:       true,
		BadRouter:     true,
		OfflineMap:    true,
		DEMCorrection: true,
		MLService:     true,
	}
	caps := BuildFromRuntime(DetectInput{
		DeploymentProfile:     "core",
		DeploymentProfileSrc:  "env",
		StorageMode:           "local",
		StorageBackend:        "local",
		ImportPathEnabledHint: true,
	}, runtime)

	assertFeature(t, caps, KeyBRouter, true, true, "")
	assertFeature(t, caps, KeyBRouterSegmentDownload, true, true, "")
	assertFeature(t, caps, KeyBadRouter, false, false, ReasonCoreProfile)
	assertFeature(t, caps, KeyOSMImport, false, false, ReasonCoreProfile)
	assertFeature(t, caps, KeyCoverageTiles, false, false, ReasonCoreProfile)
	assertFeature(t, caps, KeySegmentRecompute, false, false, ReasonCoreProfile)
	assertFeature(t, caps, KeySegmentPublish, false, false, ReasonCoreProfile)
	assertFeature(t, caps, KeyMLService, false, false, ReasonCoreProfile)
	assertFeature(t, caps, KeyAIAgent, true, true, "")
	if caps.BadRouter || caps.RouteSuggestion || caps.MLService {
		t.Fatalf("legacy full-only booleans leaked in core: %#v", caps)
	}
}

func TestBuildFeatureStatesFullReportsMissingArtifacts(t *testing.T) {
	caps := BuildFromRuntime(DetectInput{
		DeploymentProfile:    "full",
		DeploymentProfileSrc: "implicit_legacy",
		StorageMode:          "s3-external",
		StorageBackend:       "s3",
	}, RuntimeProbeState{})

	assertFeature(t, caps, KeyLibraryPaths, false, false, ReasonS3Storage)
	assertFeature(t, caps, KeyImportPathEnabled, false, false, ReasonS3Storage)
	assertFeature(t, caps, KeyBadRouter, true, false, ReasonMissingProfiles)
	assertFeature(t, caps, KeyCoverage, true, false, ReasonMissingArtifacts)
	assertFeature(t, caps, KeyMLService, true, false, ReasonMissingService)
	if caps.BadRouter || caps.Coverage || caps.MLService {
		t.Fatalf("legacy booleans should be false for missing full artifacts: %#v", caps)
	}
}

func TestDetectRuntimeFindsCoverageBinArtifacts(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "forest"), 0o755); err != nil {
		t.Fatalf("mkdir coverage layer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "forest", "47_8.bin"), []byte("fixture"), 0o644); err != nil {
		t.Fatalf("write coverage fixture: %v", err)
	}

	runtime := probeRuntime(t.Context(), nil, DetectInput{CoverageDir: dir})
	if !runtime.Coverage || !runtime.CoverageTiles {
		t.Fatalf("coverage runtime = %#v, want coverage and coverage tiles from nested .bin artifacts", runtime)
	}
}

func assertFeature(t *testing.T, caps Capabilities, key string, configured, available bool, reason string) {
	t.Helper()
	got, ok := caps.Features[key]
	if !ok {
		t.Fatalf("feature %q missing", key)
	}
	if got.Configured != configured || got.Available != available || got.Reason != reason {
		t.Fatalf("feature %q = %#v, want configured=%v available=%v reason=%q", key, got, configured, available, reason)
	}
}
