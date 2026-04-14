package osm

import (
	"testing"
)

// TestCategoryToRestoreTablesCoverage verifies that every import category
// (from categoryEnvVars, which is the authoritative list of known categories)
// either has an entry in categoryToRestoreTables OR is explicitly listed in
// restoreExemptCategories.
//
// Without this test a newly added import category without a schema-restore
// mapping would silently lose its migration columns on the next osm2pgsql
// --create run (loud warning in Pipeline.Run is the runtime safety net; this
// test is the compile-time safety net).
func TestCategoryToRestoreTablesCoverage(t *testing.T) {
	for cat := range categoryEnvVars {
		_, hasMapping := categoryToRestoreTables[cat]
		exempt := restoreExemptCategories[cat]
		if !hasMapping && !exempt {
			t.Errorf("category %q is registered in categoryEnvVars but has no entry in categoryToRestoreTables and is not in restoreExemptCategories — schema-restore will silently skip it", cat)
		}
	}
}

// TestRestoreExemptCategoriesNotInRestoreMap verifies that a category cannot
// be listed in both categoryToRestoreTables and restoreExemptCategories. The
// two sets are mutually exclusive by design: exempt means "we do not want a
// standard tableRestoreStmts mapping", mapped means "we apply the standard
// restore".
func TestRestoreExemptCategoriesNotInRestoreMap(t *testing.T) {
	for cat := range restoreExemptCategories {
		if _, ok := categoryToRestoreTables[cat]; ok {
			t.Errorf("category %q is in both categoryToRestoreTables and restoreExemptCategories — pick one", cat)
		}
	}
}

// TestCategoryToRestoreTablesPointsAtKnownTables verifies that every table
// referenced from categoryToRestoreTables has an entry in tableRestoreStmts
// (otherwise the restore loop in Pipeline.Run would iterate over an empty
// list and silently do nothing for that table).
func TestCategoryToRestoreTablesPointsAtKnownTables(t *testing.T) {
	for cat, tables := range categoryToRestoreTables {
		for _, table := range tables {
			if _, ok := tableRestoreStmts[table]; !ok {
				t.Errorf("category %q maps to table %q but tableRestoreStmts has no entry for that table", cat, table)
			}
		}
	}
}
