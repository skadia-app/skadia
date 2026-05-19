package shadowfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func migrateTestSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func insertMigrateUserFile(t *testing.T, pool *pgxpool.Pool, userID string, rec migrateRecord) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO user_files (
			user_id, file_type, file_name, content_type, storage_backend,
			storage_path, size_bytes, file_hash, parent_type, parent_id,
			has_sidecar, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10, $11, 'active')
		RETURNING id::text`,
		userID, rec.FileType, rec.FileName, rec.ContentType, rec.StorageBackend,
		rec.StoragePath, rec.SizeBytes, rec.FileHash, rec.ParentType, rec.ParentID,
		rec.HasSidecar,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert user_files row: %v", err)
	}
	return id
}

func setMigrateStorageMaintenance(t *testing.T, pool *pgxpool.Pool, value string) {
	t.Helper()
	if value == "" {
		value = "storage migration in progress"
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO system_flags (key, value, expires_at)
		VALUES ('maintenance_mode', $1, $2)
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value, set_at = NOW(), expires_at = EXCLUDED.expires_at`,
		value, time.Now().Add(time.Hour),
	); err != nil {
		t.Fatalf("set maintenance_mode: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM system_flags WHERE key = 'maintenance_mode'`)
	})
}

func TestComputeTargetPathS3UsesParentScopedKey(t *testing.T) {
	parentID := "2de7ee29-2f2d-42ce-983d-cd2ed0d5500f"
	rec := migrateRecord{
		ID:         "file-id",
		UserID:     "user-id",
		FileType:   "activity",
		FileName:   "ride.fit",
		FileHash:   strings.Repeat("a", 64),
		ParentType: "activity",
		ParentID:   &parentID,
	}

	got, opts, err := computeTargetPath(context.Background(), nil, rec, "s3", func(fileType string) string {
		if fileType != "activity" {
			t.Fatalf("bucketFn fileType = %q, want activity", fileType)
		}
		return "activity-bucket"
	}, nil, nil)
	if err != nil {
		t.Fatalf("computeTargetPath: %v", err)
	}

	want := "user-id/activities/activity/" + parentID + "/" + strings.Repeat("a", 64) + ".fit"
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if opts == nil || opts.Bucket != "activity-bucket" {
		t.Fatalf("bucket opts = %#v, want activity-bucket", opts)
	}
}

func TestUpdateMigrRecordDoesNotSilentlyFlipBackendOnDuplicatePath(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	targetPath := "user-id/activities/hash.fit"
	sourcePath := "library/activities/source.fit"

	insertMigrateUserFile(t, pool, userID, migrateRecord{
		FileType:       "activity",
		FileName:       "existing.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "s3",
		StoragePath:    targetPath,
		SizeBytes:      5,
		FileHash:       migrateTestSHA256Hex([]byte("same1")),
	})
	migratingID := insertMigrateUserFile(t, pool, userID, migrateRecord{
		FileType:       "activity",
		FileName:       "source.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "local",
		StoragePath:    sourcePath,
		SizeBytes:      5,
		FileHash:       migrateTestSHA256Hex([]byte("same2")),
	})

	err := updateMigrRecord(ctx, pool, migratingID, "s3", targetPath, migrateTestSHA256Hex([]byte("same2")))
	if err == nil {
		t.Fatal("updateMigrRecord succeeded on duplicate target path; want hard failure")
	}

	var backend, path string
	if scanErr := pool.QueryRow(ctx,
		`SELECT storage_backend, storage_path FROM user_files WHERE id = $1`,
		migratingID,
	).Scan(&backend, &path); scanErr != nil {
		t.Fatalf("read migrated row: %v", scanErr)
	}
	if backend != "local" || path != sourcePath {
		t.Fatalf("row mutated to backend=%q path=%q, want local/%s", backend, path, sourcePath)
	}
}

func TestMigrateFileFailsIfDeclaredSidecarCannotBeCopied(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	source := NewLocalBackend(sourceDir)
	target := NewLocalBackend(targetDir)
	content := []byte("activity bytes")
	sourcePath := "source/activity.fit"
	hash := migrateTestSHA256Hex(content)
	if _, _, err := source.Write(ctx, sourcePath, bytes.NewReader(content), "application/octet-stream", false, nil); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	rec := migrateRecord{
		UserID:         userID,
		FileType:       "activity",
		FileName:       "activity.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "local",
		StoragePath:    sourcePath,
		SizeBytes:      int64(len(content)),
		FileHash:       hash,
		HasSidecar:     true,
	}
	rec.ID = insertMigrateUserFile(t, pool, userID, rec)

	err := migrateFile(ctx, pool, rec, source, target, "s3", func(string) string { return "" }, targetDir, nil, nil)
	if err == nil {
		t.Fatal("migrateFile succeeded without copying declared sidecar")
	}
	if !strings.Contains(err.Error(), "sidecar") {
		t.Fatalf("migrateFile err = %v, want sidecar context", err)
	}

	var backend, path string
	if scanErr := pool.QueryRow(ctx,
		`SELECT storage_backend, storage_path FROM user_files WHERE id = $1`,
		rec.ID,
	).Scan(&backend, &path); scanErr != nil {
		t.Fatalf("read migrated row: %v", scanErr)
	}
	if backend != "local" || path != sourcePath {
		t.Fatalf("row mutated to backend=%q path=%q, want local/%s", backend, path, sourcePath)
	}
	targetPath := filepath.Join(targetDir, userID, DirForType("activity"), hash+".fit")
	if _, statErr := os.Stat(targetPath); !os.IsNotExist(statErr) {
		t.Fatalf("target primary file exists after failed sidecar copy: %v", statErr)
	}
}

func TestRunDryRunFailsWhenDeclaredSidecarMissing(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	source := NewLocalBackend(sourceDir)
	target := NewLocalBackend(targetDir)
	content := []byte("activity bytes")
	sourcePath := "source/activity.fit"
	hash := migrateTestSHA256Hex(content)
	if _, _, err := source.Write(ctx, sourcePath, bytes.NewReader(content), "application/octet-stream", false, nil); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	insertMigrateUserFile(t, pool, userID, migrateRecord{
		FileType:       "activity",
		FileName:       "activity.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "local",
		StoragePath:    sourcePath,
		SizeBytes:      int64(len(content)),
		FileHash:       hash,
		HasSidecar:     true,
	})

	err := runDryRun(ctx, pool, source, target, "local", "s3", func(string) string { return "" }, nil, nil)
	if err == nil {
		t.Fatal("runDryRun succeeded despite missing declared sidecar")
	}
	if !strings.Contains(err.Error(), "declared sidecar") {
		t.Fatalf("runDryRun err = %v, want declared sidecar context", err)
	}
}

func TestRunDryRunFailsOnExistingTargetHashMismatch(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	source := NewLocalBackend(sourceDir)
	target := NewLocalBackend(targetDir)
	content := []byte("correct activity bytes")
	sourcePath := "source/activity.fit"
	hash := migrateTestSHA256Hex(content)
	if _, _, err := source.Write(ctx, sourcePath, bytes.NewReader(content), "application/octet-stream", false, nil); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	targetPath := filepath.Join(userID, DirForType("activity"), hash+".fit")
	if _, _, err := target.Write(ctx, targetPath, bytes.NewReader([]byte("wrong target bytes")), "application/octet-stream", false, nil); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	insertMigrateUserFile(t, pool, userID, migrateRecord{
		FileType:       "activity",
		FileName:       "activity.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "local",
		StoragePath:    sourcePath,
		SizeBytes:      int64(len(content)),
		FileHash:       hash,
	})

	err := runDryRun(ctx, pool, source, target, "local", "s3", func(string) string { return "" }, nil, nil)
	if err == nil {
		t.Fatal("runDryRun succeeded despite mismatched existing target")
	}
	if !strings.Contains(err.Error(), "target conflict") {
		t.Fatalf("runDryRun err = %v, want target conflict context", err)
	}
}

func TestMigrateFileRejectsExistingTargetHashMismatch(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	source := NewLocalBackend(sourceDir)
	target := NewLocalBackend(targetDir)
	content := []byte("correct activity bytes")
	sourcePath := "source/activity.fit"
	hash := migrateTestSHA256Hex(content)
	if _, _, err := source.Write(ctx, sourcePath, bytes.NewReader(content), "application/octet-stream", false, nil); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	targetPath := filepath.Join(userID, DirForType("activity"), hash+".fit")
	if _, _, err := target.Write(ctx, targetPath, bytes.NewReader([]byte("wrong target bytes")), "application/octet-stream", false, nil); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	rec := migrateRecord{
		UserID:         userID,
		FileType:       "activity",
		FileName:       "activity.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "local",
		StoragePath:    sourcePath,
		SizeBytes:      int64(len(content)),
		FileHash:       hash,
	}
	rec.ID = insertMigrateUserFile(t, pool, userID, rec)

	err := migrateFile(ctx, pool, rec, source, target, "s3", func(string) string { return "" }, targetDir, nil, nil)
	if err == nil {
		t.Fatal("migrateFile accepted existing target with mismatched hash")
	}
	if !strings.Contains(err.Error(), "target exists") {
		t.Fatalf("migrateFile err = %v, want target-exists hash context", err)
	}

	var backend, path string
	if scanErr := pool.QueryRow(ctx,
		`SELECT storage_backend, storage_path FROM user_files WHERE id = $1`,
		rec.ID,
	).Scan(&backend, &path); scanErr != nil {
		t.Fatalf("read migrated row: %v", scanErr)
	}
	if backend != "local" || path != sourcePath {
		t.Fatalf("row mutated to backend=%q path=%q, want local/%s", backend, path, sourcePath)
	}
	got := readAllAndClose(t, mustReadBackend(t, target, targetPath))
	if string(got) != "wrong target bytes" {
		t.Fatalf("existing target content changed to %q", got)
	}
}

func TestMigrateFileDoesNotDeleteTargetClaimedDuringDBRace(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	source := NewLocalBackend(sourceDir)
	target := &migrateRaceTargetBackend{StorageBackend: NewLocalBackend(targetDir)}
	content := []byte("race-safe bytes")
	sourcePath := "source/race.fit"
	hash := migrateTestSHA256Hex(content)
	if _, _, err := source.Write(ctx, sourcePath, bytes.NewReader(content), "application/octet-stream", false, nil); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	rec := migrateRecord{
		UserID:         userID,
		FileType:       "activity",
		FileName:       "race.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "local",
		StoragePath:    sourcePath,
		SizeBytes:      int64(len(content)),
		FileHash:       hash,
	}
	rec.ID = insertMigrateUserFile(t, pool, userID, rec)
	target.afterWrite = func(path string) {
		insertMigrateUserFile(t, pool, userID, migrateRecord{
			FileType:       "activity",
			FileName:       "racer.fit",
			ContentType:    "application/octet-stream",
			StorageBackend: "s3",
			StoragePath:    path,
			SizeBytes:      int64(len(content)),
			FileHash:       hash,
		})
	}

	err := migrateFile(ctx, pool, rec, source, target, "s3", func(string) string { return "" }, targetDir, nil, nil)
	if err == nil {
		t.Fatal("migrateFile succeeded despite target path being claimed during DB update")
	}

	targetPath := filepath.Join(userID, DirForType("activity"), hash+".fit")
	got := readAllAndClose(t, mustReadBackend(t, target.StorageBackend, targetPath))
	if !bytes.Equal(got, content) {
		t.Fatalf("claimed target content = %q, want %q", got, content)
	}
	var backend, path string
	if scanErr := pool.QueryRow(ctx,
		`SELECT storage_backend, storage_path FROM user_files WHERE id = $1`,
		rec.ID,
	).Scan(&backend, &path); scanErr != nil {
		t.Fatalf("read migrating row: %v", scanErr)
	}
	if backend != "local" || path != sourcePath {
		t.Fatalf("migrating row mutated to backend=%q path=%q, want local/%s", backend, path, sourcePath)
	}
}

func TestMigrateFileHashesSourceBeforeS3PathWhenFileHashMissing(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	source := NewLocalBackend(sourceDir)
	target := NewLocalBackend(targetDir)
	content := []byte("missing hash bytes")
	sourcePath := "source/missing-hash.fit"
	hash := migrateTestSHA256Hex(content)
	if _, _, err := source.Write(ctx, sourcePath, bytes.NewReader(content), "application/octet-stream", false, nil); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	rec := migrateRecord{
		UserID:         userID,
		FileType:       "activity",
		FileName:       "missing-hash.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "local",
		StoragePath:    sourcePath,
		SizeBytes:      int64(len(content)),
		FileHash:       "",
	}
	rec.ID = insertMigrateUserFile(t, pool, userID, rec)

	if err := migrateFile(ctx, pool, rec, source, target, "s3", func(string) string { return "" }, targetDir, nil, nil); err != nil {
		t.Fatalf("migrateFile: %v", err)
	}

	wantPath := filepath.Join(userID, DirForType("activity"), hash+".fit")
	got := readAllAndClose(t, mustReadBackend(t, target, wantPath))
	if !bytes.Equal(got, content) {
		t.Fatalf("target content = %q, want %q", got, content)
	}

	var backend, storagePath, fileHash string
	if scanErr := pool.QueryRow(ctx,
		`SELECT storage_backend, storage_path, file_hash FROM user_files WHERE id = $1`,
		rec.ID,
	).Scan(&backend, &storagePath, &fileHash); scanErr != nil {
		t.Fatalf("read migrated row: %v", scanErr)
	}
	if backend != "s3" || storagePath != wantPath || fileHash != hash {
		t.Fatalf("row = backend %q path %q hash %q, want s3/%s/%s", backend, storagePath, fileHash, wantPath, hash)
	}
}

func TestFillNullHashesFailsOnUnreadableSource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	source := NewLocalBackend(t.TempDir())
	missingPath := "missing/activity.fit"

	insertMigrateUserFile(t, pool, userID, migrateRecord{
		FileType:       "activity",
		FileName:       "activity.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "local",
		StoragePath:    missingPath,
		SizeBytes:      42,
		FileHash:       "",
	})
	if _, err := pool.Exec(ctx, `UPDATE user_files SET file_hash = NULL WHERE storage_path = $1`, missingPath); err != nil {
		t.Fatalf("clear file_hash: %v", err)
	}

	err := fillNullHashes(ctx, pool, source, "local", func(string) string { return "" }, 10)
	if err == nil {
		t.Fatal("fillNullHashes succeeded on unreadable source file")
	}
	if !strings.Contains(err.Error(), "read") || !strings.Contains(err.Error(), missingPath) {
		t.Fatalf("fillNullHashes err = %v, want read failure with source path", err)
	}
}

func TestProcessBatchReturnsErrorForPersistentFileFailure(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	source := NewLocalBackend(t.TempDir())
	target := NewLocalBackend(t.TempDir())

	rec := migrateRecord{
		UserID:         userID,
		FileType:       "activity",
		FileName:       "missing.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "local",
		StoragePath:    "missing.fit",
		SizeBytes:      42,
		FileHash:       migrateTestSHA256Hex([]byte("not on disk")),
	}
	rec.ID = insertMigrateUserFile(t, pool, userID, rec)

	var totalMigrated, totalBytes, totalErrors int64
	err := processBatch(ctx, pool, []migrateRecord{rec}, source, target, "s3", "local",
		func(string) string { return "" }, t.TempDir(), nil, nil, 1,
		&totalMigrated, &totalBytes, &totalErrors, 1, time.Now())
	if err == nil {
		t.Fatal("processBatch returned nil despite a persistent file failure")
	}
	if totalErrors != 1 {
		t.Fatalf("totalErrors = %d, want 1", totalErrors)
	}
	if totalMigrated != 0 || totalBytes != 0 {
		t.Fatalf("progress = files %d bytes %d, want zero", totalMigrated, totalBytes)
	}
}

func TestRunMigrateStorageFailureLeavesDurableMaintenanceMode(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	source := NewLocalBackend(t.TempDir())
	target := NewLocalBackend(t.TempDir())

	insertMigrateUserFile(t, pool, userID, migrateRecord{
		FileType:       "activity",
		FileName:       "missing.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "local",
		StoragePath:    "missing.fit",
		SizeBytes:      42,
		FileHash:       migrateTestSHA256Hex([]byte("not on disk")),
	})

	err := RunMigrateStorage(ctx, pool, source, target, "s3", func(string) string { return "" },
		t.TempDir(), nil, nil, false, 10, 1, false)
	if err == nil {
		t.Fatal("RunMigrateStorage succeeded despite missing source file")
	}
	if !strings.Contains(err.Error(), "process batch") {
		t.Fatalf("RunMigrateStorage err = %v, want process batch failure", err)
	}
	if strings.Contains(err.Error(), "context deadline") {
		t.Fatalf("RunMigrateStorage err = %v, want immediate file failure, not context deadline", err)
	}

	var expires string
	if scanErr := pool.QueryRow(ctx,
		`SELECT expires_at::text FROM system_flags WHERE key = 'maintenance_mode'`,
	).Scan(&expires); scanErr != nil {
		t.Fatalf("read maintenance_mode: %v", scanErr)
	}
	if !strings.Contains(expires, "infinity") {
		t.Fatalf("maintenance_mode expires_at = %q, want infinity", expires)
	}
}

func TestMigratePathOnlySidecarsCopiesFusionMeta(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	activityID := "11111111-1111-4111-8111-111111111111"
	source := NewLocalBackend(t.TempDir())
	target := NewLocalBackend(t.TempDir())
	path := fmt.Sprintf("%s/activity-%s.meta", userID, activityID)
	content := []byte(`{"kind":"fusion"}`)

	if _, err := pool.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, started_at)
		 VALUES ($1, $2, 'cycling', NOW())`,
		activityID, userID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, _, err := source.Write(ctx, path, bytes.NewReader(content), "application/json", false, nil); err != nil {
		t.Fatalf("write source path-only sidecar: %v", err)
	}

	migrated, err := migratePathOnlySidecars(ctx, pool, source, target, func(string) string { return "" })
	if err != nil {
		t.Fatalf("migratePathOnlySidecars: %v", err)
	}
	if migrated != 1 {
		t.Fatalf("migrated sidecars = %d, want 1", migrated)
	}
	got := readAllAndClose(t, mustReadBackend(t, target, path))
	if !bytes.Equal(got, content) {
		t.Fatalf("target path-only sidecar = %q, want %q", got, content)
	}
}

func TestMigrateFileResolvesLocalPhysicalTargetCollision(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	source := NewLocalBackend(sourceDir)
	target := NewLocalBackend(targetDir)
	content := []byte("correct local target bytes")
	hash := migrateTestSHA256Hex(content)
	sourcePath := filepath.Join(userID, DirForType("activity"), hash+".fit")
	targetPath := filepath.Join(userID, DirForType("activity"), hash+".fit")

	if _, _, err := source.Write(ctx, sourcePath, bytes.NewReader(content), "application/octet-stream", false, nil); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if _, _, err := target.Write(ctx, targetPath, bytes.NewReader([]byte("different existing bytes")), "application/octet-stream", false, nil); err != nil {
		t.Fatalf("write colliding target file: %v", err)
	}

	rec := migrateRecord{
		UserID:         userID,
		FileType:       "activity",
		FileName:       "activity.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "s3",
		StoragePath:    sourcePath,
		SizeBytes:      int64(len(content)),
		FileHash:       hash,
	}
	rec.ID = insertMigrateUserFile(t, pool, userID, rec)

	if err := migrateFile(ctx, pool, rec, source, target, "local", func(string) string { return "" }, targetDir, nil, nil); err != nil {
		t.Fatalf("migrateFile: %v", err)
	}

	var backend, migratedPath string
	if scanErr := pool.QueryRow(ctx,
		`SELECT storage_backend, storage_path FROM user_files WHERE id = $1`,
		rec.ID,
	).Scan(&backend, &migratedPath); scanErr != nil {
		t.Fatalf("read migrated row: %v", scanErr)
	}
	if backend != "local" {
		t.Fatalf("backend = %q, want local", backend)
	}
	if migratedPath == targetPath {
		t.Fatalf("migrated path = %q, want collision suffix", migratedPath)
	}
	got := readAllAndClose(t, mustReadBackend(t, target, migratedPath))
	if !bytes.Equal(got, content) {
		t.Fatalf("migrated target content = %q, want %q", got, content)
	}
	original := readAllAndClose(t, mustReadBackend(t, target, targetPath))
	if string(original) != "different existing bytes" {
		t.Fatalf("original colliding file changed to %q", original)
	}
}

func TestMigrateFileUsesStreamingTargetWhenAvailable(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	source := NewLocalBackend(t.TempDir())
	target := &migrateStreamingOnlyTargetBackend{LocalBackend: NewLocalBackend(t.TempDir())}
	content := []byte("stream me")
	hash := migrateTestSHA256Hex(content)
	sourcePath := "source/stream.fit"

	if _, _, err := source.Write(ctx, sourcePath, bytes.NewReader(content), "application/octet-stream", false, nil); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	rec := migrateRecord{
		UserID:         userID,
		FileType:       "activity",
		FileName:       "stream.fit",
		ContentType:    "application/octet-stream",
		StorageBackend: "local",
		StoragePath:    sourcePath,
		SizeBytes:      int64(len(content)),
		FileHash:       hash,
	}
	rec.ID = insertMigrateUserFile(t, pool, userID, rec)

	if err := migrateFile(ctx, pool, rec, source, target, "s3", func(string) string { return "" }, target.BasePath(), nil, nil); err != nil {
		t.Fatalf("migrateFile: %v", err)
	}
	if !target.usedStream {
		t.Fatal("migrateFile did not use WriteStream")
	}
}

type migrateRaceTargetBackend struct {
	StorageBackend
	afterWrite func(path string)
}

func (b *migrateRaceTargetBackend) Write(ctx context.Context, path string, data io.Reader, contentType string, exclusive bool, opts *BackendOpts) (int64, string, error) {
	written, hash, err := b.StorageBackend.Write(ctx, path, data, contentType, exclusive, opts)
	if err == nil && b.afterWrite != nil {
		b.afterWrite(path)
	}
	return written, hash, err
}

type migrateStreamingOnlyTargetBackend struct {
	*LocalBackend
	usedStream bool
}

func (b *migrateStreamingOnlyTargetBackend) Write(context.Context, string, io.Reader, string, bool, *BackendOpts) (int64, string, error) {
	return 0, "", errors.New("buffered write disabled")
}

func (b *migrateStreamingOnlyTargetBackend) WriteStream(ctx context.Context, path string, data io.Reader, contentType string, exclusive bool, expectedHash string, opts *BackendOpts) (int64, string, error) {
	b.usedStream = true
	return b.LocalBackend.WriteStream(ctx, path, data, contentType, exclusive, expectedHash, opts)
}

func TestUnifiedShadowFSPutRejectsActiveMaintenanceMode(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	base := t.TempDir()
	fs := NewUnifiedShadowFS(pool, NewLocalBackend(base), nil, "local", base, NewSchemaRenderer(), nil, 1, 0, "", "", "")

	setMigrateStorageMaintenance(t, pool, "storage migration in progress")

	_, err := fs.Put(ctx, FileEntry{
		UserID:      userID,
		FileType:    "activity",
		ContentType: "application/octet-stream",
		FileName:    "blocked.fit",
		SizeBytes:   7,
		FileHash:    migrateTestSHA256Hex([]byte("blocked")),
		Tags:        map[string]string{"year": "2026", "month": "05", "day": "18", "time": "1500", "sport": "cycling"},
	}, bytes.NewReader([]byte("blocked")))
	if err == nil {
		t.Fatal("Put succeeded during active maintenance_mode")
	}
	if !errors.Is(err, ErrStorageMaintenance) {
		t.Fatalf("Put err = %v, want ErrStorageMaintenance", err)
	}

	var count int
	if scanErr := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_files WHERE user_id = $1 AND file_name = 'blocked.fit'`,
		userID,
	).Scan(&count); scanErr != nil {
		t.Fatalf("count user_files: %v", scanErr)
	}
	if count != 0 {
		t.Fatalf("user_files rows for blocked write = %d, want 0", count)
	}
}

func TestUnifiedShadowFSDeleteRejectsActiveMaintenanceMode(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	base := t.TempDir()
	fs := NewUnifiedShadowFS(pool, NewLocalBackend(base), nil, "local", base, NewSchemaRenderer(), nil, 1, 0, "", "", "")
	content := []byte("delete me")

	rec, err := fs.Put(ctx, FileEntry{
		UserID:      userID,
		FileType:    "activity",
		ContentType: "application/octet-stream",
		FileName:    "delete.fit",
		SizeBytes:   int64(len(content)),
		FileHash:    migrateTestSHA256Hex(content),
		Tags:        map[string]string{"year": "2026", "month": "05", "day": "18", "time": "1510", "sport": "cycling"},
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}
	fullPath := filepath.Join(base, rec.StoragePath)

	setMigrateStorageMaintenance(t, pool, "storage migration in progress")

	err = fs.Delete(ctx, userID, rec.ID)
	if !errors.Is(err, ErrStorageMaintenance) {
		t.Fatalf("Delete err = %v, want ErrStorageMaintenance", err)
	}

	var count int
	if scanErr := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_files WHERE id = $1 AND status = 'active'`,
		rec.ID,
	).Scan(&count); scanErr != nil {
		t.Fatalf("count user_files: %v", scanErr)
	}
	if count != 1 {
		t.Fatalf("active user_files rows after blocked Delete = %d, want 1", count)
	}
	if _, statErr := os.Stat(fullPath); statErr != nil {
		t.Fatalf("primary file missing after blocked Delete: %v", statErr)
	}
}

func TestUnifiedShadowFSPutCompanionRejectsActiveMaintenanceMode(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	base := t.TempDir()
	fs := NewUnifiedShadowFS(pool, NewLocalBackend(base), nil, "local", base, NewSchemaRenderer(), nil, 1, 0, "", "", "")
	content := []byte("primary bytes")

	rec, err := fs.Put(ctx, FileEntry{
		UserID:      userID,
		FileType:    "activity",
		ContentType: "application/octet-stream",
		FileName:    "primary.fit",
		SizeBytes:   int64(len(content)),
		FileHash:    migrateTestSHA256Hex(content),
		Tags:        map[string]string{"year": "2026", "month": "05", "day": "18", "time": "1520", "sport": "cycling"},
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	setMigrateStorageMaintenance(t, pool, "storage migration in progress")

	err = fs.PutCompanion(ctx, userID, rec.ID, ".meta", []byte(`{"blocked":true}`))
	if !errors.Is(err, ErrStorageMaintenance) {
		t.Fatalf("PutCompanion err = %v, want ErrStorageMaintenance", err)
	}

	var hasSidecar bool
	if scanErr := pool.QueryRow(ctx,
		`SELECT has_sidecar FROM user_files WHERE id = $1`,
		rec.ID,
	).Scan(&hasSidecar); scanErr != nil {
		t.Fatalf("read has_sidecar: %v", scanErr)
	}
	if hasSidecar {
		t.Fatal("has_sidecar flipped during blocked PutCompanion")
	}
	if _, statErr := os.Stat(filepath.Join(base, rec.StoragePath+".meta")); !os.IsNotExist(statErr) {
		t.Fatalf("companion sidecar exists after blocked PutCompanion: %v", statErr)
	}
}

func TestUnifiedShadowFSPutCompanionAtRejectsActiveMaintenanceMode(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	base := t.TempDir()
	fs := NewUnifiedShadowFS(pool, NewLocalBackend(base), nil, "local", base, NewSchemaRenderer(), nil, 1, 0, "", "", "")

	setMigrateStorageMaintenance(t, pool, "storage migration in progress")

	err := fs.PutCompanionAt(ctx, "fusion/activity.meta", []byte(`{"blocked":true}`))
	if !errors.Is(err, ErrStorageMaintenance) {
		t.Fatalf("PutCompanionAt err = %v, want ErrStorageMaintenance", err)
	}
	if _, statErr := os.Stat(filepath.Join(base, "fusion/activity.meta")); !os.IsNotExist(statErr) {
		t.Fatalf("path-based sidecar exists after blocked PutCompanionAt: %v", statErr)
	}
}

func TestUnifiedShadowFSArchiveRejectsActiveMaintenanceMode(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userID := createShadowFSTestUser(t, pool)
	base := t.TempDir()
	fs := NewUnifiedShadowFS(pool, NewLocalBackend(base), nil, "local", base, NewSchemaRenderer(), nil, 1, 0, "", "", "")
	content := []byte("archive me")

	rec, err := fs.Put(ctx, FileEntry{
		UserID:      userID,
		FileType:    "activity",
		ContentType: "application/octet-stream",
		FileName:    "archive.fit",
		SizeBytes:   int64(len(content)),
		FileHash:    migrateTestSHA256Hex(content),
		Tags:        map[string]string{"year": "2026", "month": "05", "day": "18", "time": "1530", "sport": "cycling"},
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}
	fullPath := filepath.Join(base, rec.StoragePath)

	setMigrateStorageMaintenance(t, pool, "storage migration in progress")

	err = fs.Archive(ctx, userID, rec.ID, "")
	if !errors.Is(err, ErrStorageMaintenance) {
		t.Fatalf("Archive err = %v, want ErrStorageMaintenance", err)
	}

	var status string
	if scanErr := pool.QueryRow(ctx,
		`SELECT status FROM user_files WHERE id = $1`,
		rec.ID,
	).Scan(&status); scanErr != nil {
		t.Fatalf("read status: %v", scanErr)
	}
	if status != "active" {
		t.Fatalf("status after blocked Archive = %q, want active", status)
	}
	if _, statErr := os.Stat(fullPath); statErr != nil {
		t.Fatalf("primary file missing after blocked Archive: %v", statErr)
	}
}

func readAllAndClose(t *testing.T, rc io.ReadCloser) []byte {
	t.Helper()
	data, err := io.ReadAll(rc)
	if closeErr := rc.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return data
}

func mustReadBackend(t *testing.T, backend StorageBackend, path string) io.ReadCloser {
	t.Helper()
	rc, err := backend.Read(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("read backend %s: %v", path, err)
	}
	return rc
}
