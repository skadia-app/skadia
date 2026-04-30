package rules

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeRuleActionsDedupesElectricAndIndoorSystemTags(t *testing.T) {
	tagNames := map[string]string{
		"tag-ebike":  "E-Bike",
		"tag-indoor": "Indoor",
		"tag-other":  "Urlaub",
	}
	raw := json.RawMessage(`[
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
	if len(actions) != 3 {
		t.Fatalf("actions len = %d, want 3: %s", len(actions), got)
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

func TestNormalizeRuleActionsRejectsContradictorySystemTagActions(t *testing.T) {
	tagNames := map[string]string{"tag-ebike": "E-Bike", "tag-indoor": "Indoor"}
	cases := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{
			name: "electric false plus e-bike add_tag",
			raw:  json.RawMessage(`[{"action":"set_electric","params":{"electric":false}},{"action":"add_tag","params":{"tag_id":"tag-ebike"}}]`),
			want: "set_electric(false)",
		},
		{
			name: "electric true plus e-bike remove_tag",
			raw:  json.RawMessage(`[{"action":"set_electric","params":{"electric":true}},{"action":"remove_tag","params":{"tag_id":"tag-ebike"}}]`),
			want: "remove_tag(E-Bike)",
		},
		{
			name: "indoor false plus indoor add_tag",
			raw:  json.RawMessage(`[{"action":"set_indoor","params":{"indoor":false}},{"action":"add_tag","params":{"tag_id":"tag-indoor"}}]`),
			want: "set_indoor(false)",
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
			{"action":"set_electric","params":{"electric":true}},
			{"action":"remove_tag","params":{"tag_id":"tag-ebike"}}
		]}
	]`)

	_, err := NormalizeRuleBranches(raw, tagNames)
	if err == nil || !strings.Contains(err.Error(), "branches[0].actions") {
		t.Fatalf("error = %v, want branch action context", err)
	}
}
