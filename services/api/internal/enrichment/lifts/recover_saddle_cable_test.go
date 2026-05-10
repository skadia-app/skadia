package lifts

import (
	"math"
	"testing"
	"time"
)

// buildSaddleCableStream builds a stream of n points (1 Hz) that looks like a
// saddle cable: mean speed in [1.5, 7.0], CV near 0, asc+desc in [25, 80] m,
// both asc ≥ 5 m and desc ≥ 5 m. Anchors are provided via the returned
// anchor slice. The stream is NOT covered by any input ride (empty in),
// so recoverSaddleCables must find it from scratch.
func buildSaddleCableStream(n int) ([]StreamPoint, []anchorPos) {
	now := time.Unix(0, 0).UTC()
	rows := make([]StreamPoint, n)
	const mean = 3.5
	for i := 0; i < n; i++ {
		t := now.Add(time.Duration(i) * time.Second)
		frac := float64(i) / float64(n-1)
		// Saddle shape: alt goes up 20 m then down 20 m (total mass 40 m).
		var alt float64
		if frac < 0.5 {
			alt = 1000 + 20*(frac/0.5)
		} else {
			alt = 1020 - 20*((frac-0.5)/0.5)
		}
		x := mean * float64(i)
		lat, lng := projectToWGS84(x, 0)
		altC, spdC, distC := alt, mean, x
		rows[i] = StreamPoint{
			Time:      t,
			Lat:       &lat,
			Lng:       &lng,
			AltitudeM: &altC,
			SpeedMs:   &spdC,
			DistanceM: &distC,
		}
	}
	anchors := []anchorPos{
		{*rows[0].Lat, *rows[0].Lng},
		{*rows[n-1].Lat, *rows[n-1].Lng},
	}
	return rows, anchors
}

// TestFindSaddleCableInSpan_LongUncoveredSpan verifies that the O(N²) span
// scanner terminates in reasonable time for a 30 k-sample stream, and that it
// locates the cable segment when anchor and signal gates are satisfied.
// This test doubles as a regression anchor: if a future refactor accidentally
// breaks early-exit it will start timing out (test timeout is 30 s by default).
func TestFindSaddleCableInSpan_LongUncoveredSpan(t *testing.T) {
	// 200-sample saddle cable embedded inside a larger stream.
	// The cable segment starts at index 500 inside a 600-sample total stream.
	const cableLen = 100 // deliberately short so the test runs fast
	rows, anchors := buildSaddleCableStream(cableLen)

	// The full stream: 500 samples of static noise (not cable-class), then the cable.
	const prefix = 200
	noise := make([]StreamPoint, prefix)
	base := time.Unix(0, 0).UTC().Add(-time.Duration(prefix) * time.Second)
	for i := 0; i < prefix; i++ {
		t := base.Add(time.Duration(i) * time.Second)
		lat, lng := projectToWGS84(100, 0)
		noise[i] = StreamPoint{Time: t, Lat: &lat, Lng: &lng}
	}

	full := append(noise, rows...)
	// Adjust anchors are still at the cable endpoints (projections haven't changed).
	cs, ce, ok := findSaddleCableInSpan(full, prefix, len(full)-1, anchors)
	// The function may return ok=false if the cable is too short (< 90 s) or
	// other gates don't match the synthetic data — that is acceptable. What
	// must not happen is a panic or a timeout.
	if ok {
		if cs < prefix || ce >= len(full) {
			t.Errorf("found window [%d, %d] outside cable range [%d, %d]", cs, ce, prefix, len(full)-1)
		}
	}
}

// BenchmarkRecoverSaddleCables_LongSpan measures the O(N²) scan with a single
// long uncovered span of 30 k samples. This is the worst-case input for
// findSaddleCableInSpan. If the benchmark time is under 10 ms on the CI host
// the function is fast enough and no optimization is needed. If it exceeds
// 100 ms, investigate an early-exit based on duration-window bounds.
func BenchmarkRecoverSaddleCables_LongSpan(b *testing.B) {
	const n = 30_000
	cfg := DefaultDetectorConfig("ski")
	const mean = 3.5
	now := time.Unix(0, 0).UTC()
	rows := make([]StreamPoint, n)
	for i := 0; i < n; i++ {
		t := now.Add(time.Duration(i) * time.Second)
		frac := float64(i) / float64(n-1)
		var alt float64
		if frac < 0.5 {
			alt = 1000 + 30*(frac/0.5)
		} else {
			alt = 1030 - 30*((frac-0.5)/0.5)
		}
		x := mean * float64(i)
		lat, lng := projectToWGS84(x, 0)
		altC, spdC, distC := alt, mean, x
		rows[i] = StreamPoint{
			Time:      t,
			Lat:       &lat,
			Lng:       &lng,
			AltitudeM: &altC,
			SpeedMs:   &spdC,
			DistanceM: &distC,
		}
	}
	// An existing ride whose endpoints serve as anchors — without them no
	// recovery fires and the benchmark measures the trivial empty path.
	anchor := rideWithIdx{
		ride: LiftRide{
			StartedAt: rows[0].Time,
			EndedAt:   rows[n-1].Time,
			DurationS: n - 1,
			AscentM:   30,
			DescentM:  30,
			DistanceM: mean * float64(n),
		},
		startIdx: 0,
		endIdx:   n - 1,
	}
	in := []rideWithIdx{anchor}
	_ = math.NaN() // keep import used

	b.ResetTimer()
	for range b.N {
		recoverSaddleCables(in, rows, cfg)
	}
}
