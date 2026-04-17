package routing

import (
	"math"
	"testing"
)

func ptr32(v float32) *float32 { return &v }

func TestComputeZScores_UniformDimension(t *testing.T) {
	metrics := []CandidateMetrics{
		{DistanceM: 1000, AscentM: 100, AvgFlat: 0.5, NodeID: 1},
		{DistanceM: 1000, AscentM: 200, AvgFlat: 0.6, NodeID: 2},
		{DistanceM: 1000, AscentM: 300, AvgFlat: 0.7, NodeID: 3},
	}
	zs, active := computeZScores(metrics)

	// Dim 0 = Distance: all same → inactive
	if active[0] {
		t.Error("expected active[0] == false for uniform distance")
	}
	for i := range zs {
		if zs[i][0] != 0 {
			t.Errorf("expected zs[%d][0] == 0, got %v", i, zs[i][0])
		}
	}

	// Dim 1 = Ascent: values differ → active
	if !active[1] {
		t.Error("expected active[1] == true for varying ascent")
	}
}

func TestComputeZScores_AllNilDim(t *testing.T) {
	metrics := []CandidateMetrics{
		{DistanceM: 1000, AscentM: 100, AvgFlat: 0.5, GreenScore: nil, NodeID: 1},
		{DistanceM: 2000, AscentM: 200, AvgFlat: 0.6, GreenScore: nil, NodeID: 2},
	}
	_, active := computeZScores(metrics)

	// Dim 3 = GreenScore: all nil → inactive
	if active[3] {
		t.Error("expected active[3] == false for all-nil GreenScore")
	}
}

func TestImputeNils_HalfNil(t *testing.T) {
	v1 := 1.0
	v3 := 3.0
	input := []*float64{&v1, nil, &v3, nil}
	out := imputeNils(input)

	// Median of [1.0, 3.0] = 2.0
	expected := []float64{1.0, 2.0, 3.0, 2.0}
	for i, e := range expected {
		if out[i] != e {
			t.Errorf("imputeNils[%d]: expected %v, got %v", i, e, out[i])
		}
	}
}

func TestMeanAndStd_Basic(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5}
	mean, std := meanAndStd(vals)
	if math.Abs(mean-3.0) > 1e-9 {
		t.Errorf("mean: expected 3.0, got %v", mean)
	}
	expectedStd := math.Sqrt(2.0)
	if math.Abs(std-expectedStd) > 1e-9 {
		t.Errorf("std: expected %v, got %v", expectedStd, std)
	}
}

func TestComputeMedian_Even(t *testing.T) {
	m := computeMedian([]float64{1, 2, 3, 4})
	if m != 2.5 {
		t.Errorf("expected 2.5, got %v", m)
	}
}

func TestComputeMedian_Odd(t *testing.T) {
	m := computeMedian([]float64{1, 2, 3})
	if m != 2.0 {
		t.Errorf("expected 2.0, got %v", m)
	}
}
