package tools

import (
	"reflect"
	"testing"
)

func TestApplyActivityFilters(t *testing.T) {
	subType := "trail"
	isVirtual := false
	isCommute := true
	conditions, params := applyActivityFilters([]string{"a.user_id = $1"}, []any{"user-1"}, ActivityFilters{
		TableAlias: "a",
		SportType:  "running",
		SubType:    &subType,
		IsVirtual:  &isVirtual,
		IsCommute:  &isCommute,
	})

	wantConditions := []string{
		"a.user_id = $1",
		"a.sport_type = $2",
		"a.sub_type = $3",
		"a.is_virtual = $4",
		"a.is_commute = $5",
	}
	wantParams := []any{"user-1", "running", "trail", false, true}
	if !reflect.DeepEqual(conditions, wantConditions) {
		t.Fatalf("conditions mismatch:\nwant %#v\ngot  %#v", wantConditions, conditions)
	}
	if !reflect.DeepEqual(params, wantParams) {
		t.Fatalf("params mismatch:\nwant %#v\ngot  %#v", wantParams, params)
	}
}

func TestApplyActivityFiltersEmptySubtypeMeansNull(t *testing.T) {
	subType := ""
	conditions, params := applyActivityFilters(nil, nil, ActivityFilters{SubType: &subType})

	if !reflect.DeepEqual(conditions, []string{"sub_type IS NULL"}) {
		t.Fatalf("unexpected conditions: %#v", conditions)
	}
	if len(params) != 0 {
		t.Fatalf("empty subtype must not add a parameter, got %#v", params)
	}
}

func TestApplySegmentEffortFilters(t *testing.T) {
	subType := ""
	inLeaderboard := true
	isIndoor := false
	conditions, params := applySegmentEffortFilters(nil, []any{"user-1"}, SegmentEffortFilters{
		SportType:     "cycling",
		SubType:       &subType,
		InLeaderboard: &inLeaderboard,
		IsIndoor:      &isIndoor,
	})

	wantConditions := []string{
		"se.sport_type = $2",
		"(se.sub_type IS NULL OR se.sub_type = 'generic')",
		"se.in_leaderboard = $3",
		"a.is_indoor = $4",
	}
	wantParams := []any{"user-1", "cycling", true, false}
	if !reflect.DeepEqual(conditions, wantConditions) {
		t.Fatalf("conditions mismatch:\nwant %#v\ngot  %#v", wantConditions, conditions)
	}
	if !reflect.DeepEqual(params, wantParams) {
		t.Fatalf("params mismatch:\nwant %#v\ngot  %#v", wantParams, params)
	}
}
