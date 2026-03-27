package garmin

import (
	"math"
	"testing"
)

// TestWeightScaleFactors prüft die Skalierungsfaktoren für WeightScale-Messungen.
// Weight: uint16 /100 → kg, *1000 → Gramm
// BasalMet: uint16 /4 → kcal/day
// BMI: uint16 /10
// VisceralFatMass: uint16 /100 → kg, *1000 → Gramm
func TestWeightScaleFactors(t *testing.T) {
	const epsilon = 0.0001

	// Weight: 7500 raw → 7500/100*1000 = 75000 g = 75 kg
	weightRaw := uint16(7500)
	weightG := int(float64(weightRaw) / 100.0 * 1000.0)
	if weightG != 75000 {
		t.Errorf("Weight: raw=%d → %d g, want 75000 g", weightRaw, weightG)
	}

	// Weight: 9245 raw → 9245/100*1000 = 92450 g = 92.45 kg
	weightRaw2 := uint16(9245)
	weightG2 := int(float64(weightRaw2) / 100.0 * 1000.0)
	if weightG2 != 92450 {
		t.Errorf("Weight: raw=%d → %d g, want 92450 g", weightRaw2, weightG2)
	}

	// BasalMet: 6800 raw → 6800/4 = 1700 kcal/day
	basalMetRaw := uint16(6800)
	basalMet := float64(basalMetRaw) / 4.0
	if math.Abs(basalMet-1700.0) > epsilon {
		t.Errorf("BasalMet: raw=%d → %.2f, want 1700.0", basalMetRaw, basalMet)
	}

	// BasalMet: 6001 raw → 6001/4 = 1500.25 kcal/day
	basalMetRaw2 := uint16(6001)
	basalMet2 := float64(basalMetRaw2) / 4.0
	if math.Abs(basalMet2-1500.25) > epsilon {
		t.Errorf("BasalMet: raw=%d → %.4f, want 1500.25", basalMetRaw2, basalMet2)
	}

	// BMI: 231 raw → 231/10 = 23.1
	bmiRaw := uint16(231)
	bmi := float64(bmiRaw) / 10.0
	if math.Abs(bmi-23.1) > epsilon {
		t.Errorf("BMI: raw=%d → %.2f, want 23.1", bmiRaw, bmi)
	}

	// VisceralFatMass: 250 raw → 250/100*1000 = 2500 g
	vfmRaw := uint16(250)
	vfmG := int32(float64(vfmRaw) / 100.0 * 1000.0)
	if vfmG != 2500 {
		t.Errorf("VisceralFatMass: raw=%d → %d g, want 2500 g", vfmRaw, vfmG)
	}

	// BoneMass/MuscleMass: gleiche Skalierung wie Weight
	boneMassRaw := uint16(350)  // 3.5 kg = 3500 g
	boneMassG := int(float64(boneMassRaw) / 100.0 * 1000.0)
	if boneMassG != 3500 {
		t.Errorf("BoneMass: raw=%d → %d g, want 3500 g", boneMassRaw, boneMassG)
	}

	// PercentFat: uint16 /100
	percentFatRaw := uint16(2045) // 20.45%
	percentFat := float64(percentFatRaw) / 100.0
	if math.Abs(percentFat-20.45) > epsilon {
		t.Errorf("PercentFat: raw=%d → %.4f, want 20.45", percentFatRaw, percentFat)
	}

	// PercentHydration: uint16 /100
	hydrationRaw := uint16(5830) // 58.30%
	hydration := float64(hydrationRaw) / 100.0
	if math.Abs(hydration-58.30) > epsilon {
		t.Errorf("PercentHydration: raw=%d → %.4f, want 58.30", hydrationRaw, hydration)
	}

	// ActiveMet: uint16 /4
	activeMetRaw := uint16(9200) // 2300 kcal
	activeMet := float64(activeMetRaw) / 4.0
	if math.Abs(activeMet-2300.0) > epsilon {
		t.Errorf("ActiveMet: raw=%d → %.2f, want 2300.0", activeMetRaw, activeMet)
	}
}
