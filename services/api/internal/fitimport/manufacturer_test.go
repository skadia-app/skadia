package fitimport

import "testing"

func TestResolveDevelopmentManufacturer(t *testing.T) {
	tests := []struct {
		productName string
		descriptor  string
		wantSource  string
		wantDevice  string
	}{
		// Apple Watch models
		{"Watch7,5", "", "Apple", "Apple Watch Ultra 2"},
		{"Watch7,12", "", "Apple", "Apple Watch Ultra 3"},
		{"Watch6,18", "", "Apple", "Apple Watch Ultra"},
		{"Watch6,17", "", "Apple", "Apple Watch Series 8 45mm Cellular"},
		{"Watch5,10", "", "Apple", "Apple Watch SE 44mm"},
		// Unknown Apple Watch — still recognized as Apple
		{"Watch99,1", "", "Apple", "Apple Watch (Watch99,1)"},
		// Amazfit — ProductName contains device name
		{"Amazfit T-Rex Ultra 2", "", "Amazfit", "Amazfit T-Rex Ultra 2"},
		{"Amazfit GTR 4", "", "Amazfit", "Amazfit GTR 4"},
		{"huami amazfit stratos 3", "", "Amazfit", "huami amazfit stratos 3"},
		// Samsung — ProductName prefix match
		{"Samsung Galaxy Watch6 Classic", "", "Samsung", "Samsung Galaxy Watch6 Classic"},
		{"samsung galaxy watch4", "", "Samsung", "samsung galaxy watch4"},
		{"Galaxy Watch Active2", "", "Samsung", "Galaxy Watch Active2"},
		// Google Pixel Watch
		{"Pixel Watch 3", "", "Google", "Pixel Watch 3"},
		// Huawei via Health Sync — descriptor identifies it
		{"iPhone17,1", "Health Sync (1)", "Huawei", ""},
		// FitnessSyncer — keep as-is, no real device
		{"FitnessSyncer", "", "FitnessSyncer", "FitnessSyncer"},
		// Unknown development device — fallback
		{"SomeRandomDevice", "", "", "SomeRandomDevice"},
		// Empty product name
		{"", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.productName, func(t *testing.T) {
			source, device := ResolveDevelopmentManufacturer(tt.productName, tt.descriptor)
			if source != tt.wantSource {
				t.Errorf("source = %q, want %q", source, tt.wantSource)
			}
			if device != tt.wantDevice {
				t.Errorf("device = %q, want %q", device, tt.wantDevice)
			}
		})
	}
}

func TestVirtualPlatformMapping(t *testing.T) {
	tests := []struct {
		mfrString  string
		wantSource string
	}{
		// Virtual cycling platforms
		{"virtualtraining", "Rouvy"},
		{"trainer_road", "TrainerRoad"},
		{"the_sufferfest", "Wahoo SYSTM"},
		{"bkool", "Bkool"},
		{"mywhoosh", "MyWhoosh"},
		{"virtugo", "Virtugo"},
		{"ictrainer", "iCTrainer"},
		{"RGT_cycling", "RGT Cycling"},
		{"zwift", "Zwift"},
		{"zwift_byte", "Zwift"},
		{"peloton", "Peloton"},
		{"wattbike", "Wattbike"},
		// Devices with non-standard casing
		{"whoop", "WHOOP"},
		{"wahoo_fitness", "Wahoo"},
		// Not in map
		{"garmin", ""},
		{"coros", ""},
	}
	for _, tt := range tests {
		t.Run(tt.mfrString, func(t *testing.T) {
			got := MapVirtualPlatform(tt.mfrString)
			if got != tt.wantSource {
				t.Errorf("MapVirtualPlatform(%q) = %q, want %q", tt.mfrString, got, tt.wantSource)
			}
		})
	}
}
