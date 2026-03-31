package rules_test

import (
	"encoding/json"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/rules"
)

func TestEvaluateGroups_AllGroupsMustMatch(t *testing.T) {
	groups := []rules.ConditionGroup{
		{Logic: "and", Conditions: []rules.Condition{
			{Field: "distance_m", Op: "gte", Value: 10000.0},
		}},
		{Logic: "and", Conditions: []rules.Condition{
			{Field: "sport_type", Op: "eq", Value: "running"},
		}},
	}
	act := map[string]any{"distance_m": 15000.0, "sport_type": "running"}
	if !rules.EvaluateGroups(groups, act) {
		t.Error("expected match")
	}
	act2 := map[string]any{"distance_m": 5000.0, "sport_type": "running"}
	if rules.EvaluateGroups(groups, act2) {
		t.Error("expected no match when first group fails")
	}
}

func TestEvaluateGroups_OrWithinGroup(t *testing.T) {
	groups := []rules.ConditionGroup{
		{Logic: "or", Conditions: []rules.Condition{
			{Field: "sport_type", Op: "eq", Value: "running"},
			{Field: "sport_type", Op: "eq", Value: "hiking"},
		}},
	}
	act := map[string]any{"sport_type": "hiking"}
	if !rules.EvaluateGroups(groups, act) {
		t.Error("expected OR match")
	}
}

func TestEvaluateGroups_Geofence_Inside(t *testing.T) {
	groups := []rules.ConditionGroup{
		{Logic: "and", Conditions: []rules.Condition{
			{Field: "geofence", Op: "inside", Value: map[string]any{
				"lat": 48.137, "lon": 11.576, "radius_m": 5000.0,
			}},
		}},
	}
	act := map[string]any{"start_lat": 48.1374, "start_lon": 11.5755}
	if !rules.EvaluateGroups(groups, act) {
		t.Error("expected inside geofence")
	}
}

func TestEvaluateGroups_Pace(t *testing.T) {
	groups := []rules.ConditionGroup{
		{Logic: "and", Conditions: []rules.Condition{
			{Field: "avg_pace_min_km", Op: "lte", Value: 5.0},
		}},
	}
	// avg_speed_ms = 3.5 m/s -> pace = 1000/(3.5*60) ~ 4.76 min/km
	act := map[string]any{"avg_speed_ms": 3.5}
	if !rules.EvaluateGroups(groups, act) {
		t.Error("expected pace match")
	}
}

func TestEvalCondition_StartEndDistanceM(t *testing.T) {
	groups := []rules.ConditionGroup{
		{Logic: "and", Conditions: []rules.Condition{
			{Field: "start_end_distance_m", Op: "lte", Value: 500.0},
		}},
	}
	act := map[string]any{"start_end_distance_m": 300.0}
	if !rules.EvaluateGroups(groups, act) {
		t.Error("expected match at 300m")
	}
	act2 := map[string]any{"start_end_distance_m": 600.0}
	if rules.EvaluateGroups(groups, act2) {
		t.Error("expected no match at 600m")
	}
}

func TestEvalCondition_SunElevation(t *testing.T) {
	groups := []rules.ConditionGroup{
		{Logic: "and", Conditions: []rules.Condition{
			{Field: "sun_elevation", Op: "lte", Value: -6.0},
		}},
	}
	act := map[string]any{"sun_elevation": -10.0}
	if !rules.EvaluateGroups(groups, act) {
		t.Error("expected match at -10°")
	}
	act2 := map[string]any{"sun_elevation": -3.0}
	if rules.EvaluateGroups(groups, act2) {
		t.Error("expected no match at -3°")
	}
}

func TestApply_SetField(t *testing.T) {
	actions := []rules.Action{
		{Action: "set_field", Params: json.RawMessage(`{"field":"is_round_trip","value":true}`)},
	}
	changes := rules.Apply(actions)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Field != "is_round_trip" {
		t.Errorf("expected field is_round_trip, got %s", changes[0].Field)
	}
	if changes[0].Value != true {
		t.Errorf("expected value true, got %v", changes[0].Value)
	}
}

func TestApply_SetField_UnknownFieldIgnored(t *testing.T) {
	actions := []rules.Action{
		{Action: "set_field", Params: json.RawMessage(`{"field":"user_id","value":"hack"}`)},
	}
	changes := rules.Apply(actions)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for unknown field, got %d", len(changes))
	}
}

func TestEvalCondition_Between(t *testing.T) {
	activity := map[string]any{
		"distance_m": float64(25000),
	}
	// Value is "min,max" string
	c := rules.Condition{Field: "distance_m", Op: "between", Value: "21097,42195"}
	if !rules.EvaluateGroups([]rules.ConditionGroup{{Logic: "and", Conditions: []rules.Condition{c}}}, activity) {
		t.Fatal("expected 25000 to be between 21097 and 42195")
	}
	// Outside range
	c2 := rules.Condition{Field: "distance_m", Op: "between", Value: "42195,100000"}
	if rules.EvaluateGroups([]rules.ConditionGroup{{Logic: "and", Conditions: []rules.Condition{c2}}}, activity) {
		t.Fatal("expected 25000 to NOT be between 42195 and 100000")
	}
	// Edge: exactly at min boundary (inclusive)
	c3 := rules.Condition{Field: "distance_m", Op: "between", Value: "25000,30000"}
	if !rules.EvaluateGroups([]rules.ConditionGroup{{Logic: "and", Conditions: []rules.Condition{c3}}}, activity) {
		t.Fatal("expected 25000 to be between 25000 and 30000 (inclusive)")
	}
}

func TestEvalCondition_HasTag_Neq(t *testing.T) {
	activity := map[string]any{
		"tags": []string{"indoor", "frost"},
	}
	makeGroups := func(c rules.Condition) []rules.ConditionGroup {
		return []rules.ConditionGroup{{Logic: "and", Conditions: []rules.Condition{c}}}
	}
	// neq = tag is NOT present → true
	if !rules.EvaluateGroups(makeGroups(rules.Condition{Field: "has_tag", Op: "neq", Value: "outdoor"}), activity) {
		t.Fatal("expected neq to return true when tag is absent")
	}
	// neq = tag IS present → false
	if rules.EvaluateGroups(makeGroups(rules.Condition{Field: "has_tag", Op: "neq", Value: "indoor"}), activity) {
		t.Fatal("expected neq to return false when tag is present")
	}
	// no tags at all → neq should be true (tag is absent)
	if !rules.EvaluateGroups(makeGroups(rules.Condition{Field: "has_tag", Op: "neq", Value: "anything"}), map[string]any{}) {
		t.Fatal("expected neq to return true when activity has no tags")
	}
}
