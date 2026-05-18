package config

import (
	"os"
	"strings"
	"testing"
)

func withBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://skadia:skadia@localhost:5432/skadia_test?sslmode=disable")
	t.Setenv("APP_URL", "http://localhost:5173")
	t.Setenv("JWT_SECRET", strings.Repeat("test-jwt-secret-", 3))
	t.Setenv("STORAGE_BACKEND", "local")
	t.Setenv("SKADIA_STORAGE_MODE", "")
	t.Setenv("SKADIA_DEPLOYMENT_PROFILE", "")
	t.Setenv("SKADIA_S3_PROVIDER", "")
	t.Setenv("SKADIA_REPLICA_COUNT", "")
	t.Setenv("SKADIA_SHARED_STORAGE", "")
	t.Setenv("GDPR_ARTIFACT_BACKEND", "")
	t.Setenv("EXPORTS_BASE_PATH", "")
	t.Setenv("IMPORTS_BASE_PATH", "")
	t.Setenv("S3_BUCKET_EXPORTS", "")
	t.Setenv("S3_BUCKET_IMPORTS", "")
	t.Setenv("S3_EXPORTS_PREFIX", "")
	t.Setenv("S3_IMPORTS_PREFIX", "")
	t.Setenv("S3_ENDPOINT", "")
	t.Setenv("S3_ACCESS_KEY", "")
	t.Setenv("S3_ACCESS_KEY_FILE", "")
	t.Setenv("S3_SECRET_KEY", "")
	t.Setenv("S3_SECRET_KEY_FILE", "")
	t.Setenv("GARAGE_ACCESS_KEY", "")
	t.Setenv("GARAGE_ACCESS_KEY_FILE", "")
	t.Setenv("GARAGE_SECRET_KEY", "")
	t.Setenv("GARAGE_SECRET_KEY_FILE", "")
	t.Setenv("S3_REGION", "")
	t.Setenv("S3_FORCE_PATH_STYLE", "")
	t.Setenv("GDPR_EXPORT_MULTIPART_THRESHOLD_BYTES", "")
	t.Setenv("GDPR_EXPORT_PART_TARGET_BYTES", "")
	t.Setenv("GDPR_EXPORT_PART_WARN_BYTES", "")
	t.Setenv("GDPR_MULTIPART_MAX_PARTS", "")
	t.Setenv("GDPR_ARTIFACT_LOCAL_QUOTA_BYTES", "")
	t.Setenv("GDPR_ARTIFACT_LOCAL_MIN_FREE_BYTES", "")
	t.Setenv("GDPR_ARTIFACT_USER_ACTIVE_BYTES", "")
	t.Setenv("GDPR_ARTIFACT_S3_QUOTA_BYTES", "")
	t.Setenv("GDPR_IMPORT_STAGING_QUOTA_BYTES", "")
}

func TestLoadReplicaStorageSafetyDefaultsForCompose(t *testing.T) {
	withBaseEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIReplicaCount != 1 || cfg.SharedStorage != "false" {
		t.Fatalf("replica/shared defaults = %d/%q, want 1/false", cfg.APIReplicaCount, cfg.SharedStorage)
	}
}

func TestLoadRejectsInvalidSharedStorage(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("SKADIA_SHARED_STORAGE", "maybe")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SKADIA_SHARED_STORAGE") {
		t.Fatalf("err = %v, want shared-storage validation", err)
	}
}

func TestLoadDeploymentProfileDefaultsToFullLegacy(t *testing.T) {
	withBaseEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DeploymentProfile != "full" {
		t.Fatalf("DeploymentProfile = %q, want full", cfg.DeploymentProfile)
	}
	if cfg.DeploymentProfileSource != "implicit_legacy" {
		t.Fatalf("DeploymentProfileSource = %q, want implicit_legacy", cfg.DeploymentProfileSource)
	}
}

func TestLoadDeploymentProfileExplicitCore(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("SKADIA_DEPLOYMENT_PROFILE", "core")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DeploymentProfile != "core" {
		t.Fatalf("DeploymentProfile = %q, want core", cfg.DeploymentProfile)
	}
	if cfg.DeploymentProfileSource != "env" {
		t.Fatalf("DeploymentProfileSource = %q, want env", cfg.DeploymentProfileSource)
	}
}

func TestLoadDeploymentProfileExplicitFull(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("SKADIA_DEPLOYMENT_PROFILE", "full")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DeploymentProfile != "full" {
		t.Fatalf("DeploymentProfile = %q, want full", cfg.DeploymentProfile)
	}
	if cfg.DeploymentProfileSource != "env" {
		t.Fatalf("DeploymentProfileSource = %q, want env", cfg.DeploymentProfileSource)
	}
}

func TestLoadRejectsInvalidDeploymentProfile(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("SKADIA_DEPLOYMENT_PROFILE", "lite")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted invalid SKADIA_DEPLOYMENT_PROFILE")
	}
}

func TestLoadGDPRArtifactDefaults(t *testing.T) {
	withBaseEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GDPRArtifactBackend != "auto" {
		t.Fatalf("GDPRArtifactBackend = %q", cfg.GDPRArtifactBackend)
	}
	if cfg.ExportsBasePath != "/data/exports" {
		t.Fatalf("ExportsBasePath = %q", cfg.ExportsBasePath)
	}
	if cfg.ImportsBasePath != "/data/imports" {
		t.Fatalf("ImportsBasePath = %q", cfg.ImportsBasePath)
	}
	if cfg.S3BucketExports != "skadia-exports" || cfg.S3BucketImports != "skadia-imports" {
		t.Fatalf("S3 buckets = %q/%q", cfg.S3BucketExports, cfg.S3BucketImports)
	}
	if cfg.S3ExportsPrefix != "gdpr/exports" || cfg.S3ImportsPrefix != "gdpr/imports" {
		t.Fatalf("S3 prefixes = %q/%q", cfg.S3ExportsPrefix, cfg.S3ImportsPrefix)
	}
}

func TestLoadS3DefaultsMatchComposeSetupAndML(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("SKADIA_STORAGE_MODE", "s3-local-garage")
	t.Setenv("STORAGE_BACKEND", "s3")
	t.Setenv("S3_ENDPOINT", "http://garage:3900")
	t.Setenv("S3_ACCESS_KEY", "test-access")
	t.Setenv("S3_SECRET_KEY", "test-secret")
	t.Setenv("S3_BUCKET", "")
	t.Setenv("S3_REGION", "")
	t.Setenv("S3_FORCE_PATH_STYLE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.S3Bucket != "skadia-originals" {
		t.Fatalf("S3Bucket = %q, want skadia-originals", cfg.S3Bucket)
	}
	if cfg.S3Region != "us-east-1" {
		t.Fatalf("S3Region = %q, want us-east-1", cfg.S3Region)
	}
	if cfg.StorageMode != "s3-local-garage" {
		t.Fatalf("StorageMode = %q, want s3-local-garage", cfg.StorageMode)
	}
	if !cfg.S3ForcePathStyle {
		t.Fatalf("S3ForcePathStyle = false, want true for local Garage endpoint")
	}
}

func TestLoadExternalS3DoesNotForcePathStyleWhenUnset(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("SKADIA_STORAGE_MODE", "s3-external")
	t.Setenv("STORAGE_BACKEND", "s3")
	t.Setenv("S3_ENDPOINT", "https://s3.example.invalid")
	t.Setenv("S3_ACCESS_KEY", "test-access")
	t.Setenv("S3_SECRET_KEY", "test-secret")
	t.Setenv("S3_REGION", "test-region")
	t.Setenv("S3_FORCE_PATH_STYLE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.StorageMode != "s3-external" {
		t.Fatalf("StorageMode = %q, want s3-external", cfg.StorageMode)
	}
	if cfg.S3ForcePathStyle {
		t.Fatalf("S3ForcePathStyle = true, want false for external S3 unless explicitly configured")
	}
}

func TestLoadRejectsExternalS3WithoutRegion(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("SKADIA_STORAGE_MODE", "s3-external")
	t.Setenv("STORAGE_BACKEND", "s3")
	t.Setenv("S3_ENDPOINT", "https://s3.example.invalid")
	t.Setenv("S3_ACCESS_KEY", "test-access")
	t.Setenv("S3_SECRET_KEY", "test-secret")
	t.Setenv("S3_REGION", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted s3-external without S3_REGION")
	}
}

func TestLoadRejectsExternalS3WithOnlyGarageCredentials(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("SKADIA_STORAGE_MODE", "s3-external")
	t.Setenv("STORAGE_BACKEND", "s3")
	t.Setenv("S3_ENDPOINT", "https://s3.example.invalid")
	t.Setenv("S3_REGION", "test-region")
	t.Setenv("GARAGE_ACCESS_KEY", "stale-garage-access")
	t.Setenv("GARAGE_SECRET_KEY", "stale-garage-secret")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted s3-external with only GARAGE_* credentials")
	}
}

func TestLoadExternalS3AcceptsExplicitS3CredentialFiles(t *testing.T) {
	accessFile := t.TempDir() + "/s3_access_key"
	secretFile := t.TempDir() + "/s3_secret_key"
	if err := os.WriteFile(accessFile, []byte("external-access\n"), 0o600); err != nil {
		t.Fatalf("write access file: %v", err)
	}
	if err := os.WriteFile(secretFile, []byte("external-secret\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	withBaseEnv(t)
	t.Setenv("SKADIA_STORAGE_MODE", "s3-external")
	t.Setenv("STORAGE_BACKEND", "s3")
	t.Setenv("S3_ENDPOINT", "https://s3.example.invalid")
	t.Setenv("S3_REGION", "test-region")
	t.Setenv("S3_ACCESS_KEY_FILE", accessFile)
	t.Setenv("S3_SECRET_KEY_FILE", secretFile)
	t.Setenv("GARAGE_ACCESS_KEY", "stale-garage-access")
	t.Setenv("GARAGE_SECRET_KEY", "stale-garage-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.S3AccessKey != "external-access" || cfg.S3SecretKey != "external-secret" {
		t.Fatalf("S3 credentials = %q/%q, want S3 *_FILE values", cfg.S3AccessKey, cfg.S3SecretKey)
	}
}

func TestLoadLocalGarageAcceptsGarageCredentialFallback(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("SKADIA_STORAGE_MODE", "s3-local-garage")
	t.Setenv("STORAGE_BACKEND", "s3")
	t.Setenv("GARAGE_ACCESS_KEY", "garage-access")
	t.Setenv("GARAGE_SECRET_KEY", "garage-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.S3AccessKey != "garage-access" || cfg.S3SecretKey != "garage-secret" {
		t.Fatalf("S3 credentials = %q/%q, want GARAGE_* fallback", cfg.S3AccessKey, cfg.S3SecretKey)
	}
}

func TestLoadRejectsExternalS3WithGarageEndpoint(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("SKADIA_STORAGE_MODE", "s3-external")
	t.Setenv("STORAGE_BACKEND", "s3")
	t.Setenv("S3_ENDPOINT", "http://garage:3900")
	t.Setenv("S3_ACCESS_KEY", "test-access")
	t.Setenv("S3_SECRET_KEY", "test-secret")
	t.Setenv("S3_REGION", "test-region")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted s3-external with local Garage endpoint")
	}
}

func TestLoadDerivesStorageModeFromLegacyProvider(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("SKADIA_STORAGE_MODE", "")
	t.Setenv("SKADIA_S3_PROVIDER", "local-garage")
	t.Setenv("STORAGE_BACKEND", "s3")
	t.Setenv("S3_ACCESS_KEY", "test-access")
	t.Setenv("S3_SECRET_KEY", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.StorageMode != "s3-local-garage" {
		t.Fatalf("StorageMode = %q, want s3-local-garage", cfg.StorageMode)
	}
	if cfg.S3Endpoint != "http://garage:3900" {
		t.Fatalf("S3Endpoint = %q, want local Garage default", cfg.S3Endpoint)
	}
}

func TestLoadRejectsContradictoryStorageModeAndBackend(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("SKADIA_STORAGE_MODE", "s3-external")
	t.Setenv("STORAGE_BACKEND", "local")
	t.Setenv("S3_REGION", "test-region")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted s3-external with STORAGE_BACKEND=local")
	}
}

func TestLoadRejectsInvalidGDPRArtifactBackend(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("GDPR_ARTIFACT_BACKEND", "ftp")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted invalid GDPR_ARTIFACT_BACKEND")
	}
}

func TestLoadRejectsSameS3BucketWithoutDistinctPrefixes(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("S3_BUCKET_EXPORTS", "skadia-artifacts")
	t.Setenv("S3_BUCKET_IMPORTS", "skadia-artifacts")
	t.Setenv("S3_EXPORTS_PREFIX", "gdpr")
	t.Setenv("S3_IMPORTS_PREFIX", "gdpr")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted identical S3 artifact bucket/prefix")
	}
}

func TestLoadParsesGDPRArtifactLimits(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("GDPR_EXPORT_MULTIPART_THRESHOLD_BYTES", "1024")
	t.Setenv("GDPR_EXPORT_PART_TARGET_BYTES", "2048")
	t.Setenv("GDPR_EXPORT_PART_WARN_BYTES", "4096")
	t.Setenv("GDPR_MULTIPART_MAX_PARTS", "17")
	t.Setenv("GDPR_ARTIFACT_LOCAL_QUOTA_BYTES", "100000")
	t.Setenv("GDPR_ARTIFACT_LOCAL_MIN_FREE_BYTES", "200000")
	t.Setenv("GDPR_ARTIFACT_USER_ACTIVE_BYTES", "300000")
	t.Setenv("GDPR_ARTIFACT_S3_QUOTA_BYTES", "400000")
	t.Setenv("GDPR_IMPORT_STAGING_QUOTA_BYTES", "500000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GDPRExportMultipartThresholdBytes != 1024 ||
		cfg.GDPRExportPartTargetBytes != 2048 ||
		cfg.GDPRExportPartWarnBytes != 4096 ||
		cfg.GDPRMultipartMaxParts != 17 ||
		cfg.GDPRArtifactLocalQuotaBytes != 100000 ||
		cfg.GDPRArtifactLocalMinFreeBytes != 200000 ||
		cfg.GDPRArtifactUserActiveBytes != 300000 ||
		cfg.GDPRArtifactS3QuotaBytes != 400000 ||
		cfg.GDPRImportStagingQuotaBytes != 500000 {
		t.Fatalf("parsed limits unexpected: %#v", cfg)
	}
}

func TestLoadRejectsUnsafeLocalCapacityDefaults(t *testing.T) {
	withBaseEnv(t)
	t.Setenv("GDPR_ARTIFACT_BACKEND", "local")
	t.Setenv("GDPR_ARTIFACT_LOCAL_MIN_FREE_BYTES", "0")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted local artifact backend without min-free guard")
	}
}
