package rules

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEvaluateSportTupleInMatchesSubtypeAndModifiersAtomically(t *testing.T) {
	condition := Condition{
		Field: "sport_tuple",
		Op:    "in",
		Value: []any{
			map[string]any{"sport_type": "cycling", "sub_type": "mountain", "is_electric": true},
			map[string]any{"sport_type": "running", "sub_type": "trail"},
		},
	}
	if !evalCondition(condition, map[string]any{"sport_type": "cycling", "sub_type": "mountain", "is_electric": true}) {
		t.Fatal("cycling/mountain electric should match")
	}
	if evalCondition(condition, map[string]any{"sport_type": "cycling", "sub_type": "mountain", "is_electric": false}) {
		t.Fatal("cycling/mountain non-electric must not match explicit electric tuple")
	}
	if !evalCondition(condition, map[string]any{"sport_type": "running", "sub_type": "trail"}) {
		t.Fatal("running/trail should match second tuple")
	}
}

func TestEvaluateSportTupleInTreatsMissingModifierAsFalse(t *testing.T) {
	falseCondition := Condition{
		Field: "sport_tuple",
		Op:    "in",
		Value: []any{map[string]any{"sport_type": "cycling", "is_electric": false}},
	}
	if !evalCondition(falseCondition, map[string]any{"sport_type": "cycling"}) {
		t.Fatal("missing activity modifier should match an explicit false sport_tuple modifier")
	}
	if !evalCondition(falseCondition, map[string]any{"sport_type": "cycling", "is_electric": nil}) {
		t.Fatal("null activity modifier should match an explicit false sport_tuple modifier")
	}

	trueCondition := Condition{
		Field: "sport_tuple",
		Op:    "in",
		Value: []any{map[string]any{"sport_type": "cycling", "is_electric": true}},
	}
	if evalCondition(trueCondition, map[string]any{"sport_type": "cycling"}) {
		t.Fatal("missing activity modifier must not match an explicit true sport_tuple modifier")
	}
}

func TestEvaluateSportTupleInSubTypeOmittedMeansAnySubtype(t *testing.T) {
	condition := Condition{Field: "sport_tuple", Op: "in", Value: []any{map[string]any{"sport_type": "running"}}}
	if !evalCondition(condition, map[string]any{"sport_type": "running", "sub_type": "trail"}) {
		t.Fatal("omitted sub_type should match any subtype under sport")
	}
}

func TestEvaluateSportTupleInExplicitEmptySubTypeMeansParentOnly(t *testing.T) {
	condition := Condition{Field: "sport_tuple", Op: "in", Value: []any{map[string]any{"sport_type": "running", "sub_type": ""}}}
	if !evalCondition(condition, map[string]any{"sport_type": "running", "sub_type": ""}) {
		t.Fatal("explicit empty subtype should match parent-only activity")
	}
	if !evalCondition(condition, map[string]any{"sport_type": "running"}) {
		t.Fatal("explicit empty subtype should match missing activity subtype")
	}
	if !evalCondition(condition, map[string]any{"sport_type": "running", "sub_type": nil}) {
		t.Fatal("explicit empty subtype should match null activity subtype")
	}
	if evalCondition(condition, map[string]any{"sport_type": "running", "sub_type": "trail"}) {
		t.Fatal("explicit empty subtype must not match trail")
	}
}

func TestEvaluateSportTupleInNormalizesActivityParentOnlySubType(t *testing.T) {
	condition := Condition{Field: "sport_tuple", Op: "in", Value: []any{map[string]any{"sport_type": "cycling", "sub_type": ""}}}
	for _, subType := range []any{"generic", " Generic ", "   "} {
		if !evalCondition(condition, map[string]any{"sport_type": "cycling", "sub_type": subType}) {
			t.Fatalf("explicit parent-only sport_tuple condition should match activity sub_type %#v", subType)
		}
	}
}

func TestEvaluateGroupsMatchesSportTupleCondition(t *testing.T) {
	groups := []ConditionGroup{{
		Logic: "and",
		Conditions: []Condition{
			{Field: "sport_tuple", Op: "in", Value: []any{
				map[string]any{"sport_type": "cycling", "sub_type": "mountain", "is_electric": true},
			}},
			{Field: "distance_m", Op: "gte", Value: 10000.0},
		},
	}}
	activity := map[string]any{
		"sport_type":  "cycling",
		"sub_type":    "mountain",
		"is_electric": true,
		"distance_m":  12000.0,
	}
	if !EvaluateGroups(groups, activity) {
		t.Fatal("grouped sport_tuple condition should match")
	}
}

func TestValidateSportTupleConditionRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		name string
		cond Condition
		want string
	}{
		{"wrong op", Condition{Field: "sport_tuple", Op: "eq", Value: []any{}}, "op"},
		{"empty value", Condition{Field: "sport_tuple", Op: "in", Value: []any{}}, "non-empty"},
		{"unknown sport", Condition{Field: "sport_tuple", Op: "in", Value: []any{map[string]any{"sport_type": "nope"}}}, "canonical sport"},
		{"wrong subtype", Condition{Field: "sport_tuple", Op: "in", Value: []any{map[string]any{"sport_type": "running", "sub_type": "mountain"}}}, "sub_type"},
		{"string modifier", Condition{Field: "sport_tuple", Op: "in", Value: []any{map[string]any{"sport_type": "cycling", "is_electric": "true"}}}, "boolean"},
		{"extra key", Condition{Field: "sport_tuple", Op: "in", Value: []any{map[string]any{"sport_type": "cycling", "foo": true}}}, "unsupported key"},
		{"duplicate tuple", Condition{Field: "sport_tuple", Op: "in", Value: []any{
			map[string]any{"sport_type": "cycling", "sub_type": "mountain", "is_electric": true},
			map[string]any{"sport_type": "cycling", "sub_type": "mountain", "is_electric": true},
		}}, "duplicate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSportTupleCondition(tt.cond, "conditions[0]")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateSportTupleConditionAcceptsCanonicalNonSelectableTuples(t *testing.T) {
	conditions := []Condition{
		{Field: "sport_tuple", Op: "in", Value: []any{map[string]any{"sport_type": "training"}}},
		{Field: "sport_tuple", Op: "in", Value: []any{map[string]any{"sport_type": "transition", "sub_type": "swim_to_bike_transition"}}},
	}
	for i, condition := range conditions {
		if err := ValidateSportTupleCondition(condition, "conditions"); err != nil {
			t.Fatalf("condition %d rejected canonical non-selectable tuple: %v", i, err)
		}
	}
}

func TestValidateSportTupleConditionAcceptsJSONRawMessageValue(t *testing.T) {
	condition := Condition{
		Field: "sport_tuple",
		Op:    "in",
		Value: json.RawMessage(`[{"sport_type":"cycling","sub_type":"mountain","is_electric":true}]`),
	}
	if err := ValidateSportTupleCondition(condition, "conditions[0]"); err != nil {
		t.Fatalf("ValidateSportTupleCondition: %v", err)
	}
}
