package komoot

import (
	"os"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestNormalizedKomootSportMappingUsesGeneratedAliases(t *testing.T) {
	src, err := os.ReadFile("types.go")
	if err != nil {
		t.Fatalf("read types.go: %v", err)
	}
	if strings.Contains(string(src), "var KomootSportMapping") {
		t.Fatal("KomootSportMapping should not be a handwritten runtime map")
	}
}

func TestNormalizedKomootSportMappingGolden(t *testing.T) {
	tests := []struct {
		input        string
		wantSport    string
		wantSubType  string
		wantElectric bool
		wantIndoor   bool
		wantVirtual  bool
	}{
		{input: "hike", wantSport: "hiking"},
		{input: "mountaineering", wantSport: "mountaineering"},
		{input: "jogging", wantSport: "running"},
		{input: "racebike", wantSport: "cycling", wantSubType: "road"},
		{input: "e_racebike", wantSport: "cycling", wantSubType: "road", wantElectric: true},
		{input: "touringbicycle", wantSport: "cycling"},
		{input: "e_touringbicycle", wantSport: "cycling", wantElectric: true},
		{input: "mtb", wantSport: "cycling", wantSubType: "mountain"},
		{input: "e_mtb", wantSport: "cycling", wantSubType: "mountain", wantElectric: true},
		{input: "mtb_easy", wantSport: "cycling", wantSubType: "gravel_cycling"},
		{input: "e_mtb_easy", wantSport: "cycling", wantSubType: "gravel_cycling", wantElectric: true},
		{input: "mtb_advanced", wantSport: "cycling", wantSubType: "mountain"},
		{input: "e_mtb_advanced", wantSport: "cycling", wantSubType: "mountain", wantElectric: true},
		{input: "citybike", wantSport: "cycling"},
		{input: "downhillbike", wantSport: "cycling", wantSubType: "downhill"},
		{input: "climbing", wantSport: "rock_climbing"},
		{input: "nordic", wantSport: "cross_country_skiing"},
		{input: "nordicwalking", wantSport: "walking", wantSubType: "speed_walking"},
		{input: "skaten", wantSport: "inline_skating"},
		{input: "skialpin", wantSport: "alpine_skiing", wantSubType: "resort"},
		{input: "skitour", wantSport: "backcountry_skiing"},
		{input: "sled", wantSport: "sledding"},
		{input: "snowboard", wantSport: "snowboarding"},
		{input: "snowshoe", wantSport: "snowshoeing"},
		{input: "unicycle", wantSport: "cycling", wantSubType: "unicycling"},
		{input: "other", wantSport: "generic"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := normalizedKomootSportMapping(tt.input)
			if !ok {
				t.Fatalf("normalizedKomootSportMapping(%q) not found", tt.input)
			}
			if got.SportType != tt.wantSport || got.SubType != tt.wantSubType || got.IsElectric != tt.wantElectric || got.IsIndoor != tt.wantIndoor || got.IsVirtual != tt.wantVirtual {
				t.Fatalf("normalizedKomootSportMapping(%q) = %#v, want %s/%s electric=%v indoor=%v virtual=%v",
					tt.input, got, tt.wantSport, tt.wantSubType, tt.wantElectric, tt.wantIndoor, tt.wantVirtual)
			}
		})
	}
}

func TestNormalizedKomootSportMappingUnknownStaysUnmapped(t *testing.T) {
	if got, ok := normalizedKomootSportMapping("definitely_unknown"); ok {
		t.Fatalf("unknown Komoot sport mapped to %#v", got)
	}
}

func TestApplyKomootSportMappingAdoptsSameParentSubtype(t *testing.T) {
	data := &model.ActivityData{SportType: "cycling"}
	mapping := SportMapping{SportType: "cycling", SubType: "mountain"}

	applyKomootSportMapping(data, mapping)

	if data.SportType != "cycling" || data.SubType != "mountain" {
		t.Fatalf("activity tuple = %s/%s, want cycling/mountain", data.SportType, data.SubType)
	}
}

func TestApplyKomootSportMappingKeepsSpecificFileSubtype(t *testing.T) {
	data := &model.ActivityData{SportType: "cycling", SubType: "gravel_cycling"}
	mapping := SportMapping{SportType: "cycling", SubType: "mountain"}

	applyKomootSportMapping(data, mapping)

	if data.SubType != "gravel_cycling" {
		t.Fatalf("SubType = %q, want existing gravel_cycling", data.SubType)
	}
}

func TestApplyKomootSportMappingPromotesModifierSignals(t *testing.T) {
	data := &model.ActivityData{SportType: "cycling"}
	mapping := SportMapping{SportType: "cycling", IsElectric: true, IsIndoor: true, IsVirtual: true}

	applyKomootSportMapping(data, mapping)

	if !data.IsElectric || !data.IsIndoor || !data.IsVirtual {
		t.Fatalf("modifiers = electric:%v indoor:%v virtual:%v, want all true", data.IsElectric, data.IsIndoor, data.IsVirtual)
	}
}

func TestKomootSportConflictDetectsSpecificSubtypeMismatch(t *testing.T) {
	platform := komootSportConflictSnapshot{Sport: "cycling", SubType: "mountain"}
	file := komootSportConflictSnapshot{Sport: "cycling", SubType: "road"}

	if !komootSportConflictSnapshotsConflict(platform, file) {
		t.Fatal("expected Komoot platform MTB vs file road cycling to queue a sport conflict")
	}
}

func TestKomootSportConflictIgnoresBroadFileSubtype(t *testing.T) {
	platform := komootSportConflictSnapshot{Sport: "cycling", SubType: "mountain"}
	file := komootSportConflictSnapshot{Sport: "cycling"}

	if komootSportConflictSnapshotsConflict(platform, file) {
		t.Fatal("broad file cycling should accept the platform subtype without conflict")
	}
}
