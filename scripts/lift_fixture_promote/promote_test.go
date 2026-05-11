package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

func TestCanonicalHashIsStableAndDetectsSampleEdits(t *testing.T) {
	fixture := tinyCanonicalFixture()
	gt := tinyFixtureGT()

	first, err := ComputeFixtureContentHash(fixture, gt)
	if err != nil {
		t.Fatalf("ComputeFixtureContentHash returned error: %v", err)
	}
	second, err := ComputeFixtureContentHash(fixture, gt)
	if err != nil {
		t.Fatalf("ComputeFixtureContentHash returned error on repeat: %v", err)
	}
	if first != second {
		t.Fatalf("hash changed between identical inputs: %q != %q", first, second)
	}

	edited := fixture
	alt := *edited.Samples[1].AltM + 0.5
	edited.Samples[1].AltM = &alt
	third, err := ComputeFixtureContentHash(edited, gt)
	if err != nil {
		t.Fatalf("ComputeFixtureContentHash after edit returned error: %v", err)
	}
	if third == first {
		t.Fatal("hash did not change after sample edit")
	}
}

func TestCanonicalLoaderRejectsUnknownFormatVersion(t *testing.T) {
	dir := t.TempDir()
	canonicalPath := filepath.Join(dir, "case.canonical.json")
	gtPath := filepath.Join(dir, "case.gt.yaml")
	if err := os.WriteFile(canonicalPath, []byte(`{"format_version":2,"scenario_kind":"single_ride","samples":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gtPath, []byte("format_version: 1\nscenario_kind: single_ride\nrides_expected: 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := LoadCanonicalPair(canonicalPath, gtPath); err == nil {
		t.Fatal("LoadCanonicalPair accepted an unknown canonical format version")
	}
}

func TestProjectToWGS84KeepsPseudoAnchorInRange(t *testing.T) {
	lat, lng, err := ProjectToWGS84(2500, -1200)
	if err != nil {
		t.Fatalf("ProjectToWGS84 returned error: %v", err)
	}
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		t.Fatalf("projected coordinate out of WGS84 range: lat=%f lng=%f", lat, lng)
	}
}

func TestValidateExtractOutputPathRequiresConfiguredLocalRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "raw_window.json")
	outside := filepath.Join(t.TempDir(), "raw_window.json")

	if err := ValidateExtractOutputPath(inside, []string{root}); err != nil {
		t.Fatalf("local output path rejected: %v", err)
	}
	if err := ValidateExtractOutputPath(outside, []string{root}); err == nil {
		t.Fatal("output outside configured local root was accepted")
	}
}

func TestPrivacySelfCheckUsesRealIdentifierSideOfReplacementList(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "privacy.txt")
	if err := os.WriteFile(listPath, []byte("safe_case_name => private_identifier\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := PrivacySelfCheck(listPath, CanonicalFixture{
		Description:  "safe_case_name",
		ScenarioKind: "single_ride",
	}, FixtureGT{
		Anchors: map[string]Anchor{"generic_anchor": {XM: 0, YM: 0}},
	})
	if err != nil {
		t.Fatalf("safe replacement side should be allowed, got: %v", err)
	}

	err = PrivacySelfCheck(listPath, CanonicalFixture{
		Description:  "contains private_identifier",
		ScenarioKind: "single_ride",
	}, FixtureGT{
		Anchors: map[string]Anchor{"generic_anchor": {XM: 0, YM: 0}},
	})
	if err == nil {
		t.Fatal("privacy self-check did not flag a real identifier")
	}
}

func TestResynthesizeWindowIsDeterministicAndIgnoresSourceSeed(t *testing.T) {
	raw := RawWindow{Samples: []RawSample{
		{DtMS: 0, XM: 0, YM: 0, AltM: ptrFloat(1000), SpeedMS: ptrFloat(2), DistanceM: ptrFloat(0)},
		{DtMS: 1000, XM: 2, YM: 0, AltM: ptrFloat(1001), SpeedMS: ptrFloat(2), DistanceM: ptrFloat(2)},
		{DtMS: 2000, XM: 4, YM: 0, AltM: ptrFloat(1002), SpeedMS: ptrFloat(2), DistanceM: ptrFloat(4)},
	}}
	opts := ResynthesizeOptions{
		RepoSeedHex:  "00112233445566778899aabbccddeeff",
		ScenarioKind: "single_ride",
		Description:  "generic single ride",
	}

	t.Setenv("SKADIA_ANON_SOURCE_SEED", "first")
	first, firstGT, err := ResynthesizeWindow(raw, opts)
	if err != nil {
		t.Fatalf("ResynthesizeWindow returned error: %v", err)
	}
	t.Setenv("SKADIA_ANON_SOURCE_SEED", "second")
	second, secondGT, err := ResynthesizeWindow(raw, opts)
	if err != nil {
		t.Fatalf("ResynthesizeWindow returned error on repeat: %v", err)
	}

	firstBytes, err := MarshalCanonical(first, firstGT)
	if err != nil {
		t.Fatalf("MarshalCanonical first: %v", err)
	}
	secondBytes, err := MarshalCanonical(second, secondGT)
	if err != nil {
		t.Fatalf("MarshalCanonical second: %v", err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("resynthesis output changed when only SKADIA_ANON_SOURCE_SEED changed")
	}
}

func TestResynthesizeWindowDoesNotPreserveSourceTurnGeometry(t *testing.T) {
	raw := RawWindow{FormatVersion: FormatVersion, Samples: []RawSample{
		{DtMS: 0, XM: 0, YM: 0, AltM: ptrFloat(1000), SpeedMS: ptrFloat(1), DistanceM: ptrFloat(0)},
		{DtMS: 1000, XM: 10, YM: 0, AltM: ptrFloat(1000), SpeedMS: ptrFloat(1), DistanceM: ptrFloat(10)},
		{DtMS: 2000, XM: 20, YM: 0, AltM: ptrFloat(1000), SpeedMS: ptrFloat(1), DistanceM: ptrFloat(20)},
		{DtMS: 3000, XM: 20, YM: 10, AltM: ptrFloat(1000), SpeedMS: ptrFloat(1), DistanceM: ptrFloat(30)},
		{DtMS: 4000, XM: 20, YM: 20, AltM: ptrFloat(1000), SpeedMS: ptrFloat(1), DistanceM: ptrFloat(40)},
	}}

	fixture, _, err := ResynthesizeWindow(raw, ResynthesizeOptions{
		RepoSeedHex:  "00112233445566778899aabbccddeeff",
		ScenarioKind: "geometry_privacy",
		Description:  "generic geometry privacy fixture",
	})
	if err != nil {
		t.Fatalf("ResynthesizeWindow returned error: %v", err)
	}

	turnAngle := sampleTurnAngle(fixture.Samples[0], fixture.Samples[1], fixture.Samples[2], fixture.Samples[3])
	if math.Abs(turnAngle-math.Pi/2) < 0.10 {
		t.Fatalf("resynthesis preserved a right-angle source turn: angle=%.3f rad", turnAngle)
	}
}

func TestDetectorEquivalenceRejectsRideMismatch(t *testing.T) {
	source := []lifts.RideWithIndices{{
		Ride: lifts.LiftRide{
			DurationS:  120,
			AscentM:    80,
			DistanceM:  300,
			Confidence: "high",
		},
	}}
	resynth := []lifts.RideWithIndices{{
		Ride: lifts.LiftRide{
			DurationS:  120,
			AscentM:    80,
			DistanceM:  380,
			Confidence: "high",
		},
	}}

	err := AssertDetectorEquivalent(source, resynth, defaultRideTolerance())
	if err == nil {
		t.Fatal("detector equivalence accepted a distance drift beyond tolerance")
	}
	diff := DetectorTraceDiff(source, resynth)
	if !strings.Contains(diff, "distance") {
		t.Fatalf("trace diff did not mention the changed metric: %s", diff)
	}
}

func TestAssertRideWithinToleranceAppliesPromotionDistanceGrowth(t *testing.T) {
	baseTime := time.Unix(0, 0).UTC()
	samples := []CanonicalSample{
		{DtMS: 0, XM: 0, YM: 0},
		{DtMS: 1000, XM: 42, YM: 0},
		{DtMS: 2000, XM: 80, YM: 0},
	}
	ride := lifts.LiftRide{
		StartedAt: baseTime.Add(time.Second),
		EndedAt:   baseTime.Add(2 * time.Second),
		DurationS: 120, AscentM: 75, DistanceM: 240, Confidence: "high",
	}
	gt := RideGT{
		RideIndex:   0,
		Confidence:  "high",
		StartAnchor: "start",
		EndAnchor:   "end",
		PromotionDistance: PromotionDistance{
			StartM: 12,
			EndM:   8,
		},
		Expected: RideExpected{
			DurationS: 120,
			AscentM:   75,
			DistanceM: 240,
		},
		Tolerance: RideTolerance{
			StartDistanceGrowthM: 30,
			EndDistanceGrowthM:   30,
			AscentM:              10,
			DescentM:             5,
			DistanceM:            20,
			DurationS:            30,
			Confidence:           "class_match",
		},
	}
	anchors := map[string]Anchor{
		"start": {XM: 0, YM: 0},
		"end":   {XM: 100, YM: 0},
	}

	if err := AssertRideWithinTolerance(ride, gt, samples, anchors); err != nil {
		t.Fatalf("AssertRideWithinTolerance rejected in-growth ride: %v", err)
	}

	samples[1].XM = 43
	if err := AssertRideWithinTolerance(ride, gt, samples, anchors); err == nil {
		t.Fatal("AssertRideWithinTolerance accepted start distance growth beyond tolerance")
	}
}

func TestRunExtractWritesStationAnchorsFromGroundTruth(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "source.gpx")
	output := filepath.Join(dir, "raw_window.json")
	gtPath := filepath.Join(dir, "gt_lifts.yaml")
	if err := os.WriteFile(input, []byte(syntheticGPX()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gtPath, []byte(syntheticGTLiftsYAML()), 0o600); err != nil {
		t.Fatal(err)
	}

	err := run([]string{
		"extract",
		"--input", input,
		"--window-from", "2026-01-01T08:00:00Z",
		"--window-to", "2026-01-01T08:00:03Z",
		"--output", output,
		"--local-root", dir,
		"--gt-lifts", gtPath,
		"--gt-file", "source.gpx",
	})
	if err != nil {
		t.Fatalf("extract command returned error: %v", err)
	}

	rawBytes, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var raw RawWindow
	if err := json.Unmarshal(rawBytes, &raw); err != nil {
		t.Fatalf("raw window is not JSON: %v", err)
	}
	if len(raw.StationAnchors) != 2 {
		t.Fatalf("raw station anchor count = %d, want 2", len(raw.StationAnchors))
	}
}

func TestRunExtractFailsWhenGroundTruthEntryIsMissing(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "source.gpx")
	output := filepath.Join(dir, "raw_window.json")
	gtPath := filepath.Join(dir, "gt_lifts.yaml")
	if err := os.WriteFile(input, []byte(syntheticGPX()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gtPath, []byte("cases: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := run([]string{
		"extract",
		"--input", input,
		"--window-from", "2026-01-01T08:00:00Z",
		"--window-to", "2026-01-01T08:00:03Z",
		"--output", output,
		"--local-root", dir,
		"--gt-lifts", gtPath,
		"--gt-file", "source.gpx",
	})
	if err == nil {
		t.Fatal("extract accepted missing gt lift entry")
	}
}

func TestRunExtractWritesRawWindowAndSourceMapping(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "source.gpx")
	output := filepath.Join(dir, "raw_window.json")
	if err := os.WriteFile(input, []byte(syntheticGPX()), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SKADIA_ANON_SOURCE_SEED", "local-test-seed")

	err := run([]string{
		"extract",
		"--input", input,
		"--window-from", "2026-01-01T08:00:00Z",
		"--window-to", "2026-01-01T08:00:03Z",
		"--output", output,
		"--local-root", dir,
	})
	if err != nil {
		t.Fatalf("extract command returned error: %v", err)
	}

	rawBytes, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var raw RawWindow
	if err := json.Unmarshal(rawBytes, &raw); err != nil {
		t.Fatalf("raw window is not JSON: %v", err)
	}
	if len(raw.Samples) != 4 {
		t.Fatalf("raw sample count = %d, want 4", len(raw.Samples))
	}

	mappingBytes, err := os.ReadFile(filepath.Join(dir, "source_mapping.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(mappingBytes, []byte("source_fingerprint_hash")) {
		t.Fatalf("source mapping missing fingerprint field: %s", mappingBytes)
	}
}

func TestRunResynthesizeWritesCanonicalAndGT(t *testing.T) {
	dir := t.TempDir()
	rawPath := filepath.Join(dir, "raw_window.json")
	canonicalPath := filepath.Join(dir, "case.canonical.json")
	gtPath := filepath.Join(dir, "case.gt.yaml")
	privacyPath := filepath.Join(dir, "privacy.txt")
	raw := RawWindow{FormatVersion: FormatVersion, Samples: []RawSample{
		{DtMS: 0, XM: 0, YM: 0, AltM: ptrFloat(1000), SpeedMS: ptrFloat(2), DistanceM: ptrFloat(0)},
		{DtMS: 1000, XM: 2, YM: 0, AltM: ptrFloat(1001), SpeedMS: ptrFloat(2), DistanceM: ptrFloat(2)},
	}}
	rawBytes, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rawPath, rawBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(privacyPath, []byte("safe_name => blocked_private_identifier\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err = run([]string{
		"resynthesize",
		"--input", rawPath,
		"--output-canonical", canonicalPath,
		"--output-gt", gtPath,
		"--scenario-kind", "single_ride",
		"--description", "safe_name",
		"--seed", "00112233445566778899aabbccddeeff",
		"--privacy-list", privacyPath,
	})
	if err != nil {
		t.Fatalf("resynthesize command returned error: %v", err)
	}

	fixture, gt, err := LoadCanonicalPair(canonicalPath, gtPath)
	if err != nil {
		t.Fatalf("generated fixture pair did not load: %v", err)
	}
	if fixture.ScenarioKind != "single_ride" || gt.ScenarioKind != "single_ride" {
		t.Fatalf("scenario_kind mismatch: canonical=%q gt=%q", fixture.ScenarioKind, gt.ScenarioKind)
	}
	if gt.FixtureContentHash == "" || gt.DetectorVerdictHash == "" {
		t.Fatalf("generated gt missing hashes: %+v", gt)
	}
}

func TestRunResynthesizeAppliesToleranceSpec(t *testing.T) {
	dir := t.TempDir()
	rawPath := filepath.Join(dir, "raw_window.json")
	tolerancePath := filepath.Join(dir, "tolerance.yaml")
	canonicalPath := filepath.Join(dir, "case.canonical.json")
	gtPath := filepath.Join(dir, "case.gt.yaml")
	rawBytes, err := json.Marshal(rawCableWindowWithStations())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rawPath, rawBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tolerancePath, []byte(`
start_distance_growth_m: 9
end_distance_growth_m: 11
ascent_m: 3
descent_m: 4
distance_m: 5
duration_s: 6
confidence: exact
`), 0o600); err != nil {
		t.Fatal(err)
	}

	err = run([]string{
		"resynthesize",
		"--input", rawPath,
		"--output-canonical", canonicalPath,
		"--output-gt", gtPath,
		"--scenario-kind", "slow_chairlift",
		"--seed", "00112233445566778899aabbccddeeff",
		"--tolerance-spec", tolerancePath,
	})
	if err != nil {
		t.Fatalf("resynthesize command returned error: %v", err)
	}
	_, gt, err := LoadCanonicalPair(canonicalPath, gtPath)
	if err != nil {
		t.Fatalf("generated fixture pair did not load: %v", err)
	}
	if len(gt.Rides) != 1 {
		t.Fatalf("ride count = %d, want 1", len(gt.Rides))
	}
	got := gt.Rides[0].Tolerance
	if got.StartDistanceGrowthM != 9 || got.EndDistanceGrowthM != 11 ||
		got.AscentM != 3 || got.DescentM != 4 || got.DistanceM != 5 ||
		got.DurationS != 6 || got.Confidence != "exact" {
		t.Fatalf("tolerance spec was not applied: %+v", got)
	}
}

func TestResynthesizeWindowWritesRideGTWithStationDistanceBaseline(t *testing.T) {
	raw := rawCableWindowWithStations()
	fixture, gt, err := ResynthesizeWindow(raw, ResynthesizeOptions{
		RepoSeedHex:  "00112233445566778899aabbccddeeff",
		ScenarioKind: "slow_chairlift",
		Description:  "generic slow cable fixture",
	})
	if err != nil {
		t.Fatalf("ResynthesizeWindow returned error: %v", err)
	}
	if fixture.RidesExpected != 1 || gt.RidesExpected != 1 || len(gt.Rides) != 1 {
		t.Fatalf("resynthesized ride counts canonical=%d gt=%d rides=%d, want 1", fixture.RidesExpected, gt.RidesExpected, len(gt.Rides))
	}
	ride := gt.Rides[0]
	if ride.StartAnchor == "" || ride.EndAnchor == "" {
		t.Fatalf("ride anchors were not assigned: %+v", ride)
	}
	if ride.PromotionDistance.StartM < 10 || ride.PromotionDistance.StartM > 14 {
		t.Fatalf("start promotion distance = %.3fm, want station offset around 12m", ride.PromotionDistance.StartM)
	}
	if ride.PromotionDistance.EndM < 12 || ride.PromotionDistance.EndM > 16 {
		t.Fatalf("end promotion distance = %.3fm, want station offset around 14m", ride.PromotionDistance.EndM)
	}
	if ride.Tolerance.StartDistanceGrowthM != 30 || ride.Tolerance.EndDistanceGrowthM != 30 {
		t.Fatalf("distance growth tolerance = %.1f/%.1f, want 30/30", ride.Tolerance.StartDistanceGrowthM, ride.Tolerance.EndDistanceGrowthM)
	}
}

func tinyCanonicalFixture() CanonicalFixture {
	return CanonicalFixture{
		FormatVersion: FormatVersion,
		Description:   "generic single ride",
		ScenarioKind:  "single_ride",
		RidesExpected: 1,
		Samples: []CanonicalSample{
			{DtMS: 0, XM: 0, YM: 0, AltM: ptrFloat(1000), SpeedMS: ptrFloat(2), DistanceM: ptrFloat(0)},
			{DtMS: 1000, XM: 2, YM: 0, AltM: ptrFloat(1001), SpeedMS: ptrFloat(2), DistanceM: ptrFloat(2)},
		},
	}
}

func tinyFixtureGT() FixtureGT {
	return FixtureGT{
		FormatVersion: FormatVersion,
		ScenarioKind:  "single_ride",
		RidesExpected: 1,
		Rides: []RideGT{{
			RideIndex:   0,
			Confidence:  "high",
			StartAnchor: "start",
			EndAnchor:   "end",
			PromotionDistance: PromotionDistance{
				StartM: 0,
				EndM:   0,
			},
			Expected: RideExpected{
				DurationS: 1,
				AscentM:   1,
				DistanceM: 2,
			},
			Tolerance: RideTolerance{
				StartDistanceGrowthM: 1,
				EndDistanceGrowthM:   1,
				AscentM:              1,
				DescentM:             1,
				DistanceM:            1,
				Confidence:           "exact",
			},
		}},
		Anchors: map[string]Anchor{
			"start": {XM: 0, YM: 0},
			"end":   {XM: 2, YM: 0},
		},
	}
}

func ptrFloat(v float64) *float64 { return &v }

func sampleTurnAngle(a, b, c, d CanonicalSample) float64 {
	v1x := b.XM - a.XM
	v1y := b.YM - a.YM
	v2x := d.XM - c.XM
	v2y := d.YM - c.YM
	dot := v1x*v2x + v1y*v2y
	mag := math.Hypot(v1x, v1y) * math.Hypot(v2x, v2y)
	if mag == 0 {
		return 0
	}
	cos := dot / mag
	if cos < -1 {
		cos = -1
	}
	if cos > 1 {
		cos = 1
	}
	return math.Acos(cos)
}

func rawCableWindowWithStations() RawWindow {
	const (
		lat0 = 47.0
		lng0 = 12.0
	)
	var samples []RawSample
	for i := 0; i <= 42; i++ {
		x := float64(i) * 22
		lat, lng := syntheticLatLng(lat0, lng0, x, 0)
		samples = append(samples, RawSample{
			DtMS:      int64(i) * 10000,
			Lat:       &lat,
			Lng:       &lng,
			AltM:      ptrFloat(1000 + float64(i)*3.2),
			SpeedMS:   ptrFloat(2.2),
			DistanceM: ptrFloat(x),
		})
	}
	valleyLat, valleyLng := syntheticLatLng(lat0, lng0, 12, 0)
	upperLat, upperLng := syntheticLatLng(lat0, lng0, 910, 0)
	return RawWindow{
		FormatVersion: FormatVersion,
		Samples:       samples,
		StationAnchors: []StationAnchor{
			{Name: "valley_a", Lat: valleyLat, Lng: valleyLng},
			{Name: "upper_a", Lat: upperLat, Lng: upperLng},
		},
	}
}

func syntheticLatLng(originLat, originLng, xM, yM float64) (lat, lng float64) {
	lat = originLat + yM/111320.0
	lng = originLng + xM/(111320.0*math.Cos(originLat*math.Pi/180.0))
	return lat, lng
}

func syntheticGPX() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="synthetic">
  <trk>
    <name>synthetic</name>
    <type>hiking</type>
    <trkseg>
      <trkpt lat="47.000000" lon="12.000000"><ele>1000</ele><time>2026-01-01T08:00:00Z</time></trkpt>
      <trkpt lat="47.000010" lon="12.000000"><ele>1001</ele><time>2026-01-01T08:00:01Z</time></trkpt>
      <trkpt lat="47.000020" lon="12.000000"><ele>1002</ele><time>2026-01-01T08:00:02Z</time></trkpt>
      <trkpt lat="47.000030" lon="12.000000"><ele>1003</ele><time>2026-01-01T08:00:03Z</time></trkpt>
    </trkseg>
  </trk>
</gpx>`
}

func syntheticGTLiftsYAML() string {
	return `cases:
  - file: source.gpx
    lifts:
      - id: generic_a
        p1: [47.000000, 12.000000]
        p2: [47.000030, 12.000000]
`
}
