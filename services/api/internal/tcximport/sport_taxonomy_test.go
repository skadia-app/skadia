package tcximport

import (
	"strings"
	"testing"
)

func TestMapTCXSportReturnsCanonicalSportTuple(t *testing.T) {
	tests := []struct {
		input       string
		wantSport   string
		wantSubType string
	}{
		{"road_cycling", "cycling", "road"},
		{"roadcycling", "cycling", "road"},
		{"trail_running", "running", "trail"},
		{"mountain_biking", "cycling", "mountain"},
		{"gravel_ride", "cycling", "gravel_cycling"},
		{"other", "unknown", ""},
		{"does_not_exist", "unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapTCXSport(tt.input)
			if got.Tuple.SportType != tt.wantSport || got.Tuple.SubType != tt.wantSubType {
				t.Fatalf("mapTCXSport(%q): got (%q, %q), want (%q, %q)",
					tt.input, got.Tuple.SportType, got.Tuple.SubType, tt.wantSport, tt.wantSubType)
			}
		})
	}
}

func TestMapTCXSportReturnsElectricPatch(t *testing.T) {
	got := mapTCXSport("ebike")
	if got.Tuple.SportType != "cycling" {
		t.Fatalf("sport = %q, want cycling", got.Tuple.SportType)
	}
	if got.Patch.IsElectric == nil || !*got.Patch.IsElectric {
		t.Fatalf("IsElectric patch = %#v, want true", got.Patch.IsElectric)
	}
}

func TestParseTCXAppliesElectricPatch(t *testing.T) {
	data, err := ParseTCX(strings.NewReader(minimalTCX("ebike")), "electric.tcx")
	if err != nil {
		t.Fatalf("ParseTCX() error = %v", err)
	}
	if data.SportType != "cycling" {
		t.Fatalf("SportType = %q, want cycling", data.SportType)
	}
	if !data.IsElectric {
		t.Fatal("IsElectric = false, want true")
	}
}

func minimalTCX(sport string) string {
	return `<TrainingCenterDatabase>
  <Activities>
    <Activity Sport="` + sport + `">
      <Id>2026-01-01T00:00:00Z</Id>
      <Lap StartTime="2026-01-01T00:00:00Z">
        <TotalTimeSeconds>60</TotalTimeSeconds>
        <DistanceMeters>10</DistanceMeters>
        <Calories>1</Calories>
        <Track>
          <Trackpoint>
            <Time>2026-01-01T00:00:00Z</Time>
            <Position><LatitudeDegrees>50.0</LatitudeDegrees><LongitudeDegrees>8.0</LongitudeDegrees></Position>
            <AltitudeMeters>100</AltitudeMeters>
            <DistanceMeters>0</DistanceMeters>
          </Trackpoint>
          <Trackpoint>
            <Time>2026-01-01T00:01:00Z</Time>
            <Position><LatitudeDegrees>50.0001</LatitudeDegrees><LongitudeDegrees>8.0001</LongitudeDegrees></Position>
            <AltitudeMeters>101</AltitudeMeters>
            <DistanceMeters>10</DistanceMeters>
          </Trackpoint>
        </Track>
      </Lap>
    </Activity>
  </Activities>
</TrainingCenterDatabase>`
}
