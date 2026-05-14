package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/color"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
)

type recordingDocumentPublisher struct {
	events    []event.DocumentUploadedEvent
	onPublish func(event.DocumentUploadedEvent) error
}

func (p *recordingDocumentPublisher) PublishDocumentUploaded(evt event.DocumentUploadedEvent) error {
	p.events = append(p.events, evt)
	if p.onPublish != nil {
		return p.onPublish(evt)
	}
	return nil
}

func TestUploadGearImageCompletesImportLogWithoutDocumentProcessing(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	gearID := insertTestGear(t, pool, userID, "Test Bike", "bike", "bike")
	fs := newActivityPhotoStorageTestFS(t, pool)
	pub := &recordingDocumentPublisher{}
	h := &Handler{DB: db, ShadowFS: fs, DocumentUploadedPublisher: pub}

	req := newGearDocumentUploadRequest(t, userID, gearID, "gear-photo.jpg", "image/jpeg", jpegTestImage(t, 24, 24, color.RGBA{R: 80, G: 120, B: 200, A: 255}, 90), "photo")
	rr := httptest.NewRecorder()
	h.UploadGearDocument(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("UploadGearDocument image status = %d, want %d; body=%s", rr.Code, http.StatusCreated, rr.Body.String())
	}
	if len(pub.events) != 0 {
		t.Fatalf("image upload published document events = %d, want 0", len(pub.events))
	}

	var docStatus string
	if err := pool.QueryRow(context.Background(), `
		SELECT processing_status
		FROM gear_documents
		WHERE user_id = $1 AND gear_id = $2 AND original_filename = 'gear-photo.jpg'`,
		userID, gearID,
	).Scan(&docStatus); err != nil {
		t.Fatalf("select gear image processing_status: %v", err)
	}
	if docStatus != "idle" {
		t.Fatalf("gear image processing_status = %q, want idle", docStatus)
	}

	var logStatus, importType, targetType, targetID, stepStatus string
	if err := pool.QueryRow(context.Background(), `
		SELECT status, import_type, metadata->>'target_type', metadata->>'target_id',
		       metadata #>> '{steps,0,status}'
		FROM import_log
		WHERE user_id = $1 AND filename = 'gear-photo.jpg'
		ORDER BY created_at DESC
		LIMIT 1`,
		userID,
	).Scan(&logStatus, &importType, &targetType, &targetID, &stepStatus); err != nil {
		t.Fatalf("select gear image import_log: %v", err)
	}
	if logStatus != "completed" || importType != "photo" || targetType != "gear_document" || targetID == "" || stepStatus != "completed" {
		t.Fatalf("gear image import_log = status %q type %q target_type %q target_id %q step %q, want completed/photo/gear_document/<id>/completed",
			logStatus, importType, targetType, targetID, stepStatus)
	}
}

func TestUploadGearPDFTracksProcessingImportLogWhenDocumentEventIsPublished(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	gearID := insertTestGear(t, pool, userID, "Test Bike", "bike", "bike")
	fs := newActivityPhotoStorageTestFS(t, pool)
	pub := &recordingDocumentPublisher{}
	h := &Handler{DB: db, ShadowFS: fs, DocumentUploadedPublisher: pub}

	req := newGearDocumentUploadRequest(t, userID, gearID, "manual.pdf", "application/pdf", []byte("%PDF-1.4\n%%EOF\n"), "manual")
	rr := httptest.NewRecorder()
	h.UploadGearDocument(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("UploadGearDocument pdf status = %d, want %d; body=%s", rr.Code, http.StatusCreated, rr.Body.String())
	}
	if len(pub.events) != 1 {
		t.Fatalf("pdf upload published document events = %d, want 1", len(pub.events))
	}

	var response struct {
		ID               string `json:"id"`
		ProcessingStatus string `json:"processing_status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if response.ID == "" {
		t.Fatal("upload response id is empty")
	}
	if response.ProcessingStatus != "pending" {
		t.Fatalf("pdf response processing_status = %q, want pending", response.ProcessingStatus)
	}

	var logStatus, importType, targetType, targetID, markdownStatus string
	if err := pool.QueryRow(context.Background(), `
		SELECT status, import_type, metadata->>'target_type', metadata->>'target_id',
		       metadata #>> '{steps,2,status}'
		FROM import_log
		WHERE user_id = $1 AND filename = 'manual.pdf'
		ORDER BY created_at DESC
		LIMIT 1`,
		userID,
	).Scan(&logStatus, &importType, &targetType, &targetID, &markdownStatus); err != nil {
		t.Fatalf("select gear pdf import_log: %v", err)
	}
	if logStatus != "processing" || importType != "document" || targetType != "gear_document" || targetID != response.ID || markdownStatus != "pending" {
		t.Fatalf("gear pdf import_log = status %q type %q target_type %q target_id %q markdown_step %q, want processing/document/gear_document/%s/pending",
			logStatus, importType, targetType, targetID, markdownStatus, response.ID)
	}
}

func TestUploadGearPDFSurvivesSynchronousProcessedCallback(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	gearID := insertTestGear(t, pool, userID, "Test Bike", "bike", "bike")
	fs := newActivityPhotoStorageTestFS(t, pool)
	h := &Handler{DB: db, ShadowFS: fs}
	pub := &recordingDocumentPublisher{
		onPublish: func(evt event.DocumentUploadedEvent) error {
			payload, err := json.Marshal(map[string]any{
				"doc_id":          evt.DocID,
				"user_id":         evt.UserID,
				"file_id":         evt.FileID,
				"success":         true,
				"markdown_status": "skipped",
			})
			if err != nil {
				return err
			}
			return h.HandleDocumentProcessedMessage(context.Background(), payload)
		},
	}
	h.DocumentUploadedPublisher = pub

	req := newGearDocumentUploadRequest(t, userID, gearID, "sync-callback.pdf", "application/pdf", []byte("%PDF-1.4\n%%EOF\n"), "manual")
	rr := httptest.NewRecorder()
	h.UploadGearDocument(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("UploadGearDocument pdf status = %d, want %d; body=%s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	var logStatus, markdownStatus string
	if err := pool.QueryRow(context.Background(), `
		SELECT status, metadata #>> '{steps,2,status}'
		FROM import_log
		WHERE user_id = $1 AND filename = 'sync-callback.pdf'
		ORDER BY created_at DESC
		LIMIT 1`,
		userID,
	).Scan(&logStatus, &markdownStatus); err != nil {
		t.Fatalf("select gear pdf import_log: %v", err)
	}
	if logStatus != "completed" || markdownStatus != "skipped" {
		t.Fatalf("gear pdf import_log after synchronous callback = status %q markdown_step %q, want completed/skipped", logStatus, markdownStatus)
	}
}

func TestUploadGearPDFPublishFailureKeepsUploadCompleted(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	gearID := insertTestGear(t, pool, userID, "Test Bike", "bike", "bike")
	fs := newActivityPhotoStorageTestFS(t, pool)
	pub := &recordingDocumentPublisher{
		onPublish: func(event.DocumentUploadedEvent) error {
			return errors.New("broker unavailable")
		},
	}
	h := &Handler{DB: db, ShadowFS: fs, DocumentUploadedPublisher: pub}

	req := newGearDocumentUploadRequest(t, userID, gearID, "broker-down.pdf", "application/pdf", []byte("%PDF-1.4\n%%EOF\n"), "manual")
	rr := httptest.NewRecorder()
	h.UploadGearDocument(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("UploadGearDocument pdf status = %d, want %d; body=%s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	var response struct {
		ID               string `json:"id"`
		ProcessingStatus string `json:"processing_status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if response.ProcessingStatus != "failed" {
		t.Fatalf("pdf response processing_status = %q, want failed", response.ProcessingStatus)
	}

	var dbStatus string
	if err := pool.QueryRow(context.Background(), `
		SELECT processing_status
		FROM gear_documents
		WHERE id = $1 AND user_id = $2`,
		response.ID, userID,
	).Scan(&dbStatus); err != nil {
		t.Fatalf("select gear document processing_status: %v", err)
	}
	if dbStatus != "failed" {
		t.Fatalf("gear document processing_status = %q, want failed", dbStatus)
	}

	var logStatus string
	var errorMsg *string
	var textStep, markdownStep string
	if err := pool.QueryRow(context.Background(), `
		SELECT status, error_msg, metadata #>> '{steps,1,status}', metadata #>> '{steps,2,status}'
		FROM import_log
		WHERE user_id = $1 AND filename = 'broker-down.pdf'
		ORDER BY created_at DESC
		LIMIT 1`,
		userID,
	).Scan(&logStatus, &errorMsg, &textStep, &markdownStep); err != nil {
		t.Fatalf("select gear pdf import_log: %v", err)
	}
	if logStatus != "completed" || errorMsg != nil || textStep != "failed" || markdownStep != "failed" {
		t.Fatalf("gear pdf import_log after publish failure = status %q error %v text %q markdown %q, want completed/<nil>/failed/failed",
			logStatus, errorMsg, textStep, markdownStep)
	}
}

func TestUploadGearDocumentSniffsPDFDespiteImageHeader(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	gearID := insertTestGear(t, pool, userID, "Test Bike", "bike", "bike")
	fs := newActivityPhotoStorageTestFS(t, pool)
	pub := &recordingDocumentPublisher{}
	h := &Handler{DB: db, ShadowFS: fs, DocumentUploadedPublisher: pub}

	req := newGearDocumentUploadRequest(t, userID, gearID, "wrong-header.pdf", "image/jpeg", []byte("%PDF-1.4\n%%EOF\n"), "manual")
	rr := httptest.NewRecorder()
	h.UploadGearDocument(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("UploadGearDocument mislabeled pdf status = %d, want %d; body=%s", rr.Code, http.StatusCreated, rr.Body.String())
	}
	if len(pub.events) != 1 {
		t.Fatalf("mislabeled pdf published document events = %d, want 1", len(pub.events))
	}
	var importType, contentType string
	if err := pool.QueryRow(context.Background(), `
		SELECT import_type, metadata->>'content_type'
		FROM import_log
		WHERE user_id = $1 AND filename = 'wrong-header.pdf'
		ORDER BY created_at DESC
		LIMIT 1`,
		userID,
	).Scan(&importType, &contentType); err != nil {
		t.Fatalf("select mislabeled pdf import_log: %v", err)
	}
	if importType != "document" || contentType != "application/pdf" {
		t.Fatalf("mislabeled pdf import_log type/content = %q/%q, want document/application/pdf", importType, contentType)
	}
}

func TestUploadGearDocumentInvalidAmountDoesNotStoreFile(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	gearID := insertTestGear(t, pool, userID, "Test Bike", "bike", "bike")
	fs := newActivityPhotoStorageTestFS(t, pool)
	h := &Handler{DB: db, ShadowFS: fs}

	req := newGearDocumentUploadRequestWithFields(t, userID, gearID, "bad-receipt.pdf", "application/pdf", []byte("%PDF-1.4\n%%EOF\n"), map[string]string{
		"doc_type":        "receipt",
		"document_amount": "-1",
	})
	rr := httptest.NewRecorder()
	h.UploadGearDocument(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("UploadGearDocument invalid amount status = %d, want %d; body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}

	var storedFiles int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM user_files
		WHERE user_id = $1 AND file_name = 'bad-receipt.pdf'`,
		userID,
	).Scan(&storedFiles); err != nil {
		t.Fatalf("count stored files: %v", err)
	}
	if storedFiles != 0 {
		t.Fatalf("stored files for invalid upload = %d, want 0", storedFiles)
	}

	var logStatus string
	if err := pool.QueryRow(context.Background(), `
		SELECT status
		FROM import_log
		WHERE user_id = $1 AND filename = 'bad-receipt.pdf'
		ORDER BY created_at DESC
		LIMIT 1`,
		userID,
	).Scan(&logStatus); err != nil {
		t.Fatalf("select invalid amount import_log: %v", err)
	}
	if logStatus != "error" {
		t.Fatalf("invalid amount import_log status = %q, want error", logStatus)
	}
}

func newGearDocumentUploadRequest(t *testing.T, userID, gearID, filename, contentType string, data []byte, docType string) *http.Request {
	fields := map[string]string{}
	if docType != "" {
		fields["doc_type"] = docType
	}
	return newGearDocumentUploadRequestWithFields(t, userID, gearID, filename, contentType, data, fields)
}

func newGearDocumentUploadRequestWithFields(t *testing.T, userID, gearID, filename, contentType string, data []byte, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := mw.WriteField(key, value); err != nil {
			t.Fatalf("write %s field: %v", key, err)
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := mw.CreatePart(header)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gear/"+gearID+"/documents", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", gearID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	ctx = withUserAndAdmin(ctx, userID, false)
	return req.WithContext(ctx)
}
