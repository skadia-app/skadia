package dataimport

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestNormalizeSportTaxonomyImportRowActivities(t *testing.T) {
	row := map[string]any{
		"sport_type":        "mountain_biking",
		"sub_type":          "",
		"parsed_sport_type": "treadmill_running",
		"parsed_sub_type":   "",
		"is_electric":       true,
		"is_indoor":         false,
	}

	got, err := normalizeSportTaxonomyImportRow("activities", row, nil)
	if err != nil {
		t.Fatalf("normalizeSportTaxonomyImportRow: %v", err)
	}
	if got["sport_type"] != "cycling" || got["sub_type"] != "mountain" {
		t.Fatalf("sport tuple: got (%#v, %#v), want (cycling, mountain)", got["sport_type"], got["sub_type"])
	}
	if got["parsed_sport_type"] != "running" || got["parsed_sub_type"] != "treadmill" {
		t.Fatalf("parsed tuple: got (%#v, %#v), want (running, treadmill)", got["parsed_sport_type"], got["parsed_sub_type"])
	}
	if got["is_electric"] != true {
		t.Fatalf("is_electric should preserve true, got %#v", got["is_electric"])
	}
	if got["is_indoor"] != true {
		t.Fatalf("is_indoor should be set by parsed treadmill tuple, got %#v", got["is_indoor"])
	}
}

func TestImportModifierPatchAppliesVirtual(t *testing.T) {
	virtual := true
	patch := sport.ModifierPatch{IsVirtual: &virtual}

	row := map[string]any{"is_virtual": false}
	applyModifierPatchToRow(row, patch)
	if row["is_virtual"] != true {
		t.Fatalf("is_virtual row patch = %#v, want true", row["is_virtual"])
	}

	merged := mergeImportModifierPatch(sport.ModifierPatch{}, patch)
	if merged.IsVirtual == nil || !*merged.IsVirtual {
		t.Fatalf("merged IsVirtual = %#v, want true", merged.IsVirtual)
	}

	filters := map[string]any{}
	applyPatchToModifierFilters(filters, patch)
	if filters["is_virtual"] != true {
		t.Fatalf("is_virtual filter patch = %#v, want true", filters["is_virtual"])
	}
}

func TestNormalizeSportTaxonomyImportRowClearsEmptyRuntimeSubTypeToNil(t *testing.T) {
	row := map[string]any{
		"sport_type":        "cycling",
		"sub_type":          "",
		"parsed_sport_type": "running",
		"parsed_sub_type":   "",
	}

	got, err := normalizeSportTaxonomyImportRow("activities", row, nil)
	if err != nil {
		t.Fatalf("normalizeSportTaxonomyImportRow: %v", err)
	}
	if got["sub_type"] != nil {
		t.Fatalf("sub_type = %#v, want nil for canonical no-subtype runtime storage", got["sub_type"])
	}
	if got["parsed_sub_type"] != nil {
		t.Fatalf("parsed_sub_type = %#v, want nil for canonical no-subtype runtime storage", got["parsed_sub_type"])
	}
}

func TestNormalizeSportTaxonomyImportRowRejectsUnknownActivitySport(t *testing.T) {
	row := map[string]any{"sport_type": "not_a_sport", "sub_type": ""}
	_, err := normalizeSportTaxonomyImportRow("activities", row, nil)
	if err == nil || !strings.Contains(err.Error(), `unknown sport_type "not_a_sport"`) {
		t.Fatalf("error: got %v, want unknown sport_type", err)
	}
}

func TestNormalizeSportTaxonomyImportRowOtherSportTables(t *testing.T) {
	tests := []struct {
		name  string
		table string
		row   map[string]any
		check func(t *testing.T, got map[string]any)
	}{
		{
			name:  "planned route derives subtype",
			table: "planned_routes",
			row:   map[string]any{"sport_type": "mountain_biking"},
			check: func(t *testing.T, got map[string]any) {
				if got["sport_type"] != "cycling" || got["sub_type"] != "mountain" {
					t.Fatalf("planned route tuple: %#v", got)
				}
			},
		},
		{
			name:  "recording group preserves tuple and modifiers",
			table: "recording_groups",
			row:   map[string]any{"sport_type": "e_mountain_biking", "sub_type": "", "is_electric": false},
			check: func(t *testing.T, got map[string]any) {
				if got["sport_type"] != "cycling" || got["sub_type"] != "mountain" {
					t.Fatalf("recording group tuple: got (%#v, %#v), want (cycling, mountain)", got["sport_type"], got["sub_type"])
				}
				if got["is_electric"] != true {
					t.Fatalf("recording group is_electric: got %#v, want true", got["is_electric"])
				}
			},
		},
		{
			name:  "segments normalize allowed predicates",
			table: "segments",
			row:   map[string]any{"allowed_predicates": []any{map[string]any{"sport": "mountain_biking"}}},
			check: func(t *testing.T, got map[string]any) {
				preds := got["allowed_predicates"].([]any)
				pred := preds[0].(map[string]any)
				if pred["sport"] != "cycling" || pred["sub_type"] != "mountain" {
					t.Fatalf("allowed_predicates: %#v", got["allowed_predicates"])
				}
			},
		},
		{
			name:  "automation rule rewrites action",
			table: "automation_rules",
			row: map[string]any{
				"actions": []any{map[string]any{"action": "set_sport_type", "params": map[string]any{"sport_type": "mountain_biking"}}},
			},
			check: func(t *testing.T, got map[string]any) {
				actions := got["actions"].([]any)
				params := actions[0].(map[string]any)["params"].(map[string]any)
				if params["sport_type"] != "cycling" || params["sub_type"] != "mountain" {
					t.Fatalf("automation action not rewritten: %#v", actions)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSportTaxonomyImportRow(tt.table, tt.row, nil)
			if err != nil {
				t.Fatalf("normalizeSportTaxonomyImportRow: %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestNormalizeSportTaxonomyImportRowRoutingProfiles(t *testing.T) {
	row := map[string]any{"sport_types": []any{"e_mountain_biking"}}
	targetCols := []targetColumn{
		{Name: "sport_types"},
		{Name: "predicates"},
		{Name: "modifier_filters"},
	}

	got, err := normalizeSportTaxonomyImportRow("routing_profiles", row, targetCols)
	if err != nil {
		t.Fatalf("normalizeSportTaxonomyImportRow: %v", err)
	}

	predicates, ok := got["predicates"].([]any)
	if !ok || len(predicates) != 1 {
		t.Fatalf("predicates: %#v", got["predicates"])
	}
	predicate := predicates[0].(map[string]any)
	if predicate["sport"] != "cycling" || predicate["sub_type"] != "mountain" {
		t.Fatalf("predicate: %#v", predicate)
	}
	if _, exists := predicate["is_electric"]; exists {
		t.Fatalf("predicate must not contain modifier fields: %#v", predicate)
	}

	if filters, ok := got["modifier_filters"].(map[string]any); !ok || filters["is_electric"] != true {
		t.Fatalf("modifier_filters: %#v", got["modifier_filters"])
	}
}

func TestNormalizeSportTaxonomyImportRowRejectsUnsupportedAutomationRule(t *testing.T) {
	row := map[string]any{
		"conditions": []any{map[string]any{"field": "sport_type", "op": "neq", "value": "mountain_biking"}},
	}
	_, err := normalizeSportTaxonomyImportRow("automation_rules", row, nil)
	if err == nil || !strings.Contains(err.Error(), "conditions") || !strings.Contains(err.Error(), "unsupported sport_type operator") {
		t.Fatalf("error: got %v, want clear automation rule field error", err)
	}
}

func TestNormalizeSportTaxonomyImportRowRejectsInvalidSegmentPredicate(t *testing.T) {
	row := map[string]any{
		"allowed_predicates": []any{map[string]any{"sport": "cycling", "sub_type": "trail"}},
	}
	_, err := normalizeSportTaxonomyImportRow("segments", row, nil)
	if err == nil || !strings.Contains(err.Error(), "allowed_predicates") {
		t.Fatalf("error: got %v, want allowed_predicates error", err)
	}
}

func TestNormalizeSportTaxonomyImportRowMigratesLegacySegmentArrays(t *testing.T) {
	row := map[string]any{
		"allowed_sport_types": []any{"mountain_biking", "trail_running"},
	}

	got, err := normalizeSportTaxonomyImportRow("segments", row, nil)
	if err != nil {
		t.Fatalf("normalizeSportTaxonomyImportRow: %v", err)
	}
	preds, ok := got["allowed_predicates"].([]any)
	if !ok || len(preds) != 2 {
		t.Fatalf("allowed_predicates = %#v, want two predicates", got["allowed_predicates"])
	}
	want := map[string]bool{
		"cycling:mountain": true,
		"running:trail":    true,
	}
	for _, raw := range preds {
		pred := raw.(map[string]any)
		key := pred["sport"].(string) + ":" + pred["sub_type"].(string)
		if !want[key] {
			t.Fatalf("unexpected predicate %#v from legacy arrays", pred)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing predicates from legacy arrays: %#v", want)
	}
}

func TestNormalizeSportTaxonomyImportRowRejectsAmbiguousLegacySegmentSubTypes(t *testing.T) {
	row := map[string]any{
		"allowed_sport_types": []any{"cycling", "running"},
		"allowed_sub_types":   []any{"mountain", "trail"},
	}

	_, err := normalizeSportTaxonomyImportRow("segments", row, nil)
	if err == nil || !strings.Contains(err.Error(), "allowed_sub_types") {
		t.Fatalf("error: got %v, want ambiguous allowed_sub_types error", err)
	}
}

func TestNormalizeSportTaxonomyImportRowAcceptsJSONRawMessages(t *testing.T) {
	raw := json.RawMessage(`[{"action":"set_sport_type","params":{"sport_type":"mountain_biking"}}]`)
	row := map[string]any{"actions": raw}

	got, err := normalizeSportTaxonomyImportRow("automation_rules", row, nil)
	if err != nil {
		t.Fatalf("normalizeSportTaxonomyImportRow: %v", err)
	}
	if _, ok := got["actions"].(json.RawMessage); !ok {
		t.Fatalf("actions should stay json.RawMessage-compatible, got %T", got["actions"])
	}
}
