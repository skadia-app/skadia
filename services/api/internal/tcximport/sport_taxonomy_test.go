package tcximport

import (
	"strings"
	"testing"
)

func TestMapTCXSportReturnsCanonicalSportTuple(t *testing.T) {
	tests := []struct {
		input        string
		wantSport    string
		wantSubType  string
		wantElectric *bool
	}{
		{"road_cycling", "cycling", "road", nil},
		{"roadcycling", "cycling", "road", nil},
		{"trail_running", "running", "trail", nil},
		{"mountain_biking", "cycling", "mountain", nil},
		{"gravel_ride", "cycling", "gravel_cycling", nil},
		{"other", "generic", "", nil},
		{"does_not_exist", "unknown", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapTCXSport(tt.input)
			if got.Tuple.SportType != tt.wantSport || got.Tuple.SubType != tt.wantSubType {
				t.Fatalf("mapTCXSport(%q): got (%q, %q), want (%q, %q)",
					tt.input, got.Tuple.SportType, got.Tuple.SubType, tt.wantSport, tt.wantSubType)
			}
			assertBoolPatch(t, "IsElectric", got.Patch.IsElectric, tt.wantElectric)
		})
	}
}

func TestMapTCXSportCoversAllConfiguredAliases(t *testing.T) {
	tests := []struct {
		input        string
		wantSport    string
		wantSubType  string
		wantElectric *bool
	}{
		{"running", "running", "", nil},
		{"run", "running", "", nil},
		{"trail_running", "running", "trail", nil},
		{"trailrunning", "running", "trail", nil},
		{"biking", "cycling", "", nil},
		{"cycling", "cycling", "", nil},
		{"ride", "cycling", "", nil},
		{"road_cycling", "cycling", "road", nil},
		{"roadcycling", "cycling", "road", nil},
		{"mountainbikeride", "cycling", "mountain", nil},
		{"mountain_biking", "cycling", "mountain", nil},
		{"ebikeride", "cycling", "", boolPtr(true)},
		{"e_bike_ride", "cycling", "", boolPtr(true)},
		{"ebike", "cycling", "", boolPtr(true)},
		{"gravelride", "cycling", "gravel_cycling", nil},
		{"gravel_ride", "cycling", "gravel_cycling", nil},
		{"hiking", "hiking", "", nil},
		{"hike", "hiking", "", nil},
		{"walking", "walking", "", nil},
		{"walk", "walking", "", nil},
		{"swimming", "swimming", "", nil},
		{"swim", "swimming", "", nil},
		{"skiing", "alpine_skiing", "", nil},
		{"alpine_skiing", "alpine_skiing", "", nil},
		{"cross_country_skiing", "cross_country_skiing", "", nil},
		{"snowboarding", "snowboarding", "", nil},
		{"strength_training", "strength_training", "", nil},
		{"strength", "strength_training", "", nil},
		{"rowing", "rowing", "", nil},
		{"kayaking", "kayaking", "", nil},
		{"multisport", "multisport", "", nil},
		{"other", "generic", "", nil},
	}
	wantIndoorByInput := map[string]*bool{
		"strength_training": boolPtr(true),
		"strength":          boolPtr(true),
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapTCXSport(tt.input)
			if got.Tuple.SportType != tt.wantSport || got.Tuple.SubType != tt.wantSubType {
				t.Fatalf("mapTCXSport(%q): got (%q, %q), want (%q, %q)",
					tt.input, got.Tuple.SportType, got.Tuple.SubType, tt.wantSport, tt.wantSubType)
			}
			assertBoolPatch(t, "IsElectric", got.Patch.IsElectric, tt.wantElectric)
			assertBoolPatch(t, "IsIndoor", got.Patch.IsIndoor, wantIndoorByInput[tt.input])
			assertBoolPatch(t, "IsVirtual", got.Patch.IsVirtual, nil)
		})
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

func boolPtr(v bool) *bool {
	return &v
}

func assertBoolPatch(t *testing.T, name string, got *bool, want *bool) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s patch = %#v, want nil", name, *got)
		}
		return
	}
	if got == nil || *got != *want {
		t.Fatalf("%s patch = %#v, want %v", name, got, *want)
	}
}
