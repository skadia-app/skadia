package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestGetImports_ImportTypeQueryParam(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertTestUser(t, pool)
	h := newTestHandler(t, pool)
	db := h.DB

	photoType := model.ImportTypePhoto
	if _, err := db.InsertImportLog(context.Background(), model.ImportLogEntry{
		UserID: userID, SourcePlatform: "upload", Filename: "img.jpg",
		Status: "queued", ImportType: &photoType,
	}); err != nil {
		t.Fatalf("insert photo entry: %v", err)
	}
	actType := model.ImportTypeActivity
	if _, err := db.InsertImportLog(context.Background(), model.ImportLogEntry{
		UserID: userID, SourcePlatform: "upload", Filename: "run.fit",
		Status: "completed", ImportType: &actType,
	}); err != nil {
		t.Fatalf("insert activity entry: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/imports?import_type=photo", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	w := httptest.NewRecorder()

	h.GetImports(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Entries []model.ImportLogEntry `json:"entries"`
		Total   int                    `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || len(resp.Entries) != 1 {
		t.Fatalf("want 1 photo entry, got total=%d len=%d", resp.Total, len(resp.Entries))
	}
	if !strings.Contains(resp.Entries[0].Filename, "img.jpg") {
		t.Fatalf("want img.jpg, got %s", resp.Entries[0].Filename)
	}
}

func TestGetImports_InvalidImportTypeReturns400(t *testing.T) {
	pool := testdb.Open(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)

	req := httptest.NewRequest("GET", "/api/v1/imports?import_type=foto", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	w := httptest.NewRecorder()

	h.GetImports(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d body=%s", w.Code, w.Body.String())
	}
}
