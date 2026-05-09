package database

// TDD coverage for m.4: typed ModifierKey constant set.

import "testing"

// TestModifierKeyColumnReturnsActivityColumn verifies that each of the four
// defined ModifierKey constants resolves to the correct activities column name
// and returns ok=true.
func TestModifierKeyColumnReturnsActivityColumn(t *testing.T) {
	cases := []struct {
		key  ModifierKey
		want string
	}{
		{ModifierIndoor, "is_indoor"},
		{ModifierCommute, "is_commute"},
		{ModifierVirtual, "is_virtual"},
		{ModifierElectric, "is_electric"},
	}
	for _, tc := range cases {
		col, ok := tc.key.Column()
		if !ok {
			t.Errorf("ModifierKey(%q).Column() ok=false, want true", tc.key)
			continue
		}
		if col != tc.want {
			t.Errorf("ModifierKey(%q).Column() = %q, want %q", tc.key, col, tc.want)
		}
	}
}

// TestModifierKeyColumnUnknownKeyReturnsFalse verifies that an arbitrary string
// cast to ModifierKey returns ("", false) without panicking.
func TestModifierKeyColumnUnknownKeyReturnsFalse(t *testing.T) {
	col, ok := ModifierKey("not_real").Column()
	if ok {
		t.Fatalf("ModifierKey(\"not_real\").Column() ok=true, want false")
	}
	if col != "" {
		t.Fatalf("ModifierKey(\"not_real\").Column() col=%q, want empty string", col)
	}
}
