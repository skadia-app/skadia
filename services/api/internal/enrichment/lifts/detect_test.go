package lifts

import (
	"testing"
	"time"
)

// makeStream synthesizes a stream of N points spaced 1 s apart, all with
// lat/lng/alt/speed populated according to the ramp parameters. Used by the
// unit tests to drive specific shapes through DetectHeuristic.
func makeStream(start time.Time, n int, latStart, lngStart, altStart, altRateMs, latRate, speedMs float64) []StreamPoint {
	out := make([]StreamPoint, n)
	for i := 0; i < n; i++ {
		t := start.Add(time.Duration(i) * time.Second)
		la := latStart + latRate*float64(i)
		ln := lngStart
		al := altStart + altRateMs*float64(i)
		sp := speedMs
		out[i] = StreamPoint{
			Time:      t,
			Lat:       &la,
			Lng:       &ln,
			AltitudeM: &al,
			SpeedMs:   &sp,
		}
	}
	return out
}

// TestStraightLiftHigh — 3 min, ~5 m/s constant speed, 1.0 m/s alt rate,
// straight line: must classify as a single high-confidence ride.
func TestStraightLiftHigh(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	now := time.Now()
	// latRate 5e-5 ≈ 5.5 m per second at lat=47 → speed via GPS-delta ~5.5 m/s
	rows := makeStream(now, 200, 47.0, 12.0, 1500, 1.0, 5e-5, 5.0)
	rides := DetectHeuristic(rows, cfg)
	if len(rides) != 1 {
		t.Fatalf("expected 1 ride, got %d", len(rides))
	}
	r := rides[0]
	if r.AscentM < 150 {
		t.Errorf("expected ascent ≥ 150, got %.1f", r.AscentM)
	}
	if r.Confidence != "high" {
		t.Errorf("expected high confidence, got %q", r.Confidence)
	}
	if r.DurationS < 60 {
		t.Errorf("expected duration ≥ 60, got %d", r.DurationS)
	}
}

// TestSpeedZeroFallback — sensor reads 0 throughout but GPS shows steady
// motion. Detector must fall back to GPS-delta speed and still emit a ride.
func TestSpeedZeroFallback(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	now := time.Now()
	// Build with speed=0 then patch every sample (workaround for makeStream)
	rows := makeStream(now, 200, 47.0, 12.0, 1500, 1.0, 5e-5, 0.0)
	zero := 0.0
	for i := range rows {
		rows[i].SpeedMs = &zero
	}
	rides := DetectHeuristic(rows, cfg)
	if len(rides) == 0 {
		t.Fatalf("expected ≥ 1 ride from GPS-delta fallback, got 0")
	}
}

// TestNegativeSteepHikingPath — slow uphill, high sinuosity (zigzag), strict
// gradient threshold. Must NOT emit a ride.
func TestNegativeSteepHikingPath(t *testing.T) {
	cfg := DefaultDetectorConfig("hiking")
	now := time.Now()
	rows := make([]StreamPoint, 200)
	for i := 0; i < 200; i++ {
		t := now.Add(time.Duration(i) * time.Second)
		// zigzag: lat oscillates ±1e-4
		zig := 0.0
		if i%2 == 0 {
			zig = 1e-4
		}
		lat := 47.0 + 1e-5*float64(i) + zig
		lng := 12.0
		alt := 1500.0 + 0.12*float64(i)
		spd := 1.0
		rows[i] = StreamPoint{
			Time:      t,
			Lat:       &lat,
			Lng:       &lng,
			AltitudeM: &alt,
			SpeedMs:   &spd,
		}
	}
	rides := DetectHeuristic(rows, cfg)
	for _, r := range rides {
		t.Errorf("did not expect any ride for steep hiking path, got %+v", r)
	}
}

// TestNegativePassRoad — straight road at 12 m/s, gentle gradient.
// Must NOT emit a ride (gradient too low for unidirectional, sinuosity
// too low to look like a flow trail, but speed is too high anyway).
func TestNegativePassRoad(t *testing.T) {
	cfg := DefaultDetectorConfig("cycling")
	now := time.Now()
	// 1.5 km horizontal, 50 m vertical — gradient ~0.03
	rows := makeStream(now, 200, 47.0, 12.0, 1500, 0.25, 1.4e-4, 12.0)
	rides := DetectHeuristic(rows, cfg)
	for _, r := range rides {
		t.Errorf("did not expect any ride for pass road, got %+v", r)
	}
}

func TestContinuousRoadClimbWithoutBoundaryCueRejected(t *testing.T) {
	cfg := DefaultDetectorConfig("cycling")
	now := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	rows := make([]StreamPoint, 0, 441)
	tcur := now
	lat := 47.0
	lng := 12.0
	alt := 900.0
	appendPoint := func(speed float64) {
		la, ln, al, spd := lat, lng, alt, speed
		rows = append(rows, StreamPoint{
			Time:      tcur,
			Lat:       &la,
			Lng:       &ln,
			AltitudeM: &al,
			SpeedMs:   &spd,
		})
	}
	appendSegment := func(seconds int, altStepM, speed float64) {
		for range seconds {
			tcur = tcur.Add(time.Second)
			lat += 3.1e-5
			alt += altStepM
			appendPoint(speed)
		}
	}
	appendPoint(3.4)
	appendSegment(120, 0.00, 3.4)
	appendSegment(240, 0.52, 3.4)
	appendSegment(80, 0.10, 3.4)

	rides := DetectHeuristic(rows, cfg)
	if len(rides) != 0 {
		t.Fatalf("expected no ride for steady uphill road motion, got %+v", rides)
	}
}

// TestEffectiveSpeedFloor — when SpeedMs is below GPSDeltaSpeedFloorMs,
// effectiveSpeed must fall through to GPS-delta even on a single sample.
func TestEffectiveSpeedFloor(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	now := time.Now()
	la1, ln1, al1 := 47.0, 12.0, 1500.0
	la2, ln2, al2 := 47.000045, 12.0, 1501.0
	zero := 0.0
	rows := []StreamPoint{
		{Time: now, Lat: &la1, Lng: &ln1, AltitudeM: &al1, SpeedMs: &zero},
		{Time: now.Add(1 * time.Second), Lat: &la2, Lng: &ln2, AltitudeM: &al2, SpeedMs: &zero},
	}
	v, ok := effectiveSpeedCfg(rows, 1, cfg)
	if !ok {
		t.Fatalf("expected ok from GPS-delta fallback")
	}
	if v < 4 || v > 6 {
		t.Errorf("expected GPS-delta speed ~5 m/s, got %.2f", v)
	}
}

func TestTrailApproachBeforeBoardingStopStartsAtStop(t *testing.T) {
	cfg := DefaultDetectorConfig("mtb_downhill")
	now := time.Date(2023, 7, 16, 14, 25, 0, 0, time.UTC)
	rows := makeSyntheticLiftApproach(now)

	var boardingStop time.Time
	for _, p := range rows {
		if p.SpeedMs != nil && *p.SpeedMs < 0.7 && p.Time.After(now.Add(3*time.Minute)) {
			boardingStop = p.Time
			break
		}
	}
	if boardingStop.IsZero() {
		t.Fatal("synthetic stream missing boarding stop")
	}

	rides := DetectHeuristic(rows, cfg)
	if len(rides) != 1 {
		t.Fatalf("expected 1 ride, got %d", len(rides))
	}
	if rides[0].StartedAt.Before(boardingStop) {
		t.Fatalf("expected ride start at or after boarding stop %s, got %s",
			boardingStop.Format("15:04:05"), rides[0].StartedAt.Format("15:04:05"))
	}
}

func makeSyntheticLiftApproach(start time.Time) []StreamPoint {
	type builder struct {
		rows []StreamPoint
		t    time.Time
		lat  float64
		lng  float64
		alt  float64
	}
	b := builder{
		t:   start,
		lat: 46.728,
		lng: 9.553,
		alt: 1550,
	}
	appendPoint := func(speed float64) {
		lat, lng, alt, spd := b.lat, b.lng, b.alt, speed
		b.rows = append(b.rows, StreamPoint{
			Time:      b.t,
			Lat:       &lat,
			Lng:       &lng,
			AltitudeM: &alt,
			SpeedMs:   &spd,
		})
	}
	appendPoint(0)
	appendSegment := func(seconds int, lngRateDeg, altRateM, speed float64) {
		for i := 0; i < seconds; i++ {
			b.t = b.t.Add(time.Second)
			b.lng += lngRateDeg
			b.alt += altRateM
			appendPoint(speed)
		}
	}

	// Fast MTB trail descent immediately before the lift approach.
	appendSegment(90, -0.00013, -0.8, 10.0)
	// Straight rolling approach: lift-like enough to fool the initial window.
	appendSegment(180, -0.00004, 0.34, 3.0)
	// Sustained boarding stop at the valley station.
	appendSegment(20, -0.000001, 0.02, 0.3)
	// Actual slow cable climb.
	appendSegment(620, -0.00003, 0.35, 2.3)
	return b.rows
}
