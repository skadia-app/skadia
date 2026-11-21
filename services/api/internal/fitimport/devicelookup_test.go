package fitimport

import (
	"strings"
	"testing"

	"github.com/muktihari/fit/profile/typedef"
)

func TestResolveProductName(t *testing.T) {
	// Erst prüfen was GarminProduct(1836) (edge_530) tatsächlich liefert
	edge530Str := typedef.GarminProduct(1836).String()

	tests := []struct {
		name           string
		manufacturerID uint16
		productID      uint16
		productName    string
		want           string
	}{
		{"Wahoo ELEMNT from map", 32, 28, "", "Wahoo ELEMNT"},
		{"Polar H10 from observed sensor product", 123, 2, "", "Polar H10"},
		{"Polar H9 from observed sensor product", 123, 3, "", "Polar H9"},
		{"Polar H10 from alternate map", 123, 133, "", "Polar H10"},
		{"Polar H9 from map", 123, 222, "", "Polar H9"},
		{"Polar OH1 from map", 123, 151, "", "Polar OH1"},
		{"Polar product_name is preserved", 123, 0, "Polar H10", "Polar H10"},
		{"FIT product_name with known manufacturer", 32, 28, "ELEMNT BOLT V2", "Wahoo ELEMNT BOLT V2"},
		{"Unknown non-garmin falls back to manufacturer", 32, 999, "", "Wahoo Fitness"},
		{"ManufacturerInvalid returns empty", 65535, 0, "", ""},
		{"Favero Assioma Duo", 263, 12, "", "Favero Assioma Duo"},
		{"Sigma ROX 12.0", 70, 44, "", "Sigma ROX 12.0"},
		{"Lezyne Super GPS", 258, 4, "", "Lezyne Super GPS"},
		{"Unknown Garmin product returns empty (RC4)", 1, 21, "", ""},
		{"Unknown Garmin product 20 returns empty (RC4)", 1, 20, "", ""},
		{"Garmin Varia RTL515 from extension map", 1, 3592, "", "Garmin Varia RTL515"},
		{"Garmin HRM 600 from extension map", 1, 4607, "", "Garmin HRM 600"},
		{"Garmin Tacx Nera from extension map", 1, 4614, "", "Garmin Tacx Nera"},
		{"Garmin FR970 from SDK", 1, 4565, "", "Garmin FR970"},
		{"Garmin FR570 Small from SDK", 1, 4574, "", "Garmin FR570 Small"},
		{"Garmin D2 Mach 2 Pro acronym formatting", 1, 5056, "", "Garmin D2 Mach 2 Pro"},
		{"Wahoo ELEMNT ACE from map", 32, 57, "", "Wahoo ELEMNT ACE"},
		{"Suunto Run 2 from map", 23, 74, "", "Suunto Run 2"},
		{"Polar Loop Gen 2 from map", 123, 269, "", "Polar Loop Gen 2"},
		{"Polar Street X from map", 123, 271, "", "Polar Street X"},
		{"COROS PACE 4 Pro from map", 294, 808, "", "COROS PACE 4 Pro"},
		{"COROS DURA 2 from map", 294, 852, "", "COROS DURA 2"},
		{"COROS NOMAD from map", 294, 861, "", "COROS NOMAD"},
		{"Amazfit Helio Strap from map", 339, 157, "", "Amazfit Helio Strap"},
		{"Amazfit Helio Strap product_name under Zepp avoids double brand", 339, 157, "Amazfit Helio Strap", "Amazfit Helio Strap"},
		{"Shimano Di2 from map", 41, 12954, "", "Shimano Di2"},
	}

	// Dynamischer Garmin-Test (SDK-versionsabhängig)
	if edge530Str != "" && !strings.HasPrefix(edge530Str, "GarminProduct") {
		tests = append(tests, struct {
			name           string
			manufacturerID uint16
			productID      uint16
			productName    string
			want           string
		}{"Garmin Edge 530 from SDK", 1, 1836, "", "Garmin " + formatDeviceNameFromEnum("", edge530Str)})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveProductName(tt.manufacturerID, tt.productID, tt.productName)
			if got != tt.want {
				t.Errorf("ResolveProductName(%d, %d, %q) = %q, want %q",
					tt.manufacturerID, tt.productID, tt.productName, got, tt.want)
			}
		})
	}
}

func TestFormatManufacturer(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"wahoo_fitness", "Wahoo Fitness"},
		{"garmin", "Garmin"},
		{"polar_electro", "Polar Electro"},
		{"sigmasport", "Sigma Sport"},
		{"", ""},
		{"ManufacturerInvalid", ""},
		{"ManufacturerInvalid(65535)", ""},
		{"ManufacturerInvalid(123)", ""},
		{"Manufacturer(999)", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := FormatManufacturer(tt.input)
			if got != tt.want {
				t.Errorf("FormatManufacturer(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
