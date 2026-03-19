package geo

import (
	"math"
	"testing"

	"github.com/paulmach/orb"
)

func TestHausdorffDistance(t *testing.T) {
	t.Run("identical lines returns 0", func(t *testing.T) {
		ls := orb.LineString{
			{8.5417, 47.3769},
			{8.5617, 47.3869},
			{8.5817, 47.3769},
		}
		got := HausdorffDistance(ls, ls)
		if got != 0.0 {
			t.Errorf("HausdorffDistance(same, same) = %f, want 0", got)
		}
	})

	t.Run("parallel lines ~100m apart", func(t *testing.T) {
		// Two parallel east-west lines ~100m apart (1 degree lat ≈ 111320m)
		// 100m ≈ 0.0009 degrees
		ls1 := orb.LineString{
			{8.5000, 47.3000},
			{8.6000, 47.3000},
		}
		ls2 := orb.LineString{
			{8.5000, 47.3009}, // ~100m north
			{8.6000, 47.3009},
		}
		got := HausdorffDistance(ls1, ls2)
		gotM := got
		if gotM < 50 || gotM > 150 {
			t.Errorf("HausdorffDistance(parallel ~100m) = %.1f m, want ~100m", gotM)
		}
	})

	t.Run("asymmetric lines", func(t *testing.T) {
		// Line A has a point far from line B, so H(A,B) > H(B,A)
		ls1 := orb.LineString{
			{8.5000, 47.3000},
			{8.5000, 47.4000}, // ~11km north
		}
		ls2 := orb.LineString{
			{8.5000, 47.3000},
			{8.5000, 47.3100}, // short ~1km
		}
		got := HausdorffDistance(ls1, ls2)
		// H(ls1, ls2) = max(directed(ls1→ls2), directed(ls2→ls1))
		// directed(ls1→ls2): end of ls1 (47.4) is ~11km from nearest in ls2 (47.31) ≈ ~10km
		if got < 5000 {
			t.Errorf("HausdorffDistance asymmetric = %.1f m, want > 5000m", got)
		}
	})

	t.Run("empty input returns Inf", func(t *testing.T) {
		ls := orb.LineString{{8.5, 47.3}}
		got := HausdorffDistance(orb.LineString{}, ls)
		if !math.IsInf(got, 1) {
			t.Errorf("HausdorffDistance(empty, ls) = %f, want +Inf", got)
		}
		got2 := HausdorffDistance(ls, orb.LineString{})
		if !math.IsInf(got2, 1) {
			t.Errorf("HausdorffDistance(ls, empty) = %f, want +Inf", got2)
		}
	})
}

func TestHausdorffDistancePlanar(t *testing.T) {
	t.Run("identical lines returns 0", func(t *testing.T) {
		ls := orb.LineString{
			{0, 0},
			{100, 0},
			{100, 100},
		}
		got := HausdorffDistancePlanar(ls, ls)
		if got != 0.0 {
			t.Errorf("HausdorffDistancePlanar(same, same) = %f, want 0", got)
		}
	})

	t.Run("parallel lines 100 units apart", func(t *testing.T) {
		ls1 := orb.LineString{
			{0, 0},
			{1000, 0},
		}
		ls2 := orb.LineString{
			{0, 100},
			{1000, 100},
		}
		got := HausdorffDistancePlanar(ls1, ls2)
		if math.Abs(got-100) > 0.001 {
			t.Errorf("HausdorffDistancePlanar(parallel 100) = %f, want 100", got)
		}
	})

	t.Run("empty input returns Inf", func(t *testing.T) {
		ls := orb.LineString{{0, 0}}
		got := HausdorffDistancePlanar(orb.LineString{}, ls)
		if !math.IsInf(got, 1) {
			t.Errorf("HausdorffDistancePlanar(empty, ls) = %f, want +Inf", got)
		}
	})
}
