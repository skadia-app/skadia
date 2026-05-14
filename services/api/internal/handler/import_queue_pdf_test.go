package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

func TestResolvePDFQueueItemCreatesGearDocumentAndCompletesLog(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	gearID := insertTestGear(t, pool, userID, "Gravel Bike", "bike", "bike")
	fs := newActivityPhotoStorageTestFS(t, pool)
	h := &Handler{DB: db, ShadowFS: fs}

	uploadReq := httptest.NewRequest(http.MethodPost, "/api/v1/upload/media", nil)
	uploadReq = uploadReq.WithContext(withUserAndAdmin(uploadReq.Context(), userID, false))
	result := h.processMediaPDF(uploadReq, userID, "rechnung.pdf", "application/pdf", []byte("%PDF-1.4\n%%EOF\n"))
	if result.QueueID == nil {
		t.Fatalf("processMediaPDF queue_id = nil, result=%#v", result)
	}
	queueID := *result.QueueID

	body := bytes.NewBufferString(`{"doc_type":"invoice","gear_id":"` + gearID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import-queue/"+queueID+"/resolve-pdf", body)
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", queueID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()

	h.ResolvePDFQueueItem(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("ResolvePDFQueueItem status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var docCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM gear_documents
		WHERE user_id = $1 AND gear_id = $2 AND doc_type = 'invoice' AND original_filename = 'rechnung.pdf'
	`, userID, gearID).Scan(&docCount); err != nil {
		t.Fatalf("count gear document: %v", err)
	}
	if docCount != 1 {
		t.Fatalf("gear document count = %d, want 1", docCount)
	}

	var queueStatus string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM import_queue WHERE id = $1 AND user_id = $2`,
		queueID, userID,
	).Scan(&queueStatus); err != nil {
		t.Fatalf("select queue status: %v", err)
	}
	if queueStatus != "resolved" {
		t.Fatalf("queue status = %q, want resolved", queueStatus)
	}

	var logStatus string
	if err := pool.QueryRow(context.Background(), `
		SELECT status
		FROM import_log
		WHERE user_id = $1 AND metadata->>'queue_id' = $2
	`, userID, queueID).Scan(&logStatus); err != nil {
		t.Fatalf("select import log status: %v", err)
	}
	if logStatus != "completed" {
		t.Fatalf("import log status = %q, want completed", logStatus)
	}
}

func TestGetImportQueueItemPreviewFallsBackToThumbnailWhenOriginalMissing(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	fs := newActivityPhotoStorageTestFS(t, pool)
	h := &Handler{DB: db, ShadowFS: fs}
	ctx := context.Background()

	origRec, err := fs.Put(ctx, shadowfs.FileEntry{
		UserID:      userID,
		FileType:    "media",
		ContentType: "image/png",
		FileName:    "photo.png",
		SizeBytes:   int64(len("original")),
	}, bytes.NewBufferString("original"))
	if err != nil {
		t.Fatalf("put original: %v", err)
	}
	thumbRec, err := fs.Put(ctx, shadowfs.FileEntry{
		UserID:      userID,
		FileType:    "thumb",
		ContentType: "image/jpeg",
		FileName:    "photo-thumb.jpg",
		SizeBytes:   int64(len("thumb")),
	}, bytes.NewBufferString("thumb"))
	if err != nil {
		t.Fatalf("put thumb: %v", err)
	}
	if err := fs.Delete(ctx, userID, origRec.ID); err != nil {
		t.Fatalf("delete original: %v", err)
	}

	queueID, err := db.CreateImportQueueItem(ctx, database.ImportQueueItem{
		UserID:           userID,
		QueueType:        "image_no_exif",
		OriginalFilename: "photo.png",
		ContentType:      "image/png",
		Metadata: map[string]any{
			"file_store_id":       origRec.ID,
			"file_store_id_thumb": thumbRec.ID,
		},
	})
	if err != nil {
		t.Fatalf("create queue item: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/import-queue/"+queueID+"/preview", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", queueID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()

	h.GetImportQueueItemPreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetImportQueueItemPreview status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg", got)
	}
	if got := rec.Body.String(); got != "thumb" {
		t.Fatalf("body = %q, want thumb fallback", got)
	}
}
