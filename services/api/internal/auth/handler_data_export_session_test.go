package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestDataExportDownloadSessionRequiresOwnershipAndKnownArtifact(t *testing.T) {
	ctx := context.Background()
	h, ownerID, jobID := newDataExportSessionFixture(t, "export body")

	req := dataExportSessionRequest(t, ownerID, jobID, `{"single":true}`)
	w := httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("owner session status = %d body=%s", w.Code, w.Body.String())
	}

	otherID := uuid.NewString()
	req = dataExportSessionRequest(t, otherID, jobID, `{"single":true}`)
	w = httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code == http.StatusOK {
		t.Fatal("wrong user received a download session")
	}
	_ = ctx
}

func TestDataExportDownloadSessionStoresHashNotRawCookieSecret(t *testing.T) {
	h, ownerID, jobID := newDataExportSessionFixture(t, "export body")

	req := dataExportSessionRequest(t, ownerID, jobID, `{"single":true}`)
	w := httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", w.Code, w.Body.String())
	}

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	rawSecret := cookies[0].Value
	if rawSecret == "" {
		t.Fatal("empty raw session cookie")
	}
	var metadata []byte
	if err := h.DB.Pool.QueryRow(context.Background(), `SELECT metadata FROM background_jobs WHERE id=$1`, jobID).Scan(&metadata); err != nil {
		t.Fatalf("metadata query: %v", err)
	}
	if strings.Contains(string(metadata), rawSecret) {
		t.Fatalf("metadata contains raw cookie secret: %s", metadata)
	}
	if !strings.Contains(string(metadata), "secret_hash") {
		t.Fatalf("metadata missing secret_hash: %s", metadata)
	}
	if strings.Contains(w.Body.String(), rawSecret) {
		t.Fatalf("response body contains raw cookie secret: %s", w.Body.String())
	}
}

func TestDataExportSessionDownloadStreamsOutsideJWTRouteGroup(t *testing.T) {
	h, ownerID, jobID := newDataExportSessionFixture(t, "export body")

	req := dataExportSessionRequest(t, ownerID, jobID, `{"single":true}`)
	w := httptest.NewRecorder()
	h.DataExportDownloadSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if strings.Contains(resp.URL, "secret") || strings.Contains(resp.URL, "token") || strings.Contains(resp.URL, "ticket") {
		t.Fatalf("download URL contains suspicious secret parameter: %s", resp.URL)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, resp.URL, nil)
	for _, cookie := range w.Result().Cookies() {
		downloadReq.AddCookie(cookie)
	}
	downloadReq = withJobAndPart(downloadReq, jobID, "export.zip")
	downloadW := httptest.NewRecorder()
	h.DataExportDownloadPart(downloadW, downloadReq)
	if downloadW.Code != http.StatusOK {
		t.Fatalf("download status = %d body=%s", downloadW.Code, downloadW.Body.String())
	}
	if downloadW.Body.String() != "export body" {
		t.Fatalf("download body = %q", downloadW.Body.String())
	}
	if got := downloadW.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := downloadW.Header().Get("X-Checksum-SHA256"); got == "" {
		t.Fatal("missing checksum header")
	}
}

func newDataExportSessionFixture(t *testing.T, content string) (*Handler, string, string) {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	userID := uuid.NewString()
	jobID := uuid.NewString()
	writer, err := store.CreateWriter(ctx, gdprartifacts.ObjectSpec{
		Kind:        "export",
		UserID:      userID,
		JobID:       jobID,
		Role:        gdprartifacts.RoleSingleZIP,
		Name:        "export.zip",
		ContentType: "application/zip",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	if _, err := writer.Write([]byte(content)); err != nil {
		t.Fatalf("Write artifact: %v", err)
	}
	ref, err := writer.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit artifact: %v", err)
	}
	artifactRaw, err := gdprartifacts.EncodeArtifact(gdprartifacts.Artifact{
		Version:   1,
		Format:    gdprartifacts.ArtifactFormatSingleZIP,
		ExpiresAt: time.Now().Add(time.Hour),
		Objects:   []gdprartifacts.ObjectRef{ref},
	})
	if err != nil {
		t.Fatalf("EncodeArtifact: %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(artifactRaw, &envelope); err != nil {
		t.Fatalf("unmarshal artifact: %v", err)
	}
	artifact := envelope["artifact"]
	if _, err := pool.Exec(ctx,
		`INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at, metadata)
		 VALUES ($1, 'gdpr_export', $2, 'completed', NOW(), NOW(), jsonb_build_object('artifact', $3::jsonb))`,
		jobID, userID, string(artifact)); err != nil {
		t.Fatalf("insert background job: %v", err)
	}
	h := &Handler{DB: &database.DB{Pool: pool}, ArtifactStore: store, shutdownCtx: ctx}
	return h, userID, jobID
}

func dataExportSessionRequest(t *testing.T, userID, jobID, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/data-export/"+jobID+"/download-session", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	return withJob(req, jobID)
}

func withJob(r *http.Request, jobID string) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("jobID", jobID)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
}

func withJobAndPart(r *http.Request, jobID, partName string) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("jobID", jobID)
	routeCtx.URLParams.Add("partName", partName)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
}
