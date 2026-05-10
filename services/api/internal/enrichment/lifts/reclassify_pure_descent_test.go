package lifts

import (
	"testing"
	"time"
)

// buildDescentStream synthesizes a stream of n points, 1 s apart, with
// constant speed and a linear altitude profile going from altStart to altEnd.
// Direction is a straight east-going line so sinuosity ≈ 1.0.
func buildDescentStream(n int, speedMs, altStart, altEnd float64) []StreamPoint {
	now := time.Unix(0, 0).UTC()
	out := make([]StreamPoint, n)
	for i := 0; i < n; i++ {
		t := now.Add(time.Duration(i) * time.Second)
		frac := float64(i) / float64(n-1)
		alt := altStart + (altEnd-altStart)*frac
		// straight east-going: lat constant, lng grows.
		x := speedMs * float64(i)
		lat, lng := projectToWGS84(x, 0)
		altC, spdC := alt, speedMs
		distC := x
		out[i] = StreamPoint{
			Time:      t,
			Lat:       &lat,
			Lng:       &lng,
			AltitudeM: &altC,
			SpeedMs:   &spdC,
			DistanceM: &distC,
		}
	}
	return out
}

// wrapRide wraps a stream as a single rideWithIdx covering all samples.
func wrapRide(rows []StreamPoint, confidence string, ascentM, descentM float64) rideWithIdx {
	return rideWithIdx{
		ride: LiftRide{
			StartedAt:  rows[0].Time,
			EndedAt:    rows[len(rows)-1].Time,
			DurationS:  len(rows) - 1,
			AscentM:    ascentM,
			DescentM:   descentM,
			DistanceM:  *rows[len(rows)-1].DistanceM,
			Confidence: confidence,
		},
		startIdx: 0,
		endIdx:   len(rows) - 1,
	}
}

func TestReclassifyPureDescentRides(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")

	type tc struct {
		name       string
		rows       []StreamPoint
		ride       rideWithIdx
		wantConf   string
	}

	// Canonical pure-descent gondola: 511 m drop over 350 s, asc 0.3 m, sin ≈ 1.0,
	// mean ~4.4 m/s, CV ≈ 0. Should promote to "high".
	descentRows := buildDescentStream(351, 4.4, 1511, 1000)
	descentRide := wrapRide(descentRows, "bidir", 0.3, 511.0)

	// mass=0 edge: both asc and desc are 0. Cannot reclassify (divide-by-zero guard).
	zeroMassRows := buildDescentStream(200, 4.4, 1000, 1000)
	zeroMassRide := wrapRide(zeroMassRows, "bidir", 0.0, 0.0)

	// Balanced asc/desc (bidir saddle): asc 50 m, desc 50 m. Ascent ratio = 0.50 → rejected.
	balancedRows := buildDescentStream(200, 4.4, 1000, 950)
	balancedRide := wrapRide(balancedRows, "bidir", 50.0, 50.0)

	// sin gate boundary: high sinuosity (speed=5, lateral oscillation via
	// buildDescentStream gives sin≈1; we force descent reject via ascentRatio).
	// Use ascent > 5 m absolute to trip the absolute ascent gate.
	highAscentRows := buildDescentStream(200, 4.4, 1511, 1000)
	highAscentRide := wrapRide(highAscentRows, "bidir", 6.0, 511.0) // ascentM > 5.0 → rejected

	// DescentM below reclassMinDescentM (50 m). Too shallow — should stay "bidir".
	shallowRows := buildDescentStream(200, 4.4, 1040, 1000)
	shallowRide := wrapRide(shallowRows, "bidir", 0.1, 40.0) // descent < 50 m

	// confidence != "bidir" should not be touched.
	highAlreadyRows := buildDescentStream(200, 4.4, 1511, 1000)
	highAlreadyRide := wrapRide(highAlreadyRows, "high", 0.0, 511.0)

	// Mean speed below reclassMinMeanSpeedMs (2.5 m/s).
	slowRows := buildDescentStream(351, 1.0, 1511, 1000)
	slowRide := wrapRide(slowRows, "bidir", 0.3, 511.0)

	cases := []tc{
		{
			name:     "canonical_gondola_descent_promotes",
			rows:     descentRows,
			ride:     descentRide,
			wantConf: "high",
		},
		{
			name:     "mass_zero_stays_bidir",
			rows:     zeroMassRows,
			ride:     zeroMassRide,
			wantConf: "bidir",
		},
		{
			name:     "balanced_asc_desc_stays_bidir",
			rows:     balancedRows,
			ride:     balancedRide,
			wantConf: "bidir",
		},
		{
			name:     "absolute_ascent_too_high_stays_bidir",
			rows:     highAscentRows,
			ride:     highAscentRide,
			wantConf: "bidir",
		},
		{
			name:     "descent_below_floor_stays_bidir",
			rows:     shallowRows,
			ride:     shallowRide,
			wantConf: "bidir",
		},
		{
			name:     "non_bidir_input_untouched",
			rows:     highAlreadyRows,
			ride:     highAlreadyRide,
			wantConf: "high",
		},
		{
			name:     "slow_mean_speed_stays_bidir",
			rows:     slowRows,
			ride:     slowRide,
			wantConf: "bidir",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := []rideWithIdx{c.ride}
			out := reclassifyPureDescentRides(in, c.rows, cfg)
			if len(out) != 1 {
				t.Fatalf("output len = %d, want 1", len(out))
			}
			if out[0].ride.Confidence != c.wantConf {
				t.Errorf("confidence = %q, want %q", out[0].ride.Confidence, c.wantConf)
			}
		})
	}
}

// TestReclassifyPureDescentRides_AltRateBoundary verifies the altRateMs > -0.05
// rejection gate: a bidir ride with net-zero altitude (saddle shape,
// altRateMs ≈ 0) must not be promoted even when descent > 50 m and
// ascent ratio is tiny.
func TestReclassifyPureDescentRides_AltRateBoundary(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	// A "saddle" stream: goes up 30 m then back down. Net altRate ≈ 0.
	n := 400
	now := time.Unix(0, 0).UTC()
	rows := make([]StreamPoint, n)
	for i := 0; i < n; i++ {
		t := now.Add(time.Duration(i) * time.Second)
		// Altitude: up 30 m then back to start over the stream.
		halfFrac := float64(i) / float64(n/2)
		var alt float64
		if i < n/2 {
			alt = 1000 + 30*halfFrac
		} else {
			alt = 1030 - 30*(float64(i-n/2)/float64(n/2))
		}
		x := 4.4 * float64(i)
		lat, lng := projectToWGS84(x, 0)
		spdC, altC, distC := 4.4, alt, x
		rows[i] = StreamPoint{
			Time:      t,
			Lat:       &lat,
			Lng:       &lng,
			AltitudeM: &altC,
			SpeedMs:   &spdC,
			DistanceM: &distC,
		}
	}
	// Claim the ride has 0.3 m ascent, 55 m descent (mimics a bidir saddle claim).
	ride := rideWithIdx{
		ride: LiftRide{
			StartedAt:  rows[0].Time,
			EndedAt:    rows[n-1].Time,
			DurationS:  n - 1,
			AscentM:    0.3,
			DescentM:   55.0,
			DistanceM:  *rows[n-1].DistanceM,
			Confidence: "bidir",
		},
		startIdx: 0,
		endIdx:   n - 1,
	}
	out := reclassifyPureDescentRides([]rideWithIdx{ride}, rows, cfg)
	// Net altitude of this stream is ≈ 0, so altRateMs > -0.05 → must stay "bidir".
	if out[0].ride.Confidence != "bidir" {
		t.Errorf("saddle ride promoted to %q, want bidir (altRate near zero)", out[0].ride.Confidence)
	}
}
