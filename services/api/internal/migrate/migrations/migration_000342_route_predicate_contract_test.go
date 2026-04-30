package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestMigration000342RoutePredicateContractCleanup(t *testing.T) {
	upBytes, err := os.ReadFile("000342_route_predicate_contract_cleanup.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	downBytes, err := os.ReadFile("000342_route_predicate_contract_cleanup.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	up := string(upBytes)
	down := string(downBytes)

	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS is_electric BOOLEAN NOT NULL DEFAULT FALSE",
		"ADD COLUMN IF NOT EXISTS is_indoor BOOLEAN NOT NULL DEFAULT FALSE",
		"ADD COLUMN IF NOT EXISTS is_virtual BOOLEAN NOT NULL DEFAULT FALSE",
		"ADD COLUMN IF NOT EXISTS is_commute BOOLEAN NOT NULL DEFAULT FALSE",
		"DROP COLUMN IF EXISTS allowed_sport_types",
		"DROP COLUMN IF EXISTS allowed_sub_types",
	} {
		if !strings.Contains(up, want) {
			t.Fatalf("up migration missing %q", want)
		}
	}
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS allowed_sport_types TEXT[]",
		"ADD COLUMN IF NOT EXISTS allowed_sub_types TEXT[]",
		"DROP COLUMN IF EXISTS is_electric",
		"DROP COLUMN IF EXISTS is_commute",
	} {
		if !strings.Contains(down, want) {
			t.Fatalf("down migration missing %q", want)
		}
	}
}
