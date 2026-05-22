package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestObjectAccessScopeHandlerCreatesListsAndRevokes(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/access-scopes", h.CreateObjectAccessScope)
	r.Get("/api/v1/access-scopes", h.ListObjectAccessScopes)
	r.Patch("/api/v1/access-scopes/{id}", h.UpdateObjectAccessScope)

	ownerID := insertTestUser(t, pool)
	viewerID := insertTestUser(t, pool)
	body := `{
		"scope_kind":"mixed",
		"label":"selected viewers",
		"grants":[
			{"principal_kind":"user","principal_user_id":"` + viewerID + `","permission":"view"},
			{"principal_kind":"link_token","permission":"view"}
		]
	}`
	rec := performObjectAccessRequest(t, r, http.MethodPost, "/api/v1/access-scopes", body, ownerID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "link_token_hash") || strings.Contains(rec.Body.String(), "sha256:") {
		t.Fatalf("create response exposed token hash: %s", rec.Body.String())
	}
	var created ObjectAccessScopeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if created.Scope.ID == "" {
		t.Fatal("scope id is empty")
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(t.Context(), `DELETE FROM object_access_scopes WHERE id = $1`, created.Scope.ID)
	})
	if len(created.Scope.Grants) != 2 {
		t.Fatalf("grant count = %d, want 2", len(created.Scope.Grants))
	}
	if len(created.LinkTokens) != 1 || created.LinkTokens[0].Token == "" {
		t.Fatalf("link token response = %#v, want one raw token", created.LinkTokens)
	}
	linkToken := created.LinkTokens[0].Token

	rec = performObjectAccessRequest(t, r, http.MethodGet, "/api/v1/access-scopes", "", ownerID)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), linkToken) || strings.Contains(rec.Body.String(), "link_token_hash") {
		t.Fatalf("list response exposed link token material: %s", rec.Body.String())
	}
	var listed struct {
		Scopes []model.ObjectAccessScope `json:"scopes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listed.Scopes) == 0 {
		t.Fatal("list returned no scopes")
	}

	var grantID string
	for _, grant := range created.Scope.Grants {
		if grant.PrincipalKind == "user" {
			grantID = grant.ID
			break
		}
	}
	if grantID == "" {
		t.Fatal("user grant id is empty")
	}
	patch := `{"revoke_grant_ids":["` + grantID + `"],"revoke_scope":true}`
	rec = performObjectAccessRequest(t, r, http.MethodPatch, "/api/v1/access-scopes/"+created.Scope.ID, patch, ownerID)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestObjectAccessScopeHandlerRejectsNonViewPermission(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/access-scopes", h.CreateObjectAccessScope)

	ownerID := insertTestUser(t, pool)
	body := `{"grants":[{"principal_kind":"user","principal_user_id":"` + ownerID + `","permission":"edit"}]}`
	rec := performObjectAccessRequest(t, r, http.MethodPost, "/api/v1/access-scopes", body, ownerID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestObjectAccessScopeHandlerRejectsUnsupportedDynamicPrincipals(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/access-scopes", h.CreateObjectAccessScope)

	ownerID := insertTestUser(t, pool)
	for _, kind := range []string{"group", "team", "organization", "role", "server", "delegated_audience", "relationship_rule"} {
		body := `{"grants":[{"principal_kind":"` + kind + `","principal_ref":"example","permission":"view"}]}`
		rec := performObjectAccessRequest(t, r, http.MethodPost, "/api/v1/access-scopes", body, ownerID)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400; body: %s", kind, rec.Code, rec.Body.String())
		}
	}
}

func performObjectAccessRequest(t *testing.T, r http.Handler, method, target, body, userID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}
