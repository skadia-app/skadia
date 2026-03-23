package routing

import "testing"

func TestCostMin(t *testing.T) {
	tests := []struct {
		name string
		ecf  EdgeCostFactors
		want float32
	}{
		{"flat lowest", EdgeCostFactors{Flat: 1.0, Up: 2.0, Down: 3.0}, 1.0},
		{"up lowest", EdgeCostFactors{Flat: 5.0, Up: 1.5, Down: 3.0}, 1.5},
		{"down lowest", EdgeCostFactors{Flat: 5.0, Up: 3.0, Down: 0.8}, 0.8},
		{"up zero uses flat", EdgeCostFactors{Flat: 2.0, Up: 0, Down: 3.0}, 2.0},
		{"down zero uses flat", EdgeCostFactors{Flat: 2.0, Up: 3.0, Down: 0}, 2.0},
		{"both zero", EdgeCostFactors{Flat: 1.5, Up: 0, Down: 0}, 1.5},
		{"blocked", EdgeCostFactors{Flat: costBlocked, Up: 0, Down: 0}, costBlocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ecf.CostMin()
			if got != tt.want {
				t.Errorf("CostMin() = %f, want %f", got, tt.want)
			}
		})
	}
}
