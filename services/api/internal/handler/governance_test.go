package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestGovernanceHandlersReturnEffectiveTargets(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	adminID := insertTestUser(t, pool)
	place := createHandlerPlace(t, h, ownerID, "public", nil)
	scope, err := h.DB.CreateGovernanceScope(context.Background(), ownerID, model.GovernanceScopeCreate{
		Kind:       "user_space",
		OwnerScope: "user",
		OriginKey:  "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope: %v", err)
	}
	t.Cleanup(func() {
		_, _ = h.DB.Pool.Exec(context.Background(), `DELETE FROM governance_scopes WHERE id = $1`, scope.ID)
	})
	if _, err := h.DB.AddGovernanceActorRole(context.Background(), ownerID, scope.ID, model.GovernanceActorRoleCreate{
		ActorType:   "user",
		ActorUserID: &adminID,
		Role:        "maintainer",
		Permissions: []string{"edit_metadata"},
	}); err != nil {
		t.Fatalf("AddGovernanceActorRole: %v", err)
	}
	if _, err := h.DB.AddGovernanceScopeTargetBinding(context.Background(), ownerID, scope.ID, model.GovernanceScopeTargetBindingCreate{
		TargetKind:       "place",
		TargetID:         place.ID,
		BindingReason:    "manual_delegation",
		PermissionsScope: "metadata",
		Source:           "owner_assignment",
	}); err != nil {
		t.Fatalf("AddGovernanceScopeTargetBinding: %v", err)
	}

	rr := httptest.NewRecorder()
	h.ListGovernanceEffectiveTargets(rr, targetRequest(http.MethodGet, "/governance/effective-targets", nil, adminID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Targets []model.GovernanceActorTarget `json:"targets"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode targets: %v", err)
	}
	if len(resp.Targets) != 1 || resp.Targets[0].TargetID != place.ID {
		t.Fatalf("targets = %#v, want governed place", resp.Targets)
	}

	filteredRR := httptest.NewRecorder()
	h.ListGovernanceEffectiveTargets(filteredRR, targetRequest(http.MethodGet, "/governance/effective-targets?permission=manage_roles&target_kind=place", nil, adminID))
	if filteredRR.Code != http.StatusOK {
		t.Fatalf("filtered status = %d body=%s", filteredRR.Code, filteredRR.Body.String())
	}
	var filteredResp struct {
		Targets []model.GovernanceActorTarget `json:"targets"`
	}
	if err := json.Unmarshal(filteredRR.Body.Bytes(), &filteredResp); err != nil {
		t.Fatalf("decode filtered targets: %v", err)
	}
	if len(filteredResp.Targets) != 0 {
		t.Fatalf("filtered targets = %#v, want none without manage_roles", filteredResp.Targets)
	}
}

func TestGovernanceHandlersListAuthorizedScopes(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	otherID := insertTestUser(t, pool)
	scope, err := h.DB.CreateGovernanceScope(context.Background(), ownerID, model.GovernanceScopeCreate{
		Kind:       "user_space",
		OwnerScope: "user",
		OriginKey:  "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope owner: %v", err)
	}
	otherScope, err := h.DB.CreateGovernanceScope(context.Background(), otherID, model.GovernanceScopeCreate{
		Kind:       "user_space",
		OwnerScope: "user",
		OriginKey:  "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope other: %v", err)
	}
	t.Cleanup(func() {
		_, _ = h.DB.Pool.Exec(context.Background(), `DELETE FROM governance_scopes WHERE id IN ($1, $2)`, scope.ID, otherScope.ID)
	})

	rr := httptest.NewRecorder()
	h.ListGovernanceScopes(rr, targetRequest(http.MethodGet, "/governance-scopes?kind=user_space&limit=10", nil, ownerID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Scopes []model.GovernanceScope `json:"scopes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode scopes: %v", err)
	}
	if len(resp.Scopes) != 1 || resp.Scopes[0].ID != scope.ID {
		t.Fatalf("scopes = %#v, want only %s", resp.Scopes, scope.ID)
	}
}

func TestGovernanceHandlersGetAuthorizedScope(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	otherID := insertTestUser(t, pool)
	scope, err := h.DB.CreateGovernanceScope(context.Background(), ownerID, model.GovernanceScopeCreate{
		Kind:       "user_space",
		OwnerScope: "user",
		OriginKey:  "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope: %v", err)
	}
	t.Cleanup(func() {
		_, _ = h.DB.Pool.Exec(context.Background(), `DELETE FROM governance_scopes WHERE id = $1`, scope.ID)
	})

	rr := httptest.NewRecorder()
	h.GetGovernanceScope(rr, targetRequestWithParam(http.MethodGet, "/governance-scopes/"+scope.ID, nil, ownerID, "id", scope.ID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Scope model.GovernanceScope `json:"scope"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode scope: %v", err)
	}
	if resp.Scope.ID != scope.ID {
		t.Fatalf("scope id = %s, want %s", resp.Scope.ID, scope.ID)
	}

	forbiddenRR := httptest.NewRecorder()
	h.GetGovernanceScope(forbiddenRR, targetRequestWithParam(http.MethodGet, "/governance-scopes/"+scope.ID, nil, otherID, "id", scope.ID))
	if forbiddenRR.Code != http.StatusNotFound {
		t.Fatalf("unauthorized status = %d body=%s, want 404", forbiddenRR.Code, forbiddenRR.Body.String())
	}
}

func TestGovernanceHandlersRejectExternalCreditPermissions(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	scope, err := h.DB.CreateGovernanceScope(context.Background(), ownerID, model.GovernanceScopeCreate{
		Kind:       "user_space",
		OwnerScope: "user",
		OriginKey:  "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope: %v", err)
	}
	t.Cleanup(func() {
		_, _ = h.DB.Pool.Exec(context.Background(), `DELETE FROM governance_scopes WHERE id = $1`, scope.ID)
	})
	body := []byte(`{"actor_type":"external_party_credit","actor_ref":"credit:test","role":"sponsor","permissions":["edit_metadata"]}`)
	rr := httptest.NewRecorder()
	h.AddGovernanceRole(rr, targetRequestWithParam(http.MethodPost, "/governance-scopes/"+scope.ID+"/roles", body, ownerID, "id", scope.ID))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rr.Code, rr.Body.String())
	}
}
