package rules

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeRuleActionsDedupesSystemTagsWhenModifierContextMatches(t *testing.T) {
	tagNames := map[string]string{
		"tag-ebike":  "E-Bike",
		"tag-indoor": "Indoor",
		"tag-other":  "Urlaub",
	}
	raw := json.RawMessage(`[
		{"action":"set_sport_type","params":{"sport_type":"cycling"}},
		{"action":"add_tag","params":{"tag_id":"tag-ebike"}},
		{"action":"set_electric","params":{"electric":true}},
		{"action":"set_indoor","params":{"indoor":true}},
		{"action":"add_tag","params":{"tag_id":"tag-indoor"}},
		{"action":"add_tag","params":{"tag_id":"tag-other"}}
	]`)

	got, err := NormalizeRuleActions(raw, tagNames)
	if err != nil {
		t.Fatalf("NormalizeRuleActions: %v", err)
	}

	var actions []Action
	if err := json.Unmarshal(got, &actions); err != nil {
		t.Fatalf("unmarshal actions: %v", err)
	}
	if len(actions) != 4 {
		t.Fatalf("actions len = %d, want 4: %s", len(actions), got)
	}
	for _, action := range actions {
		if action.Action == "add_tag" && strings.Contains(string(action.Params), "tag-ebike") {
			t.Fatalf("E-Bike add_tag was not deduped: %s", got)
		}
		if action.Action == "add_tag" && strings.Contains(string(action.Params), "tag-indoor") {
			t.Fatalf("Indoor add_tag was not deduped: %s", got)
		}
	}
}

func TestNormalizeRuleActionsOnlyDedupesEBikeTagWithCyclingContext(t *testing.T) {
	tagNames := map[string]string{"tag-ebike": "E-Bike"}

	generic, err := NormalizeRuleActions(json.RawMessage(`[
		{"action":"set_electric","params":{"electric":true}},
		{"action":"add_tag","params":{"tag_id":"tag-ebike"}}
	]`), tagNames)
	if err != nil {
		t.Fatalf("NormalizeRuleActions generic electric: %v", err)
	}
	if !strings.Contains(string(generic), "tag-ebike") {
		t.Fatalf("generic electric action must not dedupe cycling-specific E-Bike tag: %s", generic)
	}

	cycling, err := NormalizeRuleActions(json.RawMessage(`[
		{"action":"set_sport_type","params":{"sport_type":"cycling"}},
		{"action":"set_electric","params":{"electric":true}},
		{"action":"add_tag","params":{"tag_id":"tag-ebike"}}
	]`), tagNames)
	if err != nil {
		t.Fatalf("NormalizeRuleActions cycling electric: %v", err)
	}
	if strings.Contains(string(cycling), "tag-ebike") {
		t.Fatalf("cycling electric action should dedupe E-Bike tag: %s", cycling)
	}
}

func TestNormalizeRuleActionsRejectsMissingRequiredParams(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"set_gear without gear_id", json.RawMessage(`[{"action":"set_gear","params":{}}]`), "set_gear requires params.gear_id"},
		{"add_tag without tag_id", json.RawMessage(`[{"action":"add_tag","params":{}}]`), "add_tag requires params.tag_id"},
		{"set_visibility without visibility", json.RawMessage(`[{"action":"set_visibility","params":{}}]`), "set_visibility requires params.visibility"},
		{"set_sport_type without sport_type", json.RawMessage(`[{"action":"set_sport_type","params":{}}]`), "set_sport_type requires params.sport_type"},
		{"set_name without template", json.RawMessage(`[{"action":"set_name","params":{}}]`), "set_name requires params.template"},
		{"set_intensity without intensity", json.RawMessage(`[{"action":"set_intensity","params":{}}]`), "set_intensity requires params.intensity"},
		{"set_field without field", json.RawMessage(`[{"action":"set_field","params":{"value":true}}]`), "set_field requires params.field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeRuleActions(tc.raw, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestNormalizeRuleActionsRejectsContradictorySystemTagActions(t *testing.T) {
	tagNames := map[string]string{"tag-ebike": "E-Bike", "tag-indoor": "Indoor"}
	cases := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{
			name: "electric false plus e-bike add_tag",
			raw:  json.RawMessage(`[{"action":"set_sport_type","params":{"sport_type":"cycling"}},{"action":"set_electric","params":{"electric":false}},{"action":"add_tag","params":{"tag_id":"tag-ebike"}}]`),
			want: "E-Antrieb=false conflicts with add_tag(E-Bike)",
		},
		{
			name: "electric true plus e-bike remove_tag",
			raw:  json.RawMessage(`[{"action":"set_sport_type","params":{"sport_type":"cycling"}},{"action":"set_electric","params":{"electric":true}},{"action":"remove_tag","params":{"tag_id":"tag-ebike"}}]`),
			want: "E-Antrieb=true conflicts with remove_tag(E-Bike)",
		},
		{
			name: "indoor false plus indoor add_tag",
			raw:  json.RawMessage(`[{"action":"set_indoor","params":{"indoor":false}},{"action":"add_tag","params":{"tag_id":"tag-indoor"}}]`),
			want: "Indoor=false conflicts with add_tag(Indoor)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeRuleActions(tc.raw, tagNames)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestNormalizeRuleActionsRejectsConflictingModifierWrites(t *testing.T) {
	raw := json.RawMessage(`[
		{"action":"set_virtual","params":{"virtual":true}},
		{"action":"set_virtual","params":{"virtual":false}}
	]`)

	_, err := NormalizeRuleActions(raw, nil)
	if err == nil || !strings.Contains(err.Error(), "conflicting writes to is_virtual") {
		t.Fatalf("error = %v, want conflicting is_virtual writes", err)
	}
}

func TestNormalizeRuleActionsTreatsSetFieldModifierAsModifierWrite(t *testing.T) {
	raw := json.RawMessage(`[
		{"action":"set_field","params":{"field":"is_commute","value":true}},
		{"action":"set_commute","params":{"commute":false}}
	]`)

	_, err := NormalizeRuleActions(raw, nil)
	if err == nil || !strings.Contains(err.Error(), "conflicting writes to is_commute") {
		t.Fatalf("error = %v, want conflicting is_commute writes", err)
	}
}

func TestNormalizeRuleBranchesValidatesNestedActions(t *testing.T) {
	tagNames := map[string]string{"tag-ebike": "E-Bike"}
	raw := json.RawMessage(`[
		{"type":"if","condition_groups":[],"actions":[
			{"action":"set_sport_type","params":{"sport_type":"cycling"}},
			{"action":"set_electric","params":{"electric":true}},
			{"action":"remove_tag","params":{"tag_id":"tag-ebike"}}
		]}
	]`)

	_, err := NormalizeRuleBranches(raw, tagNames)
	if err == nil || !strings.Contains(err.Error(), "branches[0].actions") {
		t.Fatalf("error = %v, want branch action context", err)
	}
}

func TestNormalizeRuleActionsUsesGenericElectricModifierNameInConflicts(t *testing.T) {
	tagNames := map[string]string{"tag-ebike": "E-Bike"}
	raw := json.RawMessage(`[
		{"action":"set_sport_type","params":{"sport_type":"cycling"}},
		{"action":"set_electric","params":{"electric":false}},
		{"action":"add_tag","params":{"tag_id":"tag-ebike"}}
	]`)

	_, err := NormalizeRuleActions(raw, tagNames)
	if err == nil || !strings.Contains(err.Error(), "E-Antrieb=false conflicts with add_tag(E-Bike)") {
		t.Fatalf("error = %v, want generic E-Antrieb conflict text", err)
	}
}
