package lifts_test

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

func TestIsLiftRelevant(t *testing.T) {
	cases := []struct {
		name      string
		sport     string
		sub       string
		indoor    bool
		electric  bool
		virtual   bool
		wantTrue  bool
	}{
		// indoor → always false regardless of sport
		{"hiking indoor", "hiking", "", true, false, false, false},
		// virtual → always false
		{"cycling virtual", "cycling", "", false, false, true, false},
		// excluded cycling sub-types
		{"cycling indoor_cycling", "cycling", "indoor_cycling", false, false, false, false},
		{"cycling spin", "cycling", "spin", false, false, false, false},
		// cycling with allowed sub-type
		{"cycling mountain_bike", "cycling", "mountain_bike", false, false, false, true},
		// cycling with empty sub-type
		{"cycling no sub", "cycling", "", false, false, false, true},
		// relevant sports
		{"alpine_skiing", "alpine_skiing", "", false, false, false, true},
		{"hiking outdoor", "hiking", "", false, false, false, true},
		{"mountaineering", "mountaineering", "", false, false, false, true},
		// irrelevant sport
		{"running", "running", "", false, false, false, false},
		{"swimming", "swimming", "", false, false, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lifts.IsLiftRelevant(tc.sport, tc.sub, tc.indoor, tc.electric, tc.virtual)
			if got != tc.wantTrue {
				t.Errorf("IsLiftRelevant(%q, %q, indoor=%v, electric=%v, virtual=%v) = %v, want %v",
					tc.sport, tc.sub, tc.indoor, tc.electric, tc.virtual, got, tc.wantTrue)
			}
		})
	}
}
