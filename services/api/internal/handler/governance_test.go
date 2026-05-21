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
		Kind:             "region_scope",
		AnchorTargetKind: targetStringPtr("place"),
		AnchorTargetID:   &place.ID,
		OwnerScope:       "user",
		OriginKey:        "manual",
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
}

func TestGovernanceHandlersRejectExternalCreditPermissions(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	place := createHandlerPlace(t, h, ownerID, "public", nil)
	scope, err := h.DB.CreateGovernanceScope(context.Background(), ownerID, model.GovernanceScopeCreate{
		Kind:             "region_scope",
		AnchorTargetKind: targetStringPtr("place"),
		AnchorTargetID:   &place.ID,
		OwnerScope:       "user",
		OriginKey:        "manual",
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

func targetStringPtr(value string) *string {
	return &value
}
