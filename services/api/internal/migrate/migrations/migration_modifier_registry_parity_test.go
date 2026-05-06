package migrations_test

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestMigrationSQLModifierContractsMatchGeneratedRegistry(t *testing.T) {
	t.Run("routing profile modifier filters", func(t *testing.T) {
		upSQL, _, _ := readMigration000339SQL(t)
		block := sqlSectionBetween(t, upSQL,
			"CREATE OR REPLACE FUNCTION fn_validate_modifier_filters",
			"CREATE OR REPLACE FUNCTION fp_modifier_filters_from_sport_types",
		)
		assertSameStringSet(t, quotedModifierKeys(block), sport.SportModifierKeysWithCapability("modifier_filter"))
	})

	t.Run("gear default predicates", func(t *testing.T) {
		upSQL, _ := readMigration000349GearDefaultPredicatesSQL(t)
		block := sqlSectionBetween(t, upSQL,
			"CREATE OR REPLACE FUNCTION fn_validate_gear_default_predicates",
			"ALTER TABLE gear",
		)
		assertSameStringSet(t, quotedModifierKeys(block), sport.SportModifierKeysWithCapability("gear_default"))
	})

	t.Run("heatmap filters", func(t *testing.T) {
		upSQL := readMigrationSQLFile(t, "000350_heatmap_modifier_filters.up.sql")
		block := sqlSectionBetween(t, upSQL,
			"CREATE OR REPLACE FUNCTION skadia_heatmap_modifier_filter_matches",
			"DO $$",
		)
		assertSameStringSet(t, queryParamModifierKeys(block), sport.SportModifierKeysWithCapability("heatmap_filter"))
	})
}

func TestMigration000348CyclingElectricTagSyncMatchesGeneratedRegistry(t *testing.T) {
	upSQL, _ := readMigration000348CyclingElectricTagSyncSQL(t)
	tagSyncSQL := upSQL + "\n" + readMigrationSQLFile(t, "000221_virtual_system_tag.up.sql")
	for _, key := range sport.SportModifierKeys() {
		modifier, ok := sport.SportModifierByKey(key)
		if !ok || modifier.TagSync == nil {
			continue
		}
		if !strings.Contains(tagSyncSQL, "'"+modifier.TagSync.SystemTag+"'") {
			t.Fatalf("tag-sync SQL missing system tag %q for %s", modifier.TagSync.SystemTag, key)
		}
		if modifier.TagSync.OnlySportType != "" {
			for _, needle := range []string{
				"NEW.sport_type = '" + modifier.TagSync.OnlySportType + "'",
				"OLD.sport_type = '" + modifier.TagSync.OnlySportType + "'",
				"a.sport_type = '" + modifier.TagSync.OnlySportType + "'",
			} {
				if !strings.Contains(upSQL, needle) {
					t.Fatalf("000348 up migration missing %s guard %q", key, needle)
				}
			}
		}
	}
	if got := sport.SportModifierLabelDE("is_electric"); got == "E-Bike" {
		t.Fatal("is_electric label must stay generic; E-Bike is only the cycling tag-sync system tag")
	}
}

func readMigrationSQLFile(t *testing.T, path string) string {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(bytes)
}

func sqlSectionBetween(t *testing.T, sql, startMarker, endMarker string) string {
	t.Helper()
	start := strings.Index(sql, startMarker)
	if start < 0 {
		t.Fatalf("SQL missing start marker %q", startMarker)
	}
	rest := sql[start:]
	end := strings.Index(rest, endMarker)
	if end < 0 {
		t.Fatalf("SQL section %q missing end marker %q", startMarker, endMarker)
	}
	return rest[:end]
}

func quotedModifierKeys(sql string) []string {
	return uniqueRegexMatches(sql, regexp.MustCompile(`'((?:is)_[a-z_]+)'`))
}

func queryParamModifierKeys(sql string) []string {
	return uniqueRegexMatches(sql, regexp.MustCompile(`query_params->>'((?:is)_[a-z_]+)'`))
}

func uniqueRegexMatches(value string, pattern *regexp.Regexp) []string {
	seen := map[string]struct{}{}
	for _, match := range pattern.FindAllStringSubmatch(value, -1) {
		seen[match[1]] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func assertSameStringSet(t *testing.T, got, want []string) {
	t.Helper()
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %#v, want %#v", got, want)
	}
}
