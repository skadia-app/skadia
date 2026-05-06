package lifts

import (
	"math"
	"testing"
	"time"
)

type gateStreamBuilder struct {
	rows []StreamPoint
	t    time.Time
	x    float64
	y    float64
	alt  float64
}

func newGateStreamBuilder() *gateStreamBuilder {
	b := &gateStreamBuilder{
		t:   time.Unix(0, 0).UTC(),
		alt: 1000,
	}
	b.append(0)
	return b
}

func (b *gateStreamBuilder) mark() int {
	return len(b.rows) - 1
}

func (b *gateStreamBuilder) add(seconds int, dxM, dyM, altStepM, speedMs float64) {
	for range seconds {
		b.t = b.t.Add(time.Second)
		b.x += dxM
		b.y += dyM
		b.alt += altStepM
		b.append(speedMs)
	}
}

func (b *gateStreamBuilder) addCustom(seconds int, step func(i int) (dxM, dyM, altStepM, speedMs float64)) {
	for i := range seconds {
		dx, dy, altStep, speed := step(i)
		b.t = b.t.Add(time.Second)
		b.x += dx
		b.y += dy
		b.alt += altStep
		b.append(speed)
	}
}

func (b *gateStreamBuilder) addGap(seconds int, dxM, dyM, altStepM, speedMs float64) {
	b.t = b.t.Add(time.Duration(seconds) * time.Second)
	b.x += dxM
	b.y += dyM
	b.alt += altStepM
	b.append(speedMs)
}

func (b *gateStreamBuilder) append(speedMs float64) {
	lat, lng := projectToWGS84(b.x, b.y)
	alt := b.alt
	speed := speedMs
	b.rows = append(b.rows, StreamPoint{
		Time:      b.t,
		Lat:       &lat,
		Lng:       &lng,
		AltitudeM: &alt,
		SpeedMs:   &speed,
	})
}

func gateRide(rows []StreamPoint, start, end int, confidence string, cfg DetectorConfig) rideWithIdx {
	st := computeStats(rows, start, end, cfg)
	return rideWithIdx{
		ride: LiftRide{
			StartedAt:  rows[start].Time,
			EndedAt:    rows[end].Time,
			DurationS:  int(rows[end].Time.Sub(rows[start].Time).Seconds()),
			AscentM:    st.altPosDelta,
			DescentM:   st.altNegDelta,
			DistanceM:  st.cumDist,
			Confidence: confidence,
		},
		startIdx: start,
		endIdx:   end,
	}
}

func TestSmartRecordingGapRejectsInitialWindowAndSpeedFallback(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	b := newGateStreamBuilder()
	b.add(10, 4, 0, 0.8, 0)
	gapIdx := b.mark() + 1
	b.addGap(cfg.SmartRecMaxGapS+10, 4, 0, 0.8, 0)

	if _, _, ok := buildInitialWindow(b.rows, 0, cfg); ok {
		t.Fatal("buildInitialWindow accepted a window across a smart-recording gap")
	}
	if speed, ok := effectiveSpeedCfg(b.rows, gapIdx, cfg); ok {
		t.Fatalf("effectiveSpeedCfg returned %.2f across a smart-recording gap", speed)
	}
}

func TestClassifyMediumFallbackWithoutUsableSpeed(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	ws := stats{
		altsOK:    true,
		altRateMs: cfg.NoSpeedMinAltRateMs + 0.15,
		durationS: float64(cfg.NoSpeedMinDurationS + 10),
		sinuosity: 1.0,
	}
	if got := classify(ws, cfg); got != "medium" {
		t.Fatalf("classify() = %q, want medium", got)
	}
	if !validateRide(ws, "medium", cfg) {
		t.Fatal("validateRide rejected a no-speed medium fallback window")
	}
}

func TestClassifyAndValidateBidirectionalFallback(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	ws := stats{
		altsOK:      true,
		speedsOK:    true,
		meanSpeed:   7.0,
		speedCV:     0.01,
		sinuosity:   cfg.BidirSinuosityMax - 0.01,
		altRateMs:   0.02,
		altPosDelta: 80,
		altNegDelta: 70,
		durationS:   float64(cfg.BidirMinDurationS + 30),
	}
	if got := classify(ws, cfg); got != "bidir" {
		t.Fatalf("classify() = %q, want bidir", got)
	}
	if !validateRide(ws, "bidir", cfg) {
		t.Fatal("validateRide rejected a bidirectional cable-shaped window")
	}

	pureAscent := ws
	pureAscent.altNegDelta = 0
	if validateRide(pureAscent, "bidir", cfg) {
		t.Fatal("validateRide accepted a bidirectional candidate without both altitude directions")
	}

	slow := ws
	slow.meanSpeed = 3.5
	if validateRide(slow, "bidir", cfg) {
		t.Fatal("validateRide accepted a bidirectional candidate below the cable-speed floor")
	}
}

func TestValidateHighRejectsHikeBandWithoutCableLockStep(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	ws := stats{
		altsOK:      true,
		speedsOK:    true,
		meanSpeed:   1.8,
		speedCV:     0.02,
		sinuosity:   1.04,
		altRateMs:   0.30,
		altPosDelta: 70,
		durationS:   240,
	}
	if validateRide(ws, "high", cfg) {
		t.Fatal("validateRide accepted a hike-band candidate without cable-lock-step geometry")
	}

	cableLockStep := ws
	cableLockStep.sinuosity = 1.01
	if !validateRide(cableLockStep, "high", cfg) {
		t.Fatal("validateRide rejected a hike-band candidate with cable-lock-step geometry")
	}
}

func TestValidateHighRejectsSlowBandLowGradient(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	ws := stats{
		altsOK:      true,
		speedsOK:    true,
		meanSpeed:   1.5,
		speedCV:     0.01,
		sinuosity:   1.0,
		altRateMs:   0.20,
		altPosDelta: 70,
		durationS:   300,
	}
	if validateRide(ws, "high", cfg) {
		t.Fatal("validateRide accepted a slow-band low-gradient candidate")
	}
}

func TestValidateHighRejectsAscendingMonotonicityRegression(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	ws := stats{
		altsOK:      true,
		speedsOK:    true,
		meanSpeed:   3.0,
		speedCV:     0.01,
		sinuosity:   1.0,
		altRateMs:   0.60,
		altPosDelta: 100,
		altNegDelta: 20,
		durationS:   180,
	}
	if validateRide(ws, "high", cfg) {
		t.Fatal("validateRide accepted an ascending ride with excessive descent component")
	}
}

func TestSpeedCVTrimIgnoresBoundaryAcceleration(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	b := newGateStreamBuilder()
	b.add(30, 1, 0, 0.8, 1)
	b.add(120, 4, 0, 0.8, 4)
	b.add(30, 1, 0, 0.8, 1)

	ws := computeStats(b.rows, 0, b.mark(), cfg)
	if ws.speedCV >= cfg.MaxSpeedCV {
		t.Fatalf("trimmed speedCV = %.3f, want below %.3f", ws.speedCV, cfg.MaxSpeedCV)
	}
}

func TestBackwardExtensionRelaxedCVAdmitsBoardingAcceleration(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	b := newGateStreamBuilder()
	b.addCustom(70, func(i int) (float64, float64, float64, float64) {
		speed := 0.8 + 3.2*float64(i)/69.0
		return speed, 0, 0.65, speed
	})
	stableStart := b.mark()
	b.add(160, 4, 0, 0.8, 4)

	rides := DetectHeuristic(b.rows, cfg)
	if len(rides) != 1 {
		t.Fatalf("DetectHeuristic returned %d rides, want 1", len(rides))
	}
	if !rides[0].StartedAt.Before(b.rows[stableStart].Time) {
		t.Fatalf("ride start %s did not move before stable cable sample %s",
			rides[0].StartedAt, b.rows[stableStart].Time)
	}
}

func TestBoundarySpeedDropRequiresFlankingStop(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	b := newGateStreamBuilder()
	b.add(30, 0, 0, 0, 0)
	start := b.mark()
	b.add(120, 4, 0, 0.8, 4)
	end := b.mark()
	b.add(30, 0, 0, 0, 0)

	if !boundarySpeedDropOK(b.rows, start, end, cfg) {
		t.Fatal("boundarySpeedDropOK missed flanking stationary samples")
	}

	moving := newGateStreamBuilder()
	moving.add(30, 3, 0, 0.2, 3)
	start = moving.mark()
	moving.add(120, 4, 0, 0.8, 4)
	end = moving.mark()
	moving.add(30, 3, 0, 0.2, 3)
	if boundarySpeedDropOK(moving.rows, start, end, cfg) {
		t.Fatal("boundarySpeedDropOK accepted a candidate without a boundary speed drop")
	}
}

func TestRoadClimbExitBoundaryRequiresStopOrFastDeparture(t *testing.T) {
	cfg := DefaultDetectorConfig("cycling")
	b := newGateStreamBuilder()
	b.add(120, 4, 0, 0.8, 4)
	end := b.mark()
	b.add(60, 3, 0, 0.1, 3)
	if roadClimbExitBoundaryOK(b.rows, end, cfg) {
		t.Fatal("roadClimbExitBoundaryOK accepted a slow continuous exit without cue")
	}

	fast := newGateStreamBuilder()
	fast.add(120, 4, 0, 0.8, 4)
	end = fast.mark()
	fast.add(60, 9, 0, 0.1, 9)
	if !roadClimbExitBoundaryOK(fast.rows, end, cfg) {
		t.Fatal("roadClimbExitBoundaryOK rejected a fast departure cue")
	}
}

func TestExtendCandidateStopsOnStopOverflow(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	b := newGateStreamBuilder()
	b.add(100, 4, 0, 0.8, 4)
	stopStart := b.mark()
	b.add(cfg.StopToleranceS+30, 0, 0, 0, 0)

	ws := computeStats(b.rows, 0, cfg.WindowDurationS, cfg)
	end := extendCandidate(b.rows, 0, cfg.WindowDurationS, "high", ws, cfg)
	if end > stopStart+1 {
		t.Fatalf("extendCandidate ended at %d, want near stop start %d", end, stopStart)
	}
}

func TestExtendCandidateBidirStopsOnCurvedTail(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	b := newGateStreamBuilder()
	b.add(150, 6, 0, 0.0, 6)
	straightEnd := b.mark()
	b.addCustom(90, func(i int) (float64, float64, float64, float64) {
		dy := 5.0
		if i%2 == 0 {
			dy = -5.0
		}
		return 1.0, dy, 0, 6
	})

	ws := computeStats(b.rows, 0, cfg.WindowDurationS, cfg)
	end := extendCandidate(b.rows, 0, cfg.WindowDurationS, "bidir", ws, cfg)
	if end >= b.mark()-10 {
		t.Fatalf("extendCandidate bidir end = %d, want before curved tail end %d", end, b.mark())
	}
	if end < straightEnd-30 {
		t.Fatalf("extendCandidate bidir ended too early at %d before straight section end %d", end, straightEnd)
	}
}

func TestExtendCandidateBackwardUsesCablePlateauBypass(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	b := newGateStreamBuilder()
	b.add(60, 4, 0, 0.8, 4)
	b.add(35, 4, 0, 0.0, 4)
	oldStart := b.mark() + 1
	b.add(120, 4, 0, 0.8, 4)
	end := b.mark()

	ws := computeStats(b.rows, oldStart, end, cfg)
	newStart := extendCandidateBackward(b.rows, oldStart, end, "high", ws, cfg, 0)
	if newStart >= oldStart-30 {
		t.Fatalf("extendCandidateBackward start = %d, want it to cross cable-speed plateau before %d", newStart, oldStart)
	}

	queue := newGateStreamBuilder()
	queue.add(60, 4, 0, 0.8, 4)
	queue.add(70, 1, 0, 0.0, 1)
	oldStart = queue.mark() + 1
	queue.add(120, 4, 0, 0.8, 4)
	end = queue.mark()
	ws = computeStats(queue.rows, oldStart, end, cfg)
	newStart = extendCandidateBackward(queue.rows, oldStart, end, "high", ws, cfg, 0)
	if newStart < oldStart-10 {
		t.Fatalf("extendCandidateBackward crossed slow boarding plateau: start = %d, oldStart = %d", newStart, oldStart)
	}
}

func TestBackwardStallMeanSpeedDistinguishesCablePlateauFromQueue(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	cable := newGateStreamBuilder()
	cable.add(40, 4, 0, 0, 4)
	if got := backwardStallMeanSpeed(cable.rows, 1, cable.mark(), cfg); got < 3.5 {
		t.Fatalf("backwardStallMeanSpeed cable mean = %.2f, want cable-speed band", got)
	}

	queue := newGateStreamBuilder()
	queue.add(40, 1, 0, 0, 1)
	if got := backwardStallMeanSpeed(queue.rows, 1, queue.mark(), cfg); got >= 2.5 {
		t.Fatalf("backwardStallMeanSpeed queue mean = %.2f, want below cable plateau threshold", got)
	}
}

func TestTrimLeadingLowGradientTrimsBoardingShuffleButKeepsCableCruise(t *testing.T) {
	boarding := newGateStreamBuilder()
	boarding.add(70, 1.2, 0, 0.10, 1.2)
	boarding.add(180, 4, 0, 0.85, 4)
	got := trimLeadingLowGradient(boarding.rows, 0, boarding.mark(), "high")
	if got == 0 {
		t.Fatal("trimLeadingLowGradient did not trim boarding shuffle")
	}

	cable := newGateStreamBuilder()
	cable.add(70, 4, 0, 0.10, 4)
	cable.add(180, 4, 0, 0.85, 4)
	got = trimLeadingLowGradient(cable.rows, 0, cable.mark(), "high")
	if got != 0 {
		t.Fatalf("trimLeadingLowGradient trimmed cable-cruise plateau to %d", got)
	}
}

func TestTrimTrailingLowGradientTrimsWalkOffButKeepsCableContinuation(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	walkOff := newGateStreamBuilder()
	walkOff.add(180, 4, 0, 0.8, 4)
	walkOff.addCustom(80, func(i int) (float64, float64, float64, float64) {
		dy := 1.5
		if i%2 == 0 {
			dy = -1.5
		}
		return 1.0, dy, 0.0, 1.2
	})
	end := walkOff.mark()
	if got := trimTrailingLowGradient(walkOff.rows, 0, end, "high", cfg); got >= end {
		t.Fatalf("trimTrailingLowGradient returned %d, want below %d", got, end)
	}

	cable := newGateStreamBuilder()
	cable.add(180, 4, 0, 0.8, 4)
	cable.add(80, 4, 0, 0.0, 4)
	end = cable.mark()
	if got := trimTrailingLowGradient(cable.rows, 0, end, "high", cfg); got != end {
		t.Fatalf("trimTrailingLowGradient trimmed cable continuation to %d, want %d", got, end)
	}
}

func TestTrimTrailingHighSinuosityTrimsCurvedWalkOff(t *testing.T) {
	b := newGateStreamBuilder()
	b.add(180, 4, 0, 0.8, 4)
	b.addCustom(50, func(i int) (float64, float64, float64, float64) {
		dy := 6.0
		if i%2 == 0 {
			dy = -6.0
		}
		return 0.5, dy, -0.1, 4
	})
	end := b.mark()
	if got := trimTrailingHighSinuosity(b.rows, 0, end, "high"); got >= end {
		t.Fatalf("trimTrailingHighSinuosity returned %d, want below %d", got, end)
	}
	if got := trimTrailingHighSinuosity(b.rows, 0, end, "bidir"); got != end {
		t.Fatalf("trimTrailingHighSinuosity trimmed bidir ride to %d, want %d", got, end)
	}
}

func TestTrimTrailingDisembarkDiscontinuityFindsAltRateStep(t *testing.T) {
	b := newGateStreamBuilder()
	b.add(120, 4, 0, 0.8, 4)
	b.add(40, 2, 0, 0.02, 2)
	end := b.mark()
	if got := trimTrailingDisembarkDiscontinuity(b.rows, 0, end, "high"); got >= end {
		t.Fatalf("trimTrailingDisembarkDiscontinuity returned %d, want below %d", got, end)
	}
	if got := trimTrailingDisembarkDiscontinuity(b.rows, 0, end, "bidir"); got != end {
		t.Fatalf("trimTrailingDisembarkDiscontinuity trimmed bidir ride to %d, want %d", got, end)
	}
}

func TestTrimTrailingLowSpeedSamplesDropsStationaryTail(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	b := newGateStreamBuilder()
	b.add(120, 5, 0, 0.8, 5)
	stopStart := b.mark()
	b.add(6, 0, 0, 0, 0)
	if got := trimTrailingLowSpeedSamples(b.rows, 0, b.mark(), cfg); got > stopStart {
		t.Fatalf("trimTrailingLowSpeedSamples returned %d, want at or before %d", got, stopStart)
	}
}

func TestTrimTrailingAltPlateauTrimsWalkOffButKeepsCableCruise(t *testing.T) {
	walkOff := newGateStreamBuilder()
	walkOff.add(100, 5, 0, 0.5, 5)
	walkOff.add(8, 2, 0, 0.0, 2)
	end := walkOff.mark()
	if got := trimTrailingAltPlateau(walkOff.rows, 0, end); got >= end {
		t.Fatalf("trimTrailingAltPlateau returned %d, want below %d", got, end)
	}

	cable := newGateStreamBuilder()
	cable.add(100, 5, 0, 0.5, 5)
	cable.add(8, 4, 0, 0.0, 4)
	end = cable.mark()
	if got := trimTrailingAltPlateau(cable.rows, 0, end); got != end {
		t.Fatalf("trimTrailingAltPlateau trimmed cable-cruise plateau to %d, want %d", got, end)
	}
}

func TestTrimTrailingChunksDropsSlowTail(t *testing.T) {
	b := newGateStreamBuilder()
	b.add(120, 4, 0, 0.7, 4)
	b.add(40, 1, 0, 0.05, 1)
	end := b.mark()
	if got := trimTrailingChunks(b.rows, 0, end); got >= end {
		t.Fatalf("trimTrailingChunks returned %d, want below %d", got, end)
	}
}

func TestMergeMidLiftSplitSubclasses(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")

	t.Run("standard merge", func(t *testing.T) {
		b := newGateStreamBuilder()
		b.add(80, 4, 0, 2.0, 4)
		prevEnd := b.mark()
		b.add(5, 4, 0, 2.0, 4)
		curStart := b.mark()
		b.add(80, 4, 0, 2.0, 4)
		in := []rideWithIdx{
			gateRide(b.rows, 0, prevEnd, "medium", cfg),
			gateRide(b.rows, curStart, b.mark(), "medium", cfg),
		}
		if got := mergeMidLiftSplits(in, b.rows, cfg); len(got) != 1 {
			t.Fatalf("mergeMidLiftSplits returned %d rides, want 1", len(got))
		}
	})

	t.Run("continuous hold samples", func(t *testing.T) {
		b := newGateStreamBuilder()
		b.add(80, 4, 0, 2.0, 4)
		prevEnd := b.mark()
		b.add(180, 0, 0, 0, 0)
		curStart := b.mark()
		b.add(80, 4, 0, 2.0, 4)
		in := []rideWithIdx{
			gateRide(b.rows, 0, prevEnd, "medium", cfg),
			gateRide(b.rows, curStart, b.mark(), "medium", cfg),
		}
		if got := mergeMidLiftSplits(in, b.rows, cfg); len(got) != 1 {
			t.Fatalf("mergeMidLiftSplits returned %d rides, want 1", len(got))
		}
	})

	t.Run("smart recording hold", func(t *testing.T) {
		b := newGateStreamBuilder()
		b.add(80, 4, 0, 2.0, 4)
		prevEnd := b.mark()
		b.addGap(180, 10, 0, 10, 4)
		curStart := b.mark()
		b.add(80, 4, 0, 2.0, 4)
		in := []rideWithIdx{
			gateRide(b.rows, 0, prevEnd, "medium", cfg),
			gateRide(b.rows, curStart, b.mark(), "medium", cfg),
		}
		if got := mergeMidLiftSplits(in, b.rows, cfg); len(got) != 1 {
			t.Fatalf("mergeMidLiftSplits returned %d rides, want 1", len(got))
		}
	})

	t.Run("bidir no merge", func(t *testing.T) {
		b := newGateStreamBuilder()
		b.add(80, 4, 0, 2.0, 4)
		prevEnd := b.mark()
		b.add(5, 4, 0, 2.0, 4)
		curStart := b.mark()
		b.add(80, 4, 0, 2.0, 4)
		in := []rideWithIdx{
			gateRide(b.rows, 0, prevEnd, "bidir", cfg),
			gateRide(b.rows, curStart, b.mark(), "medium", cfg),
		}
		if got := mergeMidLiftSplits(in, b.rows, cfg); len(got) != 2 {
			t.Fatalf("mergeMidLiftSplits returned %d rides, want 2", len(got))
		}
	})

	t.Run("direction mismatch", func(t *testing.T) {
		b := newGateStreamBuilder()
		b.add(80, 4, 0, 2.0, 4)
		prevEnd := b.mark()
		b.add(5, 4, 0, -2.0, 4)
		curStart := b.mark()
		b.add(80, 4, 0, -2.0, 4)
		in := []rideWithIdx{
			gateRide(b.rows, 0, prevEnd, "medium", cfg),
			gateRide(b.rows, curStart, b.mark(), "medium", cfg),
		}
		if got := mergeMidLiftSplits(in, b.rows, cfg); len(got) != 2 {
			t.Fatalf("mergeMidLiftSplits returned %d rides, want 2", len(got))
		}
	})

	t.Run("merged validation", func(t *testing.T) {
		b := newGateStreamBuilder()
		b.add(80, 4, 0, 0.2, 4)
		prevEnd := b.mark()
		b.add(20, 1, 0, 0, 1)
		curStart := b.mark()
		b.add(80, 4, 0, 0.2, 4)
		in := []rideWithIdx{
			gateRide(b.rows, 0, prevEnd, "high", cfg),
			gateRide(b.rows, curStart, b.mark(), "high", cfg),
		}
		if got := mergeMidLiftSplits(in, b.rows, cfg); len(got) != 2 {
			t.Fatalf("mergeMidLiftSplits returned %d rides, want 2", len(got))
		}
	})
}

func TestSinuosityTailPeelAcceptsLeadingSubWindow(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	b := newGateStreamBuilder()
	b.add(140, 4, 0, 0.8, 4)
	validEnd := b.mark()
	b.addCustom(120, func(i int) (float64, float64, float64, float64) {
		angle := float64(i) / 8
		return 1.2, 3 * math.Sin(angle), 0.0, 2.5
	})

	full := computeStats(b.rows, 0, b.mark(), cfg)
	if validateRide(full, "high", cfg) {
		t.Fatal("full synthetic stitched window unexpectedly validates")
	}
	leading := computeStats(b.rows, 0, validEnd, cfg)
	if !validateRide(leading, "high", cfg) {
		t.Fatal("leading sub-window should validate after a tail-peel retry")
	}
}
