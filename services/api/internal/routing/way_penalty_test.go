package routing

import "testing"

func TestApplyPenaltyToECF_Monotone(t *testing.T) {
	ecf := EdgeCostFactors{Flat: 1.5, Up: 2.0, Down: 1.0, WayClass: 3}
	original := ecf
	applyPenaltyToECF(&ecf, 1.3)
	if ecf.Flat <= original.Flat {
		t.Errorf("Flat not increased: %v → %v", original.Flat, ecf.Flat)
	}
	if ecf.Up <= original.Up {
		t.Errorf("Up not increased: %v → %v", original.Up, ecf.Up)
	}
	if ecf.Down <= original.Down {
		t.Errorf("Down not increased: %v → %v", original.Down, ecf.Down)
	}
	if ecf.WayClass != original.WayClass {
		t.Errorf("WayClass changed: %v → %v", original.WayClass, ecf.WayClass)
	}
}

func TestApplyPenaltyToECF_Identity(t *testing.T) {
	ecf := EdgeCostFactors{Flat: 1.5, Up: 2.0, Down: 1.0}
	original := ecf
	applyPenaltyToECF(&ecf, 1.0)
	if ecf != original {
		t.Errorf("penalty=1.0 should be no-op, got %v", ecf)
	}
}

func TestApplyPenaltyToECF_PanicOnSubUnity(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for penalty < 1.0")
		}
	}()
	ecf := EdgeCostFactors{Flat: 1.5}
	applyPenaltyToECF(&ecf, 0.8)
}

func TestLookupWayPenalty_NilMap(t *testing.T) {
	got := lookupWayPenalty(123, nil, nil)
	if got != 1.0 {
		t.Errorf("nil penalties should return 1.0, got %v", got)
	}
}
