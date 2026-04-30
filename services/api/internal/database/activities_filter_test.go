package database

import (
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
