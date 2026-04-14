package geocoding

import "testing"

func TestRoundCoord(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{47.26891234, 47.269},
		{11.3931112, 11.393},
		{-0.0004999, 0.000},
		{-0.0005001, -0.001},
		{47.2685, 47.269}, // banker's? — math.Round rounds half-away-from-zero
		{0, 0},
	}
	for _, c := range cases {
		got := RoundCoord(c.in)
		// allow tiny FP drift
		if got < c.want-1e-9 || got > c.want+1e-9 {
			t.Errorf("RoundCoord(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCacheKey(t *testing.T) {
	cases := []struct {
		in   float64
		want int
	}{
		{47.26891234, 47269},
		{11.3931112, 11393},
		{-47.26891234, -47269},
		{0, 0},
	}
	for _, c := range cases {
		if got := cacheKey(c.in); got != c.want {
			t.Errorf("cacheKey(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestResult_Empty covers the negative-cache sentinel used by the cache layer:
// an empty *Result must round-trip through Store→Lookup as a cache hit, and
// Empty() is how the hot path distinguishes it from a positive hit.
func TestResult_Empty(t *testing.T) {
	if !(*Result)(nil).Empty() {
		t.Error("nil Result must report Empty=true")
	}
	if !(&Result{}).Empty() {
		t.Error("zero-value Result must report Empty=true")
	}
	if (&Result{City: "X"}).Empty() {
		t.Error("Result{City:X} must NOT be Empty")
	}
	if (&Result{CountryCode: "AT"}).Empty() {
		t.Error("Result{CountryCode:AT} must NOT be Empty")
	}
}
