package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/settings"
)

func TestAdminGetSettingsDoesNotMaskEmptyEncryptedSMTPPassword(t *testing.T) {
	st := settings.NewStoreForTest(map[string]string{
		"smtp.host":     "smtp.example.test",
		"smtp.port":     "25",
		"smtp.username": "",
		"smtp.password": "",
		"smtp.from":     "noreply@example.test",
		"smtp.tls":      "none",
	})
	h := &Handler{Settings: st}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)

	h.AdminGetSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var smtp map[string]settings.CachedSetting
	if err := json.Unmarshal(raw["smtp"], &smtp); err != nil {
		t.Fatalf("decode smtp settings: %v", err)
	}
	password := smtp["password"]
	if password.Value != "" || password.HasValue {
		t.Fatalf("smtp password value/has_value = %q/%v, want empty/false", password.Value, password.HasValue)
	}
}

func TestAdminGetSettingsExposesOAuthEncryptionReadiness(t *testing.T) {
	st := settings.NewStoreForTest(map[string]string{})
	h := &Handler{
		Settings: st,
		Config:   &HandlerConfig{OAuthEncryptionKey: ""},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)

	h.AdminGetSettings(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var meta map[string]string
	if err := json.Unmarshal(raw["_meta"], &meta); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	if meta["oauth_encryption_configured"] != "false" {
		t.Fatalf("oauth_encryption_configured=%q, want false", meta["oauth_encryption_configured"])
	}
}
