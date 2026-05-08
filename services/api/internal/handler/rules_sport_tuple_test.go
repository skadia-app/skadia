package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestRewriteCreateRuleSportTaxonomyAcceptsValidSportTuple(t *testing.T) {
	create := model.AutomationRuleCreate{
		Name: "Tuple",
		ConditionGroups: json.RawMessage(`[
			{"logic":"and","conditions":[
				{"field":"sport_tuple","op":"in","value":[{"sport_type":"cycling","sub_type":"mountain","is_electric":true}]}
			]}
		]`),
		Actions: json.RawMessage(`[]`),
	}
	if err := rewriteCreateRuleSportTaxonomy(&create); err != nil {
		t.Fatalf("rewriteCreateRuleSportTaxonomy: %v", err)
	}
}

func TestRewriteCreateRuleSportTaxonomyRejectsInvalidSportTupleSubtype(t *testing.T) {
	create := model.AutomationRuleCreate{
		Name: "Invalid Tuple",
		Conditions: json.RawMessage(`[
			{"field":"sport_tuple","op":"in","value":[{"sport_type":"running","sub_type":"mountain"}]}
		]`),
		Actions: json.RawMessage(`[]`),
	}
	err := rewriteCreateRuleSportTaxonomy(&create)
	if err == nil || !strings.Contains(err.Error(), "sub_type") {
		t.Fatalf("error = %v, want sub_type rejection", err)
	}
}

func TestRewriteUpdateRuleSportTaxonomyRejectsIncomingMixedTupleAndLegacyList(t *testing.T) {
	conditions := json.RawMessage(`[
		{"field":"sport_type","op":"in","value":["running"]},
		{"field":"sport_tuple","op":"in","value":[{"sport_type":"cycling"}]}
	]`)
	update := model.AutomationRuleUpdate{Conditions: &conditions}
	err := rewriteUpdateRuleSportTaxonomy(&update)
	if err == nil || !strings.Contains(err.Error(), "mixed sport_type in and sport_tuple in") {
		t.Fatalf("error = %v, want mixed-shape rejection", err)
	}
}

func TestRewriteUpdateRuleSportTaxonomyPreservesLegacySportTypeIn(t *testing.T) {
	conditions := json.RawMessage(`[{"field":"sport_type","op":"in","value":["running","swimming"]}]`)
	update := model.AutomationRuleUpdate{Conditions: &conditions}
	if err := rewriteUpdateRuleSportTaxonomy(&update); err != nil {
		t.Fatalf("rewriteUpdateRuleSportTaxonomy: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(*update.Conditions, &got); err != nil {
		t.Fatalf("unmarshal conditions: %v", err)
	}
	if got[0]["field"] != "sport_type" || got[0]["op"] != "in" {
		t.Fatalf("legacy sport_type in was not preserved: %s", *update.Conditions)
	}
}

func TestUpdateRuleReturnsBadRequestForStoredAndPatchMixedShape(t *testing.T) {
	pool := fusionTestPool(t)
	userID := insertTestUser(t, pool)
	h := newTestHandler(t, pool)

	var ruleID string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO automation_rules (user_id, name, conditions, conditions_logic, condition_groups, actions)
		VALUES ($1, 'Tuple', '[]'::jsonb, 'and',
			'[{"logic":"and","conditions":[{"field":"sport_tuple","op":"in","value":[{"sport_type":"cycling"}]}]}]'::jsonb,
			'[]'::jsonb)
		RETURNING id`, userID).Scan(&ruleID); err != nil {
		t.Fatalf("insert rule: %v", err)
	}

	body := bytes.NewBufferString(`{"conditions":[{"field":"sport_type","op":"in","value":["running"]}]}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/automation/rules/"+ruleID, body)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", ruleID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.UpdateRule(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want 400", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "mixed sport_type in and sport_tuple in") {
		t.Fatalf("body = %s, want mixed-shape validation error", rr.Body.String())
	}
}
