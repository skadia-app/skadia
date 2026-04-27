package handler

// Integration tests for GetMapLayers, CreateMapLayer, PatchMapLayer,
// DeleteMapLayer, BulkSortMapLayers.
//
// Requires TEST_DATABASE_URL; skipped when unset.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
)

// newMapLayerHandler returns a Handler backed by the live test DB.
func newMapLayerHandler(t *testing.T) *Handler {
	t.Helper()
	pool := fusionTestPool(t) // defined in testhelpers_test.go
	return &Handler{DB: &database.DB{Pool: pool}}
}

// insertTestMapLayer inserts a raster layer and schedules cleanup.
func insertTestMapLayer(t *testing.T, h *Handler, key, name string) int64 {
	t.Helper()
	tpl := "https://tile.example.com/{z}/{x}/{y}.png"
	body := map[string]any{
		"key":          key,
		"role":         "base",
		"name":         name,
		"source_kind":  "raster_tiles",
		"url_template": tpl,
		"enabled":      true,
		"sort_order":   100,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/map-layers",
		bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), "admin-test-uid", true))
	rr := httptest.NewRecorder()
	h.CreateMapLayer(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("CreateMapLayer setup: got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	t.Cleanup(func() {
		h.DB.Pool.Exec(context.Background(), "DELETE FROM map_layers WHERE id = $1", resp.ID) //nolint:errcheck
	})
	return resp.ID
}

func TestGetMapLayers_Returns7PlusSeed(t *testing.T) {
	h := newMapLayerHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/map-layers", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), "any-user", false))
	rr := httptest.NewRecorder()
	h.GetMapLayers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Layers           []map[string]any            `json:"layers"`
		SystemVisibility map[string]map[string]any   `json:"system_visibility"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Layers) < 7 {
		t.Errorf("expected at least 7 layers (seed), got %d", len(resp.Layers))
	}
	if len(resp.SystemVisibility) != 14 {
		t.Errorf("expected 14 system_visibility entries, got %d", len(resp.SystemVisibility))
	}
}

func TestCreateMapLayer_IsSystemForcedFalse(t *testing.T) {
	h := newMapLayerHandler(t)
	tpl := "https://tile.example.com/{z}/{x}/{y}.png"
	body := map[string]any{
		"key":          "test-force-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		"role":         "base",
		"name":         "Test Force",
		"source_kind":  "raster_tiles",
		"url_template": tpl,
		"is_system":    true, // client tries to set this — must be ignored
		"enabled":      true,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/map-layers", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), "admin", true))
	rr := httptest.NewRecorder()
	h.CreateMapLayer(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var row struct {
		ID       int64 `json:"id"`
		IsSystem bool  `json:"is_system"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &row); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if row.IsSystem {
		t.Error("is_system must be false regardless of what client sends")
	}
	// Cleanup
	h.DB.Pool.Exec(context.Background(), "DELETE FROM map_layers WHERE id = $1", row.ID) //nolint:errcheck
}

func TestPatchMapLayer_NameChange(t *testing.T) {
	h := newMapLayerHandler(t)
	id := insertTestMapLayer(t, h, "patch-name-"+strconv.FormatInt(time.Now().UnixNano(), 36), "Original Name")

	body := map[string]any{"name": "Updated Name"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/admin/map-layers/"+strconv.FormatInt(id, 10), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchMapLayer(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPatchMapLayer_SystemLayer_ImmutableField(t *testing.T) {
	h := newMapLayerHandler(t)
	var sysID int64
	err := h.DB.Pool.QueryRow(context.Background(),
		`SELECT id, updated_at FROM map_layers WHERE is_system = true LIMIT 1`).Scan(&sysID, new(time.Time))
	if err != nil {
		t.Skipf("no system layer: %v", err)
	}

	body := map[string]any{
		"url_template": "https://new.example.com/{z}/{x}/{y}.png",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/admin/map-layers/"+strconv.FormatInt(sysID, 10), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	// URL changes on system layers require If-Match — but system layer will
	// reject before we even get to that check (ErrSystemLayerImmutable).
	req.Header.Set("X-If-Match-Updated-At", time.Now().Format(time.RFC3339Nano))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(sysID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchMapLayer(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPatchMapLayer_UrlTemplate_NoIfMatch_Returns428(t *testing.T) {
	h := newMapLayerHandler(t)
	id := insertTestMapLayer(t, h, "patch-no-ifmatch-"+strconv.FormatInt(time.Now().UnixNano(), 36), "No IfMatch Test")

	body := map[string]any{"url_template": "https://new.example.com/{z}/{x}/{y}.png"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/admin/map-layers/"+strconv.FormatInt(id, 10), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	// No X-If-Match-Updated-At — must return 428 Precondition Required.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchMapLayer(rr, req)

	if rr.Code != http.StatusPreconditionRequired {
		t.Errorf("expected 428, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPatchMapLayer_StaleIfMatch_Returns412(t *testing.T) {
	h := newMapLayerHandler(t)
	id := insertTestMapLayer(t, h, "stale-ifmatch-"+strconv.FormatInt(time.Now().UnixNano(), 36), "Stale Test")

	past := time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano)
	body := map[string]any{"url_template": "https://new.example.com/{z}/{x}/{y}.png"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/admin/map-layers/"+strconv.FormatInt(id, 10), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-If-Match-Updated-At", past)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchMapLayer(rr, req)

	if rr.Code != http.StatusPreconditionFailed {
		t.Errorf("expected 412, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteMapLayer_SystemLayer_Returns409(t *testing.T) {
	h := newMapLayerHandler(t)
	var sysID int64
	err := h.DB.Pool.QueryRow(context.Background(),
		`SELECT id FROM map_layers WHERE is_system = true LIMIT 1`).Scan(&sysID)
	if err != nil {
		t.Skipf("no system layer: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/admin/map-layers/"+strconv.FormatInt(sysID, 10), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(sysID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.DeleteMapLayer(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBulkSortMapLayers_ReflectsNewOrder(t *testing.T) {
	h := newMapLayerHandler(t)
	ts := strconv.FormatInt(time.Now().UnixNano(), 36)
	id1 := insertTestMapLayer(t, h, "sort-a-"+ts, "Sort A")
	id2 := insertTestMapLayer(t, h, "sort-b-"+ts, "Sort B")

	body := []map[string]any{
		{"id": id1, "sort_order": 50},
		{"id": id2, "sort_order": 10},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/map-layers/sort", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.BulkSortMapLayers(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	var so1, so2 int32
	h.DB.Pool.QueryRow(context.Background(), "SELECT sort_order FROM map_layers WHERE id=$1", id1).Scan(&so1) //nolint:errcheck
	h.DB.Pool.QueryRow(context.Background(), "SELECT sort_order FROM map_layers WHERE id=$1", id2).Scan(&so2) //nolint:errcheck
	if so1 != 50 || so2 != 10 {
		t.Errorf("sort_order mismatch: id1=%d (want 50), id2=%d (want 10)", so1, so2)
	}
}

// Ensure middleware is used (withUserAndAdmin uses middleware.UserIDKey).
var _ = middleware.UserIDKey
