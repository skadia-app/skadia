package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
	"gopkg.in/yaml.v3"
)

const FormatVersion = 1

type CanonicalFixture struct {
	FormatVersion int               `json:"format_version"`
	Description   string            `json:"description,omitempty"`
	ScenarioKind  string            `json:"scenario_kind"`
	RidesExpected int               `json:"rides_expected"`
	Samples       []CanonicalSample `json:"samples"`
}

type CanonicalSample struct {
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

type FixtureGT struct {
	FormatVersion       int               `yaml:"format_version"`
	ScenarioKind        string            `yaml:"scenario_kind"`
	RidesExpected       int               `yaml:"rides_expected"`
	Rides               []RideGT          `yaml:"rides,omitempty"`
	Anchors             map[string]Anchor `yaml:"anchors,omitempty"`
	FixtureContentHash  string            `yaml:"fixture_content_hash,omitempty"`
	DetectorVerdictHash string            `yaml:"detector_verdict_hash,omitempty"`
}

type Anchor struct {
	XM float64 `yaml:"x_m"`
	YM float64 `yaml:"y_m"`
}

type RideGT struct {
	RideIndex         int               `yaml:"ride_index"`
	Confidence        string            `yaml:"confidence"`
	StartAnchor       string            `yaml:"start_anchor"`
	EndAnchor         string            `yaml:"end_anchor"`
	PromotionDistance PromotionDistance `yaml:"promotion_distance"`
	Expected          RideExpected      `yaml:"expected"`
	Tolerance         RideTolerance     `yaml:"tolerance"`
}

type PromotionDistance struct {
	StartM float64 `yaml:"start_m"`
	EndM   float64 `yaml:"end_m"`
}

type RideExpected struct {
	DurationS int     `yaml:"duration_s"`
	AscentM   float64 `yaml:"ascent_m"`
	DescentM  float64 `yaml:"descent_m"`
	DistanceM float64 `yaml:"distance_m"`
}

type RideTolerance struct {
	StartDistanceGrowthM float64 `yaml:"start_distance_growth_m"`
	EndDistanceGrowthM   float64 `yaml:"end_distance_growth_m"`
	AscentM              float64 `yaml:"ascent_m"`
	DescentM             float64 `yaml:"descent_m"`
	DistanceM            float64 `yaml:"distance_m"`
	DurationS            int     `yaml:"duration_s"`
	Confidence           string  `yaml:"confidence"`
}

type RawWindow struct {
	FormatVersion  int             `json:"format_version"`
	Samples        []RawSample     `json:"samples"`
	StationAnchors []StationAnchor `json:"station_anchors,omitempty"`
}

type RawSample struct {
	DtMS      int64    `json:"dt_ms"`
	Lat       *float64 `json:"lat,omitempty"`
	Lng       *float64 `json:"lng,omitempty"`
	XM        float64  `json:"x_m,omitempty"`
	YM        float64  `json:"y_m,omitempty"`
	AltM      *float64 `json:"alt_m"`
	SpeedMS   *float64 `json:"speed_ms"`
	DistanceM *float64 `json:"distance_m"`
}

type StationAnchor struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

type ResynthesizeOptions struct {
	RepoSeedHex  string
	ScenarioKind string
	Description  string
}

type rawFrame struct {
	originLat float64
	originLng float64
	originXM  float64
	originYM  float64
	useLatLng bool
	cos       float64
	sin       float64
}

func LoadCanonicalPair(canonicalPath, gtPath string) (CanonicalFixture, FixtureGT, error) {
	var fixture CanonicalFixture
	canonicalBytes, err := os.ReadFile(canonicalPath)
	if err != nil {
		return fixture, FixtureGT{}, err
	}
	if err := json.Unmarshal(canonicalBytes, &fixture); err != nil {
		return fixture, FixtureGT{}, err
	}
	if fixture.FormatVersion != FormatVersion {
		return fixture, FixtureGT{}, fmt.Errorf("unsupported canonical format_version %d", fixture.FormatVersion)
	}

	var gt FixtureGT
	gtBytes, err := os.ReadFile(gtPath)
	if err != nil {
		return fixture, gt, err
	}
	if err := yaml.Unmarshal(gtBytes, &gt); err != nil {
		return fixture, gt, err
	}
	if gt.FormatVersion != FormatVersion {
		return fixture, gt, fmt.Errorf("unsupported gt format_version %d", gt.FormatVersion)
	}
	return fixture, gt, nil
}

func ProjectToWGS84(xM, yM float64) (lat, lng float64, err error) {
	const lat0 = 47.0
	const lng0 = 12.0
	lat = lat0 + yM/111320.0
	lng = lng0 + xM/(111320.0*math.Cos(lat0*math.Pi/180.0))
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return 0, 0, fmt.Errorf("projected coordinate out of WGS84 range")
	}
	return lat, lng, nil
}

func CanonicalToStream(f CanonicalFixture) ([]lifts.StreamPoint, error) {
	if f.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("unsupported canonical format_version %d", f.FormatVersion)
	}
	base := time.Unix(0, 0).UTC()
	rows := make([]lifts.StreamPoint, 0, len(f.Samples))
	for i, s := range f.Samples {
		if i > 0 && s.DtMS < f.Samples[i-1].DtMS {
			return nil, fmt.Errorf("sample %d dt_ms is not monotonic", i)
		}
		lat, lng, err := ProjectToWGS84(s.XM, s.YM)
		if err != nil {
			return nil, fmt.Errorf("sample %d: %w", i, err)
		}
		rows = append(rows, lifts.StreamPoint{
			Time:      base.Add(time.Duration(s.DtMS) * time.Millisecond),
			Lat:       &lat,
			Lng:       &lng,
			AltitudeM: s.AltM,
			SpeedMs:   s.SpeedMS,
			DistanceM: s.DistanceM,
		})
	}
	return rows, nil
}

func ComputeFixtureContentHash(f CanonicalFixture, gt FixtureGT) (string, error) {
	if f.FormatVersion != FormatVersion {
		return "", fmt.Errorf("unsupported canonical format_version %d", f.FormatVersion)
	}
	if gt.FormatVersion != FormatVersion {
		return "", fmt.Errorf("unsupported gt format_version %d", gt.FormatVersion)
	}
	var b bytes.Buffer
	writeInt(&b, int64(f.FormatVersion))
	writeString(&b, f.Description)
	writeString(&b, f.ScenarioKind)
	writeInt(&b, int64(f.RidesExpected))
	writeInt(&b, int64(len(f.Samples)))
	for _, s := range f.Samples {
		writeInt(&b, s.DtMS)
		writeFloat(&b, s.XM)
		writeFloat(&b, s.YM)
		writeFloatPtr(&b, s.AltM)
		writeFloatPtr(&b, s.SpeedMS)
		writeFloatPtr(&b, s.DistanceM)
		writeIntPtr(&b, s.HRBPM)
		writeIntPtr(&b, s.PowerW)
		writeFloatPtr(&b, s.TemperatureC)
	}
	writeString(&b, gt.ScenarioKind)
	writeInt(&b, int64(gt.RidesExpected))
	writeInt(&b, int64(len(gt.Rides)))
	for _, r := range gt.Rides {
		writeInt(&b, int64(r.RideIndex))
		writeString(&b, r.Confidence)
		writeString(&b, r.StartAnchor)
		writeString(&b, r.EndAnchor)
		writeFloat(&b, r.PromotionDistance.StartM)
		writeFloat(&b, r.PromotionDistance.EndM)
		writeInt(&b, int64(r.Expected.DurationS))
		writeFloat(&b, r.Expected.AscentM)
		writeFloat(&b, r.Expected.DescentM)
		writeFloat(&b, r.Expected.DistanceM)
		writeTolerance(&b, r.Tolerance)
	}
	keys := make([]string, 0, len(gt.Anchors))
	for k := range gt.Anchors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	writeInt(&b, int64(len(keys)))
	for _, k := range keys {
		writeString(&b, k)
		writeFloat(&b, gt.Anchors[k].XM)
		writeFloat(&b, gt.Anchors[k].YM)
	}
	sum := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(sum[:]), nil
}

func ComputeDetectorVerdictHash(rides []lifts.RideWithIndices) string {
	var b bytes.Buffer
	writeInt(&b, FormatVersion)
	writeInt(&b, int64(len(rides)))
	for _, r := range rides {
		writeInt(&b, int64(r.Ride.DurationS))
		writeFloat(&b, r.Ride.AscentM)
		writeFloat(&b, r.Ride.DescentM)
		writeFloat(&b, math.Round(r.Ride.DistanceM))
		writeString(&b, r.Ride.Confidence)
	}
	sum := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(sum[:8])
}

func AssertRideWithinTolerance(ride lifts.LiftRide, want RideGT, samples []CanonicalSample, anchors map[string]Anchor) error {
	startSample, ok := canonicalSampleAt(samples, ride.StartedAt)
	if !ok {
		return fmt.Errorf("ride start %s has no matching sample", ride.StartedAt.Format(time.RFC3339))
	}
	endSample, ok := canonicalSampleAt(samples, ride.EndedAt)
	if !ok {
		return fmt.Errorf("ride end %s has no matching sample", ride.EndedAt.Format(time.RFC3339))
	}
	startAnchor, ok := anchors[want.StartAnchor]
	if !ok {
		return fmt.Errorf("start anchor %q not found", want.StartAnchor)
	}
	endAnchor, ok := anchors[want.EndAnchor]
	if !ok {
		return fmt.Errorf("end anchor %q not found", want.EndAnchor)
	}
	startDistance := math.Hypot(startSample.XM-startAnchor.XM, startSample.YM-startAnchor.YM)
	startLimit := want.PromotionDistance.StartM + want.Tolerance.StartDistanceGrowthM
	if startDistance > startLimit {
		return fmt.Errorf("start distance %.3fm exceeds promotion baseline %.3fm + growth %.3fm", startDistance, want.PromotionDistance.StartM, want.Tolerance.StartDistanceGrowthM)
	}
	endDistance := math.Hypot(endSample.XM-endAnchor.XM, endSample.YM-endAnchor.YM)
	endLimit := want.PromotionDistance.EndM + want.Tolerance.EndDistanceGrowthM
	if endDistance > endLimit {
		return fmt.Errorf("end distance %.3fm exceeds promotion baseline %.3fm + growth %.3fm", endDistance, want.PromotionDistance.EndM, want.Tolerance.EndDistanceGrowthM)
	}
	if !confidenceAllowed(ride.Confidence, want.Confidence, want.Tolerance.Confidence) {
		return fmt.Errorf("confidence %q outside expected %q with mode %q", ride.Confidence, want.Confidence, want.Tolerance.Confidence)
	}
	if !withinAbs(ride.AscentM, want.Expected.AscentM, want.Tolerance.AscentM) {
		return fmt.Errorf("ascent %.3fm outside expected %.3fm tolerance %.3fm", ride.AscentM, want.Expected.AscentM, want.Tolerance.AscentM)
	}
	if !withinAbs(ride.DescentM, want.Expected.DescentM, want.Tolerance.DescentM) {
		return fmt.Errorf("descent %.3fm outside expected %.3fm tolerance %.3fm", ride.DescentM, want.Expected.DescentM, want.Tolerance.DescentM)
	}
	if !withinAbs(ride.DistanceM, want.Expected.DistanceM, want.Tolerance.DistanceM) {
		return fmt.Errorf("distance %.3fm outside expected %.3fm tolerance %.3fm", ride.DistanceM, want.Expected.DistanceM, want.Tolerance.DistanceM)
	}
	if absInt(ride.DurationS-want.Expected.DurationS) > want.Tolerance.DurationS {
		return fmt.Errorf("duration %ds outside expected %ds tolerance %ds", ride.DurationS, want.Expected.DurationS, want.Tolerance.DurationS)
	}
	return nil
}

func canonicalSampleAt(samples []CanonicalSample, ts time.Time) (CanonicalSample, bool) {
	base := time.Unix(0, 0).UTC()
	dtMS := int64(ts.Sub(base) / time.Millisecond)
	for _, sample := range samples {
		if sample.DtMS == dtMS {
			return sample, true
		}
	}
	return CanonicalSample{}, false
}

func confidenceAllowed(got, want, mode string) bool {
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

func withinAbs(got, want, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance
}

func ValidateExtractOutputPath(outputPath string, localRoots []string) error {
	if outputPath == "" {
		return errors.New("output path is required")
	}
	absOut, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}
	for _, root := range localRoots {
		if root == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(absRoot, absOut)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, "../") && rel != "." {
			return nil
		}
	}
	return fmt.Errorf("extract output must be under a configured local-only directory")
}

func PrivacySelfCheck(listPath string, fixture CanonicalFixture, gt FixtureGT) error {
	if listPath == "" {
		return nil
	}
	terms, err := privacyTerms(listPath)
	if err != nil {
		return err
	}
	fields := []string{fixture.Description, fixture.ScenarioKind}
	for k := range gt.Anchors {
		fields = append(fields, k)
	}
	for _, field := range fields {
		for _, term := range terms {
			if term != "" && strings.Contains(strings.ToLower(field), strings.ToLower(term)) {
				return fmt.Errorf("privacy self-check failed: generated metadata contains a blocked identifier")
			}
		}
	}
	return nil
}

func privacyTerms(listPath string) ([]string, error) {
	data, err := os.ReadFile(listPath)
	if err != nil {
		return nil, err
	}
	var terms []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if left, right, ok := strings.Cut(line, "=>"); ok {
			_ = left
			line = strings.TrimSpace(right)
		}
		if line != "" {
			terms = append(terms, line)
		}
	}
	return terms, nil
}

func ResynthesizeWindow(raw RawWindow, opts ResynthesizeOptions) (CanonicalFixture, FixtureGT, error) {
	seed, err := effectiveSeed(raw, opts.RepoSeedHex)
	if err != nil {
		return CanonicalFixture{}, FixtureGT{}, err
	}
	frame, err := newRawFrame(raw, seed)
	if err != nil {
		return CanonicalFixture{}, FixtureGT{}, err
	}
	samples := make([]CanonicalSample, 0, len(raw.Samples))
	var totalDist float64
	for i, s := range raw.Samples {
		if i > 0 {
			totalDist += rawSegmentDistance(raw.Samples[i-1], s)
		}
		x, y, err := frame.sampleXY(s)
		if err != nil {
			return CanonicalFixture{}, FixtureGT{}, fmt.Errorf("sample %d: %w", i, err)
		}
		dist := s.DistanceM
		if dist == nil {
			d := totalDist
			dist = &d
		}
		samples = append(samples, CanonicalSample{
			DtMS:      s.DtMS,
			XM:        roundMeters(x),
			YM:        roundMeters(y),
			AltM:      copyFloat(s.AltM),
			SpeedMS:   copyFloat(s.SpeedMS),
			DistanceM: copyFloat(dist),
		})
	}
	fixture := CanonicalFixture{
		FormatVersion: FormatVersion,
		Description:   opts.Description,
		ScenarioKind:  opts.ScenarioKind,
		Samples:       samples,
	}
	rows, err := CanonicalToStream(fixture)
	if err != nil {
		return CanonicalFixture{}, FixtureGT{}, err
	}
	rides := lifts.DetectHeuristicWithIndices(rows, lifts.DefaultDetectorConfig(""))
	anchors, err := buildFixtureAnchors(raw, frame, samples)
	if err != nil {
		return CanonicalFixture{}, FixtureGT{}, err
	}
	fixture.RidesExpected = len(rides)
	gt := FixtureGT{
		FormatVersion: FormatVersion,
		ScenarioKind:  opts.ScenarioKind,
		RidesExpected: len(rides),
		Rides:         buildRideGT(rides, samples, anchors),
		Anchors:       anchors,
	}
	hash, err := ComputeFixtureContentHash(fixture, gt)
	if err != nil {
		return CanonicalFixture{}, FixtureGT{}, err
	}
	gt.FixtureContentHash = hash
	gt.DetectorVerdictHash = ComputeDetectorVerdictHash(rides)
	return fixture, gt, nil
}

func MarshalCanonical(f CanonicalFixture, gt FixtureGT) ([]byte, error) {
	canonical, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	gtBytes, err := yaml.Marshal(gt)
	if err != nil {
		return nil, err
	}
	out := append(canonical, '\n')
	out = append(out, "---\n"...)
	out = append(out, gtBytes...)
	return out, nil
}

func effectiveSeed(raw RawWindow, repoSeedHex string) ([32]byte, error) {
	var zero [32]byte
	repoSeed, err := hex.DecodeString(strings.TrimSpace(repoSeedHex))
	if err != nil {
		return zero, fmt.Errorf("decode repo seed: %w", err)
	}
	rawBytes, err := json.Marshal(raw)
	if err != nil {
		return zero, err
	}
	h := sha256.New()
	h.Write(repoSeed)
	rawHash := sha256.Sum256(rawBytes)
	h.Write(rawHash[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}

func newRawFrame(raw RawWindow, seed [32]byte) (rawFrame, error) {
	if len(raw.Samples) == 0 {
		return rawFrame{}, fmt.Errorf("raw window contains no samples")
	}
	angle := seedAngle(seed)
	frame := rawFrame{
		cos: math.Cos(angle),
		sin: math.Sin(angle),
	}
	first := raw.Samples[0]
	if first.Lat != nil && first.Lng != nil {
		frame.originLat = *first.Lat
		frame.originLng = *first.Lng
		frame.useLatLng = true
		return frame, nil
	}
	frame.originXM = first.XM
	frame.originYM = first.YM
	return frame, nil
}

func seedAngle(seed [32]byte) float64 {
	const maxUint64Float = float64(^uint64(0))
	return (float64(binary.BigEndian.Uint64(seed[8:16])) / maxUint64Float) * 2 * math.Pi
}

func (f rawFrame) sampleXY(sample RawSample) (float64, float64, error) {
	var x, y float64
	if f.useLatLng {
		if sample.Lat == nil || sample.Lng == nil {
			return 0, 0, fmt.Errorf("sample is missing lat/lng in lat/lng-backed raw frame")
		}
		x, y = latLngToMeters(f.originLat, f.originLng, *sample.Lat, *sample.Lng)
	} else {
		x = sample.XM - f.originXM
		y = sample.YM - f.originYM
	}
	x, y = rotateXY(x, y, f.cos, f.sin)
	return x, y, nil
}

func (f rawFrame) stationXY(station StationAnchor) (float64, float64, error) {
	if !f.useLatLng {
		return 0, 0, fmt.Errorf("station anchors require lat/lng-backed raw samples")
	}
	x, y := latLngToMeters(f.originLat, f.originLng, station.Lat, station.Lng)
	x, y = rotateXY(x, y, f.cos, f.sin)
	return x, y, nil
}

func rotateXY(x, y, cosAngle, sinAngle float64) (float64, float64) {
	return roundMeters(x*cosAngle - y*sinAngle), roundMeters(x*sinAngle + y*cosAngle)
}

func buildFixtureAnchors(raw RawWindow, frame rawFrame, samples []CanonicalSample) (map[string]Anchor, error) {
	anchors := make(map[string]Anchor)
	if len(raw.StationAnchors) > 0 {
		for i, station := range raw.StationAnchors {
			if station.Name == "" {
				station.Name = fmt.Sprintf("station_%02d", i)
			}
			x, y, err := frame.stationXY(station)
			if err != nil {
				return nil, err
			}
			anchors[station.Name] = Anchor{XM: x, YM: y}
		}
	}
	if len(anchors) == 0 && len(samples) > 0 {
		anchors["start"] = Anchor{XM: 0, YM: 0}
		end := samples[len(samples)-1]
		anchors["end"] = Anchor{XM: end.XM, YM: end.YM}
	}
	return anchors, nil
}

func buildRideGT(rides []lifts.RideWithIndices, samples []CanonicalSample, anchors map[string]Anchor) []RideGT {
	out := make([]RideGT, 0, len(rides))
	for i, ride := range rides {
		startSample := samples[ride.StartSampleIdx]
		endSample := samples[ride.EndSampleIdx]
		startAnchor, startDistance := nearestAnchor(startSample, anchors)
		endAnchor, endDistance := nearestAnchor(endSample, anchors)
		out = append(out, RideGT{
			RideIndex:   i,
			Confidence:  ride.Ride.Confidence,
			StartAnchor: startAnchor,
			EndAnchor:   endAnchor,
			PromotionDistance: PromotionDistance{
				StartM: roundMeters(startDistance),
				EndM:   roundMeters(endDistance),
			},
			Expected: RideExpected{
				DurationS: ride.Ride.DurationS,
				AscentM:   roundMeters(ride.Ride.AscentM),
				DescentM:  roundMeters(ride.Ride.DescentM),
				DistanceM: roundMeters(ride.Ride.DistanceM),
			},
			Tolerance: defaultRideTolerance(),
		})
	}
	return out
}

func nearestAnchor(sample CanonicalSample, anchors map[string]Anchor) (string, float64) {
	keys := make([]string, 0, len(anchors))
	for key := range anchors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	bestName := ""
	bestDistance := math.Inf(1)
	for _, key := range keys {
		anchor := anchors[key]
		distance := math.Hypot(sample.XM-anchor.XM, sample.YM-anchor.YM)
		if distance < bestDistance {
			bestName = key
			bestDistance = distance
		}
	}
	return bestName, bestDistance
}

func defaultRideTolerance() RideTolerance {
	return RideTolerance{
		StartDistanceGrowthM: 30,
		EndDistanceGrowthM:   30,
		AscentM:              15,
		DescentM:             10,
		DistanceM:            50,
		DurationS:            30,
		Confidence:           "class_match",
	}
}

func latLngToMeters(originLat, originLng, lat, lng float64) (x, y float64) {
	const metersPerDegree = 111320.0
	y = (lat - originLat) * metersPerDegree
	x = (lng - originLng) * metersPerDegree * math.Cos(originLat*math.Pi/180.0)
	return x, y
}

func rawSegmentDistance(a, b RawSample) float64 {
	switch {
	case a.Lat != nil && a.Lng != nil && b.Lat != nil && b.Lng != nil:
		return lifts.Haversine(*a.Lat, *a.Lng, *b.Lat, *b.Lng)
	default:
		return math.Hypot(b.XM-a.XM, b.YM-a.YM)
	}
}

func writeTolerance(b *bytes.Buffer, tol RideTolerance) {
	writeFloat(b, tol.StartDistanceGrowthM)
	writeFloat(b, tol.EndDistanceGrowthM)
	writeFloat(b, tol.AscentM)
	writeFloat(b, tol.DescentM)
	writeFloat(b, tol.DistanceM)
	writeInt(b, int64(tol.DurationS))
	writeString(b, tol.Confidence)
}

func writeString(b *bytes.Buffer, s string) {
	writeInt(b, int64(len(s)))
	b.WriteString(s)
}

func writeInt(b *bytes.Buffer, v int64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(v))
	b.Write(buf[:])
}

func writeFloat(b *bytes.Buffer, v float64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], math.Float64bits(v))
	b.Write(buf[:])
}

func writeFloatPtr(b *bytes.Buffer, v *float64) {
	if v == nil {
		b.WriteByte(0)
		return
	}
	b.WriteByte(1)
	writeFloat(b, *v)
}

func writeIntPtr(b *bytes.Buffer, v *int) {
	if v == nil {
		b.WriteByte(0)
		return
	}
	b.WriteByte(1)
	writeInt(b, int64(*v))
}

func copyFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func roundMeters(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
