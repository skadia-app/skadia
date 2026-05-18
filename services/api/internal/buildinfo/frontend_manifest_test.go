package buildinfo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFrontendManifestReadsBuildInfo(t *testing.T) {
	dir := t.TempDir()
	raw := `{"service":"frontend","git_commit":"abc123","commit_time":"2026-05-17T09:58:00Z","build_time":"2026-05-17T10:00:00Z","config_version":"2"}`
	if err := os.WriteFile(filepath.Join(dir, "skadia-build.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := ReadFrontendManifest(context.Background(), dir)
	if !ok {
		t.Fatal("ReadFrontendManifest ok=false")
	}
	if got.Service != "frontend" || got.GitCommit != "abc123" || got.CommitTime != "2026-05-17T09:58:00Z" || got.BuildTime != "2026-05-17T10:00:00Z" || got.ConfigVersion != "2" {
		t.Fatalf("ReadFrontendManifest() = %+v", got)
	}
}

func TestReadFrontendManifestUsesDefaultsForEmptyValues(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "skadia-build.json"), []byte(`{"service":"frontend"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := ReadFrontendManifest(context.Background(), dir)
	if !ok {
		t.Fatal("ReadFrontendManifest ok=false")
	}
	if got.GitCommit != "unknown" || got.CommitTime != "unknown" || got.BuildTime != "unknown" || got.ConfigVersion != "unknown" {
		t.Fatalf("ReadFrontendManifest() = %+v", got)
	}
}
