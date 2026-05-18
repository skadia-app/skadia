package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestMigration_ActivityVisitedCountries_AddsPointAndVisitedCountryColumns(t *testing.T) {
	upBytes, err := os.ReadFile("000373_activity_visited_countries.up.sql")
	if err != nil {
		t.Fatalf("read up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000373_activity_visited_countries.down.sql")
	if err != nil {
		t.Fatalf("read down.sql: %v", err)
	}
	upSQL := string(upBytes)
	downSQL := string(downBytes)

	for _, needle := range []string{
		"ADD COLUMN IF NOT EXISTS end_country TEXT",
		"ADD COLUMN IF NOT EXISTS end_country_code TEXT",
		"ADD COLUMN IF NOT EXISTS location_country TEXT",
		"ADD COLUMN IF NOT EXISTS location_country_code TEXT",
		"ADD COLUMN IF NOT EXISTS visited_country_codes TEXT[] NOT NULL DEFAULT '{}'",
		"CREATE INDEX IF NOT EXISTS activities_visited_country_codes_gin_idx",
		"USING GIN (visited_country_codes)",
	} {
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("up.sql missing %q", needle)
		}
	}

	for _, needle := range []string{
		"DROP INDEX IF EXISTS activities_visited_country_codes_gin_idx",
		"DROP COLUMN IF EXISTS visited_country_codes",
		"DROP COLUMN IF EXISTS location_country_code",
		"DROP COLUMN IF EXISTS location_country",
		"DROP COLUMN IF EXISTS end_country_code",
		"DROP COLUMN IF EXISTS end_country",
	} {
		if !strings.Contains(downSQL, needle) {
			t.Fatalf("down.sql missing %q", needle)
		}
	}
}
