package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/settings"
)

func TestGetLibraryDirResponseShape(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)

	// Wire settings — inline SettingsStore with the test pool.
	db := &database.DB{Pool: pool}
	store := settings.NewSettingsStore(db, "", "local")

	// Upsert the setting so store.Get returns "true".
	_, err := pool.Exec(context.Background(),
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		"library.user_can_edit_dir_name", "true")
	if err != nil {
		t.Fatalf("upsert setting: %v", err)
	}
	if err := store.Reload(context.Background()); err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), //nolint:errcheck
			`DELETE FROM server_settings WHERE key = 'library.user_can_edit_dir_name'`)
	})

	h := &Handler{
		DB:       db,
		Settings: store,
	}

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)
	req := httptest.NewRequest("GET", "/api/v1/settings/library-dir", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.GetLibraryDir(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Must have new fields.
	if _, ok := body["effective_name"]; !ok {
		t.Error("missing effective_name")
	}
	if _, ok := body["editable"]; !ok {
		t.Error("missing editable")
	}
	if _, ok := body["backend"]; !ok {
		t.Error("missing backend")
	}
	if _, ok := body["admin_managed_reason"]; !ok {
		t.Error("missing admin_managed_reason")
	}

	// canEdit=true, backend=local → editable=true.
	if body["editable"] != true {
		t.Errorf("editable: got %v, want true (canEdit=true, local backend)", body["editable"])
	}
	// backend must be "local" (no Config set).
	if body["backend"] != "local" {
		t.Errorf("backend: got %v, want local", body["backend"])
	}
	// effective_name must equal userID (no library_dir_name set).
	if body["effective_name"] != userID {
		t.Errorf("effective_name: got %v, want %q", body["effective_name"], userID)
	}
	// admin_managed_reason must be null (canEdit=true, backend=local).
	if body["admin_managed_reason"] != nil {
		t.Errorf("admin_managed_reason: got %v, want null", body["admin_managed_reason"])
	}
}

func TestGetLibraryDirLockdownReason(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)

	db := &database.DB{Pool: pool}
	store := settings.NewSettingsStore(db, "", "local")

	// Set user_can_edit_dir_name = "false".
	_, err := pool.Exec(context.Background(),
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		"library.user_can_edit_dir_name", "false")
	if err != nil {
		t.Fatalf("upsert setting: %v", err)
	}
	if err := store.Reload(context.Background()); err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), //nolint:errcheck
			`DELETE FROM server_settings WHERE key = 'library.user_can_edit_dir_name'`)
	})

	h := &Handler{
		DB:       db,
		Settings: store,
	}

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)
	req := httptest.NewRequest("GET", "/api/v1/settings/library-dir", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.GetLibraryDir(w, req)

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["editable"] != false {
		t.Errorf("editable: got %v, want false (admin_locked)", body["editable"])
	}
	if body["admin_managed_reason"] != "admin_locked" {
		t.Errorf("admin_managed_reason: got %v, want admin_locked", body["admin_managed_reason"])
	}
}

func TestGetLibraryDirS3BackendReason(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)

	db := &database.DB{Pool: pool}
	store := settings.NewSettingsStore(db, "", "s3")

	// Even with user_can_edit_dir_name = "true", s3 wins in priority.
	_, err := pool.Exec(context.Background(),
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		"library.user_can_edit_dir_name", "true")
	if err != nil {
		t.Fatalf("upsert setting: %v", err)
	}
	if err := store.Reload(context.Background()); err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), //nolint:errcheck
			`DELETE FROM server_settings WHERE key = 'library.user_can_edit_dir_name'`)
	})

	h := &Handler{
		DB:       db,
		Settings: store,
		Config:   &HandlerConfig{StorageBackend: "s3"},
	}

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)
	req := httptest.NewRequest("GET", "/api/v1/settings/library-dir", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.GetLibraryDir(w, req)

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["backend"] != "s3" {
		t.Errorf("backend: got %v, want s3", body["backend"])
	}
	if body["editable"] != false {
		t.Errorf("editable: got %v, want false (s3 backend)", body["editable"])
	}
	if body["admin_managed_reason"] != "s3_backend" {
		t.Errorf("admin_managed_reason: got %v, want s3_backend", body["admin_managed_reason"])
	}
}
