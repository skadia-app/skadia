// services/api/internal/shadowfs/pii_sync_test.go
package shadowfs_test

// CI guard: ensures the PII blacklist in settings/keys.go (local copy kept to
// avoid an import cycle) stays in sync with the canonical list in shadowfs/pii.go.
//
// Background: settings ↛ shadowfs is forbidden (import cycle), so settings/keys.go
// maintains its own copy of the PII variable list. This test fails if the two lists
// diverge — forcing the developer to update both locations together.
//
// See also: R2 MED #4 (Runde-2-Backend-Logic-Fixes).

import (
	"sort"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

func TestPIIBlacklistInSync(t *testing.T) {
	settingsList := settings.PIIBlacklistForTest()
	shadowfsList := shadowfs.PIIBlacklistForTest()

	// Sort both for canonical comparison (order differences are irrelevant for
	// correctness but would cause false failures without sorting).
	sCopy := append([]string(nil), settingsList...)
	dCopy := append([]string(nil), shadowfsList...)
	sort.Strings(sCopy)
	sort.Strings(dCopy)

	if len(sCopy) != len(dCopy) {
		t.Errorf("PII blacklist length mismatch: settings has %d entries, shadowfs has %d entries",
			len(sCopy), len(dCopy))
	}

	// Build lookup maps for diff reporting.
	inSettings := make(map[string]bool, len(sCopy))
	for _, v := range sCopy {
		inSettings[v] = true
	}
	inShadowfs := make(map[string]bool, len(dCopy))
	for _, v := range dCopy {
		inShadowfs[v] = true
	}

	for _, v := range sCopy {
		if !inShadowfs[v] {
			t.Errorf("settings/keys.go has PII entry %q but shadowfs/pii.go does not — update both", v)
		}
	}
	for _, v := range dCopy {
		if !inSettings[v] {
			t.Errorf("shadowfs/pii.go has PII entry %q but settings/keys.go does not — update both", v)
		}
	}
}
