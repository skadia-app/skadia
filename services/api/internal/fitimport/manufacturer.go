package fitimport

import (
	"fmt"
	"strings"
)

// appleWatchModels maps Apple machine identifiers to human-readable names.
// Source: public device registry DevelopmentFitSdkMapping.php + public identifier list
// License: MIT, Copyright (c) 2017 upstream registry — https://github.com/public device registry
var appleWatchModels = map[string]string{
	"Watch1,1":  "Apple Watch 38mm",
	"Watch1,2":  "Apple Watch 42mm",
	"Watch2,3":  "Apple Watch Series 2 38mm",
	"Watch2,4":  "Apple Watch Series 2 42mm",
	"Watch2,6":  "Apple Watch Series 1 38mm",
	"Watch2,7":  "Apple Watch Series 1 42mm",
	"Watch3,1":  "Apple Watch Series 3 38mm Cellular",
	"Watch3,2":  "Apple Watch Series 3 42mm Cellular",
	"Watch3,3":  "Apple Watch Series 3 38mm",
	"Watch3,4":  "Apple Watch Series 3 42mm",
	"Watch4,1":  "Apple Watch Series 4 40mm",
	"Watch4,2":  "Apple Watch Series 4 44mm",
	"Watch4,3":  "Apple Watch Series 4 40mm Cellular",
	"Watch4,4":  "Apple Watch Series 4 44mm Cellular",
	"Watch5,1":  "Apple Watch Series 5 40mm",
	"Watch5,2":  "Apple Watch Series 5 44mm",
	"Watch5,3":  "Apple Watch Series 5 40mm Cellular",
	"Watch5,4":  "Apple Watch Series 5 44mm Cellular",
	"Watch5,9":  "Apple Watch SE 40mm",
	"Watch5,10": "Apple Watch SE 44mm",
	"Watch5,11": "Apple Watch SE 40mm Cellular",
	"Watch5,12": "Apple Watch SE 44mm Cellular",
	"Watch6,1":  "Apple Watch Series 6 40mm",
	"Watch6,2":  "Apple Watch Series 6 44mm",
	"Watch6,3":  "Apple Watch Series 6 40mm Cellular",
	"Watch6,4":  "Apple Watch Series 6 44mm Cellular",
	"Watch6,6":  "Apple Watch Series 7 41mm",
	"Watch6,7":  "Apple Watch Series 7 45mm",
	"Watch6,8":  "Apple Watch Series 7 41mm Cellular",
	"Watch6,9":  "Apple Watch Series 7 45mm Cellular",
	"Watch6,10": "Apple Watch SE 2 40mm",
	"Watch6,11": "Apple Watch SE 2 44mm",
	"Watch6,12": "Apple Watch SE 2 40mm Cellular",
	"Watch6,13": "Apple Watch SE 2 44mm Cellular",
	"Watch6,14": "Apple Watch Series 8 41mm",
	"Watch6,15": "Apple Watch Series 8 45mm",
	"Watch6,16": "Apple Watch Series 8 41mm Cellular",
	"Watch6,17": "Apple Watch Series 8 45mm Cellular",
	"Watch6,18": "Apple Watch Ultra",
	"Watch7,1":  "Apple Watch Series 9 41mm",
	"Watch7,2":  "Apple Watch Series 9 45mm",
	"Watch7,3":  "Apple Watch Series 9 41mm Cellular",
	"Watch7,4":  "Apple Watch Series 9 45mm Cellular",
	"Watch7,5":  "Apple Watch Ultra 2",
	"Watch7,8":  "Apple Watch Series 10 42mm",
	"Watch7,9":  "Apple Watch Series 10 46mm",
	"Watch7,10": "Apple Watch Series 10 42mm Cellular",
	"Watch7,11": "Apple Watch Series 10 46mm Cellular",
	"Watch7,12": "Apple Watch Ultra 3",
	"Watch7,13": "Apple Watch SE 3 42mm",
	"Watch7,14": "Apple Watch SE 3 46mm",
	"Watch7,15": "Apple Watch SE 3 42mm Cellular",
	"Watch7,16": "Apple Watch SE 3 46mm Cellular",
	"Watch7,17": "Apple Watch Series 11 42mm",
	"Watch7,18": "Apple Watch Series 11 46mm",
	"Watch7,19": "Apple Watch Series 11 42mm Cellular",
	"Watch7,20": "Apple Watch Series 11 46mm Cellular",
}

// ResolveDevelopmentManufacturer identifies the real manufacturer and device name
// for FIT files with manufacturer_id=255 ("development").
// Returns (source, device). Empty source means "not identified, keep default".
func ResolveDevelopmentManufacturer(productName, descriptor string) (source, device string) {
	if productName == "" && descriptor == "" {
		return "", ""
	}

	// 1. Apple Watch: productName matches "Watch{N},{M}" pattern
	if strings.HasPrefix(productName, "Watch") {
		if name, ok := appleWatchModels[productName]; ok {
			return "Apple", name
		}
		// Unknown model but clearly Apple Watch
		if len(productName) > 5 && productName[5] >= '0' && productName[5] <= '9' {
			return "Apple", fmt.Sprintf("Apple Watch (%s)", productName)
		}
	}

	// 2. Amazfit: productName starts with "Amazfit" or "huami amazfit"
	pnLower := strings.ToLower(productName)
	if strings.HasPrefix(pnLower, "amazfit") || strings.HasPrefix(pnLower, "huami amazfit") {
		return "Amazfit", productName
	}

	// 3. Samsung: productName starts with "samsung" or "galaxy watch"
	// Samsung Health exports use "Samsung Galaxy Watch6 Classic" etc.
	// device name mapping.php confirms prefix-matching pattern (MIT, upstream registry 2017)
	if strings.HasPrefix(pnLower, "samsung") || strings.HasPrefix(pnLower, "galaxy watch") {
		return "Samsung", productName
	}

	// 4. Google Pixel Watch: productName contains "pixel watch"
	if strings.Contains(pnLower, "pixel watch") {
		return "Google", productName
	}

	// 5. Huawei via Health Sync: descriptor contains "Health Sync"
	if strings.Contains(descriptor, "Health Sync") {
		return "Huawei", ""
	}

	// 6. Known third-party sync apps — keep source as app name
	switch {
	case strings.Contains(pnLower, "fitnesssyncer"):
		return "FitnessSyncer", productName
	case strings.Contains(pnLower, "rungap"):
		return "RunGap", productName
	case strings.Contains(pnLower, "healthfit"):
		return "HealthFit", productName
	case strings.Contains(pnLower, "ismoothrun"):
		return "iSmoothRun", productName
	}

	// 7. iPhone model strings (from HealthKit export apps) — source is the exporting app
	if strings.HasPrefix(productName, "iPhone") {
		return "", productName
	}

	// Not identified — return productName as device for display
	return "", productName
}

// virtualPlatformMap maps FIT manufacturer enum strings to display names.
// Must cover at minimum all manufacturers from the isVirtual detection in parser.go
// (ManufacturerZwift, ZwiftByte, Bkool, Mywhoosh, Virtualtraining, Ictrainer, Virtugo)
// plus additional known platforms and devices with non-standard casing.
var virtualPlatformMap = map[string]string{
	// Virtual cycling platforms
	"virtualtraining": "Rouvy",
	"trainer_road":    "TrainerRoad",
	"the_sufferfest":  "Wahoo SYSTM",
	"bkool":           "Bkool",
	"mywhoosh":        "MyWhoosh",
	"virtugo":         "Virtugo",
	"ictrainer":       "iCTrainer",
	"RGT_cycling":     "RGT Cycling",
	"zwift":           "Zwift",
	"zwift_byte":      "Zwift",
	"peloton":         "Peloton",
	// Indoor bike/trainer hardware
	"wattbike": "Wattbike",
	// Devices with non-standard casing
	"whoop":         "WHOOP",
	"wahoo_fitness": "Wahoo",
}

// MapVirtualPlatform returns the display name for a virtual platform or device
// manufacturer that isn't handled by the main switch in extractFITSource.
// Returns "" if no mapping exists.
func MapVirtualPlatform(mfrString string) string {
	return virtualPlatformMap[mfrString]
}
