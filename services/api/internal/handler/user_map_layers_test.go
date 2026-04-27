package handler

// Integration tests for GetUserMapLayers, CreateUserMapLayer,
// PatchUserMapLayer, DeleteUserMapLayer, BulkSortUserMapLayers.
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

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func newUserMapLayerHandler(t *testing.T) *Handler {
	t.Helper()
	pool := fusionTestPool(t)
	return &Handler{
		DB: &database.DB{Pool: pool},
		Settings: settings.NewStoreForTest(map[string]string{
			"library.user_map_layer_limit":                "50",
			"library.user_map_layer_rate_limit_per_10min": "60",
		}),
	}
}

// insertTestUserMapLayer inserts a layer for uid and schedules cleanup.
func insertTestUserMapLayer(t *testing.T, h *Handler, uid, name string) int64 {
	t.Helper()
	body := map[string]any{
		"name":         name,
		"url_template": "https://tiles.example.com/{z}/{x}/{y}.png",
		"enabled":      true,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/map-layers/user/", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.CreateUserMapLayer(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("insertTestUserMapLayer: got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	t.Cleanup(func() {
		h.DB.Pool.Exec(context.Background(), "DELETE FROM user_map_layers WHERE id = $1", resp.ID) //nolint:errcheck
	})
	return resp.ID
}

func TestUserIsolation(t *testing.T) {
	pool := fusionTestPool(t)
	h := newUserMapLayerHandler(t)
	h.DB = &database.DB{Pool: pool}

	uidA := insertTestUser(t, pool)
	uidB := insertTestUser(t, pool)

	insertTestUserMapLayer(t, h, uidA, "Layer A")

	// User B must not see User A's layers.
	req := httptest.NewRequest(http.MethodGet, "/map-layers/user/", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uidB))
	rr := httptest.NewRecorder()
	h.GetUserMapLayers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("user B saw %d layers (should be 0); isolation broken", len(list))
	}
}

func TestCreateUserMapLayer_LimitReached(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	h := &Handler{
		DB: &database.DB{Pool: pool},
		Settings: settings.NewStoreForTest(map[string]string{
			"library.user_map_layer_limit":                "1",
			"library.user_map_layer_rate_limit_per_10min": "60",
		}),
	}

	insertTestUserMapLayer(t, h, uid, "First Layer")

	body := map[string]any{
		"name":         "Second Layer",
		"url_template": "https://tiles.example.com/{z}/{x}/{y}.png",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/map-layers/user/", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.CreateUserMapLayer(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] != "limit_reached" {
		t.Errorf("expected error=limit_reached, got %q", resp["error"])
	}
}

func TestCreateUserMapLayer_PrivateIP_Blocked(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	h := newUserMapLayerHandler(t)
	h.DB = &database.DB{Pool: pool}

	body := map[string]any{
		"name":         "Private IP",
		"url_template": "http://192.168.1.1/{z}/{x}/{y}.png",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/map-layers/user/", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.CreateUserMapLayer(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] != "private_address_blocked" {
		t.Errorf("expected error=private_address_blocked, got %q", resp["error"])
	}
}

func TestCreateUserMapLayer_DuplicateName_Returns409(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	h := newUserMapLayerHandler(t)
	h.DB = &database.DB{Pool: pool}

	insertTestUserMapLayer(t, h, uid, "My Layer")

	body := map[string]any{
		"name":         "My Layer",
		"url_template": "https://other.example.com/{z}/{x}/{y}.png",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/map-layers/user/", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.CreateUserMapLayer(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateUserMapLayer_XSSInAttribution_IsSanitized(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	h := newUserMapLayerHandler(t)
	h.DB = &database.DB{Pool: pool}

	xss := `<a href="https://safe.com">OK</a><script>alert(1)</script>`
	body := map[string]any{
		"name":         "XSS Test " + uuid.New().String()[:8],
		"url_template": "https://tiles.example.com/{z}/{x}/{y}.png",
		"attribution":  xss,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/map-layers/user/", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.CreateUserMapLayer(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID          int64   `json:"id"`
		Attribution *string `json:"attribution,omitempty"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	t.Cleanup(func() {
		h.DB.Pool.Exec(context.Background(), "DELETE FROM user_map_layers WHERE id = $1", resp.ID) //nolint:errcheck
	})
	if resp.Attribution == nil {
		t.Fatal("attribution should be present")
	}
	if *resp.Attribution == xss {
		t.Error("XSS payload was not sanitized")
	}
}

// TestPatchUserMapLayer_OpacityFractional verifies finding #7 on the user path.
func TestPatchUserMapLayer_OpacityFractional(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	h := newUserMapLayerHandler(t)
	h.DB = &database.DB{Pool: pool}
	id := insertTestUserMapLayer(t, h, uid, "OpacityFrac")

	body := map[string]any{"default_opacity": 33.5}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/map-layers/user/"+strconv.FormatInt(id, 10), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchUserMapLayer(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for fractional opacity, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestBulkSortUserMapLayers_EmptyBody verifies finding #19 on the user path.
func TestBulkSortUserMapLayers_EmptyBody(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	h := newUserMapLayerHandler(t)
	h.DB = &database.DB{Pool: pool}

	body := []map[string]any{}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/map-layers/user/sort", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.BulkSortUserMapLayers(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty bulk-sort, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["error"] != "items_required" {
		t.Errorf("expected error=items_required, got %q", resp["error"])
	}
}

func TestDeleteUserMapLayer_UserIsolation(t *testing.T) {
	pool := fusionTestPool(t)
	uidA := insertTestUser(t, pool)
	uidB := insertTestUser(t, pool)
	h := newUserMapLayerHandler(t)
	h.DB = &database.DB{Pool: pool}

	layerID := insertTestUserMapLayer(t, h, uidA, "A's Layer")

	// User B tries to delete User A's layer — must 404.
	req := httptest.NewRequest(http.MethodDelete, "/map-layers/user/"+strconv.FormatInt(layerID, 10), nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uidB))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(layerID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.DeleteUserMapLayer(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for cross-user delete, got %d: %s", rr.Code, rr.Body.String())
	}
}
