package dataimport_test

import (
	"slices"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/auth"
	"github.com/skadia-app/skadia/services/api/internal/dataimport"
)

func TestGDPRExportedTablesAreImportableOrExplicitlySkipped(t *testing.T) {
	covered := dataimport.ImportCoverageTableNamesForGDPRGuard()
	skipped := dataimport.ImportSkippedTableRationalesForGDPRGuard()

	var missing []string
	for _, table := range auth.ExportedTables {
		if !covered[table] && skipped[table] == "" {
			missing = append(missing, table)
		}
	}
	slices.Sort(missing)

	if len(missing) > 0 {
		t.Fatalf("GDPR-exported tables missing dataimport coverage: %v", missing)
	}
}
