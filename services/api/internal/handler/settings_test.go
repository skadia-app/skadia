package handler

// Unit tests for GetSettingsLimits — no DB required.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/settings"
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

func TestGetMeIncludesLocaleAndUnits(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	uid := insertTestUser(t, pool)

	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET locale = 'fr', units = 'imperial' WHERE id = $1`, uid); err != nil {
		t.Fatalf("set user preferences: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.GetMe(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Locale string `json:"locale"`
		Units  string `json:"units"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Locale != "fr" {
		t.Fatalf("locale = %q, want fr", resp.Locale)
	}
	if resp.Units != "imperial" {
		t.Fatalf("units = %q, want imperial", resp.Units)
	}
}

func TestUpdateMePersistsLocaleAndUnits(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	uid := insertTestUser(t, pool)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/me",
		strings.NewReader(`{"locale":"it","units":"imperial"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.UpdateMe(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Locale string `json:"locale"`
		Units  string `json:"units"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Locale != "it" || resp.Units != "imperial" {
		t.Fatalf("response locale/units = %q/%q, want it/imperial", resp.Locale, resp.Units)
	}

	var locale, units string
	if err := pool.QueryRow(context.Background(),
		`SELECT locale, units FROM users WHERE id = $1`, uid).Scan(&locale, &units); err != nil {
		t.Fatalf("select user preferences: %v", err)
	}
	if locale != "it" || units != "imperial" {
		t.Fatalf("stored locale/units = %q/%q, want it/imperial", locale, units)
	}
}

func TestUpdateMeRejectsInvalidLocaleAndUnits(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	uid := insertTestUser(t, pool)

	tests := []struct {
		name string
		body string
	}{
		{name: "locale", body: `{"locale":"pt"}`},
		{name: "units", body: `{"units":"nautical"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/me", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uid))
			rr := httptest.NewRecorder()
			h.UpdateMe(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}
