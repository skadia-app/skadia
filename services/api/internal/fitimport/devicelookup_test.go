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
		{"Polar H10 from map", 123, 2, "", "Polar H10"},
		{"FIT product_name with known manufacturer", 32, 28, "ELEMNT BOLT V2", "Wahoo ELEMNT BOLT V2"},
		{"Unknown non-garmin falls back to manufacturer", 32, 999, "", "Wahoo Fitness"},
		{"ManufacturerInvalid returns empty", 65535, 0, "", ""},
		{"Favero Assioma Duo", 263, 12, "", "Favero Assioma Duo"},
		{"Sigma ROX 12.0", 70, 44, "", "Sigma ROX 12.0"},
		{"Lezyne Super GPS", 258, 4, "", "Lezyne Super GPS"},
		{"Unknown Garmin product returns empty (RC4)", 1, 21, "", ""},
		{"Unknown Garmin product 20 returns empty (RC4)", 1, 20, "", ""},
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
