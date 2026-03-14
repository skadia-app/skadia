package analytics

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TestCalculateEddington
// ---------------------------------------------------------------------------

func TestCalculateEddington(t *testing.T) {
	tests := []struct {
		name   string
		values []float64 // must be sorted descending (as returned by DB)
		want   int
	}{
		{
			name:   "empty",
			values: []float64{},
			want:   0,
		},
		{
			name:   "single below 1",
			values: []float64{0.5},
			want:   0,
		},
		{
			name:   "single exactly 1",
			values: []float64{1.0},
			want:   1,
		},
		{
			name: "classic example E=3",
			// i=0: 5>=1 ✓, i=1: 4>=2 ✓, i=2: 3>=3 ✓, i=3: 2>=4 ✗ → break
			values: []float64{5, 4, 3, 2, 1},
			want:   3,
		},
		{
			// [4,3,2,1] descending sorted:
			// i=0: 4>=1 ✓ → e=1
			// i=1: 3>=2 ✓ → e=2
			// i=2: 2>=3? ✗ → break  → E=2
			name:   "four descending",
			values: []float64{4, 3, 2, 1},
			want:   2,
		},
		{
			// [15,14,13,12,11,10,10,10,10,10,9,8]
			// i=9: value[9]=10 >= 10 ✓ → e=10
			// i=10: value[10]=9 >= 11 ✗ → break → E=10
			name:   "large plateau",
			values: []float64{15, 14, 13, 12, 11, 10, 10, 10, 10, 10, 9, 8},
			want:   10,
		},
		{
			// buildEddingtonSlice(70) = [70,69,...,1]
			// i=34: value[34]=36 >= 35 ✓ → e=35
			// i=35: value[35]=35 >= 36? ✗ → break → E=35
			name:   "cyclist example E=35",
			values: buildEddingtonSlice(70),
			want:   35,
		},
		{
			name:   "all zeros",
			values: []float64{0, 0, 0},
			want:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateEddington(tc.values)
			if got != tc.want {
				t.Errorf("calculateEddington(%v) = %d, want %d", tc.values, got, tc.want)
			}
		})
	}
}

// buildEddingtonSlice creates a slice of n values where the i-th value
// (0-indexed) equals n-i, producing an Eddington number of exactly n.
func buildEddingtonSlice(n int) []float64 {
	vals := make([]float64, n)
	for i := range n {
		vals[i] = float64(n - i)
	}
	return vals
}

// ---------------------------------------------------------------------------
// TestCalculateStreaks
// ---------------------------------------------------------------------------

func daySeq(dates ...string) []time.Time {
	times := make([]time.Time, len(dates))
	for i, d := range dates {
		t, _ := time.Parse("2006-01-02", d)
		times[i] = t
	}
	return times
}

func TestCalculateDayStreaks(t *testing.T) {
	tests := []struct {
		name           string
		days           []time.Time
		wantMax        int
		wantHistoryLen int // just check number of segments
	}{
		{
			name:           "empty",
			days:           []time.Time{},
			wantMax:        0,
			wantHistoryLen: 0,
		},
		{
			name:           "single day",
			days:           daySeq("2025-01-01"),
			wantMax:        1,
			wantHistoryLen: 1,
		},
		{
			name:           "three consecutive days",
			days:           daySeq("2025-01-01", "2025-01-02", "2025-01-03"),
			wantMax:        3,
			wantHistoryLen: 1,
		},
		{
			name: "two separate streaks",
			days: daySeq(
				"2025-01-01", "2025-01-02", "2025-01-03",
				// gap
				"2025-01-10", "2025-01-11",
			),
			wantMax:        3,
			wantHistoryLen: 2,
		},
		{
			name:           "alternating days (no streak)",
			days:           daySeq("2025-01-01", "2025-01-03", "2025-01-05"),
			wantMax:        1,
			wantHistoryLen: 3,
		},
		{
			name: "long streak then short",
			days: daySeq(
				"2025-01-01", "2025-01-02", "2025-01-03",
				"2025-01-04", "2025-01-05",
				// gap
				"2025-01-10",
			),
			wantMax:        5,
			wantHistoryLen: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, got, history := calculateDayStreaks(tc.days)
			if got != tc.wantMax {
				t.Errorf("maxStreak = %d, want %d", got, tc.wantMax)
			}
			if len(history) != tc.wantHistoryLen {
				t.Errorf("history len = %d, want %d (history=%v)", len(history), tc.wantHistoryLen, history)
			}
		})
	}
}

func TestCalculateWeekStreaks(t *testing.T) {
	tests := []struct {
		name    string
		days    []time.Time
		wantMax int
	}{
		{
			name:    "empty",
			days:    []time.Time{},
			wantMax: 0,
		},
		{
			name:    "single day",
			days:    daySeq("2025-01-06"), // Monday week 2
			wantMax: 1,
		},
		{
			name:    "two days same week",
			days:    daySeq("2025-01-06", "2025-01-08"),
			wantMax: 1,
		},
		{
			name:    "two consecutive weeks",
			days:    daySeq("2025-01-06", "2025-01-13"),
			wantMax: 2,
		},
		{
			name:    "three consecutive weeks",
			days:    daySeq("2025-01-06", "2025-01-13", "2025-01-20"),
			wantMax: 3,
		},
		{
			name:    "week gap breaks streak",
			days:    daySeq("2025-01-06", "2025-01-13", "2025-01-27"),
			wantMax: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, got := calculateWeekStreaks(tc.days)
			if got != tc.wantMax {
				t.Errorf("maxWeekStreak = %d, want %d", got, tc.wantMax)
			}
		})
	}
}
