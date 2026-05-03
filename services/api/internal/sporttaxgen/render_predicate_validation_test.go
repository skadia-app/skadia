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

	got, err := RenderPredicateValidationSQL(taxonomy)
	if err != nil {
		t.Fatalf("RenderPredicateValidationSQL() error = %v", err)
	}
	src := string(got)
	for _, needle := range []string{
		"CREATE OR REPLACE FUNCTION fn_validate_predicates(p JSONB)",
		"LANGUAGE sql",
		"IMMUTABLE",
		"('cycling'), ('running')",
		"('cycling','mountain'), ('cycling','road'), ('running','trail')",
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
	got, err := RenderPredicateValidationSQL(taxonomy)
	if err != nil {
		t.Fatalf("RenderPredicateValidationSQL() error = %v", err)
	}
	src := string(got)
	if !strings.Contains(src, "('odd''sport')") || !strings.Contains(src, "('odd''sport','sub''value')") {
		t.Fatalf("generated SQL did not SQL-escape values:\n%s", src)
	}
}
