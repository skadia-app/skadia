package handler

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing"
)

func mkLeg(dist, quality float32) *routing.RouteResult {
	return &routing.RouteResult{
		DistanceM:  float64(dist),
		DistSum:    dist,
		QualitySum: dist * quality,
	}
}

func TestPassesQualityGate_GoodLeg1_GoodLeg2(t *testing.T) {
	opts := DefaultRoundTripOptions()
	if !passesQualityGate(mkLeg(5000, 1.1), mkLeg(5000, 1.2), 1.4, opts) {
		t.Error("small diff should pass")
	}
}

func TestPassesQualityGate_GoodLeg1_BadLeg2(t *testing.T) {
	opts := DefaultRoundTripOptions()
	if passesQualityGate(mkLeg(5000, 0.9), mkLeg(5000, 3.5), 1.4, opts) {
		t.Error("bad leg2 should fail both gates")
	}
}

func TestPassesQualityGate_BaselineClamp(t *testing.T) {
	opts := DefaultRoundTripOptions()
	// leg2 AvgFlat=1.9, diff from leg1=1.0 (>0.5), but 1.9 <= 1.5*1.4=2.1
	if !passesQualityGate(mkLeg(5000, 0.9), mkLeg(5000, 1.9), 1.4, opts) {
		t.Error("leg2 within baseline should pass")
	}
}

func TestPassesQualityGate_DistanceGate(t *testing.T) {
	opts := DefaultRoundTripOptions()
	// 2x longer, quality OK
	if passesQualityGate(mkLeg(5000, 1.0), mkLeg(10000, 1.1), 1.4, opts) {
		t.Error("2x longer should fail distance gate (limit 1.8x)")
	}
}

func TestPassesQualityGate_LongButQualitative(t *testing.T) {
	opts := DefaultRoundTripOptions()
	// 1.7x longer, quality OK
	if !passesQualityGate(mkLeg(5000, 1.0), mkLeg(8500, 1.2), 1.4, opts) {
		t.Error("1.7x longer quality leg2 should pass")
	}
}
