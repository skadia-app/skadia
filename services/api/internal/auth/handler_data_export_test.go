package auth

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBuildDataExportStatusResponseIncludesDownloadUntilExpiry(t *testing.T) {
	updatedAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	metadata, err := json.Marshal(map[string]any{
		"sha256":     "abc123",
		"size_bytes": 1048576,
		"file_path":  "/tmp/skadia-export-job-1.zip",
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	resp := buildDataExportStatusResponse("job-1", "completed", updatedAt, nil, metadata, updatedAt.Add(time.Hour))

	if resp["download_url"] != "/api/v1/auth/data-export/job-1/download" {
		t.Fatalf("download_url = %v", resp["download_url"])
	}
	if resp["expires_at"] != updatedAt.Add(24*time.Hour).Format(time.RFC3339) {
		t.Fatalf("expires_at = %v", resp["expires_at"])
	}
	if resp["sha256"] != "abc123" {
		t.Fatalf("sha256 = %v", resp["sha256"])
	}
}

func TestBuildDataExportStatusResponseOmitsDownloadWhenFilePathExpired(t *testing.T) {
	updatedAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	metadata, err := json.Marshal(map[string]any{
		"sha256":     "abc123",
		"size_bytes": 1048576,
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	resp := buildDataExportStatusResponse("job-1", "completed", updatedAt, nil, metadata, updatedAt.Add(time.Hour))

	if _, ok := resp["download_url"]; ok {
		t.Fatalf("download_url should be omitted after file_path cleanup: %v", resp["download_url"])
	}
	if _, ok := resp["expires_at"]; ok {
		t.Fatalf("expires_at should be omitted after file_path cleanup: %v", resp["expires_at"])
	}
}

func TestBuildDataExportStatusResponseOmitsDownloadAfterExpiryWindow(t *testing.T) {
	updatedAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	metadata, err := json.Marshal(map[string]any{
		"sha256":     "abc123",
		"size_bytes": 1048576,
		"file_path":  "/tmp/skadia-export-job-1.zip",
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	resp := buildDataExportStatusResponse("job-1", "completed", updatedAt, nil, metadata, updatedAt.Add(25*time.Hour))

	if _, ok := resp["download_url"]; ok {
		t.Fatalf("download_url should be omitted after expiry: %v", resp["download_url"])
	}
	if resp["expires_at"] != updatedAt.Add(24*time.Hour).Format(time.RFC3339) {
		t.Fatalf("expires_at = %v", resp["expires_at"])
	}
}
