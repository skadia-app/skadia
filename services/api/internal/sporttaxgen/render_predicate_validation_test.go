package sporttaxgen

import (
	"strings"
	"testing"
)

func TestRenderPredicateValidationSQLDeterministic(t *testing.T) {
	taxonomy := Taxonomy{
		Sports: []Sport{
			{SportType: "running", SubTypes: []SubType{{SubType: "trail"}}},
			{SportType: "cycling", SubTypes: []SubType{{SubType: "mountain"}, {SubType: "road"}}},
		},
	}

	got, err := RenderPredicateValidationSQL(taxonomy, predicateValidationSegmentProjectionFixture())
	if err != nil {
		t.Fatalf("RenderPredicateValidationSQL() error = %v", err)
	}
	src := string(got)
	for _, needle := range []string{
		"Validates persisted sport.Predicate arrays only: sport/sub_type keys, no rule modifiers.",
		"Empty arrays are valid here for explicit no-match segment predicates; table constraints add non-empty checks where needed.",
		"CREATE OR REPLACE FUNCTION fn_validate_predicates(p JSONB)",
		"LANGUAGE sql",
		"IMMUTABLE",
		"SET search_path = public",
		"('cycling'), ('running')",
		"('cycling','mountain'), ('cycling','road'), ('running','trail')",
		"CREATE OR REPLACE FUNCTION fn_validate_segment_predicates_for_type(seg_type TEXT, p JSONB)",
		"('standard','cycling',''), ('standard','cycling','mountain'), ('standard','running','')",
		"pred_keys.key NOT IN ('sport', 'sub_type')",
		"pred->>'sub_type' = ''",
	} {
		if !strings.Contains(src, needle) {
			t.Fatalf("generated SQL missing %q:\n%s", needle, src)
		}
	}
	if !strings.HasSuffix(src, "\n") {
		t.Fatal("generated SQL must end with one trailing newline")
	}
	if strings.HasSuffix(src, "\n\n") {
		t.Fatal("generated SQL must end with exactly one trailing newline")
	}
}

func TestRenderPredicateValidationSQLEscapesValues(t *testing.T) {
	taxonomy := Taxonomy{
		Sports: []Sport{
			{SportType: "odd'sport", SubTypes: []SubType{{SubType: "sub'value"}}},
		},
	}
	got, err := RenderPredicateValidationSQL(taxonomy, ExpandedSegmentProjection{
		SegmentTypes: []ExpandedSegmentProjectionSegmentType{{Key: "standard", ActiveDomains: []string{"real_world_gps"}}},
		Entries: []ExpandedSegmentProjectionEntry{
			{SegmentType: "standard", Domain: "real_world_gps", SportType: "odd'sport", SubType: "sub'value"},
		},
	})
	if err != nil {
		t.Fatalf("RenderPredicateValidationSQL() error = %v", err)
	}
	src := string(got)
	if !strings.Contains(src, "('odd''sport')") ||
		!strings.Contains(src, "('odd''sport','sub''value')") ||
		!strings.Contains(src, "('standard','odd''sport','sub''value')") {
		t.Fatalf("generated SQL did not SQL-escape values:\n%s", src)
	}
}

func predicateValidationSegmentProjectionFixture() ExpandedSegmentProjection {
	return ExpandedSegmentProjection{
		SegmentTypes: []ExpandedSegmentProjectionSegmentType{
			{Key: "standard", ActiveDomains: []string{"real_world_gps"}},
			{Key: "wintersport", ActiveDomains: []string{"winter"}},
			{Key: "lift"},
		},
		Entries: []ExpandedSegmentProjectionEntry{
			{SegmentType: "standard", Domain: "real_world_gps", SportType: "running"},
			{SegmentType: "standard", Domain: "real_world_gps", SportType: "cycling"},
			{SegmentType: "standard", Domain: "real_world_gps", SportType: "cycling", SubType: "mountain"},
			{SegmentType: "standard", Domain: "inactive", SportType: "cycling", SubType: "road"},
			{SegmentType: "wintersport", Domain: "winter", SportType: "running"},
		},
	}
}
