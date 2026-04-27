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
	"strings"
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

// TestPatchMapLayer_OpacityFractional_Rejected verifies finding #7: a JSON
// number with a non-integer fraction must be rejected before SMALLINT cast.
func TestPatchMapLayer_OpacityFractional_Rejected(t *testing.T) {
	h := newMapLayerHandler(t)
	id := insertTestMapLayer(t, h, "opacity-frac-"+strconv.FormatInt(time.Now().UnixNano(), 36), "Opacity Frac")

	body := map[string]any{"default_opacity": 42.7}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/admin/map-layers/"+strconv.FormatInt(id, 10), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchMapLayer(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for fractional opacity, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] != "invalid_opacity" {
		t.Errorf("expected error=invalid_opacity, got %q", resp["error"])
	}
}

// TestPatchMapLayer_StyleURLTooLong verifies finding #4/#13: style_url longer
// than 2048 chars is rejected with style_url_too_long before reaching the DB.
func TestPatchMapLayer_StyleURLTooLong(t *testing.T) {
	h := newMapLayerHandler(t)
	id := insertTestMapLayer(t, h, "style-long-"+strconv.FormatInt(time.Now().UnixNano(), 36), "Style Long")

	long := "https://example.com/" + strings.Repeat("x", 2050) + "/style.json"
	body := map[string]any{"style_url": long}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/admin/map-layers/"+strconv.FormatInt(id, 10), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-If-Match-Updated-At", time.Now().Format(time.RFC3339Nano))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchMapLayer(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for too-long style_url, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["error"] != "style_url_too_long" {
		t.Errorf("expected error=style_url_too_long, got %q", resp["error"])
	}
}

// TestPatchMapLayer_StyleURLBadScheme verifies an ftp:// or javascript: scheme
// is rejected (#4).
func TestPatchMapLayer_StyleURLBadScheme(t *testing.T) {
	h := newMapLayerHandler(t)
	id := insertTestMapLayer(t, h, "style-scheme-"+strconv.FormatInt(time.Now().UnixNano(), 36), "Style Scheme")

	body := map[string]any{"style_url": "ftp://example.com/style.json"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/admin/map-layers/"+strconv.FormatInt(id, 10), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-If-Match-Updated-At", time.Now().Format(time.RFC3339Nano))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchMapLayer(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for bad style_url scheme, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestPatchMapLayer_StyleURLRelativeAllowed: same-origin /api/v1/ paths must pass.
func TestPatchMapLayer_StyleURLRelativeAllowed(t *testing.T) {
	h := newMapLayerHandler(t)
	// Seed a vector_style layer so style_url is a meaningful field.
	tpl := "https://tile.example.com/{z}/{x}/{y}.png"
	_ = tpl
	body := map[string]any{
		"key":         "style-rel-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		"role":        "base",
		"name":        "Style Rel",
		"source_kind": "vector_style",
		"style_url":   "/api/v1/tiles/style.json",
		"enabled":     true,
		"sort_order":  100,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/map-layers", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), "admin", true))
	rr := httptest.NewRecorder()
	h.CreateMapLayer(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for /api/v1/ style_url, got %d: %s", rr.Code, rr.Body.String())
	}
	var row struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &row)
	t.Cleanup(func() {
		h.DB.Pool.Exec(context.Background(), "DELETE FROM map_layers WHERE id = $1", row.ID) //nolint:errcheck
	})
}

// TestCreateMapLayer_InvalidCapability verifies finding #12: an unknown
// capability key on POST is rejected.
func TestCreateMapLayer_InvalidCapability(t *testing.T) {
	h := newMapLayerHandler(t)
	bogus := "totally_made_up_capability"
	body := map[string]any{
		"key":                 "bad-cap-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		"role":                "base",
		"name":                "Bad Cap",
		"source_kind":         "raster_tiles",
		"url_template":        "https://tile.example.com/{z}/{x}/{y}.png",
		"requires_capability": bogus,
		"enabled":             true,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/map-layers", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), "admin", true))
	rr := httptest.NewRecorder()
	h.CreateMapLayer(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for unknown capability, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["error"] != "invalid_capability" {
		t.Errorf("expected error=invalid_capability, got %q", resp["error"])
	}
}

// TestPatchMapLayer_InvalidCapability verifies #12 on the PATCH path.
func TestPatchMapLayer_InvalidCapability(t *testing.T) {
	h := newMapLayerHandler(t)
	id := insertTestMapLayer(t, h, "patch-bad-cap-"+strconv.FormatInt(time.Now().UnixNano(), 36), "Patch Bad Cap")

	body := map[string]any{"requires_capability": "nope_not_real"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/admin/map-layers/"+strconv.FormatInt(id, 10), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchMapLayer(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for unknown capability, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["error"] != "invalid_capability" {
		t.Errorf("expected error=invalid_capability, got %q", resp["error"])
	}
}

// TestBulkSortMapLayers_EmptyBody verifies finding #19: empty items array
// returns 422 items_required instead of silently succeeding.
func TestBulkSortMapLayers_EmptyBody(t *testing.T) {
	h := newMapLayerHandler(t)
	body := []map[string]any{}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/map-layers/sort", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.BulkSortMapLayers(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty bulk-sort, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["error"] != "items_required" {
		t.Errorf("expected error=items_required, got %q", resp["error"])
	}
}

// TestBulkSortMapLayers_CtxCanceled verifies finding #9: a canceled request
// short-circuits the retry loop instead of returning serialization_retry_exhausted.
func TestBulkSortMapLayers_CtxCanceled(t *testing.T) {
	h := newMapLayerHandler(t)
	ts := strconv.FormatInt(time.Now().UnixNano(), 36)
	id := insertTestMapLayer(t, h, "ctx-cancel-"+ts, "Ctx Cancel")

	body := []map[string]any{{"id": id, "sort_order": 1}}
	b, _ := json.Marshal(body)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled
	req := httptest.NewRequest(http.MethodPost, "/admin/map-layers/sort", bytes.NewReader(b)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.BulkSortMapLayers(rr, req)

	// Either 0 (handler returned without writing) or any non-409 status is
	// acceptable; what we MUST NOT see is 409 serialization_retry_exhausted.
	if rr.Code == http.StatusConflict {
		var resp map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp["error"] == "serialization_retry_exhausted" {
			t.Fatalf("canceled context produced serialization_retry_exhausted; ctx-cancel short-circuit broken")
		}
	}
}

// Ensure middleware is used (withUserAndAdmin uses middleware.UserIDKey).
var _ = middleware.UserIDKey
