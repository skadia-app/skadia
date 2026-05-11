package handler

import (
	"encoding/json"
	"errors"
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

// Phase 4b.A.2.1b — verify the IsValidSystemType invariant gate in
// commute_rule.go. If someone removes "commute_auto" from
// automation.OnDemandSystemTypes, the gate must abort BEFORE any INSERT
// is attempted (otherwise we'd produce a row that no sweep manages).

func TestValidateCommuteAutoSystemType_DefaultPasses(t *testing.T) {
	// Default validator delegates to automation.IsValidSystemType.
	// The current spec list contains "commute_auto" in OnDemandSystemTypes,
	// so the default validator must return nil.
	if err := validateCommuteAutoSystemType(); err != nil {
		t.Errorf("default validator must accept commute_auto, got: %v", err)
	}
}

func TestValidateCommuteAutoSystemType_RejectsWhenInvariantBroken(t *testing.T) {
	// Replace the validator with one that simulates "commute_auto missing
	// from the whitelist" — the gate must return an error so the handler
	// can return 500 without attempting an INSERT.
	original := commuteSystemTypeValidator
	defer func() { commuteSystemTypeValidator = original }()
	commuteSystemTypeValidator = func(s string) bool {
		return false // simulate missing from whitelist
	}

	err := validateCommuteAutoSystemType()
	if err == nil {
		t.Fatal("expected error when validator returns false, got nil")
	}
	if !errors.Is(err, errCommuteAutoNotInWhitelist) {
		t.Errorf("expected errCommuteAutoNotInWhitelist, got %v", err)
	}
}
