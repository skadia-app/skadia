package database

import (
	"slices"
	"testing"
)

func TestNormalizeCountryCodes(t *testing.T) {
	got := normalizeCountryCodes([]string{" ch ", "IT", "ch", "", "CHE", "d", "1!", "D3"})
	want := []string{"CH", "IT"}
	if !slices.Equal(got, want) {
		t.Fatalf("normalizeCountryCodes() = %#v, want %#v", got, want)
	}
}
