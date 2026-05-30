package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

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

func TestGetFileSchemasContextBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("requires DB")
	}
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)

	db := &database.DB{Pool: pool}
	store := settings.NewSettingsStore(db, "", "local")

	// Set user_can_edit_schemas = "true".
	_, err := pool.Exec(context.Background(),
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		"library.user_can_edit_schemas", "true")
	if err != nil {
		t.Fatalf("upsert setting: %v", err)
	}
	if err := store.Reload(context.Background()); err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), //nolint:errcheck
			`DELETE FROM server_settings WHERE key = 'library.user_can_edit_schemas'`)
	})

	h := &Handler{
		DB:       db,
		Settings: store,
	}

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)
	req := httptest.NewRequest("GET", "/api/v1/settings/file-schemas-context", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.GetFileSchemasContext(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	for _, field := range []string{"editable", "backend", "admin_managed_reason"} {
		if _, ok := body[field]; !ok {
			t.Errorf("missing field %q", field)
		}
	}
	// canEdit=true, backend=local → editable=true, reason=null.
	if body["editable"] != true {
		t.Errorf("editable: got %v, want true", body["editable"])
	}
	if body["backend"] != "local" {
		t.Errorf("backend: got %v, want local", body["backend"])
	}
	if body["admin_managed_reason"] != nil {
		t.Errorf("admin_managed_reason: got %v, want null", body["admin_managed_reason"])
	}
}

func TestGetFileSchemasContextS3IsNotEditableEvenWhenUserSchemasEnabled(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	store := settings.NewStoreForTest(map[string]string{
		"library.user_can_edit_schemas": "true",
	})
	h := &Handler{
		DB:       db,
		Settings: store,
		Config:   &HandlerConfig{StorageBackend: "s3"},
	}

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)
	req := httptest.NewRequest("GET", "/api/v1/settings/file-schemas-context", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.GetFileSchemasContext(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Editable           bool    `json:"editable"`
		Backend            string  `json:"backend"`
		AdminManagedReason *string `json:"admin_managed_reason"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Editable {
		t.Fatalf("editable = true, want false for s3 backend")
	}
	if body.Backend != "s3" {
		t.Fatalf("backend = %q, want s3", body.Backend)
	}
	if body.AdminManagedReason == nil || *body.AdminManagedReason != "s3_backend" {
		t.Fatalf("admin_managed_reason = %v, want s3_backend", body.AdminManagedReason)
	}
}

func TestFileSchemaMutationsRejectS3Backend(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	store := settings.NewStoreForTest(map[string]string{
		"library.user_can_edit_schemas": "true",
	})
	h := &Handler{
		DB:       db,
		Settings: store,
		Config:   &HandlerConfig{StorageBackend: "s3"},
	}
	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)

	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		handler http.HandlerFunc
	}{
		{
			name:    "update schema",
			method:  http.MethodPut,
			path:    "/api/v1/settings/file-schemas/activity",
			body:    `{"schema":"{year}/{filename}"}`,
			handler: h.UpdateFileSchema,
		},
		{
			name:    "rename preview",
			method:  http.MethodPost,
			path:    "/api/v1/settings/file-schemas/activity/rename-preview",
			body:    `{"schema":"{year}/{filename}"}`,
			handler: h.PreviewFileRename,
		},
		{
			name:    "rename execute",
			method:  http.MethodPost,
			path:    "/api/v1/settings/file-schemas/activity/rename-execute",
			body:    `{}`,
			handler: h.ExecuteFileRename,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("type", "activity")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			w := httptest.NewRecorder()
			tt.handler(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "s3_backend") {
				t.Fatalf("response does not mention s3_backend: %s", w.Body.String())
			}
		})
	}
}

func TestUpdateLibraryDirLockdown403(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)

	db := &database.DB{Pool: pool}
	store := settings.NewSettingsStore(db, "", "local")

	// Set user_can_edit_dir_name = "false" → gate must block.
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
	body := `{"library_dir_name":"my-library"}`
	req := httptest.NewRequest("PUT", "/api/v1/settings/library-dir",
		strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.UpdateLibraryDir(w, req)

	if w.Code != 403 {
		t.Errorf("expected 403 (admin_locked), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetFileSchemasResponseIsArrayWithNewFields(t *testing.T) {
	if testing.Short() {
		t.Skip("requires DB")
	}
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)

	db := &database.DB{Pool: pool}
	store := settings.NewSettingsStore(db, "", "local")
	if err := store.Reload(context.Background()); err != nil {
		t.Fatalf("reload settings: %v", err)
	}

	h := &Handler{
		DB:       db,
		Settings: store,
	}

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)
	req := httptest.NewRequest("GET", "/api/v1/settings/file-schemas", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.GetFileSchemas(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Top-level shape must remain an array with 5 entries.
	if len(body) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(body))
	}

	for _, entry := range body {
		for _, field := range []string{"file_type", "schema", "effective_schema", "is_user_override"} {
			if _, ok := entry[field]; !ok {
				t.Errorf("entry %v missing field %q", entry["file_type"], field)
			}
		}
		// With no user override, effective_schema must be non-empty (default).
		if entry["effective_schema"] == "" {
			t.Errorf("entry %v: effective_schema must be non-empty", entry["file_type"])
		}
		// With no user override stored, is_user_override must be false.
		if entry["is_user_override"] != false {
			t.Errorf("entry %v: is_user_override: got %v, want false", entry["file_type"], entry["is_user_override"])
		}
	}
}

func TestUpdateFileSchemaPIIRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("requires DB")
	}
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)

	db := &database.DB{Pool: pool}
	store := settings.NewSettingsStore(db, "", "local")

	// Enable user_can_edit_schemas so the gate passes, then hit PII check.
	_, err := pool.Exec(context.Background(),
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		"library.user_can_edit_schemas", "true")
	if err != nil {
		t.Fatalf("upsert setting: %v", err)
	}
	if err := store.Reload(context.Background()); err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), //nolint:errcheck
			`DELETE FROM server_settings WHERE key = 'library.user_can_edit_schemas'`)
	})

	h := &Handler{
		DB:       db,
		Settings: store,
	}

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)
	// "notes" is a PII variable.
	reqBody := `{"schema":"{year}/{notes}"}`
	req := httptest.NewRequest("PUT", "/api/v1/settings/file-schemas/activity",
		strings.NewReader(reqBody)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Wire chi router so URLParam works.
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("type", "activity")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

	h.UpdateFileSchema(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 (PII), got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "notes") {
		t.Errorf("expected 'notes' in error message, got: %s", w.Body.String())
	}
}

func TestUpdateFileSchemaLockdown(t *testing.T) {
	if testing.Short() {
		t.Skip("requires DB")
	}
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)

	db := &database.DB{Pool: pool}
	store := settings.NewSettingsStore(db, "", "local")

	// Set user_can_edit_schemas = "false" → gate must block.
	_, err := pool.Exec(context.Background(),
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		"library.user_can_edit_schemas", "false")
	if err != nil {
		t.Fatalf("upsert setting: %v", err)
	}
	if err := store.Reload(context.Background()); err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), //nolint:errcheck
			`DELETE FROM server_settings WHERE key = 'library.user_can_edit_schemas'`)
	})

	h := &Handler{
		DB:       db,
		Settings: store,
	}

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)
	reqBody := `{"schema":"{year}/{month}/{title}"}`
	req := httptest.NewRequest("PUT", "/api/v1/settings/file-schemas/activity",
		strings.NewReader(reqBody)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("type", "activity")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

	h.UpdateFileSchema(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403 (admin_locked), got %d: %s", w.Code, w.Body.String())
	}
}

// TestUpdateShadowFSSettingsCompatLockdown verifies that the legacy
// PUT /settings/shadow-fs endpoint honours the library.user_can_edit_schemas
// admin gate (regression guard — closed 2026-04-24).
func TestUpdateShadowFSSettingsCompatLockdown(t *testing.T) {
	if testing.Short() {
		t.Skip("requires DB")
	}
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)

	db := &database.DB{Pool: pool}
	store := settings.NewSettingsStore(db, "", "local")

	_, err := pool.Exec(context.Background(),
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		"library.user_can_edit_schemas", "false")
	if err != nil {
		t.Fatalf("upsert setting: %v", err)
	}
	if err := store.Reload(context.Background()); err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), //nolint:errcheck
			`DELETE FROM server_settings WHERE key = 'library.user_can_edit_schemas'`)
	})

	h := &Handler{
		DB:       db,
		Settings: store,
	}

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)
	reqBody := `{"schema":"{year}/{month}/{title}"}`
	req := httptest.NewRequest("PUT", "/api/v1/settings/shadow-fs",
		strings.NewReader(reqBody)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.UpdateShadowFSSettingsCompat(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403 (admin_locked), got %d: %s", w.Code, w.Body.String())
	}
}

// TestUpdateShadowFSSettingsCompatPII verifies that the legacy endpoint
// rejects PII variables in schema templates (regression guard).
func TestUpdateShadowFSSettingsCompatPII(t *testing.T) {
	if testing.Short() {
		t.Skip("requires DB")
	}
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)

	db := &database.DB{Pool: pool}
	store := settings.NewSettingsStore(db, "", "local")

	// Enable schema edits so the lockdown gate passes — PII gate must still fire.
	_, err := pool.Exec(context.Background(),
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		"library.user_can_edit_schemas", "true")
	if err != nil {
		t.Fatalf("upsert setting: %v", err)
	}
	if err := store.Reload(context.Background()); err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), //nolint:errcheck
			`DELETE FROM server_settings WHERE key = 'library.user_can_edit_schemas'`)
	})

	h := &Handler{
		DB:       db,
		Settings: store,
	}

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)
	// "notes" is a PII variable.
	reqBody := `{"schema":"{year}/{notes}"}`
	req := httptest.NewRequest("PUT", "/api/v1/settings/shadow-fs",
		strings.NewReader(reqBody)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.UpdateShadowFSSettingsCompat(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 (PII), got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "notes") {
		t.Errorf("expected 'notes' in error message, got: %s", w.Body.String())
	}
}

func TestUpdateLibraryDirS3Backend403(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)

	db := &database.DB{Pool: pool}
	store := settings.NewSettingsStore(db, "", "s3")

	// Even with user_can_edit_dir_name = "true", s3 backend wins.
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
	body := `{"library_dir_name":"my-library"}`
	req := httptest.NewRequest("PUT", "/api/v1/settings/library-dir",
		strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.UpdateLibraryDir(w, req)

	if w.Code != 403 {
		t.Errorf("expected 403 (s3_backend), got %d: %s", w.Code, w.Body.String())
	}
}
