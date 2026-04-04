package fitimport

import (
	"fmt"

	"github.com/muktihari/fit/profile/mesgdef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// DeveloperAppInfo holds metadata for a known developer app.
type DeveloperAppInfo struct {
	Name     string
	Category string
}

// knownDeveloperApps maps lowercase hex application_id → app metadata.
// Categories always use the "app_" prefix to avoid collision with ANT+ device types.
var knownDeveloperApps = map[string]DeveloperAppInfo{
	// Stryd — 3 App-IDs, alle → derselbe devices-Eintrag "Stryd"
	"18fb2cf01a4b430dad66988c847421f4": {Name: "Stryd", Category: "app_running_power"},
	"660a581e5301460c8f2f034c8b6dc90f": {Name: "Stryd", Category: "app_running_power"},
	"00000000000000000000000000000047": {Name: "Stryd", Category: "app_running_power"},

	// FORM Schwimmbrille
	"ca3ce193fffc4fae97408a2d4c41d196": {Name: "FORM", Category: "app_swim_goggle"},

	// Bike Power
	"d8f70721853a40be80dd316738ee1198": {Name: "Daefore", Category: "app_bike_power"},
	"741afa11025048e286b514bd47e29391": {Name: "Wahoo RGT", Category: "app_bike_power"},

	// Wearable
	"7a657070": {Name: "Zepp", Category: "app_wearable"},
}

// LookupDeveloperApp looks up a known developer app by its lowercase hex application_id.
// Returns (name, category, true) if found, or ("", "", false) if unknown.
func LookupDeveloperApp(hexID string) (name, category string, ok bool) {
	info, found := knownDeveloperApps[hexID]
	if !found {
		return "", "", false
	}
	return info.Name, info.Category, true
}

// ExtractDeveloperApps extracts known developer apps from FIT DeveloperDataId messages.
// Unknown and suppressed apps (export_metadata, garmin_internal) are silently skipped.
func ExtractDeveloperApps(devDataIds []*mesgdef.DeveloperDataId) []model.DeveloperAppRecord {
	seen := map[string]bool{}
	var result []model.DeveloperAppRecord
	for _, d := range devDataIds {
		if d.ApplicationId == nil {
			continue
		}
		hex := fmt.Sprintf("%x", d.ApplicationId)
		if seen[hex] {
			continue
		}
		seen[hex] = true

		info, known := knownDeveloperApps[hex]
		if !known {
			continue
		}

		result = append(result, model.DeveloperAppRecord{
			ApplicationID: d.ApplicationId,
			AppName:       info.Name,
			AppCategory:   info.Category,
		})
	}
	return result
}
