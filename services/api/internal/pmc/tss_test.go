package pmc

import (
	"math"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// --- NP Tests ---

func TestCalcNP_ConstantPower(t *testing.T) {
	stream := make([]float64, 60)
	for i := range stream { stream[i] = 200 }
	np := CalcNP(stream)
	if math.Abs(np-200) > 0.1 {
		t.Errorf("CalcNP constant 200W = %f, want 200", np)
	}
}

func TestCalcNP_VariablePower(t *testing.T) {
	// Blocks of 60s at 300W then 60s at 100W: variation is coarser than the
	// 30s SMA window, so NP > average power (200W) due to the 4th-power weighting.
	stream := make([]float64, 120)
	for i := 0; i < 60; i++ { stream[i] = 300 }
	for i := 60; i < 120; i++ { stream[i] = 100 }
	np := CalcNP(stream)
	if np <= 200 {
		t.Errorf("CalcNP variable power = %f, want > 200", np)
	}
}

func TestCalcNP_TooShort(t *testing.T) {
	stream := make([]float64, 20)
	for i := range stream { stream[i] = 200 }
	if np := CalcNP(stream); np != 0 {
		t.Errorf("CalcNP short stream = %f, want 0", np)
	}
}

func TestCalcNP_WithZeros(t *testing.T) {
	stream := make([]float64, 60)
	for i := 0; i < 30; i++ { stream[i] = 200 }
	np := CalcNP(stream)
	if np >= 200 {
		t.Errorf("CalcNP with zeros = %f, want < 200", np)
	}
}

func TestRepairDropouts(t *testing.T) {
	stream := []float64{200, 200, 0, 0, 200, 200}
	repaired := repairDropouts(stream)
	if repaired[2] == 0 || repaired[3] == 0 {
		t.Error("repairDropouts should interpolate short gaps")
	}
}

func TestRepairDropouts_LongGap(t *testing.T) {
	stream := make([]float64, 20)
	stream[0] = 200; stream[1] = 200
	// Gap of 10 zeros
	stream[12] = 200; stream[13] = 200
	repaired := repairDropouts(stream)
	if repaired[5] != 0 {
		t.Error("repairDropouts should NOT interpolate gaps > 5 samples")
	}
}

// --- hrTSS Tests ---

func TestCalcHrTSS_OneHourAtLTHR(t *testing.T) {
	stream := make([]int, 3600)
	for i := range stream { stream[i] = 170 }
	tss := CalcHrTSS(stream, 170, 60, 190)
	if math.Abs(tss-100) > 1.0 {
		t.Errorf("CalcHrTSS 1h at LTHR = %f, want ~100", tss)
	}
}

func TestCalcHrTSS_BelowRestingHR(t *testing.T) {
	stream := make([]int, 60)
	for i := range stream { stream[i] = 50 }
	tss := CalcHrTSS(stream, 170, 60, 190)
	if tss != 0 {
		t.Errorf("CalcHrTSS below resting = %f, want 0", tss)
	}
}

func TestCalcHrTSS_InvalidLTHR(t *testing.T) {
	stream := make([]int, 60)
	for i := range stream { stream[i] = 120 }
	tss := CalcHrTSS(stream, 50, 60, 190)
	if tss != 0 {
		t.Errorf("CalcHrTSS invalid LTHR = %f, want 0", tss)
	}
}

func TestCalcHrTSSAvg_OneHourAtLTHR(t *testing.T) {
	tss := CalcHrTSSAvg(170, 170, 60, 3600)
	if math.Abs(tss-100) > 1.0 {
		t.Errorf("CalcHrTSSAvg 1h at LTHR = %f, want ~100", tss)
	}
}

// --- Edwards TRIMP Tests ---

func TestCalcEdwardsTRIMP_AllZone1(t *testing.T) {
	maxHR, restHR := 200, 60
	targetHR := restHR + int(0.55*float64(maxHR-restHR))
	stream := make([]int, 60)
	for i := range stream { stream[i] = targetHR }
	trimp := CalcEdwardsTRIMP(stream, restHR, maxHR)
	if math.Abs(trimp-1.0) > 0.1 {
		t.Errorf("Edwards TRIMP 60s zone1 = %f, want ~1.0", trimp)
	}
}

func TestCalcEdwardsTRIMP_AllZone5(t *testing.T) {
	maxHR, restHR := 200, 60
	targetHR := restHR + int(0.95*float64(maxHR-restHR))
	stream := make([]int, 60)
	for i := range stream { stream[i] = targetHR }
	trimp := CalcEdwardsTRIMP(stream, restHR, maxHR)
	if math.Abs(trimp-5.0) > 0.1 {
		t.Errorf("Edwards TRIMP 60s zone5 = %f, want ~5.0", trimp)
	}
}

func TestCalcEdwardsTRIMP_InvalidHRRange(t *testing.T) {
	// restingHR=200 > maxHR=60 → hrRange negative → 0
	trimp := CalcEdwardsTRIMP([]int{120}, 200, 60)
	if trimp != 0 {
		t.Errorf("Edwards TRIMP invalid range = %f, want 0", trimp)
	}
}

// --- Minetti / NGP Tests ---

func TestMinettiCr_Flat(t *testing.T) {
	cr := minettiCr(0)
	if math.Abs(cr-3.6) > 0.01 {
		t.Errorf("minettiCr(0) = %f, want 3.6", cr)
	}
}

func TestMinettiCr_Uphill10(t *testing.T) {
	cr := minettiCr(0.10)
	if cr < 5.0 || cr > 6.5 {
		t.Errorf("minettiCr(0.10) = %f, want ~5.77", cr)
	}
}

func TestCalcNGP_FlatTerrain(t *testing.T) {
	speeds := make([]float64, 60)
	grades := make([]float64, 60)
	for i := range speeds { speeds[i] = 3.0 }
	ngp := CalcNGP(speeds, grades, nil)
	if math.Abs(ngp-3.0) > 0.1 {
		t.Errorf("CalcNGP flat = %f, want ~3.0", ngp)
	}
}

// --- sTSS Tests ---

func TestCalcSwimTSS(t *testing.T) {
	// 3000m in 3300s, CSS = 1.0 m/s → IF=0.909 → sTSS≈75.7
	tss := CalcSwimTSS(3000, 3300, 1.0)
	if math.Abs(tss-75.7) > 1.0 {
		t.Errorf("CalcSwimTSS = %f, want ~75.7", tss)
	}
}

func TestCalcSwimTSS_ZeroInputs(t *testing.T) {
	if CalcSwimTSS(0, 3300, 1.0) != 0 { t.Error("zero distance") }
	if CalcSwimTSS(3000, 0, 1.0) != 0 { t.Error("zero time") }
	if CalcSwimTSS(3000, 3300, 0) != 0 { t.Error("zero CSS") }
}

// --- ComputeTSS Orchestrator Tests ---

func TestComputeTSS_PowerPreferred(t *testing.T) {
	th := &Thresholds{FTP: 200, LTHR: 170, RestingHR: 60, MaxHR: 190,
		Sources: map[string]string{}}
	power := make([]float64, 60)
	hr := make([]int, 60)
	for i := range power { power[i] = 200; hr[i] = 150 }
	r := ComputeTSS(2, 0, 3600, nil, nil, nil, power, hr, nil, nil, nil, th)
	if r.TSSMethod != "power_np" {
		t.Errorf("method = %s, want power_np", r.TSSMethod)
	}
	if r.TRIMP == nil {
		t.Error("TRIMP should be computed even when power is primary")
	}
	if r.ComputedNPW == nil {
		t.Error("computed NP should be set")
	}
	if r.ComputedIF == nil {
		t.Error("computed IF should be set")
	}
}

func TestComputeTSS_FallbackToHrTSS(t *testing.T) {
	th := &Thresholds{FTP: 200, LTHR: 170, RestingHR: 60, MaxHR: 190,
		Sources: map[string]string{}}
	hr := make([]int, 3600)
	for i := range hr { hr[i] = 150 }
	r := ComputeTSS(2, 0, 3600, nil, nil, nil, nil, hr, nil, nil, nil, th)
	if r.TSSMethod != "hr_stream" {
		t.Errorf("method = %s, want hr_stream", r.TSSMethod)
	}
	if r.ComputedIF != nil {
		t.Error("hrTSS should have nil IF")
	}
}

func TestComputeTSS_NoData(t *testing.T) {
	th := &Thresholds{FTP: 200, LTHR: 170, RestingHR: 60, MaxHR: 190,
		Sources: map[string]string{}}
	r := ComputeTSS(2, 0, 3600, nil, nil, nil, nil, nil, nil, nil, nil, th)
	if r.TSSMethod != "none" {
		t.Errorf("method = %s, want none", r.TSSMethod)
	}
	if r.ComputedTSS != nil {
		t.Error("no-data should have nil TSS")
	}
}

func TestComputeTSS_PowerAvgFallback(t *testing.T) {
	th := &Thresholds{FTP: 200, LTHR: 170, RestingHR: 60, MaxHR: 190,
		Sources: map[string]string{}}
	avgP := 180.0
	// Short power stream (<30) + avgPower → power_avg
	r := ComputeTSS(2, 0, 3600, &avgP, nil, nil, make([]float64, 10), nil, nil, nil, nil, th)
	if r.TSSMethod != "power_avg" {
		t.Errorf("method = %s, want power_avg", r.TSSMethod)
	}
}

func TestComputeTSS_StrengthTraining(t *testing.T) {
	th := &Thresholds{FTP: 200, LTHR: 170, RestingHR: 60, MaxHR: 190,
		Sources: map[string]string{}}
	hr := make([]int, 1800)
	for i := range hr { hr[i] = 140 }
	r := ComputeTSS(10, 0, 1800, nil, nil, nil, nil, hr, nil, nil, nil, th)
	if r.TSSMethod != "hr_stream" {
		t.Errorf("strength training method = %s, want hr_stream", r.TSSMethod)
	}
}

func TestComputeTSS_SwimmingWithCSS(t *testing.T) {
	css := 1.0
	th := &Thresholds{FTP: 200, LTHR: 170, RestingHR: 60, MaxHR: 190,
		SpeedThreshold: &css, Sources: map[string]string{}}
	dist := 3000.0
	r := ComputeTSS(5, 0, 3300, nil, nil, &dist, nil, nil, nil, nil, nil, th)
	if r.TSSMethod != "pace_swim" {
		t.Errorf("swim method = %s, want pace_swim", r.TSSMethod)
	}
	if r.ComputedIF == nil {
		t.Error("swim should have IF")
	}
}

func TestComputeTSS_HrAvgFallback(t *testing.T) {
	th := &Thresholds{FTP: 200, LTHR: 170, RestingHR: 60, MaxHR: 190,
		Sources: map[string]string{}}
	avgHR := 150.0
	// No streams at all, only avgHR
	r := ComputeTSS(2, 0, 3600, nil, &avgHR, nil, nil, nil, nil, nil, nil, th)
	if r.TSSMethod != "hr_avg" {
		t.Errorf("method = %s, want hr_avg", r.TSSMethod)
	}
}

func TestExtractPowerStream_SkipsNullRows(t *testing.T) {
	// Simulate Apple Watch: alternating power/no-power rows per timestamp
	points := make([]model.StreamPoint, 120)
	now := time.Now()
	for i := 0; i < 120; i++ {
		points[i].Time = now.Add(time.Duration(i/2) * time.Second) // 2 rows per second
		if i%2 == 0 {
			// Power row
			p := int16(200)
			points[i].PowerW = &p
		}
		// Odd rows: PowerW = nil (speed-only subtype)
	}

	stream := ExtractPowerStream(points)

	// Should contain 60 values (one per unique timestamp), not 120
	if len(stream) != 60 {
		t.Errorf("len = %d, want 60 (deduplicated by timestamp)", len(stream))
	}
	// All values should be 200, no zeros from NULL rows
	for i, v := range stream {
		if v != 200 {
			t.Errorf("stream[%d] = %f, want 200", i, v)
			break
		}
	}
}
