package lifts

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const canonicalFixtureFormatVersion = 1

type canonicalFixture struct {
	FormatVersion int               `json:"format_version"`
	Description   string            `json:"description"`
	ScenarioKind  string            `json:"scenario_kind"`
	RidesExpected int               `json:"rides_expected"`
	Samples       []canonicalSample `json:"samples"`
}

type canonicalFixtureCase struct {
	Fixture canonicalFixture
	Points  []StreamPoint
	GT      fixtureGT
}

type canonicalSample struct {
	DtMS         int64    `json:"dt_ms"`
	XM           float64  `json:"x_m"`
	YM           float64  `json:"y_m"`
	AltM         *float64 `json:"alt_m"`
	SpeedMS      *float64 `json:"speed_ms"`
	DistanceM    *float64 `json:"distance_m"`
	HRBPM        *int     `json:"hr_bpm,omitempty"`
	PowerW       *int     `json:"power_w,omitempty"`
	TemperatureC *float64 `json:"temperature_c,omitempty"`
}

type fixtureGT struct {
	FormatVersion       int               `yaml:"format_version"`
	ScenarioKind        string            `yaml:"scenario_kind"`
	RidesExpected       int               `yaml:"rides_expected"`
	Rides               []rideGT          `yaml:"rides"`
	Anchors             map[string]anchor `yaml:"anchors"`
	FixtureContentHash  string            `yaml:"fixture_content_hash"`
	DetectorVerdictHash string            `yaml:"detector_verdict_hash"`
}

type anchor struct {
	XM float64 `yaml:"x_m"`
	YM float64 `yaml:"y_m"`
}

type rideGT struct {
	RideIndex         int               `yaml:"ride_index"`
	Confidence        string            `yaml:"confidence"`
	StartAnchor       string            `yaml:"start_anchor"`
	EndAnchor         string            `yaml:"end_anchor"`
	PromotionDistance promotionDistance `yaml:"promotion_distance"`
	Expected          rideExpected      `yaml:"expected"`
	Tolerance         rideTolerance     `yaml:"tolerance"`
}

type promotionDistance struct {
	StartM float64 `yaml:"start_m"`
	EndM   float64 `yaml:"end_m"`
}

type rideExpected struct {
	DurationS int     `yaml:"duration_s"`
	AscentM   float64 `yaml:"ascent_m"`
	DescentM  float64 `yaml:"descent_m"`
	DistanceM float64 `yaml:"distance_m"`
}

type rideTolerance struct {
	StartDistanceGrowthM float64 `yaml:"start_distance_growth_m"`
	EndDistanceGrowthM   float64 `yaml:"end_distance_growth_m"`
	AscentM              float64 `yaml:"ascent_m"`
	DescentM             float64 `yaml:"descent_m"`
	DistanceM            float64 `yaml:"distance_m"`
	DurationS            int     `yaml:"duration_s"`
	Confidence           string  `yaml:"confidence"`
}

type synthSpec struct {
	durationS    int
	meanSpeedMs  float64
	altRateMs    float64
	sinuosity    float64
	speedCV      float64
	sampleRateHz int
}

func loadCanonicalFixture(t *testing.T, name string) canonicalFixtureCase {
	t.Helper()
	dir := filepath.Join("testdata")
	canonicalPath := filepath.Join(dir, name+".canonical.json")
	gtPath := filepath.Join(dir, name+".gt.yaml")

	canonicalBytes, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical fixture: %v", err)
	}
	var fixture canonicalFixture
	if err := json.Unmarshal(canonicalBytes, &fixture); err != nil {
		t.Fatalf("parse canonical fixture: %v", err)
	}
	if fixture.FormatVersion != canonicalFixtureFormatVersion {
		t.Fatalf("canonical format_version = %d, want %d", fixture.FormatVersion, canonicalFixtureFormatVersion)
	}

	gtBytes, err := os.ReadFile(gtPath)
	if err != nil {
		t.Fatalf("read fixture gt: %v", err)
	}
	var gt fixtureGT
	if err := yaml.Unmarshal(gtBytes, &gt); err != nil {
		t.Fatalf("parse fixture gt: %v", err)
	}
	if gt.FormatVersion != canonicalFixtureFormatVersion {
		t.Fatalf("gt format_version = %d, want %d", gt.FormatVersion, canonicalFixtureFormatVersion)
	}
	contentHash := computeFixtureContentHash(t, fixture, gt)
	if gt.FixtureContentHash != contentHash {
		t.Fatalf("fixture_content_hash = %q, want %q", gt.FixtureContentHash, contentHash)
	}

	base := time.Unix(0, 0).UTC()
	points := make([]StreamPoint, 0, len(fixture.Samples))
	for i, sample := range fixture.Samples {
		if i > 0 && sample.DtMS < fixture.Samples[i-1].DtMS {
			t.Fatalf("sample %d dt_ms is not monotonic", i)
		}
		lat, lng := projectToWGS84(sample.XM, sample.YM)
		points = append(points, StreamPoint{
			Time:      base.Add(time.Duration(sample.DtMS) * time.Millisecond),
			Lat:       &lat,
			Lng:       &lng,
			AltitudeM: sample.AltM,
			SpeedMs:   sample.SpeedMS,
			DistanceM: sample.DistanceM,
		})
	}
	verdictHash := computeDetectorVerdictHash(DetectHeuristicWithIndices(points, DefaultDetectorConfig("")))
	if gt.DetectorVerdictHash == "" || gt.DetectorVerdictHash == "pending" {
		t.Fatalf("detector_verdict_hash must be promoted, got %q; current detector verdict hash is %q", gt.DetectorVerdictHash, verdictHash)
	}
	if gt.DetectorVerdictHash != verdictHash {
		t.Logf("detector_verdict_hash drift: stored=%s current=%s", gt.DetectorVerdictHash, verdictHash)
	}
	return canonicalFixtureCase{Fixture: fixture, Points: points, GT: gt}
}

func projectToWGS84(xM, yM float64) (lat, lng float64) {
	const lat0 = 47.0
	const lng0 = 12.0
	lat = lat0 + yM/111320.0
	lng = lng0 + xM/(111320.0*math.Cos(lat0*math.Pi/180.0))
	return lat, lng
}

func assertRideWithinTolerance(t *testing.T, got LiftRide, want rideGT, anchors map[string]anchor, samples []canonicalSample) {
	t.Helper()
	if err := checkRideWithinTolerance(got, want, anchors, samples); err != nil {
		t.Fatal(err)
	}
}

func checkRideWithinTolerance(got LiftRide, want rideGT, anchors map[string]anchor, samples []canonicalSample) error {
	start, ok := canonicalSampleAt(samples, got.StartedAt)
	if !ok {
		return fmt.Errorf("no sample for ride start %s", got.StartedAt)
	}
	end, ok := canonicalSampleAt(samples, got.EndedAt)
	if !ok {
		return fmt.Errorf("no sample for ride end %s", got.EndedAt)
	}

	startAnchor, ok := anchors[want.StartAnchor]
	if !ok {
		return fmt.Errorf("missing start anchor %q", want.StartAnchor)
	}
	endAnchor, ok := anchors[want.EndAnchor]
	if !ok {
		return fmt.Errorf("missing end anchor %q", want.EndAnchor)
	}

	startDist := math.Hypot(start.XM-startAnchor.XM, start.YM-startAnchor.YM)
	if limit := want.PromotionDistance.StartM + want.Tolerance.StartDistanceGrowthM; startDist > limit {
		return fmt.Errorf("start distance %.1fm exceeds promotion baseline %.1fm + growth %.1fm", startDist, want.PromotionDistance.StartM, want.Tolerance.StartDistanceGrowthM)
	}
	endDist := math.Hypot(end.XM-endAnchor.XM, end.YM-endAnchor.YM)
	if limit := want.PromotionDistance.EndM + want.Tolerance.EndDistanceGrowthM; endDist > limit {
		return fmt.Errorf("end distance %.1fm exceeds promotion baseline %.1fm + growth %.1fm", endDist, want.PromotionDistance.EndM, want.Tolerance.EndDistanceGrowthM)
	}

	if !confidenceMatches(got.Confidence, want.Confidence, want.Tolerance.Confidence) {
		return fmt.Errorf("confidence = %q, want %q with mode %q", got.Confidence, want.Confidence, want.Tolerance.Confidence)
	}
	if !withinAbs(got.AscentM, want.Expected.AscentM, want.Tolerance.AscentM) {
		return fmt.Errorf("ascent = %.1f, want %.1f ± %.1f", got.AscentM, want.Expected.AscentM, want.Tolerance.AscentM)
	}
	if !withinAbs(got.DescentM, want.Expected.DescentM, want.Tolerance.DescentM) {
		return fmt.Errorf("descent = %.1f, want %.1f ± %.1f", got.DescentM, want.Expected.DescentM, want.Tolerance.DescentM)
	}
	if !withinAbs(got.DistanceM, want.Expected.DistanceM, want.Tolerance.DistanceM) {
		return fmt.Errorf("distance = %.1f, want %.1f ± %.1f", got.DistanceM, want.Expected.DistanceM, want.Tolerance.DistanceM)
	}
	if absInt(got.DurationS-want.Expected.DurationS) > want.Tolerance.DurationS {
		return fmt.Errorf("duration = %d, want %d ± %d", got.DurationS, want.Expected.DurationS, want.Tolerance.DurationS)
	}
	return nil
}

func synthCableTrack(t *testing.T, spec synthSpec) []StreamPoint {
	t.Helper()
	if spec.durationS <= 0 {
		t.Fatal("durationS must be positive")
	}
	if spec.sampleRateHz <= 0 {
		spec.sampleRateHz = 1
	}
	stepS := 1.0 / float64(spec.sampleRateHz)
	samples := int(float64(spec.durationS)*float64(spec.sampleRateHz)) + 1
	base := time.Unix(0, 0).UTC()
	out := make([]StreamPoint, 0, samples)
	for i := 0; i < samples; i++ {
		elapsed := float64(i) * stepS
		x := spec.meanSpeedMs * elapsed
		if spec.sinuosity > 1.0 {
			x += math.Sin(elapsed/15.0) * (spec.sinuosity - 1.0) * 10
		}
		lat, lng := projectToWGS84(x, 0)
		alt := 1000 + spec.altRateMs*elapsed
		speed := spec.meanSpeedMs
		dist := spec.meanSpeedMs * elapsed
		out = append(out, StreamPoint{
			Time:      base.Add(time.Duration(elapsed * float64(time.Second))),
			Lat:       &lat,
			Lng:       &lng,
			AltitudeM: &alt,
			SpeedMs:   &speed,
			DistanceM: &dist,
		})
	}
	return out
}

func canonicalSampleAt(samples []canonicalSample, ts time.Time) (canonicalSample, bool) {
	base := time.Unix(0, 0).UTC()
	dtMS := int64(ts.Sub(base) / time.Millisecond)
	for _, sample := range samples {
		if sample.DtMS == dtMS {
			return sample, true
		}
	}
	return canonicalSample{}, false
}

func confidenceMatches(got, want, mode string) bool {
	if mode == "" || mode == "exact" {
		return got == want
	}
	if mode != "class_match" {
		return false
	}
	if got == want {
		return true
	}
	return (got == "high" || got == "medium") && (want == "high" || want == "medium")
}

func computeFixtureContentHash(t *testing.T, fixture canonicalFixture, gt fixtureGT) string {
	t.Helper()
	var b bytes.Buffer
	writeHashInt(&b, int64(fixture.FormatVersion))
	writeHashString(&b, fixture.Description)
	writeHashString(&b, fixture.ScenarioKind)
	writeHashInt(&b, int64(fixture.RidesExpected))
	writeHashInt(&b, int64(len(fixture.Samples)))
	for _, sample := range fixture.Samples {
		writeHashInt(&b, sample.DtMS)
		writeHashFloat(&b, sample.XM)
		writeHashFloat(&b, sample.YM)
		writeHashFloatPtr(&b, sample.AltM)
		writeHashFloatPtr(&b, sample.SpeedMS)
		writeHashFloatPtr(&b, sample.DistanceM)
		writeHashIntPtr(&b, sample.HRBPM)
		writeHashIntPtr(&b, sample.PowerW)
		writeHashFloatPtr(&b, sample.TemperatureC)
	}
	writeHashString(&b, gt.ScenarioKind)
	writeHashInt(&b, int64(gt.RidesExpected))
	writeHashInt(&b, int64(len(gt.Rides)))
	for _, ride := range gt.Rides {
		writeHashInt(&b, int64(ride.RideIndex))
		writeHashString(&b, ride.Confidence)
		writeHashString(&b, ride.StartAnchor)
		writeHashString(&b, ride.EndAnchor)
		writeHashFloat(&b, ride.PromotionDistance.StartM)
		writeHashFloat(&b, ride.PromotionDistance.EndM)
		writeHashInt(&b, int64(ride.Expected.DurationS))
		writeHashFloat(&b, ride.Expected.AscentM)
		writeHashFloat(&b, ride.Expected.DescentM)
		writeHashFloat(&b, ride.Expected.DistanceM)
		writeHashTolerance(&b, ride.Tolerance)
	}
	keys := make([]string, 0, len(gt.Anchors))
	for key := range gt.Anchors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	writeHashInt(&b, int64(len(keys)))
	for _, key := range keys {
		writeHashString(&b, key)
		writeHashFloat(&b, gt.Anchors[key].XM)
		writeHashFloat(&b, gt.Anchors[key].YM)
	}
	sum := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(sum[:])
}

func computeDetectorVerdictHash(rides []RideWithIndices) string {
	var b bytes.Buffer
	writeHashInt(&b, canonicalFixtureFormatVersion)
	writeHashInt(&b, int64(len(rides)))
	for _, ride := range rides {
		writeHashInt(&b, int64(ride.Ride.DurationS))
		writeHashFloat(&b, ride.Ride.AscentM)
		writeHashFloat(&b, ride.Ride.DescentM)
		writeHashFloat(&b, math.Round(ride.Ride.DistanceM))
		writeHashString(&b, ride.Ride.Confidence)
	}
	sum := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(sum[:8])
}

func withinAbs(got, want, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance
}

func writeHashTolerance(b *bytes.Buffer, tolerance rideTolerance) {
	writeHashFloat(b, tolerance.StartDistanceGrowthM)
	writeHashFloat(b, tolerance.EndDistanceGrowthM)
	writeHashFloat(b, tolerance.AscentM)
	writeHashFloat(b, tolerance.DescentM)
	writeHashFloat(b, tolerance.DistanceM)
	writeHashInt(b, int64(tolerance.DurationS))
	writeHashString(b, tolerance.Confidence)
}

func writeHashString(b *bytes.Buffer, value string) {
	writeHashInt(b, int64(len(value)))
	b.WriteString(value)
}

func writeHashInt(b *bytes.Buffer, value int64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(value))
	b.Write(buf[:])
}

func writeHashFloat(b *bytes.Buffer, value float64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], math.Float64bits(value))
	b.Write(buf[:])
}

func writeHashFloatPtr(b *bytes.Buffer, value *float64) {
	if value == nil {
		b.WriteByte(0)
		return
	}
	b.WriteByte(1)
	writeHashFloat(b, *value)
}

func writeHashIntPtr(b *bytes.Buffer, value *int) {
	if value == nil {
		b.WriteByte(0)
		return
	}
	b.WriteByte(1)
	writeHashInt(b, int64(*value))
}

func assertDelta(t *testing.T, label string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %.1f, want %.1f ± %.1f", label, got, want, tolerance)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func mustFloat(v float64) *float64 {
	return &v
}

func debugFixtureSample(x, alt float64, dtMS int64) canonicalSample {
	return canonicalSample{
		DtMS:      dtMS,
		XM:        x,
		YM:        0,
		AltM:      mustFloat(alt),
		SpeedMS:   mustFloat(5),
		DistanceM: mustFloat(x),
	}
}
