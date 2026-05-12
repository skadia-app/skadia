package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
)

func TestBuildGDPRArtifactStoreAutoFollowsStorageBackend(t *testing.T) {
	exportsBase, importsBase := privateArtifactDirs(t)
	cfg := &config.Config{
		StorageBackend:      "local",
		GDPRArtifactBackend: "auto",
		ExportsBasePath:     exportsBase,
		ImportsBasePath:     importsBase,
	}
	store, err := buildGDPRArtifactStore(cfg)
	if err != nil {
		t.Fatalf("buildGDPRArtifactStore(local auto): %v", err)
	}
	if _, ok := store.(*gdprartifacts.LocalStore); !ok {
		t.Fatalf("auto/local store = %T, want *gdprartifacts.LocalStore", store)
	}

	cfg.StorageBackend = "s3"
	cfg.S3Endpoint = "http://localhost:3900"
	cfg.S3AccessKey = "key"
	cfg.S3SecretKey = "secret"
	cfg.S3Region = "garage"
	cfg.S3BucketExports = "skadia-exports"
	cfg.S3BucketImports = "skadia-imports"
	cfg.S3ExportsPrefix = "gdpr/exports"
	cfg.S3ImportsPrefix = "gdpr/imports"
	store, err = buildGDPRArtifactStore(cfg)
	if err != nil {
		t.Fatalf("buildGDPRArtifactStore(s3 auto): %v", err)
	}
	if _, ok := store.(*gdprartifacts.S3Store); !ok {
		t.Fatalf("auto/s3 store = %T, want *gdprartifacts.S3Store", store)
	}
}

func TestBuildGDPRArtifactStoreRejectsS3WithoutCredentials(t *testing.T) {
	cfg := &config.Config{
		StorageBackend:      "local",
		GDPRArtifactBackend: "s3",
		S3Endpoint:          "http://localhost:3900",
		S3BucketExports:     "skadia-exports",
		S3BucketImports:     "skadia-imports",
		S3ExportsPrefix:     "gdpr/exports",
		S3ImportsPrefix:     "gdpr/imports",
	}
	if _, err := buildGDPRArtifactStore(cfg); err == nil {
		t.Fatal("buildGDPRArtifactStore accepted S3 artifacts without credentials")
	}
}

func TestStartupRejectsUnwritableLocalArtifactDirs(t *testing.T) {
	root := t.TempDir()
	exportsBase := filepath.Join(root, "exports")
	importsBase := filepath.Join(root, "imports")
	for _, dir := range []string{exportsBase, importsBase} {
		if err := os.Mkdir(dir, 0o500); err != nil {
			t.Fatalf("Mkdir(%s): %v", dir, err)
		}
	}
	cfg := &config.Config{
		StorageBackend:      "local",
		GDPRArtifactBackend: "local",
		ExportsBasePath:     exportsBase,
		ImportsBasePath:     importsBase,
	}
	if _, err := buildGDPRArtifactStore(cfg); err == nil {
		t.Fatal("buildGDPRArtifactStore accepted unwritable local artifact dirs")
	}
}

func privateArtifactDirs(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	exportsBase := filepath.Join(root, "exports")
	importsBase := filepath.Join(root, "imports")
	for _, dir := range []string{exportsBase, importsBase} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatalf("Mkdir(%s): %v", dir, err)
		}
	}
	return exportsBase, importsBase
}
