package database

import "testing"

func TestIsValidSacList(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"T1", true},
		{"T1,T2,T3", true},
		{"T6", true},
		{"T7", false},    // ungültig
		{"T0", false},    // ungültig
		{"T1,T9", false}, // eines ungültig
		{"'; DROP TABLE", false},
	}
	for _, tt := range tests {
		if got := IsValidSacList(tt.in); got != tt.want {
			t.Errorf("IsValidSacList(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestIsValidMtbList(t *testing.T) {
	if !IsValidMtbList("S0,S1,S2,S3,S4,S5") {
		t.Error("S0-S5 should be valid")
	}
	if IsValidMtbList("S6") {
		t.Error("S6 should be invalid")
	}
}

func TestIsValidSurfaceList(t *testing.T) {
	if !IsValidSurfaceList("Asphalt,Schotter (grob)") {
		t.Error("standard surfaces should be valid")
	}
	if IsValidSurfaceList("Asphalt,Wechselnd") {
		t.Error("Wechselnd should not be filterable")
	}
	if IsValidSurfaceList("'; DROP") {
		t.Error("injection attempt")
	}
}
