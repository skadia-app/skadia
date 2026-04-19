package graph

import "testing"

func TestMtbScaleShort(t *testing.T) {
	cases := map[uint8]string{0: "", 1: "S0", 2: "S1", 3: "S2", 6: "S5", 7: ""}
	for v, want := range cases {
		if got := MtbScaleShort(v); got != want {
			t.Errorf("MtbScaleShort(%d)=%q want %q", v, got, want)
		}
	}
}

func TestSacScaleShort(t *testing.T) {
	if SacScaleShort(4) != "T4" {
		t.Errorf("SacScaleShort(4) want T4")
	}
	if SacScaleShort(0) != "" || SacScaleShort(7) != "" {
		t.Errorf("SacScaleShort bounds")
	}
}

func TestViaFerrataScaleShort(t *testing.T) {
	if ViaFerrataScaleShort(2) != "K2" {
		t.Errorf("ViaFerrataScaleShort(2) want K2")
	}
}
