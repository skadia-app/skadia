package handler

import (
	"encoding/json"
	"strings"
	"testing"

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
