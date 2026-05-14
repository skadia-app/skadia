package handler

import (
	"context"
	"image/color"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUploadMediaQueuedItemUsesShadowFSStorageMetadata(t *testing.T) {
	for _, target := range []string{"local", "s3"} {
		t.Run(target, func(t *testing.T) {
			pool := fusionTestPool(t)
			db := &database.DB{Pool: pool}
			userID := insertTestUser(t, pool)
			fs := newUploadMediaQueueStorageFS(t, pool, target)
			h := &Handler{DB: db, ShadowFS: fs, ImportService: importer.NewService(db, fs, nil)}

			req := httptest.NewRequest("POST", "/api/v1/upload/media", nil)
			req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
			result := h.queueMediaFile(
				req,
				userID,
				"synthetic-manual-orphan-"+target+".jpg",
				"image/jpeg",
				jpegTestImage(t, 32, 32, color.RGBA{R: 40, G: 80, B: 160, A: 255}, 90),
				"image_no_match",
				nil,
			)
			if result.Status != "queued" || result.QueueID == nil {
				t.Fatalf("queueMediaFile result = %#v, want queued with queue id", result)
			}

			items, err := db.ListImportQueue(context.Background(), userID, "all")
			if err != nil {
				t.Fatalf("ListImportQueue: %v", err)
			}
			if len(items) != 1 {
				t.Fatalf("import queue items = %d, want 1", len(items))
			}
			item := items[0]
			if item.StorageBackend == nil || *item.StorageBackend != target {
				t.Fatalf("storage_backend = %v, want real ShadowFS backend %s", item.StorageBackend, target)
			}
			if item.StoragePath == nil || *item.StoragePath == "" {
				t.Fatalf("storage_path = %v, want real ShadowFS storage path", item.StoragePath)
			}
			if target == "s3" && !strings.HasPrefix(*item.StoragePath, userID+"/media/") {
				t.Fatalf("storage_path = %q, want s3 media key", *item.StoragePath)
			}
		})
	}
}

func newUploadMediaQueueStorageFS(t *testing.T, pool *pgxpool.Pool, target string) shadowfs.ShadowFS {
	t.Helper()
	dir := t.TempDir()
	backend := shadowfs.NewLocalBackend(dir)
	var local, s3 shadowfs.StorageBackend
	switch target {
	case "local":
		local = backend
	case "s3":
		s3 = backend
	default:
		t.Fatalf("unknown target backend %q", target)
	}
	return shadowfs.NewUnifiedShadowFS(pool, local, s3, target, dir, shadowfs.NewSchemaRenderer(), nil, 1, 0, "skadia-originals", "", "skadia-originals")
}
