package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/google/uuid"
)

func komootImportTestHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	pool := testdb.Open(t)
	userID := uuid.NewString()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "komoot-"+userID[:8], "komoot-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &Handler{DB: &database.DB{Pool: pool}}, userID
}

func getKomootCredentialStatus(t *testing.T, h *Handler, userID string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/import/komoot/credentials", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rr := httptest.NewRecorder()
	h.KomootCredentialsStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func TestKomootCredentialsStatusHidesTemporaryCredentials(t *testing.T) {
	h, userID := komootImportTestHandler(t)
	if err := h.DB.SaveKomootCredentials(context.Background(), userID, database.KomootCredentials{
		Email:             "temporary@example.com",
		EncryptedPassword: "encrypted",
		KomootUserID:      "komoot-user",
		SyncEnabled:       false,
		ImportPlanned:     true,
		ImportRecorded:    true,
		ImportPhotos:      true,
	}); err != nil {
		t.Fatalf("SaveKomootCredentials: %v", err)
	}

	body := getKomootCredentialStatus(t, h, userID)
	if body["has_credentials"] != false {
		t.Fatalf("has_credentials = %v, want false for temporary credentials", body["has_credentials"])
	}
	if body["email"] != "" {
		t.Fatalf("email = %q, want empty for temporary credentials", body["email"])
	}
}

func TestKomootCredentialsStatusShowsPersistentCredentials(t *testing.T) {
	h, userID := komootImportTestHandler(t)
	if err := h.DB.SaveKomootCredentials(context.Background(), userID, database.KomootCredentials{
		Email:             "persistent@example.com",
		EncryptedPassword: "encrypted",
		KomootUserID:      "komoot-user",
		SyncEnabled:       true,
		ImportPlanned:     true,
		ImportRecorded:    true,
		ImportPhotos:      true,
	}); err != nil {
		t.Fatalf("SaveKomootCredentials: %v", err)
	}

	body := getKomootCredentialStatus(t, h, userID)
	if body["has_credentials"] != true {
		t.Fatalf("has_credentials = %v, want true for persistent credentials", body["has_credentials"])
	}
	if body["email"] != "persistent@example.com" {
		t.Fatalf("email = %q, want persistent@example.com", body["email"])
	}
}

func TestShouldDeleteKomootCredentialsAfterImport(t *testing.T) {
	tests := []struct {
		name  string
		req   KomootStartRequest
		creds *database.KomootCredentials
		want  bool
	}{
		{
			name: "explicit delete requested",
			req:  KomootStartRequest{DeleteCredentialsAfter: true},
			creds: &database.KomootCredentials{
				SyncEnabled: true,
			},
			want: true,
		},
		{
			name: "temporary credentials are deleted even if request says keep",
			req:  KomootStartRequest{DeleteCredentialsAfter: false},
			creds: &database.KomootCredentials{
				SyncEnabled: false,
			},
			want: true,
		},
		{
			name: "persistent credentials are kept",
			req:  KomootStartRequest{DeleteCredentialsAfter: false},
			creds: &database.KomootCredentials{
				SyncEnabled: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldDeleteKomootCredentialsAfterImport(tt.req, tt.creds); got != tt.want {
				t.Fatalf("shouldDeleteKomootCredentialsAfterImport() = %v, want %v", got, tt.want)
			}
		})
	}
}

type fakeKomootProgressUpdater struct {
	current int64
	total   int64
	calls   []string
}

func (f *fakeKomootProgressUpdater) UpdateBackgroundJobProgress(_ context.Context, _ string, current int64) error {
	f.current = current
	f.calls = append(f.calls, fmt.Sprintf("current:%d", current))
	return nil
}

func (f *fakeKomootProgressUpdater) UpdateBackgroundJobTotal(_ context.Context, _ string, total int64) error {
	f.total = total
	f.calls = append(f.calls, fmt.Sprintf("total:%d", total))
	return nil
}

func TestUpdateKomootImportProgressUsesImporterTotal(t *testing.T) {
	progress := &fakeKomootProgressUpdater{total: 946}

	updateKomootImportProgress(context.Background(), progress, "job-id", 33, 129)

	if progress.current != 33 {
		t.Fatalf("current = %d, want 33", progress.current)
	}
	if progress.total != 129 {
		t.Fatalf("total = %d, want importer total 129", progress.total)
	}
	wantCalls := []string{"total:129", "current:33"}
	if fmt.Sprint(progress.calls) != fmt.Sprint(wantCalls) {
		t.Fatalf("calls = %v, want %v", progress.calls, wantCalls)
	}
}
