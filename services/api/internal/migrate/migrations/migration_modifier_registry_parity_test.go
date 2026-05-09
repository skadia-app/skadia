package migrations_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// systemModifierTagSyncSQLCoverageEntry pins the four facts a tag_sync
// modifier needs in order to be safely covered by the SQL trigger family
// and the resync function. New tag_sync modifiers must add an entry here OR
// list themselves in systemModifierTagSyncSQLExclusions with a tracking
// reason. The new SQL resync/trigger integration tests reach into this map
// for their assertions, so it is the single source of truth for which
// modifiers the SQL layer must handle.
type systemModifierTagSyncSQLCoverageEntry struct {
	ModifierKey     string
	ActivityColumn  string
	SystemTagName   string
	OnlySportType   string
	InboundFlagSync bool
}

// systemModifierTagSyncSQLCoverage describes every registry-backed
// tag_sync modifier the SQL trigger family + resync function handle.
// Keep this list in lockstep with sport.SportModifierKeys() and
// modifiers.yaml `tag_sync:` blocks.
var systemModifierTagSyncSQLCoverage = map[string]systemModifierTagSyncSQLCoverageEntry{
	"is_indoor": {
		ModifierKey:     "is_indoor",
		ActivityColumn:  "is_indoor",
		SystemTagName:   "Indoor",
		InboundFlagSync: true,
	},
	"is_commute": {
		ModifierKey:     "is_commute",
		ActivityColumn:  "is_commute",
		SystemTagName:   "Pendeln",
		InboundFlagSync: true,
	},
	"is_virtual": {
		ModifierKey:     "is_virtual",
		ActivityColumn:  "is_virtual",
		SystemTagName:   "virtuell",
		InboundFlagSync: false, // virtual is set by trusted callers; no tags-array→flag derivation
	},
	"is_electric": {
		ModifierKey:     "is_electric",
		ActivityColumn:  "is_electric",
		SystemTagName:   "E-Bike",
		OnlySportType:   "cycling",
		InboundFlagSync: true,
	},
}

// systemModifierTagSyncSQLExclusions is the escape valve: a future modifier
// with a `tag_sync:` block that intentionally has NO SQL trigger/resync
// handling must list itself here together with the reason and the followup
// plan that will eventually add coverage. The map is empty in the initial
// implementation; populate sparingly and review in the next plan.
var systemModifierTagSyncSQLExclusions = map[string]string{
	// "future_modifier": "reason and tracking plan",
}

func TestMigrationSQLModifierContractsMatchGeneratedRegistry(t *testing.T) {
	t.Run("routing profile modifier filters", func(t *testing.T) {
		upSQL := currentUpMigrationSQLForFunction(t, "fn_validate_modifier_filters")
		block := sqlSectionBetween(t, upSQL,
			"CREATE OR REPLACE FUNCTION fn_validate_modifier_filters",
			"CREATE OR REPLACE FUNCTION fp_modifier_filters_from_sport_types",
		)
		assertSameStringSet(t, modifierKeysAllowedBySQLValidator(block), sport.SportModifierKeysWithCapability("modifier_filter"))
	})

	t.Run("gear default predicates", func(t *testing.T) {
		upSQL := currentUpMigrationSQLForFunction(t, "fn_validate_gear_default_predicates")
		block := sqlSectionBetween(t, upSQL,
			"CREATE OR REPLACE FUNCTION fn_validate_gear_default_predicates",
			"ALTER TABLE gear",
		)
		assertSameStringSet(t, modifierKeysAllowedBySQLValidator(block), sport.SportModifierKeysWithCapability("gear_default"))
	})

	t.Run("heatmap filters", func(t *testing.T) {
		upSQL := currentUpMigrationSQLForFunction(t, "skadia_heatmap_modifier_filter_matches")
		block := sqlSectionBetween(t, upSQL,
			"CREATE OR REPLACE FUNCTION skadia_heatmap_modifier_filter_matches",
			"DO $$",
		)
		assertSameStringSet(t, queryParamModifierKeys(block), sport.SportModifierKeysWithCapability("heatmap_filter"))
	})
}

func TestMigration000350CyclingElectricTagSyncMatchesGeneratedRegistry(t *testing.T) {
	upSQL, _ := readMigration000350CyclingElectricTagSyncSQL(t)
	tagSyncSQL := allUpMigrationSQLContainingAnySystemTag(t, sport.SportModifierKeys())
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
					t.Fatalf("000350 up migration missing %s guard %q", key, needle)
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

func currentUpMigrationSQLForFunction(t *testing.T, functionName string) string {
	t.Helper()
	matches, err := filepath.Glob("*.up.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	sort.Strings(matches)
	for i := len(matches) - 1; i >= 0; i-- {
		sql := readMigrationSQLFile(t, matches[i])
		if !strings.Contains(sql, functionName) {
			continue
		}
		createMarker := "CREATE OR REPLACE FUNCTION " + functionName
		if strings.Contains(sql, createMarker) {
			return sql
		}
		if strings.Contains(sql, "DROP FUNCTION") {
			t.Fatalf("latest migration mentioning %s is %s and drops it without a current replacement", functionName, matches[i])
		}
	}
	t.Fatalf("no up migration contains current definition for function %q", functionName)
	return ""
}

func allUpMigrationSQLContainingAnySystemTag(t *testing.T, modifierKeys []string) string {
	t.Helper()
	tagNames := map[string]struct{}{}
	for _, key := range modifierKeys {
		modifier, ok := sport.SportModifierByKey(key)
		if ok && modifier.TagSync != nil {
			tagNames[modifier.TagSync.SystemTag] = struct{}{}
		}
	}
	matches, err := filepath.Glob("*.up.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	sort.Strings(matches)
	var out strings.Builder
	for _, match := range matches {
		sql := readMigrationSQLFile(t, match)
		for tagName := range tagNames {
			if strings.Contains(sql, "'"+tagName+"'") {
				out.WriteString(sql)
				out.WriteString("\n")
				break
			}
		}
	}
	return out.String()
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

func modifierKeysAllowedBySQLValidator(sql string) []string {
	matches := regexp.MustCompile(`(?:kv\.)?key\s+NOT\s+IN\s*\(([^)]*)\)`).FindAllStringSubmatch(sql, -1)
	if len(matches) == 0 {
		return nil
	}
	return uniqueRegexMatches(matches[len(matches)-1][1], regexp.MustCompile(`'([a-z][a-z0-9_]*)'`))
}

func queryParamModifierKeys(sql string) []string {
	return uniqueRegexMatches(sql, regexp.MustCompile(`query_params->>'([a-z][a-z0-9_]*)'`))
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

// TestSystemModifierTagSyncSQLCoverageMatchesGeneratedRegistry is the parity
// guard the plan calls out as Task 2. Every registry-backed `tag_sync`
// modifier in the generated registry must either appear in
// systemModifierTagSyncSQLCoverage with current SQL handling or appear in
// systemModifierTagSyncSQLExclusions with a tracking reason. A future fifth
// modifier that adds a tag_sync block but no SQL trigger/resync coverage
// must fail this test loudly, naming its key.
func TestSystemModifierTagSyncSQLCoverageMatchesGeneratedRegistry(t *testing.T) {
	registryKeys := map[string]sport.SportModifierTagSync{}
	for _, key := range sport.SportModifierKeys() {
		modifier, ok := sport.SportModifierByKey(key)
		if !ok || modifier.TagSync == nil {
			continue
		}
		registryKeys[key] = *modifier.TagSync
	}

	// Every registry key must either be covered or explicitly excluded.
	for key := range registryKeys {
		_, covered := systemModifierTagSyncSQLCoverage[key]
		_, excluded := systemModifierTagSyncSQLExclusions[key]
		if !covered && !excluded {
			t.Errorf("modifier key %q has tag_sync in the generated registry but no SQL coverage; "+
				"add it to systemModifierTagSyncSQLCoverage or list it in "+
				"systemModifierTagSyncSQLExclusions with a tracking plan", key)
		}
		if covered && excluded {
			t.Errorf("modifier key %q is both covered and excluded; pick one", key)
		}
	}
	// Coverage entries must point at modifier keys that still exist.
	for key := range systemModifierTagSyncSQLCoverage {
		if _, ok := registryKeys[key]; !ok {
			t.Errorf("systemModifierTagSyncSQLCoverage references unknown modifier key %q; "+
				"the generated registry no longer has a tag_sync block for it", key)
		}
	}
	// Exclusion entries must refer to modifier keys that exist AND have
	// tag_sync blocks; otherwise the exclusion is dead code.
	for key, reason := range systemModifierTagSyncSQLExclusions {
		if reason == "" {
			t.Errorf("systemModifierTagSyncSQLExclusions[%q] has no reason; tracking text is required", key)
		}
		if _, ok := registryKeys[key]; !ok {
			t.Errorf("systemModifierTagSyncSQLExclusions[%q] does not exist in the generated registry", key)
		}
	}

	// Coverage facts must match the registry exactly. Catches drift in
	// system tag name, only-sport guard, etc.
	for key, cov := range systemModifierTagSyncSQLCoverage {
		ts, ok := registryKeys[key]
		if !ok {
			continue
		}
		if cov.SystemTagName != ts.SystemTag {
			t.Errorf("modifier %q SystemTagName = %q, registry TagSync.SystemTag = %q",
				key, cov.SystemTagName, ts.SystemTag)
		}
		if cov.OnlySportType != ts.OnlySportType {
			t.Errorf("modifier %q OnlySportType = %q, registry TagSync.OnlySportType = %q",
				key, cov.OnlySportType, ts.OnlySportType)
		}
		// ActivityColumn must match the modifier's ActivityColumn (the
		// Postgres column the SQL triggers branch on).
		mod, _ := sport.SportModifierByKey(key)
		if cov.ActivityColumn != mod.ActivityColumn {
			t.Errorf("modifier %q ActivityColumn = %q, registry ActivityColumn = %q",
				key, cov.ActivityColumn, mod.ActivityColumn)
		}
	}
}

// TestSystemModifierTagSyncSQLCoverageReferencedByResyncMigration cross-checks
// that the actual resync migration body references each covered modifier's
// activity column and system tag name. Without this guard a developer could
// add a coverage entry without wiring it into the SQL.
func TestSystemModifierTagSyncSQLCoverageReferencedByResyncMigration(t *testing.T) {
	upBytes, err := os.ReadFile("000353_system_modifier_tag_sync_hardening.up.sql")
	if err != nil {
		// Glob fallback covers branches still on the 000999 placeholder
		// before scripts/finalize-migration.sh runs.
		matches, _ := filepath.Glob("*_system_modifier_tag_sync_hardening.up.sql")
		if len(matches) == 0 {
			t.Skip("system_modifier_tag_sync_hardening migration not present yet")
			return
		}
		sort.Strings(matches)
		upBytes, err = os.ReadFile(matches[len(matches)-1])
		if err != nil {
			t.Fatalf("read hardening up.sql: %v", err)
		}
	}
	upSQL := string(upBytes)
	for key, cov := range systemModifierTagSyncSQLCoverage {
		if _, excluded := systemModifierTagSyncSQLExclusions[key]; excluded {
			continue
		}
		// System tag name must appear in the resync function body.
		if !strings.Contains(upSQL, "'"+cov.SystemTagName+"'") {
			t.Errorf("hardening up.sql does not reference system tag name %q for modifier %s",
				cov.SystemTagName, key)
		}
		// Activity column must appear in the resync function body so
		// the function can branch on the modifier flag.
		if !strings.Contains(upSQL, fmt.Sprintf("a.%s", cov.ActivityColumn)) &&
			!strings.Contains(upSQL, fmt.Sprintf("NEW.%s", cov.ActivityColumn)) {
			t.Errorf("hardening up.sql does not reference activity column %s for modifier %s",
				cov.ActivityColumn, key)
		}
		// Sport guard, if any, must show up next to the activity column.
		if cov.OnlySportType != "" && !strings.Contains(upSQL, "sport_type = '"+cov.OnlySportType+"'") {
			t.Errorf("hardening up.sql missing sport guard sport_type = %q for modifier %s",
				cov.OnlySportType, key)
		}
	}
}
