package rules

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateGearConditionJSONAcceptsWhitelistedGearFields(t *testing.T) {
	err := ValidateGearConditionJSON(nil, json.RawMessage(`[
		{"logic":"and","conditions":[
			{"field":"gear_id","op":"is_not_null","value":null},
			{"field":"gear_total_elevation_loss_m","op":"gte","value":1000},
			{"field":"gear_total_moving_time_s","op":"gte","value":3600}
		]}
	]`), nil)
	if err != nil {
		t.Fatalf("ValidateGearConditionJSON: %v", err)
	}
}

func TestValidateGearConditionJSONRejectsUnknownGearFields(t *testing.T) {
	err := ValidateGearConditionJSON(nil, json.RawMessage(`[
		{"logic":"and","conditions":[
			{"field":"gearr_total_distance_m","op":"gte","value":1}
		]}
	]`), nil)
	if err == nil || !strings.Contains(err.Error(), `unknown gear condition field "gearr_total_distance_m"`) {
		t.Fatalf("error = %v, want unknown gear field", err)
	}
}

func TestValidateGearConditionJSONValidatesBranchConditionGroups(t *testing.T) {
	err := ValidateGearConditionJSON(nil, nil, json.RawMessage(`[
		{"type":"if","condition_groups":[
			{"logic":"and","conditions":[
				{"field":"gear_deleted_at","op":"is_not_null","value":null}
			]}
		],"actions":[]}
	]`))
	if err == nil || !strings.Contains(err.Error(), `branches[0].condition_groups[0][0]`) {
		t.Fatalf("error = %v, want branch condition context", err)
	}
}
