package database

import (
	"reflect"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestBuildFilterSQLCompositeSportTuples(t *testing.T) {
	mountain := "mountain"
	trail := "trail"
	clause, args := buildFilterSQL(model.ActivityFilters{
		SportPredicates: []sport.Predicate{
			{Sport: "cycling", SubType: &mountain},
			{Sport: "running", SubType: &trail},
		},
	}, 1)

	if strings.Contains(clause, "sport_type = ANY") || strings.Contains(clause, "sub_type = ANY") {
		t.Fatalf("expected tuple OR predicate, got cross-product clause %q", clause)
	}
	if !strings.Contains(clause, "sport_type = $2 AND COALESCE(sub_type, '') = $3") {
		t.Fatalf("missing cycling mountain tuple in %q", clause)
	}
	if !strings.Contains(clause, "sport_type = $4 AND COALESCE(sub_type, '') = $5") {
		t.Fatalf("missing running trail tuple in %q", clause)
	}
	if len(args) != 4 || args[0] != "cycling" || args[1] != "mountain" || args[2] != "running" || args[3] != "trail" {
		t.Fatalf("args = %#v, want cycling/mountain and running/trail", args)
	}
}

func TestBuildFilterSQLIncludesCommuteModifier(t *testing.T) {
	isCommute := true
	clause, args := buildFilterSQL(model.ActivityFilters{
		SportTypes: []string{"cycling"},
		IsCommute:  &isCommute,
	}, 1)

	if !strings.Contains(clause, "sport_type = ANY($2)") {
		t.Fatalf("missing sport_type clause in %q", clause)
	}
	if !strings.Contains(clause, "is_commute = $3") {
		t.Fatalf("missing is_commute clause in %q", clause)
	}
	if len(args) != 2 || args[1] != true {
		t.Fatalf("args = %#v, want second arg true", args)
	}
}

func TestBuildFilterSQLIncludesPendingLiftReview(t *testing.T) {
	clause, args := buildFilterSQL(model.ActivityFilters{
		LiftReview: "pending",
	}, 1)

	if !strings.Contains(clause, "FROM activity_lift_candidates alc") {
		t.Fatalf("missing lift candidate EXISTS in %q", clause)
	}
	if strings.Contains(clause, "alc.source = 'heuristic'") {
		t.Fatalf("pending review filter must include OSM candidates too, got %q", clause)
	}
	if !strings.Contains(clause, "NOT alc.confirmed") || !strings.Contains(clause, "NOT alc.rejected_by_user") {
		t.Fatalf("missing open candidate predicates in %q", clause)
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}

func TestBuildFilterSQLCountryCodesUsesVisitedCountries(t *testing.T) {
	clause, args := buildFilterSQL(model.ActivityFilters{CountryCodes: []string{"CH", "IT"}}, 1)
	if !strings.Contains(clause, "visited_country_codes && $2::text[]") {
		t.Fatalf("country filter clause = %s", clause)
	}
	if want := []any{[]string{"CH", "IT"}}; !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}
