package komoot

import "testing"

func TestKomootRouteSportTuplePreservesSubtypeAndElectricFlag(t *testing.T) {
	tests := []struct {
		name         string
		sport        string
		wantSport    string
		wantSub      string
		wantElectric bool
	}{
		{name: "racebike", sport: "racebike", wantSport: "cycling", wantSub: "road"},
		{name: "e racebike", sport: "e_racebike", wantSport: "cycling", wantSub: "road", wantElectric: true},
		{name: "mtb", sport: "mtb", wantSport: "cycling", wantSub: "mountain"},
		{name: "e mtb", sport: "e_mtb", wantSport: "cycling", wantSub: "mountain", wantElectric: true},
		{name: "intentional broad other", sport: "other", wantSport: "generic"},
		{name: "unknown fallback", sport: "definitely_unknown", wantSport: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSport, gotSub, gotElectric := komootRouteSportTuple(tt.sport)
			if gotSport != tt.wantSport || gotSub != tt.wantSub || gotElectric != tt.wantElectric {
				t.Fatalf("komootRouteSportTuple(%q) = %s/%s electric=%v, want %s/%s electric=%v",
					tt.sport, gotSport, gotSub, gotElectric, tt.wantSport, tt.wantSub, tt.wantElectric)
			}
		})
	}
}
