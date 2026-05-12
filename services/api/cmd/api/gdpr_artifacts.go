package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
)

func buildGDPRArtifactStore(cfg *config.Config) (gdprartifacts.Store, error) {
	backend := cfg.GDPRArtifactBackend
	if backend == "" || backend == "auto" {
		backend = cfg.StorageBackend
	}
	switch backend {
	case "local":
		if err := validateWritableArtifactDir(cfg.ExportsBasePath); err != nil {
			return nil, fmt.Errorf("exports artifact directory: %w", err)
		}
		if err := validateWritableArtifactDir(cfg.ImportsBasePath); err != nil {
			return nil, fmt.Errorf("imports artifact directory: %w", err)
		}
		return gdprartifacts.NewLocalStoreWithOptions(cfg.ExportsBasePath, cfg.ImportsBasePath, gdprartifacts.LocalCapacityOptions{
			QuotaBytes:   cfg.GDPRArtifactLocalQuotaBytes,
			MinFreeBytes: cfg.GDPRArtifactLocalMinFreeBytes,
		})
	case "s3":
		if cfg.S3AccessKey == "" || cfg.S3SecretKey == "" {
			return nil, fmt.Errorf("S3_ACCESS_KEY and S3_SECRET_KEY are required when GDPR_ARTIFACT_BACKEND resolves to s3")
		}
		return gdprartifacts.NewS3Store(gdprartifacts.S3Config{
			Endpoint:        cfg.S3Endpoint,
			AccessKey:       cfg.S3AccessKey,
			SecretKey:       cfg.S3SecretKey,
			Region:          cfg.S3Region,
			ForcePathStyle:  cfg.S3ForcePathStyle,
			ExportBucket:    cfg.S3BucketExports,
			ImportBucket:    cfg.S3BucketImports,
			ExportPrefix:    cfg.S3ExportsPrefix,
			ImportPrefix:    cfg.S3ImportsPrefix,
			PartSizeBytes:   cfg.GDPRExportPartTargetBytes,
			ReadBlockBytes:  8 << 20,
			ReadCacheBlocks: 4,
		})
	default:
		return nil, fmt.Errorf("unsupported GDPR artifact backend %q", backend)
	}
}

func ensureGDPRArtifactLifecycle(ctx context.Context, store gdprartifacts.Store) {
	ensurer, ok := store.(gdprartifacts.LifecycleEnsurer)
	if !ok {
		return
	}
	if err := ensurer.EnsureLifecycle(ctx); err != nil {
		slog.Warn("gdpr artifact lifecycle setup skipped", "error", err)
	}
}

func validateWritableArtifactDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("path is required")
	}
	clean := filepath.Clean(dir)
	info, err := os.Lstat(clean)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("must not be a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("must be a directory")
	}
	if info.Mode().Perm()&0o200 == 0 {
		return fmt.Errorf("owner write bit is required")
	}
	probe, err := os.CreateTemp(clean, ".skadia-artifact-probe-*")
	if err != nil {
		return err
	}
	probeName := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probeName)
		return err
	}
	renamed := probeName + ".renamed"
	if err := os.Rename(probeName, renamed); err != nil {
		_ = os.Remove(probeName)
		return err
	}
	return os.Remove(renamed)
}
