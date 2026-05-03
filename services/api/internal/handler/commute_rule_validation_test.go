package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateCommuteRuleBranchesRejectsMalformedSportTuple(t *testing.T) {
	branches := json.RawMessage(`[
		{"type":"if","condition_groups":[{"logic":"and","conditions":[
			{"field":"sport_tuple","op":"in","value":[{"sport_type":"running","sub_type":"mountain"}]}
		]}],"actions":[]}
	]`)
	err := validateCommuteRuleBranches(branches)
	if err == nil || !strings.Contains(err.Error(), "sub_type") {
		t.Fatalf("error = %v, want sub_type rejection", err)
	}
}
