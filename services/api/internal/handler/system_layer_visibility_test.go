package handler

// Integration tests for GetSystemLayerKeys and PatchSystemLayerVisibility.
// Requires TEST_DATABASE_URL; skipped when unset.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/go-chi/chi/v5"
)

func TestGetSystemLayerKeys_Returns14(t *testing.T) {
	h := &Handler{} // no DB needed — code constant, no DB round-trip
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-layers/keys", nil)
	rr := httptest.NewRecorder()
	h.GetSystemLayerKeys(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var keys []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &keys); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(keys) != len(model.SystemLayers) {
		t.Errorf("expected %d keys, got %d", len(model.SystemLayers), len(keys))
	}
}

func TestGetSystemLayerKeys_IsPublic(t *testing.T) {
	// Endpoint must work without a user in context (public route).
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-layers/keys", nil)
	// No user context set.
	rr := httptest.NewRecorder()
	h.GetSystemLayerKeys(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("public endpoint returned %d, expected 200", rr.Code)
	}
}

func TestPatchSystemLayerVisibility_EnabledFalse(t *testing.T) {
	pool := fusionTestPool(t)
	h := &Handler{DB: &database.DB{Pool: pool}}

	body := map[string]any{"enabled": false}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/admin/system-layer-visibility/waters", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("key", "waters")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchSystemLayerVisibility(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Reset to true so we don't pollute other tests.
	t.Cleanup(func() {
		pool.Exec(context.Background(), //nolint:errcheck
			`UPDATE system_layer_visibility SET enabled = true WHERE layer_key = 'waters'`)
	})
}

func TestPatchSystemLayerVisibility_UnknownKey_Returns404(t *testing.T) {
	pool := fusionTestPool(t)
	h := &Handler{DB: &database.DB{Pool: pool}}

	body := map[string]any{"enabled": false}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/admin/system-layer-visibility/nonexistent", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("key", "nonexistent")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchSystemLayerVisibility(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPatchSystemLayerVisibility_UnknownField_Returns400(t *testing.T) {
	pool := fusionTestPool(t)
	h := &Handler{DB: &database.DB{Pool: pool}}

	body := map[string]any{"nonexistent_field": true}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/admin/system-layer-visibility/waters", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("key", "waters")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchSystemLayerVisibility(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// patchOpacity issues a PATCH /admin/system-layer-visibility/waters with the
// given default_opacity value and returns the recorder.
func patchOpacity(t *testing.T, h *Handler, value any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"default_opacity": value})
	req := httptest.NewRequest(http.MethodPatch, "/admin/system-layer-visibility/waters", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("key", "waters")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.PatchSystemLayerVisibility(rr, req)
	return rr
}

func TestPatchSystemLayerVisibility_OpacityOutOfRange_Negative(t *testing.T) {
	pool := fusionTestPool(t)
	h := &Handler{DB: &database.DB{Pool: pool}}

	rr := patchOpacity(t, h, -5)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("invalid_opacity")) {
		t.Errorf("expected invalid_opacity error code, got %s", rr.Body.String())
	}
}

func TestPatchSystemLayerVisibility_OpacityOutOfRange_TooLarge(t *testing.T) {
	pool := fusionTestPool(t)
	h := &Handler{DB: &database.DB{Pool: pool}}

	rr := patchOpacity(t, h, 150)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("invalid_opacity")) {
		t.Errorf("expected invalid_opacity error code, got %s", rr.Body.String())
	}
}

func TestPatchSystemLayerVisibility_OpacityBoundary_Zero(t *testing.T) {
	pool := fusionTestPool(t)
	h := &Handler{DB: &database.DB{Pool: pool}}

	rr := patchOpacity(t, h, 0)
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), //nolint:errcheck
			`UPDATE system_layer_visibility SET default_opacity = NULL WHERE layer_key = 'waters'`)
	})
}

func TestPatchSystemLayerVisibility_OpacityBoundary_Hundred(t *testing.T) {
	pool := fusionTestPool(t)
	h := &Handler{DB: &database.DB{Pool: pool}}

	rr := patchOpacity(t, h, 100)
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), //nolint:errcheck
			`UPDATE system_layer_visibility SET default_opacity = NULL WHERE layer_key = 'waters'`)
	})
}
