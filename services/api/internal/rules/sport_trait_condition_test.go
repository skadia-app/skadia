package rules

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEvaluateSportTraitConditionsMatchEffectiveTupleTraits(t *testing.T) {
	tests := []struct {
		name      string
		condition Condition
		activity  map[string]any
	}{
		{
			name:      "water environment",
			condition: Condition{Field: "sport_environment", Op: "in", Value: []any{"water"}},
			activity:  map[string]any{"sport_type": "swimming", "sub_type": "open_water"},
		},
		{
			name:      "board equipment across water sport",
			condition: Condition{Field: "sport_equipment_family", Op: "in", Value: []any{"board"}},
			activity:  map[string]any{"sport_type": "surfing"},
		},
		{
			name:      "paddle movement from taxonomy traits",
			condition: Condition{Field: "sport_movement_family", Op: "in", Value: []any{"paddle"}},
			activity:  map[string]any{"sport_type": "stand_up_paddleboarding"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !evalCondition(tt.condition, tt.activity) {
				t.Fatalf("expected %s to match %#v", tt.condition.Field, tt.activity)
			}
		})
	}
}

func TestEvaluateSportTraitConditionRejectsUnknownTupleAndNonIntersection(t *testing.T) {
	if evalCondition(Condition{Field: "sport_equipment_family", Op: "in", Value: []any{"board"}}, map[string]any{"sport_type": "running"}) {
		t.Fatal("running must not match board equipment")
	}
	if evalCondition(Condition{Field: "sport_environment", Op: "in", Value: []any{"water"}}, map[string]any{"sport_type": "nope"}) {
		t.Fatal("unknown sport must not match trait condition")
	}
}

func TestValidateSportTraitConditionRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		name string
		cond Condition
		want string
	}{
		{"wrong op", Condition{Field: "sport_environment", Op: "eq", Value: []any{"water"}}, "op"},
		{"empty value", Condition{Field: "sport_environment", Op: "in", Value: []any{}}, "non-empty"},
		{"string scalar", Condition{Field: "sport_environment", Op: "in", Value: "water"}, "array"},
		{"unknown field", Condition{Field: "sport_propulsion_mode", Op: "in", Value: []any{"human"}}, "unsupported trait field"},
		{"unknown value", Condition{Field: "sport_equipment_family", Op: "in", Value: []any{"hoverboard"}}, "unknown trait value"},
		{"non string value", Condition{Field: "sport_movement_family", Op: "in", Value: []any{1}}, "string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSportTraitCondition(tt.cond, "conditions[0]")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateSportTaxonomyRuleJSONAcceptsTraitConditionsInBranches(t *testing.T) {
	input := SportTaxonomyRuleJSON{Branches: json.RawMessage(`[
		{"type":"if","condition_groups":[{"logic":"and","conditions":[
			{"field":"sport_environment","op":"in","value":["water"]},
			{"field":"sport_equipment_family","op":"in","value":["board"]},
			{"field":"sport_movement_family","op":"in","value":["paddle"]}
		]}],"actions":[]}
	]`)}
	if _, _, err := NormalizeAndValidateSportTaxonomyRuleJSON(input); err != nil {
		t.Fatalf("NormalizeAndValidateSportTaxonomyRuleJSON: %v", err)
	}
}

func TestValidateSportTaxonomyRuleJSONRejectsUnknownTraitValues(t *testing.T) {
	input := SportTaxonomyRuleJSON{ConditionGroups: json.RawMessage(`[
		{"logic":"and","conditions":[
			{"field":"sport_environment","op":"in","value":["space"]}
		]}
	]`)}
	_, _, err := NormalizeAndValidateSportTaxonomyRuleJSON(input)
	if err == nil || !strings.Contains(err.Error(), "condition_groups[0].conditions[0]") || !strings.Contains(err.Error(), "unknown trait value") {
		t.Fatalf("error = %v, want surface-specific trait rejection", err)
	}
}
