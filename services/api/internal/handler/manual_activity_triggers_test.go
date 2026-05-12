//go:build integration

package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// handlerTestShadowFS is a minimal ShadowFS implementation that also writes a
// row into user_files so that the activities.file_id FK constraint is satisfied.
// It is used only in handler integration tests.
type handlerTestShadowFS struct {
	mu       sync.Mutex
	blobs    map[string][]byte // key -> content (file ID or arbitrary path)
	pool     *pgxpool.Pool
	basePath string
}

func newHandlerTestShadowFS(pool *pgxpool.Pool) *handlerTestShadowFS {
	return &handlerTestShadowFS{blobs: make(map[string][]byte), pool: pool}
}

func newHandlerTestShadowFSWithBasePath(pool *pgxpool.Pool, basePath string) *handlerTestShadowFS {
	return &handlerTestShadowFS{blobs: make(map[string][]byte), pool: pool, basePath: basePath}
}

func (m *handlerTestShadowFS) Put(ctx context.Context, entry shadowfs.FileEntry, data io.Reader) (shadowfs.FileRecord, error) {
	content, err := io.ReadAll(data)
	if err != nil {
		return shadowfs.FileRecord{}, err
	}
	id := uuid.New().String()
	m.mu.Lock()
	m.blobs[id] = content
	m.mu.Unlock()
	rec := shadowfs.FileRecord{
		ID:        id,
		UserID:    entry.UserID,
		FileType:  entry.FileType,
		SizeBytes: int64(len(content)),
		Status:    "active",
	}
	storagePath := "/mock/" + rec.ID
	if m.basePath != "" {
		storagePath = filepath.Join(rec.UserID, shadowfs.DirForType(entry.FileType), entry.FileName)
		fullPath := filepath.Join(m.basePath, storagePath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return shadowfs.FileRecord{}, err
		}
		if err := os.WriteFile(fullPath, content, 0o644); err != nil {
			return shadowfs.FileRecord{}, err
		}
	}
	// Insert a minimal user_files row so FK constraints are satisfied.
	_, dbErr := m.pool.Exec(ctx, `
		INSERT INTO user_files
			(id, user_id, file_type, storage_backend, storage_path,
			 content_type, file_name, size_bytes, has_sidecar, is_renaming, status)
		VALUES ($1, $2, $3, 'local', $4, $5, $6, $7, false, false, 'active')
		ON CONFLICT (id) DO NOTHING
	`, rec.ID, rec.UserID, entry.FileType,
		storagePath,
		entry.ContentType, entry.FileName, rec.SizeBytes)
	if dbErr != nil {
		return shadowfs.FileRecord{}, dbErr
	}
	return rec, nil
}

func (m *handlerTestShadowFS) Get(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (m *handlerTestShadowFS) Delete(_ context.Context, _, _ string) error           { return nil }
func (m *handlerTestShadowFS) DeleteAllForUser(_ context.Context, _ string) error    { return nil }
func (m *handlerTestShadowFS) DeleteCompanions(_ context.Context, _, _ string) error { return nil }
func (m *handlerTestShadowFS) List(_ context.Context, _ string, _ *string) ([]shadowfs.FileRecord, error) {
	return nil, nil
}
func (m *handlerTestShadowFS) StreamToZip(_ context.Context, _ string, _ *zip.Writer) ([]string, error) {
	return nil, nil
}
func (m *handlerTestShadowFS) StreamExportFiles(_ context.Context, _ string, _ shadowfs.ExportFileSink) ([]string, error) {
	return nil, nil
}
func (m *handlerTestShadowFS) Resolve(_ context.Context, _, _ string) (shadowfs.ResolvedFile, error) {
	return shadowfs.ResolvedFile{}, nil
}
func (m *handlerTestShadowFS) DiskUsage(_ context.Context, _ string) (int64, error) { return 0, nil }
func (m *handlerTestShadowFS) Archive(_ context.Context, _, _, _ string) error      { return nil }
func (m *handlerTestShadowFS) PutCompanion(ctx context.Context, userID, fileID, suffix string, data []byte) error {
	if m.basePath != "" && fileID != "" {
		var storagePath string
		if err := m.pool.QueryRow(ctx,
			`SELECT storage_path FROM user_files WHERE id = $1 AND user_id = $2`,
			fileID, userID,
		).Scan(&storagePath); err != nil {
			return err
		}
		companionPath := filepath.Join(m.basePath, storagePath+suffix)
		if err := os.MkdirAll(filepath.Dir(companionPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(companionPath, data, 0o644); err != nil {
			return err
		}
		if _, err := m.pool.Exec(ctx, `UPDATE user_files SET has_sidecar = true WHERE id = $1 AND user_id = $2`, fileID, userID); err != nil {
			return err
		}
	}
	return nil
}
func (m *handlerTestShadowFS) GetCompanion(_ context.Context, _, _, _ string) ([]byte, error) {
	return nil, nil
}
func (m *handlerTestShadowFS) PutCompanionAt(_ context.Context, path string, data []byte) error {
	m.mu.Lock()
	m.blobs[path] = data
	m.mu.Unlock()
	return nil
}
func (m *handlerTestShadowFS) GetCompanionAt(_ context.Context, path string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d, ok := m.blobs[path]; ok {
		return d, nil
	}
	return nil, shadowfs.ErrNotFound
}
func (m *handlerTestShadowFS) DeleteCompanionAt(_ context.Context, _ string) error { return nil }

// noopWeatherFetcher satisfies enrichment.WeatherFetcher without network calls.
type noopWeatherFetcher struct{}

func (noopWeatherFetcher) FetchReadings(_ context.Context, _ []enrichment.WeatherRequest) ([]model.WeatherReading, error) {
	return nil, nil
}

// noopReverseGeocoder satisfies enrichment.ReverseGeocoder without DB/network calls.
type noopReverseGeocoder struct{}

func (noopReverseGeocoder) Reverse(_ context.Context, _ *database.DB, _ string, _, _ float64) (*enrichment.GeoResult, error) {
	return nil, nil
}

// fakeStage1Deps returns a Stage1Deps wired with no-op fakes.
func fakeStage1Deps() *enrichment.Stage1Deps {
	return &enrichment.Stage1Deps{
		Weather:  noopWeatherFetcher{},
		Geocoder: noopReverseGeocoder{},
	}
}

// manualActivityBody builds a minimal valid JSON body for POST /api/v1/activities/manual.
func manualActivityBody(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"sport_type":     "cycling",
		"started_at":     "2026-05-01T10:00:00Z",
		"elapsed_time_s": 3600.0,
	})
	if err != nil {
		t.Fatalf("manualActivityBody: marshal: %v", err)
	}
	return b
}

// TestCreateManualActivity_TriggersAnalyticsInvalidate verifies that a
// successful POST /api/v1/activities/manual fires
// AnalyticsInvalidator.PublishAnalyticsInvalidate with the authenticated
// user's ID (manual_activity.go trigger site).
func TestCreateManualActivity_TriggersAnalyticsInvalidate(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)

	fake := &event.FakeInvalidator{}
	h := &Handler{
		DB:                   &database.DB{Pool: pool},
		ShadowFS:             newHandlerTestShadowFS(pool),
		AnalyticsInvalidator: fake,
		Stage1Deps:           fakeStage1Deps(),
		// Stage2ReadyPublisher left nil: manual activities skip GPS so Stage2 is not fired.
	}

	body := manualActivityBody(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/manual", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), uid, false))
	rec := httptest.NewRecorder()
	h.CreateManualActivity(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateManualActivity: code = %d body = %s, want 201", rec.Code, rec.Body.String())
	}

	calls := fake.CallsCopy()
	if len(calls) != 1 {
		t.Fatalf("FakeInvalidator.Calls = %d, want 1", len(calls))
	}
	if calls[0] != uid {
		t.Errorf("FakeInvalidator.Calls[0] = %q, want %q", calls[0], uid)
	}
}

// TestCreateManualActivity_InvalidBody_NoInvalidate verifies that a malformed
// request (missing sport_type) returns 400 before reaching the Invalidate call.
func TestCreateManualActivity_InvalidBody_NoInvalidate(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)

	fake := &event.FakeInvalidator{}
	h := &Handler{
		DB:                   &database.DB{Pool: pool},
		ShadowFS:             newHandlerTestShadowFS(pool),
		AnalyticsInvalidator: fake,
		Stage1Deps:           fakeStage1Deps(),
	}

	// sport_type is required; omitting it triggers a 400 before any DB write.
	b, _ := json.Marshal(map[string]any{
		"started_at":     "2026-05-01T10:00:00Z",
		"elapsed_time_s": 3600.0,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/manual", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), uid, false))
	rec := httptest.NewRecorder()
	h.CreateManualActivity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid body: code = %d body = %s, want 400", rec.Code, rec.Body.String())
	}
	if got := fake.CallsCopy(); len(got) != 0 {
		t.Errorf("FakeInvalidator.Calls = %v, want [] (no write should have occurred)", got)
	}
}

func TestCreateManualActivityRejectsNonSelectableSportTuple(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)

	h := &Handler{
		DB:         &database.DB{Pool: pool},
		ShadowFS:   newHandlerTestShadowFS(pool),
		Stage1Deps: fakeStage1Deps(),
	}

	body, err := json.Marshal(map[string]any{
		"sport_type":       "unknown",
		"started_at":       "2026-05-01T10:00:00Z",
		"elapsed_time_s":   3600.0,
		"distance_m":       1000.0,
		"elevation_gain_m": 10.0,
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/manual", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), uid, false))
	rec := httptest.NewRecorder()

	h.CreateManualActivity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("CreateManualActivity: code = %d body = %s, want 400", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not selectable") {
		t.Fatalf("CreateManualActivity body = %s, want not selectable error", rec.Body.String())
	}
}

func TestCreateManualActivity_RefreshesLibraryPathAfterFinalFields(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	basePath := t.TempDir()
	store := settings.NewStoreForTest(map[string]string{
		"library.default_schema.activity": "{year}/{title}_{sport}_{modifiers}",
	})

	h := &Handler{
		DB:         &database.DB{Pool: pool},
		ShadowFS:   newHandlerTestShadowFSWithBasePath(pool, basePath),
		Settings:   store,
		Config:     &HandlerConfig{LibraryBasePath: basePath},
		Stage1Deps: fakeStage1Deps(),
	}

	body, err := json.Marshal(map[string]any{
		"sport_type":     "running",
		"title":          "MorningRun",
		"started_at":     "2026-05-01T10:00:00Z",
		"elapsed_time_s": 3600.0,
		"is_indoor":      true,
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/manual", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), uid, false))
	rec := httptest.NewRecorder()

	h.CreateManualActivity(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateManualActivity: code = %d body = %s, want 201", rec.Code, rec.Body.String())
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ID == "" {
		t.Fatal("response id is empty")
	}
	var storagePath string
	if err := pool.QueryRow(context.Background(), `
		SELECT uf.storage_path
		FROM user_files uf
		JOIN activities a ON a.file_id = uf.id
		WHERE a.id = $1 AND a.user_id = $2`,
		response.ID, uid).Scan(&storagePath); err != nil {
		t.Fatalf("query refreshed storage_path: %v", err)
	}

	if !strings.HasPrefix(storagePath, filepath.Join(uid, shadowfs.DirForType("activity"), "2026")+string(os.PathSeparator)) {
		t.Fatalf("storage_path = %q, want user activity path under 2026", storagePath)
	}
	if !strings.Contains(storagePath, "MorningRun_running_indoor") {
		t.Fatalf("storage_path = %q, want final title, sport, and modifier", storagePath)
	}
	if _, err := os.Stat(filepath.Join(basePath, storagePath)); err != nil {
		t.Fatalf("refreshed file missing on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(basePath, storagePath+".meta")); err != nil {
		t.Fatalf("refreshed sidecar missing on disk: %v", err)
	}
}
