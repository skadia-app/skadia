package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// doAdminGetLibraryDir issues GET /admin/users/{id}/library-dir.
func doAdminGetLibraryDir(t *testing.T, h *Handler, targetID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/"+targetID+"/library-dir", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", targetID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.GetAdminUserLibraryDir(rr, req)
	return rr
}

// doAdminPutLibraryDir issues PUT /admin/users/{id}/library-dir.
func doAdminPutLibraryDir(t *testing.T, h *Handler, targetID string, bodyJSON string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+targetID+"/library-dir",
		bytes.NewBufferString(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", targetID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.UpdateAdminUserLibraryDir(rr, req)
	return rr
}

// TestAdminSetUserLibraryDir: admin sets a label for a user; GET returns it.
func TestAdminSetUserLibraryDir(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)

	userID := insertTestUser(t, pool)

	// Set library_dir_name via admin endpoint.
	rr := doAdminPutLibraryDir(t, h, userID, `{"library_dir_name":"my-admin-label"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var put map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&put); err != nil {
		t.Fatalf("decode PUT response: %v", err)
	}
	if put["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", put["status"])
	}

	// GET should return the set label.
	rr = doAdminGetLibraryDir(t, h, userID)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var get map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&get); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if get["library_dir_name"] != "my-admin-label" {
		t.Fatalf("expected library_dir_name=my-admin-label, got %v", get["library_dir_name"])
	}
	if get["effective_name"] != "my-admin-label" {
		t.Fatalf("expected effective_name=my-admin-label, got %v", get["effective_name"])
	}
}

// TestAdminSetUserLibraryDirForeignUUID: setting a label equal to another user's UUID
// returns 200 + status "library_dir_name_clashes_with_user_uuid".
func TestAdminSetUserLibraryDirForeignUUID(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)

	userA := insertTestUser(t, pool)
	userB := insertTestUser(t, pool)

	// Try to set userB's label to userA's UUID string.
	body, _ := json.Marshal(map[string]string{"library_dir_name": userA})
	rr := doAdminPutLibraryDir(t, h, userB, string(body))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "library_dir_name_clashes_with_user_uuid" {
		t.Fatalf("expected clashes_with_user_uuid, got %q", resp["status"])
	}
}

// TestAdminSetUserLibraryDirDuplicate: setting the same label for two different users
// returns 200 + status "library_dir_name_taken" for the second.
func TestAdminSetUserLibraryDirDuplicate(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)

	userA := insertTestUser(t, pool)
	userB := insertTestUser(t, pool)

	// Set label for userA.
	rr := doAdminPutLibraryDir(t, h, userA, `{"library_dir_name":"shared-label"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("first PUT: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Set same label for userB — should be taken.
	rr = doAdminPutLibraryDir(t, h, userB, `{"library_dir_name":"shared-label"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("second PUT: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "library_dir_name_taken" {
		t.Fatalf("expected library_dir_name_taken, got %q", resp["status"])
	}
}

// TestAdminResetUserLibraryDir: Body {"library_dir_name": null} sets NULL in DB.
func TestAdminResetUserLibraryDir(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)

	userID := insertTestUser(t, pool)

	// Set a label first.
	rr := doAdminPutLibraryDir(t, h, userID, `{"library_dir_name":"to-be-cleared"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("set PUT: expected 200, got %d", rr.Code)
	}

	// Reset to null.
	rr = doAdminPutLibraryDir(t, h, userID, `{"library_dir_name":null}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("reset PUT: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", resp["status"])
	}

	// GET should now return null library_dir_name and effective_name = userID.
	rr = doAdminGetLibraryDir(t, h, userID)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET after reset: expected 200, got %d", rr.Code)
	}
	var get map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&get); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if get["library_dir_name"] != nil {
		t.Fatalf("expected library_dir_name=null, got %v", get["library_dir_name"])
	}
	if get["effective_name"] != userID {
		t.Fatalf("expected effective_name=%s, got %v", userID, get["effective_name"])
	}
}
