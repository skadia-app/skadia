package handler

// Unit tests for GetSettingsLimits — no DB required.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/google/uuid"
)

func newLimitsHandler(t *testing.T, overrides map[string]string) *Handler {
	t.Helper()
	defaults := map[string]string{
		"library.user_map_layer_limit":                "50",
		"library.user_map_layer_rate_limit_per_10min": "60",
	}
	for k, v := range overrides {
		defaults[k] = v
	}
	return &Handler{
		Settings: settings.NewStoreForTest(defaults),
	}
}

func TestGetSettingsLimits_Authed(t *testing.T) {
	h := newLimitsHandler(t, nil)
	uid := uuid.New().String()

	req := httptest.NewRequest(http.MethodGet, "/settings/limits", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.GetSettingsLimits(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]int
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["user_map_layer_limit"] != 50 {
		t.Errorf("user_map_layer_limit: want 50, got %d", resp["user_map_layer_limit"])
	}
	if resp["user_map_layer_rate_limit_per_10min"] != 60 {
		t.Errorf("user_map_layer_rate_limit_per_10min: want 60, got %d", resp["user_map_layer_rate_limit_per_10min"])
	}
}

func TestGetSettingsLimits_CustomValues(t *testing.T) {
	h := newLimitsHandler(t, map[string]string{
		"library.user_map_layer_limit":                "25",
		"library.user_map_layer_rate_limit_per_10min": "10",
	})
	uid := uuid.New().String()

	req := httptest.NewRequest(http.MethodGet, "/settings/limits", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.GetSettingsLimits(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]int
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["user_map_layer_limit"] != 25 {
		t.Errorf("user_map_layer_limit: want 25, got %d", resp["user_map_layer_limit"])
	}
	if resp["user_map_layer_rate_limit_per_10min"] != 10 {
		t.Errorf("user_map_layer_rate_limit_per_10min: want 10, got %d", resp["user_map_layer_rate_limit_per_10min"])
	}
}
