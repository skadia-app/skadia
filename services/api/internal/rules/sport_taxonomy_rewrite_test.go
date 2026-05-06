package rules

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestSportTaxonomyRewriteActions(t *testing.T) {
	input := SportTaxonomyRuleJSON{
		Actions: json.RawMessage(`[
			{"action":"set_sport_type","params":{"sport_type":"e_mountain_biking"}},
			{"action":"add_tag","params":{"tag_id":"11111111-1111-1111-1111-111111111111"}}
		]`),
		Branches: json.RawMessage(`[
			{"type":"if","condition_groups":[],"actions":[{"action":"set_sport_type","params":{"sport_type":"treadmill_running"}}]}
		]`),
	}

	got, warnings, err := RewriteSportTaxonomyRuleJSON(input)
	if err != nil {
		t.Fatalf("RewriteSportTaxonomyRuleJSON: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings: got %#v, want none", warnings)
	}

	var actions []map[string]any
	if err := json.Unmarshal(got.Actions, &actions); err != nil {
		t.Fatalf("unmarshal actions: %v", err)
	}
	if len(actions) != 3 {
		t.Fatalf("actions len: got %d, want 3", len(actions))
	}
	params, _ := actions[0]["params"].(map[string]any)
	if actions[0]["action"] != "set_sport_type" || params["sport_type"] != "cycling" || params["sub_type"] != "mountain" {
		t.Fatalf("first action not canonical tuple: %#v", actions[0])
	}
	if actions[1]["action"] != "set_electric" {
		t.Fatalf("second action: got %#v, want set_electric modifier action", actions[1])
	}
	electricParams, _ := actions[1]["params"].(map[string]any)
	if electricParams["electric"] != true {
		t.Fatalf("set_electric params: got %#v, want electric=true", electricParams)
	}
	if actions[2]["action"] != "add_tag" {
		t.Fatalf("non-sport action was not preserved: %#v", actions[2])
	}

	var branches []map[string]any
	if err := json.Unmarshal(got.Branches, &branches); err != nil {
		t.Fatalf("unmarshal branches: %v", err)
	}
	branchActions, _ := branches[0]["actions"].([]any)
	if len(branchActions) != 2 {
		t.Fatalf("branch actions len: got %d, want 2", len(branchActions))
	}
}

func TestSportTaxonomyRewriteActionsAllowsIdempotentModifierWrite(t *testing.T) {
	input := SportTaxonomyRuleJSON{
		Actions: json.RawMessage(`[
			{"action":"set_electric","params":{"electric":true}},
			{"action":"set_sport_type","params":{"sport_type":"e_mountain_biking"}}
		]`),
	}

	got, _, err := RewriteSportTaxonomyRuleJSON(input)
	if err != nil {
		t.Fatalf("RewriteSportTaxonomyRuleJSON: %v", err)
	}
	var actions []map[string]any
	if err := json.Unmarshal(got.Actions, &actions); err != nil {
		t.Fatalf("unmarshal actions: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("actions len = %d, want original modifier plus sport tuple only: %#v", len(actions), actions)
	}
}

func TestSportTaxonomyRewriteActionsRejectsModifierWriteConflict(t *testing.T) {
	input := SportTaxonomyRuleJSON{
		Actions: json.RawMessage(`[
			{"action":"set_electric","params":{"electric":false}},
			{"action":"set_sport_type","params":{"sport_type":"e_mountain_biking"}}
		]`),
	}

	_, _, err := RewriteSportTaxonomyRuleJSON(input)
	if err == nil || !strings.Contains(err.Error(), "conflicting writes to is_electric") {
		t.Fatalf("error = %v, want conflicting writes to is_electric", err)
	}
}

func TestSportTaxonomyRewriteActionsRejectsNonSelectableSetSportType(t *testing.T) {
	input := SportTaxonomyRuleJSON{
		Actions: json.RawMessage(`[
			{"action":"set_sport_type","params":{"sport_type":"unknown"}}
		]`),
	}

	_, _, err := NormalizeAndValidateSportTaxonomyRuleJSON(input)
	if err == nil || !strings.Contains(err.Error(), "not selectable") {
		t.Fatalf("error = %v, want not selectable rejection", err)
	}
}

func TestSportTaxonomyRewriteSetFieldSportType(t *testing.T) {
	input := SportTaxonomyRuleJSON{
		Actions: json.RawMessage(`[
			{"action":"set_field","params":{"field":"sport_type","value":"mountain_biking"}}
		]`),
	}

	got, _, err := RewriteSportTaxonomyRuleJSON(input)
	if err != nil {
		t.Fatalf("RewriteSportTaxonomyRuleJSON: %v", err)
	}
	var actions []map[string]any
	if err := json.Unmarshal(got.Actions, &actions); err != nil {
		t.Fatalf("unmarshal actions: %v", err)
	}
	params, _ := actions[0]["params"].(map[string]any)
	if actions[0]["action"] != "set_sport_type" || params["sport_type"] != "cycling" || params["sub_type"] != "mountain" {
		t.Fatalf("set_field sport_type was not canonicalized: %#v", actions)
	}
}

func TestSportTaxonomyRewriteConditions(t *testing.T) {
	input := SportTaxonomyRuleJSON{
		ConditionGroups: json.RawMessage(`[
			{"logic":"and","conditions":[{"field":"sport_type","op":"eq","value":"mountain_biking"}]}
		]`),
	}

	got, _, err := RewriteSportTaxonomyRuleJSON(input)
	if err != nil {
		t.Fatalf("RewriteSportTaxonomyRuleJSON: %v", err)
	}

	var groups []ConditionGroup
	if err := json.Unmarshal(got.ConditionGroups, &groups); err != nil {
		t.Fatalf("unmarshal condition groups: %v", err)
	}
	if len(groups) != 1 || len(groups[0].Conditions) != 2 {
		t.Fatalf("conditions: got %#v, want sport_type and sub_type tuple", groups)
	}
	if groups[0].Conditions[0].Field != "sport_type" || groups[0].Conditions[0].Value != "cycling" {
		t.Fatalf("sport condition not canonical: %#v", groups[0].Conditions[0])
	}
	if groups[0].Conditions[1].Field != "sub_type" || groups[0].Conditions[1].Value != "mountain" {
		t.Fatalf("sub_type condition not canonical: %#v", groups[0].Conditions[1])
	}
}

func TestSportTaxonomyRewritePreservesCanonicalNonEqAndInConditions(t *testing.T) {
	input := SportTaxonomyRuleJSON{
		Conditions: json.RawMessage(`[
			{"field":"sport_type","op":"neq","value":"cycling"},
			{"field":"sport_type","op":"in","value":["running","swimming"]}
		]`),
		ConditionGroups: json.RawMessage(`[
			{"logic":"and","conditions":[
				{"field":"sport_type","op":"not_in","value":["hiking","walking"]}
			]}
		]`),
	}

	got, _, err := RewriteSportTaxonomyRuleJSON(input)
	if err != nil {
		t.Fatalf("RewriteSportTaxonomyRuleJSON: %v", err)
	}
	assertRawJSONEqual(t, got.Conditions, input.Conditions, "conditions")
	assertRawJSONEqual(t, got.ConditionGroups, input.ConditionGroups, "condition_groups")
}

func TestValidateSportTaxonomyRuleJSONAcceptsSportTupleInBranches(t *testing.T) {
	input := SportTaxonomyRuleJSON{Branches: json.RawMessage(`[
		{"type":"if","condition_groups":[{"logic":"and","conditions":[
			{"field":"sport_tuple","op":"in","value":[{"sport_type":"cycling","sub_type":"mountain","is_electric":true}]}
		]}],"actions":[]}
	]`)}
	if _, _, err := NormalizeAndValidateSportTaxonomyRuleJSON(input); err != nil {
		t.Fatalf("NormalizeAndValidateSportTaxonomyRuleJSON: %v", err)
	}
}

func TestValidateSportTaxonomyRuleJSONRejectsMixedLegacyAndTupleIn(t *testing.T) {
	input := SportTaxonomyRuleJSON{ConditionGroups: json.RawMessage(`[
		{"logic":"and","conditions":[
			{"field":"sport_type","op":"in","value":["running"]},
			{"field":"sport_tuple","op":"in","value":[{"sport_type":"cycling"}]}
		]}
	]`)}
	_, _, err := NormalizeAndValidateSportTaxonomyRuleJSON(input)
	if err == nil || !strings.Contains(err.Error(), "mixed sport_type in and sport_tuple in") {
		t.Fatalf("error = %v, want mixed-shape rejection", err)
	}
}

func TestValidateSportTaxonomyRuleJSONRejectsMixedShapeAcrossBranchesAndFlatConditions(t *testing.T) {
	input := SportTaxonomyRuleJSON{
		Conditions: json.RawMessage(`[{"field":"sport_type","op":"not_in","value":["running"]}]`),
		Branches: json.RawMessage(`[
			{"type":"if","condition_groups":[{"logic":"and","conditions":[
				{"field":"sport_tuple","op":"in","value":[{"sport_type":"cycling"}]}
			]}],"actions":[]}
		]`),
	}
	_, _, err := NormalizeAndValidateSportTaxonomyRuleJSON(input)
	if err == nil || !strings.Contains(err.Error(), "mixed sport_type in and sport_tuple in") {
		t.Fatalf("error = %v, want mixed-shape rejection", err)
	}
}

func TestValidateSportTaxonomyRuleJSONPreservesLegacySportTypeIn(t *testing.T) {
	input := SportTaxonomyRuleJSON{Conditions: json.RawMessage(`[
		{"field":"sport_type","op":"in","value":["running","swimming"]}
	]`)}
	got, _, err := NormalizeAndValidateSportTaxonomyRuleJSON(input)
	if err != nil {
		t.Fatalf("NormalizeAndValidateSportTaxonomyRuleJSON: %v", err)
	}
	assertRawJSONEqual(t, got.Conditions, input.Conditions, "conditions")
}

func TestValidateSportTaxonomyRuleJSONRejectsInvalidSportTupleSubtype(t *testing.T) {
	input := SportTaxonomyRuleJSON{ConditionGroups: json.RawMessage(`[
		{"logic":"and","conditions":[
			{"field":"sport_tuple","op":"in","value":[{"sport_type":"running","sub_type":"mountain"}]}
		]}
	]`)}
	_, _, err := NormalizeAndValidateSportTaxonomyRuleJSON(input)
	if err == nil || !strings.Contains(err.Error(), "condition_groups[0].conditions[0]") || !strings.Contains(err.Error(), "sub_type") {
		t.Fatalf("error = %v, want surface-specific sub_type rejection", err)
	}
}

func TestValidateSportTaxonomyRuleJSONRejectsMalformedTopLevelJSONShapes(t *testing.T) {
	tests := []struct {
		name  string
		input SportTaxonomyRuleJSON
		want  string
	}{
		{
			name:  "conditions object",
			input: SportTaxonomyRuleJSON{Conditions: json.RawMessage(`{"field":"sport_tuple","op":"in","value":[{"sport_type":"cycling"}]}`)},
			want:  "conditions must be a JSON array",
		},
		{
			name:  "condition groups object",
			input: SportTaxonomyRuleJSON{ConditionGroups: json.RawMessage(`{"logic":"and","conditions":[{"field":"sport_tuple","op":"in","value":[{"sport_type":"cycling"}]}]}`)},
			want:  "condition_groups must be a JSON array",
		},
		{
			name:  "branches object",
			input: SportTaxonomyRuleJSON{Branches: json.RawMessage(`{"type":"if","condition_groups":[],"actions":[]}`)},
			want:  "branches must be a JSON array",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := NormalizeAndValidateSportTaxonomyRuleJSON(tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestSportTaxonomyRewriteRejectsUnsupportedSemantics(t *testing.T) {
	tests := []struct {
		name  string
		input SportTaxonomyRuleJSON
		want  string
	}{
		{
			name:  "neq condition",
			input: SportTaxonomyRuleJSON{Conditions: json.RawMessage(`[{"field":"sport_type","op":"neq","value":"mountain_biking"}]`)},
			want:  "unsupported sport_type operator",
		},
		{
			name: "or group",
			input: SportTaxonomyRuleJSON{ConditionGroups: json.RawMessage(`[
				{"logic":"or","conditions":[{"field":"sport_type","op":"eq","value":"mountain_biking"}]}
			]`)},
			want: "or group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := RewriteSportTaxonomyRuleJSON(tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error: got %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestSportConditionNeedsTupleRewriteIncludesVirtualPatch(t *testing.T) {
	virtual := true
	if !sportConditionNeedsTupleRewrite(
		"cycling",
		sport.Tuple{SportType: "cycling"},
		sport.ModifierPatch{IsVirtual: &virtual},
	) {
		t.Fatal("virtual modifier patch should require tuple rewrite handling")
	}
}

func assertRawJSONEqual(t *testing.T, got, want json.RawMessage, label string) {
	t.Helper()
	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("unmarshal got %s: %v", label, err)
	}
	var wantValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("unmarshal want %s: %v", label, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("%s changed:\n got  %s\n want %s", label, got, want)
	}
}
