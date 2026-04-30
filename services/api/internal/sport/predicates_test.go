package sport

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPredicatesMatchSubtypeStates(t *testing.T) {
	mountain := "mountain"
	empty := ""

	tests := []struct {
		name     string
		preds    []Predicate
		sport    string
		subType  *string
		expected bool
	}{
		{name: "nil predicate slice matches all", preds: nil, sport: "cycling", subType: &mountain, expected: true},
		{name: "empty predicate slice matches never", preds: []Predicate{}, sport: "cycling", subType: &mountain, expected: false},
		{name: "open sport predicate matches nil subtype", preds: []Predicate{{Sport: "cycling"}}, sport: "cycling", expected: true},
		{name: "open sport predicate matches exact subtype", preds: []Predicate{{Sport: "cycling"}}, sport: "cycling", subType: &mountain, expected: true},
		{name: "open sport predicate matches empty subtype", preds: []Predicate{{Sport: "cycling"}}, sport: "cycling", subType: &empty, expected: true},
		{name: "exact subtype matches same subtype", preds: []Predicate{{Sport: "cycling", SubType: &mountain}}, sport: "cycling", subType: &mountain, expected: true},
		{name: "exact subtype rejects nil subtype", preds: []Predicate{{Sport: "cycling", SubType: &mountain}}, sport: "cycling", expected: false},
		{name: "exact subtype rejects different subtype", preds: []Predicate{{Sport: "cycling", SubType: &mountain}}, sport: "cycling", subType: strPtr("road"), expected: false},
		{name: "empty subtype sentinel matches nil subtype", preds: []Predicate{{Sport: "cycling", SubType: &empty}}, sport: "cycling", expected: true},
		{name: "empty subtype sentinel matches empty subtype", preds: []Predicate{{Sport: "cycling", SubType: &empty}}, sport: "cycling", subType: &empty, expected: true},
		{name: "empty subtype sentinel rejects concrete subtype", preds: []Predicate{{Sport: "cycling", SubType: &empty}}, sport: "cycling", subType: &mountain, expected: false},
		{name: "sport mismatch rejects", preds: []Predicate{{Sport: "cycling"}}, sport: "running", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PredicatesMatch(tt.preds, tt.sport, tt.subType); got != tt.expected {
				t.Fatalf("PredicatesMatch() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestValidatePredicates(t *testing.T) {
	mountain := "mountain"
	empty := ""
	generic := "generic"

	tests := []struct {
		name       string
		preds      []Predicate
		allowEmpty bool
		wantErr    bool
	}{
		{name: "rejects empty routing profile list", preds: []Predicate{}, allowEmpty: false, wantErr: true},
		{name: "allows empty list when requested", preds: []Predicate{}, allowEmpty: true},
		{name: "accepts open canonical sport", preds: []Predicate{{Sport: "cycling"}}},
		{name: "accepts exact subtype", preds: []Predicate{{Sport: "cycling", SubType: &mountain}}},
		{name: "accepts empty subtype sentinel", preds: []Predicate{{Sport: "cycling", SubType: &empty}}},
		{name: "rejects generic subtype storage sentinel", preds: []Predicate{{Sport: "cycling", SubType: &generic}}, wantErr: true},
		{name: "rejects unknown sport", preds: []Predicate{{Sport: "does_not_exist"}}, wantErr: true},
		{name: "rejects unknown subtype", preds: []Predicate{{Sport: "cycling", SubType: strPtr("does_not_exist")}}, wantErr: true},
		{name: "rejects legacy sport alias", preds: []Predicate{{Sport: "mountain_biking"}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePredicates(tt.preds, tt.allowEmpty)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePredicates() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizePredicates(t *testing.T) {
	preds, err := NormalizePredicates([]Predicate{
		{Sport: "mountain_biking"},
		{Sport: "gravel_cycling"},
		{Sport: "cycling", SubType: strPtr("road")},
	})
	if err != nil {
		t.Fatalf("NormalizePredicates() error = %v", err)
	}

	expected := []Predicate{
		{Sport: "cycling", SubType: strPtr("mountain")},
		{Sport: "cycling", SubType: strPtr("gravel_cycling")},
		{Sport: "cycling", SubType: strPtr("road")},
	}
	if !reflect.DeepEqual(preds, expected) {
		t.Fatalf("NormalizePredicates() = %#v, want %#v", preds, expected)
	}
}

func TestPredicateJSONOmitsNullSubtype(t *testing.T) {
	data, err := json.Marshal([]Predicate{{Sport: "cycling"}})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if string(data) != `[{"sport":"cycling"}]` {
		t.Fatalf("json = %s, want no explicit null subtype", data)
	}
}

func TestModifiersMatch(t *testing.T) {
	trueValue := true
	falseValue := false
	state := ActivityModifierState{
		IsElectric: true,
		IsIndoor:   false,
		IsVirtual:  true,
		IsCommute:  false,
	}

	tests := []struct {
		name     string
		filters  ModifierFilters
		expected bool
	}{
		{name: "omitted filters match", expected: true},
		{name: "explicit matching electric filter matches", filters: ModifierFilters{IsElectric: &trueValue}, expected: true},
		{name: "explicit nonmatching electric filter rejects", filters: ModifierFilters{IsElectric: &falseValue}, expected: false},
		{name: "all present filters match exactly", filters: ModifierFilters{IsElectric: &trueValue, IsIndoor: &falseValue, IsVirtual: &trueValue, IsCommute: &falseValue}, expected: true},
		{name: "one present filter mismatch rejects", filters: ModifierFilters{IsElectric: &trueValue, IsIndoor: &trueValue}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ModifiersMatch(tt.filters, state); got != tt.expected {
				t.Fatalf("ModifiersMatch() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPredicatesFromSportTypesAndBack(t *testing.T) {
	preds, err := PredicatesFromSportTypes([]string{"mountain_biking", "cycling", "e_bike"})
	if err != nil {
		t.Fatalf("PredicatesFromSportTypes() error = %v", err)
	}

	expected := []Predicate{
		{Sport: "cycling", SubType: strPtr("mountain")},
		{Sport: "cycling"},
		{Sport: "cycling"},
	}
	if !reflect.DeepEqual(preds, expected) {
		t.Fatalf("predicates = %#v, want %#v", preds, expected)
	}

	sportTypes := SportTypesFromPredicates(preds)
	if !reflect.DeepEqual(sportTypes, []string{"cycling"}) {
		t.Fatalf("sport types = %#v, want [cycling]", sportTypes)
	}
}

func TestPredicatesFromFilterTokens(t *testing.T) {
	preds, err := PredicatesFromFilterTokens("cycling:mountain,running:trail,cycling")
	if err != nil {
		t.Fatalf("PredicatesFromFilterTokens() error = %v", err)
	}

	expected := []Predicate{
		{Sport: "cycling", SubType: strPtr("mountain")},
		{Sport: "running", SubType: strPtr("trail")},
		{Sport: "cycling"},
	}
	if !reflect.DeepEqual(preds, expected) {
		t.Fatalf("PredicatesFromFilterTokens() = %#v, want %#v", preds, expected)
	}
}

func strPtr(v string) *string {
	return &v
}
