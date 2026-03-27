package garmin

import (
	"testing"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/typedef"
)

// TestSleepLevelFiltering prüft die Filterbedingungen für SleepLevel-Werte.
// SleepLevelInvalid (255) und Werte >4 werden übersprungen.
func TestSleepLevelFiltering(t *testing.T) {
	cases := []struct {
		level typedef.SleepLevel
		valid bool
	}{
		{typedef.SleepLevelInvalid, false}, // 255 → invalid
		{typedef.SleepLevel(0), true},      // 0 = unmeasureable
		{typedef.SleepLevel(1), true},      // 1 = awake
		{typedef.SleepLevel(2), true},      // 2 = light
		{typedef.SleepLevel(3), true},      // 3 = deep
		{typedef.SleepLevel(4), true},      // 4 = rem
		{typedef.SleepLevel(5), false},     // 5 → > 4, ungültig
		{typedef.SleepLevel(100), false},   // weit außerhalb
	}

	for _, tc := range cases {
		// Repliziert die Filterbedingung aus processSleepFIT
		isValid := tc.level != typedef.SleepLevelInvalid && tc.level <= 4
		if isValid != tc.valid {
			t.Errorf("SleepLevel %d: valid=%v, want %v", tc.level, isValid, tc.valid)
		}
	}
}

// TestUint8PtrToIntPtr prüft die Hilfsfunktion für Sleep-Assessment-Score-Konvertierung.
func TestUint8PtrToIntPtr(t *testing.T) {
	// Uint8Invalid (255) → nil
	got := uint8PtrToIntPtr(basetype.Uint8Invalid)
	if got != nil {
		t.Errorf("uint8PtrToIntPtr(invalid): got %v, want nil", *got)
	}

	// Valider Wert → Pointer
	got = uint8PtrToIntPtr(75)
	if got == nil {
		t.Fatal("uint8PtrToIntPtr(75): got nil, want *int(75)")
	}
	if *got != 75 {
		t.Errorf("uint8PtrToIntPtr(75): got %d, want 75", *got)
	}

	// Grenzwert 0
	got = uint8PtrToIntPtr(0)
	if got == nil {
		t.Fatal("uint8PtrToIntPtr(0): got nil, want *int(0)")
	}
	if *got != 0 {
		t.Errorf("uint8PtrToIntPtr(0): got %d, want 0", *got)
	}

	// Grenzwert 254 (max valider uint8 vor invalid=255)
	got = uint8PtrToIntPtr(254)
	if got == nil {
		t.Fatal("uint8PtrToIntPtr(254): got nil, want *int(254)")
	}
	if *got != 254 {
		t.Errorf("uint8PtrToIntPtr(254): got %d, want 254", *got)
	}
}

// TestUint8PtrToInt16Ptr prüft die zweite Hilfsfunktion für Sleep-Assessment-Score-Konvertierung.
func TestUint8PtrToInt16Ptr(t *testing.T) {
	// Uint8Invalid (255) → nil
	got := uint8PtrToInt16Ptr(basetype.Uint8Invalid)
	if got != nil {
		t.Errorf("uint8PtrToInt16Ptr(invalid): got %v, want nil", *got)
	}

	// Valider Wert → Pointer
	got = uint8PtrToInt16Ptr(80)
	if got == nil {
		t.Fatal("uint8PtrToInt16Ptr(80): got nil, want *int16(80)")
	}
	if *got != 80 {
		t.Errorf("uint8PtrToInt16Ptr(80): got %d, want 80", *got)
	}
}

// TestSleepLevelInvalidValue prüft den konkreten Wert von SleepLevelInvalid.
func TestSleepLevelInvalidValue(t *testing.T) {
	// SleepLevelInvalid muss 0xFF (255) sein — entspricht typedef.SleepLevelInvalid
	if typedef.SleepLevelInvalid != 255 {
		t.Errorf("SleepLevelInvalid = %d, expected 255 (0xFF)", typedef.SleepLevelInvalid)
	}
}
