// services/api/internal/handler/admin_library_settings_test.go
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/settings"
)

// newSettingsHandler returns a Handler with a test-only settings store pre-loaded
// with the given key/value pairs. No DB is required.
func newSettingsHandler(vals map[string]string) *Handler {
	return &Handler{
		Settings: settings.NewStoreForTest(vals),
	}
}

func TestGetAdminLibrarySettingsShape(t *testing.T) {
	h := newSettingsHandler(map[string]string{
		"library.user_can_edit_dir_name":   "true",
		"library.user_can_edit_schemas":    "false",
		"library.default_schema.activity":  "{year}/{month}",
		"library.default_schema.media":     "{year}",
		"library.default_schema.document":  "",
		"library.default_schema.health":    "",
		"library.default_schema.route":     "{sport_type}",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/library-settings", nil)
	rr := httptest.NewRecorder()
	h.GetAdminLibrarySettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if _, ok := resp["user_can_edit_dir_name"]; !ok {
		t.Error("missing key user_can_edit_dir_name")
	}
	if _, ok := resp["user_can_edit_schemas"]; !ok {
		t.Error("missing key user_can_edit_schemas")
	}
	schemas, ok := resp["default_schemas"].(map[string]any)
	if !ok {
		t.Fatal("default_schemas missing or wrong type")
	}
	for _, ft := range userFacingSchemaFileTypes {
		if _, ok := schemas[ft]; !ok {
			t.Errorf("default_schemas missing key %q", ft)
		}
	}
	if len(schemas) != 5 {
		t.Errorf("expected 5 default_schemas keys, got %d", len(schemas))
	}

	// Verify boolean decoding
	if resp["user_can_edit_dir_name"] != true {
		t.Errorf("user_can_edit_dir_name: want true, got %v", resp["user_can_edit_dir_name"])
	}
	if resp["user_can_edit_schemas"] != false {
		t.Errorf("user_can_edit_schemas: want false, got %v", resp["user_can_edit_schemas"])
	}
}

func TestUpdateAdminLibrarySettingsPIIRejected(t *testing.T) {
	h := newSettingsHandler(nil)

	body := map[string]any{
		"user_can_edit_dir_name": false,
		"user_can_edit_schemas":  false,
		"default_schemas": map[string]string{
			"activity": "{year}/{notes}", // "notes" is PII-blacklisted
		},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/admin/library-settings", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	// Inject user ID into context (RequireUserID reads from middleware context)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, "admin-test-user")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.UpdateAdminLibrarySettings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateAdminLibrarySettingsUnknownTypeRejected(t *testing.T) {
	h := newSettingsHandler(nil)

	body := map[string]any{
		"user_can_edit_dir_name": false,
		"user_can_edit_schemas":  false,
		"default_schemas": map[string]string{
			"report": "{year}", // "report" is not in userFacingSchemaFileTypes
		},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/admin/library-settings", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, "admin-test-user")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h.UpdateAdminLibrarySettings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var errResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	detail, _ := errResp["detail"].(string)
	if detail == "" {
		t.Error("expected non-empty error detail")
	}
}

func TestUpdateAdminLibrarySettingsRequiresAdmin(t *testing.T) {
	// Admin enforcement is handled at the middleware level in main.go
	// (RequireAdmin middleware wraps the /admin/* routes). There is no
	// handler-level admin check to unit-test here — this is intentional.
	// Integration test would require a running server with middleware wired.
	t.Skip("admin enforcement is middleware-level; see main.go /admin route group")
}
