package dem

import "testing"

func TestNoOpReader(t *testing.T) {
	r := &NoOpReader{}
	defer r.Close()

	if got := r.ElevationAt(47.0, 8.0); got != NoData {
		t.Errorf("ElevationAt = %d, want %d", got, NoData)
	}

	batch := r.ElevationBatch([]int32{470000000}, []int32{80000000})
	if len(batch) != 1 || batch[0] != NoData {
		t.Errorf("ElevationBatch = %v, want [%d]", batch, NoData)
	}
}
