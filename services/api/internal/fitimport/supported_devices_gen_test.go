package fitimport

import (
	"cmp"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/muktihari/fit/profile/typedef"
)

// update flag: go test ./internal/fitimport/ -run TestUpdateSupportedDevices -update
var update = flag.Bool("update", false, "update SUPPORTED_DEVICES.md")

func repoRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "..")
}

// ---------------------------------------------------------------------------
// Skip list — internal chipsets, software, accessories
// ---------------------------------------------------------------------------

var skipProducts = map[string]bool{
	// Internal HRM chipsets
	"axh01": true,
	"axb01": true,
	"axb02": true,
	"axs01": true,
	// Internal development/test hardware
	"dsi_alf01": true,
	"dsi_alf02": true,
	"alf04":     true,
	"amx":       true,
	// Airoha GNSS chipset
	"gnss": true,
	// Development / software tools
	"connectiq_simulator":    true,
	"android_antplus_plugin": true,
	"connect":                true,
	"training_center":        true,
	// Tacx training software (not hardware)
	"tacx_training_app_win":          true,
	"tacx_training_app_mac":          true,
	"tacx_training_app_mac_catalyst": true,
	"tacx_training_app_android":      true,
	"tacx_training_app_ios":          true,
	"tacx_training_app_legacy":       true,
	// ANT+ messaging aliases (duplicates)
	"hrm_run_single_byte_product_id":  true,
	"hrm_tri_single_byte_product_id":  true,
	"hrm4_run_single_byte_product_id": true,
	"fr225_single_byte_product_id":    true,
	"gen3_bsm_single_byte_product_id": true,
	"gen3_bcm_single_byte_product_id": true,
	"hrm_fit_single_byte_product_id":  true,
	// Accessories / non-standalone devices
	"varia_remote": true,
	// Action cameras (discontinued)
	"virb_elite":    true,
	"virb_remote":   true,
	"virbx":         true,
	"virbxe":        true,
	"virb_ultra_30": true,
	"virb_360":      true,
	// Specialty accessories
	"chirp":        true,
	"nautix":       true,
	"edge_remote":  true,
	"approach_z80": true,
	"approach_j1":  true,
	"truswing":     true,
}

// ---------------------------------------------------------------------------
// Display name overrides for enum strings that formatDeviceNameFromEnum
// cannot produce correctly.
// ---------------------------------------------------------------------------

var displayNameOverrides = map[string]string{
	// Venu SQ series: "venusq" has no underscore separating "venu" and "sq"
	"venusq":          "Venu SQ",
	"venusq2":         "Venu SQ 2",
	"venusq2music":    "Venu SQ 2 Music",
	"venusq_music":    "Venu SQ Music",
	"venusq_music_v2": "Venu SQ Music V2",
	// Approach: capital S jammed into token, no underscore
	"approachS44": "Approach S44",
	"approachS50": "Approach S50",
	// Instinct E: capital E not split from "instinct"
	"instinctE_40mm": "Instinct E 40mm",
	"instinctE_45mm": "Instinct E 45mm",
	// D2 series: formatEnumToken splits "d2" as alpha="d" + digit="2" before acronym check
	"d2airvenu":         "D2 Air Venu",
	"d2charlie":         "D2 Charlie",
	"d2_bravo":          "D2 Bravo",
	"d2_bravo_titanium": "D2 Bravo Titanium",
	"d2_air_x10":        "D2 Air X10",
	"d2_air_x15":        "D2 Air X15",
	"d2_mach1_pro":      "D2 Mach 1 Pro",
	"d2_mach2":          "D2 Mach 2",
	// GPSMAP: acronym match keeps number attached ("GPSMAP66"), need space
	"gpsmap66":  "GPSMAP 66",
	"gpsmap66i": "GPSMAP 66i",
	// Descent Mk3i: "i" suffix should be lowercase (indoor variant)
	"descent_mk3i": "Descent Mk3i",
	// Foretrex: slash lost because underscores are word separators
	"foretrex_601_701": "Foretrex 601/701",
	// Tacx Flux/Flux S: SDK enum has double "flux" in name
	"tacx_flux_flux_s_smart": "Tacx Flux/Flux S Smart",
	// Vivoactive 3 Music: "m" = Music, "w" = WiFi, "l" = LTE
	"vivoactive3m_w": "Vivoactive 3 Music",
	"vivoactive3m_l": "Vivoactive 3 Music LTE",
	// FR235 Lite: Asia-only budget variant
	"fr235l_asia": "FR235 Lite",
	// Regional alias mapping to Vivoactive 3 Music base
	"vivo_active3m": "Vivoactive 3 Music",
}

// ---------------------------------------------------------------------------
// Regional suffixes stripped before deduplication
// ---------------------------------------------------------------------------

var regionalSuffixes = []string{
	"_china", "_japan", "_korea", "_taiwan", "_asia", "_sea",
	"_apac", "_emea", "_chn", "_twn", "_jpn", "_kor",
	"_thai", "_ww", "_russia", "_hebrew", "_bontrager", "_daimler",
}

func stripRegionalSuffix(s string) string {
	for {
		stripped := false
		for _, sfx := range regionalSuffixes {
			if strings.HasSuffix(s, sfx) {
				s = s[:len(s)-len(sfx)]
				stripped = true
				break
			}
		}
		if !stripped {
			return s
		}
	}
}

// ---------------------------------------------------------------------------
// Garmin enum categorization
// ---------------------------------------------------------------------------

func categorizeGarminEnum(enumStr string) (category, sensorType string) {
	s := strings.ToLower(enumStr)

	if skipProducts[s] {
		return "skip", ""
	}

	switch {
	case strings.HasPrefix(s, "hrm") || s == "ohr":
		return "sensor", "Heart Rate"
	case strings.HasPrefix(s, "vector") || strings.HasPrefix(s, "rally"):
		return "sensor", "Power Meter"
	case strings.HasPrefix(s, "varia"):
		if strings.Contains(s, "radar") || strings.Contains(s, "rct") || strings.Contains(s, "vision") {
			return "sensor", "Radar"
		}
		return "sensor", "Lighting"
	case strings.HasPrefix(s, "running_dynamics"):
		return "sensor", "Running Dynamics"
	case s == "sdm4":
		return "sensor", "Running Dynamics"
	case s == "bsm" || strings.HasPrefix(s, "gen3_bsm"):
		return "sensor", "Speed"
	case s == "bcm" || strings.HasPrefix(s, "gen3_bcm"):
		return "sensor", "Cadence"
	case strings.HasPrefix(s, "index"):
		return "sensor", "Weight Scale"
	case s == "tempe":
		return "sensor", "Temperature"
	}

	if strings.HasPrefix(s, "tacx") {
		return "trainer", ""
	}

	return "recording", ""
}

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

type deviceEntry struct {
	manufacturer string
	model        string
	sensorType   string // only for sensors
	source       string // SDK, Map, Pattern
	category     string // recording, sensor, trainer, virtual, devapp
}

// ---------------------------------------------------------------------------
// Manufacturer classification for productNameMap entries
// ---------------------------------------------------------------------------

type mfrClass struct {
	brand    string
	category string // recording, sensor, trainer
	sType    string // sensor type, only relevant when category==sensor
}

// productLevelOverrides maps specific [2]uint16 keys to overrides.
type productOverride struct {
	category string
	sType    string
	skip     bool
}

var mfrClassMap = map[uint16]mfrClass{
	6:   {brand: "SRM", category: "sensor", sType: "Power Meter"},
	7:   {brand: "Quarq", category: "sensor", sType: "Power Meter"},
	9:   {brand: "Powertap", category: "sensor", sType: "Power Meter"},
	16:  {brand: "Timex", category: "recording"},
	23:  {brand: "Suunto", category: "recording"},
	29:  {brand: "Power2max", category: "sensor", sType: "Power Meter"},
	32:  {brand: "Wahoo", category: "recording"},
	38:  {brand: "o_synce", category: "recording"},
	45:  {brand: "Xplova", category: "recording"},
	54:  {brand: "IpBike", category: "recording"},
	69:  {brand: "Stages", category: "sensor", sType: "Power Meter"},
	70:  {brand: "Sigma Sport", category: "recording"},
	71:  {brand: "TomTom", category: "recording"},
	73:  {brand: "Wattbike", category: "trainer"},
	83:  {brand: "Scosche", category: "sensor", sType: "Heart Rate"},
	89:  {brand: "Tacx", category: "trainer"},
	98:  {brand: "BSX Athletics", category: "sensor", sType: "Muscle Oxygen"},
	106: {brand: "Morpheus", category: "recording"},
	107: {brand: "Magene", category: "recording"},
	123: {brand: "Polar", category: "recording"},
	132: {brand: "Cycplus", category: "recording"},
	135: {brand: "Coospo", category: "recording"},
	258: {brand: "Lezyne", category: "recording"},
	263: {brand: "Favero", category: "sensor", sType: "Power Meter"},
	268: {brand: "SRAM", category: "sensor", sType: "Shifting"},
	289: {brand: "Hammerhead", category: "recording"},
	292: {brand: "XOSS", category: "recording"},
	294: {brand: "COROS", category: "recording"},
}

var productOverrides = map[[2]uint16]productOverride{
	// Wahoo product-level overrides
	{32, 8}:  {category: "sensor", sType: "Heart Rate"},
	{32, 41}: {category: "sensor", sType: "Power Meter"},
	// Xplova Noza S → Smart Trainer
	{45, 20051}: {category: "trainer"},
	// Polar HRM overrides
	{123, 2}:   {category: "sensor", sType: "Heart Rate"},
	{123, 7}:   {category: "sensor", sType: "Heart Rate"},
	{123, 133}: {category: "sensor", sType: "Heart Rate"},
	{123, 151}: {category: "sensor", sType: "Heart Rate"},
	{123, 222}: {category: "sensor", sType: "Heart Rate"},
	{123, 235}: {category: "sensor", sType: "Heart Rate"},
	// COROS POD → Running Dynamics
	{294, 11}: {category: "sensor", sType: "Running Dynamics"},
	{294, 12}: {category: "sensor", sType: "Running Dynamics"},
	// Tacx skip entries
	{89, 4274}: {skip: true}, // Tacx Rouvy — software, already in virtualPlatformMap
	{89, 3111}: {skip: true}, // Tacx Breakaway — film system, not a trainer
	// Powertap Joule head units are recording devices, not power meters
	{9, 14}: {category: "recording"}, // Powertap Joule 2.0
	{9, 18}: {category: "recording"}, // Powertap Joule
	{9, 19}: {category: "recording"}, // Powertap Joule GPS
	{9, 22}: {category: "recording"}, // Powertap Joule GPS+
}

type tableFootnotes struct {
	indexByNote map[string]int
	notes       []string
}

func (f *tableFootnotes) marker(note string) string {
	if f.indexByNote == nil {
		f.indexByNote = map[string]int{}
	}
	if n, ok := f.indexByNote[note]; ok {
		return fmt.Sprintf("<sup>%d</sup>", n)
	}
	f.notes = append(f.notes, note)
	n := len(f.notes)
	f.indexByNote[note] = n
	return fmt.Sprintf("<sup>%d</sup>", n)
}

func (f *tableFootnotes) formatModel(model string) string {
	base, note, ok := splitTrailingParenthetical(model)
	if !ok {
		return model
	}
	note = formatFootnoteText(note)
	if note == "" {
		return base
	}
	return base + f.marker(note)
}

func splitTrailingParenthetical(s string) (base, note string, ok bool) {
	idx := strings.LastIndex(s, " (")
	if idx < 0 || !strings.HasSuffix(s, ")") {
		return "", "", false
	}
	base = strings.TrimSpace(s[:idx])
	note = strings.TrimSpace(strings.TrimSuffix(s[idx+2:], ")"))
	return base, note, base != "" && note != ""
}

func formatFootnoteText(note string) string {
	if note == "" {
		return ""
	}
	if note[0] >= 'a' && note[0] <= 'z' {
		note = string(note[0]-('a'-'A')) + note[1:]
	}
	if !strings.HasSuffix(note, ".") {
		note += "."
	}
	return note
}

var modelPrefixAliases = map[string][]string{
	"Sigma Sport": {"Sigma "},
}

func displayModelWithoutManufacturer(e deviceEntry) string {
	model := strings.TrimSpace(e.model)
	if strings.EqualFold(model, e.manufacturer) {
		return "App"
	}
	for _, prefix := range append([]string{e.manufacturer + " "}, modelPrefixAliases[e.manufacturer]...) {
		model = strings.TrimPrefix(model, prefix)
	}
	return strings.TrimSpace(model)
}

// ---------------------------------------------------------------------------
// Markdown generation
// ---------------------------------------------------------------------------

func generateSupportedDevicesMD() string {
	var b strings.Builder

	w := func(format string, args ...any) {
		if len(args) == 0 {
			b.WriteString(format)
		} else {
			fmt.Fprintf(&b, format, args...)
		}
	}

	// Collect all data
	recording, sensors, trainers, virtuals, devApps := collectEntries()

	// ---- Header ----
	w("<!-- Generated file — do not edit manually. -->\n")
	w("<!-- Run: go test ./internal/fitimport/ -run TestUpdateSupportedDevices -update -->\n")
	w("\n# Supported Devices\n")

	// ---- Section 1: How to Add a New Device ----
	w("\n## How to Add a New Device\n\n")
	w("To add support for a new device, edit the appropriate source file below, then run the update command to regenerate this file:\n\n")
	w("```\ngo test ./internal/fitimport/ -run TestUpdateSupportedDevices -update\n```\n\n")
	w("| Source File | When to Use | Mechanism |\n")
	w("|---|---|---|\n")
	w("| [`services/api/internal/fitimport/devicelookup.go`](services/api/internal/fitimport/devicelookup.go) | Hardware item with known `[manufacturer_id, product_id]` pair not in FIT SDK | Static map lookup |\n")
	w("| [`services/api/internal/fitimport/manufacturer.go`](services/api/internal/fitimport/manufacturer.go) | Development devices (mfr=255): Apple Watch, Samsung, Amazfit, etc. | String pattern matching on product_name |\n")
	w("| [`services/api/internal/fitimport/developer_app_registry.go`](services/api/internal/fitimport/developer_app_registry.go) | Third-party apps with known Application ID | Application ID hex lookup |\n")

	// ---- Section 2: Generic Support ----
	w("\n## Generic Support\n\n")
	w("Detected recording devices and most sensors with serial numbers appear on activity pages and are also listed under Gear (`/hardware/recorder` and `/hardware/sensor`). They can also be used in activity filters and automation rules.\n\n")
	w("FIT files from any device are imported. All ANT+ and Bluetooth Low Energy sensors are automatically recognized. Data is fully imported even if the specific model name is not listed below. GPX and TCX files can also be imported, but they usually contain less device and sensor metadata.\n\n")
	w("This catalog lists recognized recording devices, sensors, sync apps, developer apps, smart trainers, and virtual platforms.\n\n")
	w("- Sensor data (heart rate, power, cadence, speed, temperature, etc.) is imported regardless of whether the sensor model is known\n")
	w("- FIT Developer Field values are imported when present in activity streams; recognized developer apps are listed below.\n")
	w("- Unknown devices may show only the manufacturer name or no device name; the data itself is unaffected\n")
	w("\n### Sync Apps\n\n")
	w("FitnessSyncer, RunGap, HealthFit, and iSmoothRun are recognized as import sources. Activities imported via these apps retain the original recording device information where available.\n")

	// ---- Section 3: Recording Devices ----
	w("\n## Recording Devices\n\n")
	slices.SortFunc(recording, func(a, b deviceEntry) int {
		if c := cmp.Compare(a.manufacturer, b.manufacturer); c != 0 {
			return c
		}
		return cmp.Compare(displayModelWithoutManufacturer(a), displayModelWithoutManufacturer(b))
	})
	recordingFootnotes := tableFootnotes{}
	w("| Manufacturer | Model | Source |\n")
	w("|---|---|---|\n")
	for _, e := range recording {
		model := recordingFootnotes.formatModel(displayModelWithoutManufacturer(e))
		w("| %s | %s | %s |\n", e.manufacturer, model, e.source)
	}
	if len(recordingFootnotes.notes) > 0 {
		w("\n**Recording device notes:**\n\n")
		for i, note := range recordingFootnotes.notes {
			w("<sup>%d</sup> %s\n\n", i+1, note)
		}
	}

	// ---- Section 4: Sensors ----
	w("\n## Sensors\n\n")
	w("Some sensor-like devices and apps, including Stryd, use FIT Developer Fields instead of standard ANT+/BLE sensor records. If a sensor is not listed here, also check [Developer Apps](#developer-apps).\n\n")
	slices.SortFunc(sensors, func(a, b deviceEntry) int {
		if c := cmp.Compare(a.manufacturer, b.manufacturer); c != 0 {
			return c
		}
		return cmp.Compare(a.model, b.model)
	})
	w("| Manufacturer | Model | Type | Source |\n")
	w("|---|---|---|---|\n")
	for _, e := range sensors {
		model := strings.TrimPrefix(e.model, e.manufacturer+" ")
		w("| %s | %s | %s | %s |\n", e.manufacturer, model, e.sensorType, e.source)
	}

	// ---- Section 5: Developer Apps ----
	w("\n## Developer Apps\n\n")
	w("Developer Apps use FIT Developer Fields to embed custom data and are detected by Application ID. Multiple application IDs can resolve to the same app entry.\n\n")
	slices.SortFunc(devApps, func(a, b deviceEntry) int {
		return cmp.Compare(a.model, b.model)
	})
	w("| App | Category | Source |\n")
	w("|---|---|---|\n")
	for _, e := range devApps {
		w("| %s | %s | %s |\n", e.model, e.sensorType, e.source)
	}

	// ---- Section 6: Smart Trainers & Virtual Platforms ----
	w("\n## Smart Trainers & Virtual Platforms\n\n")
	w("### Smart Trainers\n\n")
	slices.SortFunc(trainers, func(a, b deviceEntry) int {
		if c := cmp.Compare(a.manufacturer, b.manufacturer); c != 0 {
			return c
		}
		return cmp.Compare(a.model, b.model)
	})
	w("| Manufacturer | Model | Source |\n")
	w("|---|---|---|\n")
	for _, e := range trainers {
		model := strings.TrimPrefix(e.model, e.manufacturer+" ")
		w("| %s | %s | %s |\n", e.manufacturer, model, e.source)
	}

	w("\n### Virtual Platforms\n\n")
	slices.SortFunc(virtuals, func(a, b deviceEntry) int {
		return cmp.Compare(a.model, b.model)
	})
	w("| Platform | Source |\n")
	w("|---|---|\n")
	for _, e := range virtuals {
		w("| %s | %s |\n", e.model, e.source)
	}

	// ---- Footer ----
	w("\n---\n\n")
	w("**Source column legend:**\n")
	w("- **SDK** — Resolved via FIT SDK enum (GarminProduct, FaveroProduct)\n")
	w("- **Map** — Resolved via static lookup in [devicelookup.go](services/api/internal/fitimport/devicelookup.go)\n")
	w("- **Pattern** — Resolved via string pattern matching in [manufacturer.go](services/api/internal/fitimport/manufacturer.go)\n")
	w("- **Registry** — Resolved via Application ID in [developer_app_registry.go](services/api/internal/fitimport/developer_app_registry.go)\n")

	return b.String()
}

// collectEntries collects all device entries from all sources.
func collectEntries() (recording, sensors, trainers, virtuals, devApps []deviceEntry) {
	// ---- 1. GarminProduct SDK enum ----
	// Track which Garmin product IDs are covered by the SDK enum (for productNameMap dedup later).
	garminSDKProductIDs := map[uint16]bool{}
	// Dedup by normalized (regional-stripped) lowered enum key — first occurrence wins.
	garminSeen := map[string]bool{}

	for _, gp := range typedef.ListGarminProduct() {
		raw := gp.String()
		if strings.HasPrefix(raw, "GarminProduct") {
			continue
		}
		garminSDKProductIDs[uint16(gp)] = true

		cat, sType := categorizeGarminEnum(raw)
		if cat == "skip" {
			continue
		}

		lowered := strings.ToLower(raw)
		normalized := stripRegionalSuffix(lowered)
		if garminSeen[normalized] {
			continue
		}
		garminSeen[normalized] = true

		// Resolve display name
		var displayName string
		if override, ok := displayNameOverrides[raw]; ok {
			displayName = "Garmin " + override
		} else {
			displayName = formatDeviceNameFromEnum("Garmin", raw)
		}

		e := deviceEntry{
			manufacturer: "Garmin",
			model:        displayName,
			sensorType:   sType,
			source:       "SDK",
			category:     cat,
		}
		switch cat {
		case "recording":
			recording = append(recording, e)
		case "sensor":
			sensors = append(sensors, e)
		case "trainer":
			trainers = append(trainers, e)
		}
	}

	// ---- 2. FaveroProduct SDK enum ----
	faveroSDKProductIDs := map[uint16]bool{}
	for _, fp := range typedef.ListFaveroProduct() {
		raw := fp.String()
		if strings.HasPrefix(raw, "FaveroProduct") {
			continue
		}
		faveroSDKProductIDs[uint16(fp)] = true

		displayName := formatDeviceNameFromEnum("Favero", raw)
		sensors = append(sensors, deviceEntry{
			manufacturer: "Favero",
			model:        displayName,
			sensorType:   "Power Meter",
			source:       "SDK",
			category:     "sensor",
		})
	}

	// ---- 3. productNameMap ----
	// Build a sorted key list for deterministic output.
	type mapKey struct {
		mfrID     uint16
		productID uint16
	}
	keys := make([]mapKey, 0, len(productNameMap))
	for k := range productNameMap {
		keys = append(keys, mapKey{k[0], k[1]})
	}
	slices.SortFunc(keys, func(a, b mapKey) int {
		if c := cmp.Compare(a.mfrID, b.mfrID); c != 0 {
			return c
		}
		return cmp.Compare(a.productID, b.productID)
	})

	// Dedup by (mfrID, displayName, category) tuple
	mapSeen := map[string]bool{}

	for _, k := range keys {
		key := [2]uint16{k.mfrID, k.productID}
		displayName := productNameMap[key]

		// mfr=1 (Garmin): skip if product_id already in GarminProduct SDK
		if k.mfrID == 1 && garminSDKProductIDs[k.productID] {
			continue
		}
		// mfr=263 (Favero): skip if product_id already in FaveroProduct SDK
		if k.mfrID == 263 && faveroSDKProductIDs[k.productID] {
			continue
		}

		// Product-level override?
		if po, ok := productOverrides[key]; ok {
			if po.skip {
				continue
			}
		}

		// Garmin (mfr=1) entries not in SDK — use categorizeGarminEnum on display name
		var cat, sType, brand string
		if k.mfrID == 1 {
			brand = "Garmin"
			// Infer category from the display name (e.g. "Garmin Varia RTL515" → sensor/Radar)
			nameLower := strings.ToLower(displayName)
			if strings.Contains(nameLower, "varia") {
				cat, sType = "sensor", "Radar"
			} else {
				cat = "recording"
			}
		} else {
			// Determine manufacturer class
			mc, ok := mfrClassMap[k.mfrID]
			if !ok {
				// Unknown manufacturer — skip
				continue
			}
			brand = mc.brand

			// Resolve category and type: product override takes precedence
			cat = mc.category
			sType = mc.sType
		}
		if po, ok := productOverrides[key]; ok {
			if po.category != "" {
				cat = po.category
				sType = po.sType
			}
		}

		// Dedup by (mfrID, displayName, category)
		dedupKey := fmt.Sprintf("%d|%s|%s", k.mfrID, displayName, cat)
		if mapSeen[dedupKey] {
			continue
		}
		mapSeen[dedupKey] = true

		// Tacx dedup: if display name already covered by SDK entry, skip
		// SDK entries have "Garmin " prefix (e.g. "Garmin Tacx Neo Smart"), Map entries don't
		if k.mfrID == 89 {
			alreadyCovered := false
			for _, t := range trainers {
				if t.source == "SDK" && (t.model == displayName || t.model == "Garmin "+displayName) {
					alreadyCovered = true
					break
				}
			}
			if alreadyCovered {
				continue
			}
		}

		e := deviceEntry{
			manufacturer: brand,
			model:        displayName,
			sensorType:   sType,
			source:       "Map",
			category:     cat,
		}
		switch cat {
		case "recording":
			recording = append(recording, e)
		case "sensor":
			sensors = append(sensors, e)
		case "trainer":
			trainers = append(trainers, e)
		}
	}

	// ---- 4. Apple Watch — single aggregate entry ----
	recording = append(recording, deviceEntry{
		manufacturer: "Apple",
		model:        "All Watch models (detected via product name)",
		source:       "Pattern",
		category:     "recording",
	})

	// ---- 5. Product-name device brands ----
	for _, entry := range []struct {
		manufacturer string
		model        string
	}{
		{manufacturer: "Amazfit", model: "All models (detected via product name)"},
		{manufacturer: "Google", model: "All Pixel Watch models (detected via product name)"},
		{manufacturer: "Huawei", model: "All wearable models (detected via file descriptor; exact model unavailable)"},
		{manufacturer: "Polar", model: "All models with product_name metadata (detected via product name)"},
		{manufacturer: "Samsung", model: "All Galaxy Watch models (detected via product name)"},
	} {
		recording = append(recording, deviceEntry{
			manufacturer: entry.manufacturer,
			model:        entry.model,
			source:       "Pattern",
			category:     "recording",
		})
	}

	// ---- 6. virtualPlatformMap ----
	exclude := map[string]bool{
		"wattbike":      true,
		"wahoo_fitness": true,
		"whoop":         true,
	}
	virtualSeen := map[string]bool{}
	// Sort map keys for deterministic output
	vKeys := make([]string, 0, len(virtualPlatformMap))
	for k := range virtualPlatformMap {
		vKeys = append(vKeys, k)
	}
	slices.Sort(vKeys)
	for _, k := range vKeys {
		if exclude[k] {
			continue
		}
		displayName := virtualPlatformMap[k]
		if virtualSeen[displayName] {
			continue
		}
		virtualSeen[displayName] = true
		virtuals = append(virtuals, deviceEntry{
			model:    displayName,
			source:   "Pattern",
			category: "virtual",
		})
	}

	// ---- 7. knownDeveloperApps ----
	// Deduplicate by app name: multiple application IDs can resolve to one entry.
	appSeen := map[string]bool{}
	for _, info := range knownDeveloperApps {
		if appSeen[info.Name] {
			continue
		}
		appSeen[info.Name] = true
		devApps = append(devApps, deviceEntry{
			model:      info.Name,
			sensorType: formatDevAppCategory(info.Category),
			source:     "Registry",
			category:   "devapp",
		})
	}

	return recording, sensors, trainers, virtuals, devApps
}

// formatDevAppCategory converts internal category strings to display names.
func formatDevAppCategory(cat string) string {
	switch cat {
	case "app_running_power":
		return "Running Power"
	case "app_swim_goggle":
		return "Swim Goggle"
	case "app_bike_power":
		return "Bike Power"
	case "app_wearable":
		return "Wearable"
	case "app_body_temp":
		return "Body Temperature"
	case "app_sport_watch":
		return "Sport Watch"
	case "app_radar":
		return "Radar"
	case "app_training":
		return "Training App"
	default:
		if strings.HasPrefix(cat, "app_") {
			panic(fmt.Sprintf("unsupported developer app category %q", cat))
		}
		return cat
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestSupportedDevicesRecordingDevicesTableFormat(t *testing.T) {
	generated := generateSupportedDevicesMD()
	sectionStart := strings.Index(generated, "\n## Recording Devices\n")
	if sectionStart < 0 {
		t.Fatal("Recording Devices section not found")
	}
	sectionEnd := strings.Index(generated[sectionStart:], "\n## Sensors\n")
	if sectionEnd < 0 {
		t.Fatal("Sensors section not found after Recording Devices")
	}
	section := generated[sectionStart : sectionStart+sectionEnd]

	if !strings.Contains(section, "| Manufacturer | Model | Source |\n|---|---|---|\n") {
		t.Fatal("Recording Devices section should use a single Manufacturer/Model/Source table")
	}
	if strings.Contains(section, "\n### ") {
		t.Fatal("Recording Devices section should not be split into per-manufacturer subsections")
	}
	if !strings.Contains(section, "| COROS | APEX | Map |") {
		t.Fatal("Recording Devices table should not repeat the manufacturer name in the model column")
	}
	if !strings.Contains(section, "| Sigma Sport | ROX 11.1 | Map |") ||
		strings.Contains(section, "| Sigma Sport | Sigma ") {
		t.Fatal("Recording Devices table should remove model-prefix aliases for manufacturer names")
	}
	if !strings.Contains(section, "| IpBike | App | Map |") {
		t.Fatal("Recording Devices table should avoid exact manufacturer/model repetition")
	}
	for _, row := range []string{
		"| Polar | Grit X | Map |",
		"| Polar | Grit X Pro | Map |",
		"| Polar | Grit X2 | Map |",
		"| Polar | Ignite 3 | Map |",
		"| Polar | Pacer Pro | Map |",
		"| Polar | Vantage M3 | Map |",
		"| Polar | Vantage V3 | Map |",
	} {
		if !strings.Contains(section, row) {
			t.Fatalf("Recording Devices table should include Polar map capability row %q", row)
		}
	}
	productNameMarker := requireRecordingNoteMarker(t, section, "Detected via product name.")
	fileDescriptorMarker := requireRecordingNoteMarker(t, section, "Detected via file descriptor; exact model unavailable.")

	if !strings.Contains(section, "| Amazfit | All models"+productNameMarker+" | Pattern |") {
		t.Fatal("parenthetical detection notes should be moved to local note markers")
	}
	if !strings.Contains(section, "| Apple | All Watch models"+productNameMarker+" | Pattern |") {
		t.Fatal("Apple aggregate entry should keep the all-model scope in the table")
	}
	if !strings.Contains(section, "| Google | All Pixel Watch models"+productNameMarker+" | Pattern |") {
		t.Fatal("Google aggregate entry should keep the all-model scope in the table")
	}
	if !strings.Contains(section, "| Huawei | All wearable models"+fileDescriptorMarker+" | Pattern |") ||
		strings.Contains(section, "| Huawei | All models") {
		t.Fatal("Huawei aggregate entry should be scoped to all wearable models, not all Huawei models")
	}
	if !strings.Contains(section, "| Polar | All models with product_name metadata"+productNameMarker+" | Pattern |") {
		t.Fatal("Polar aggregate entry should expose product_name-based recording-device detection")
	}
	if !strings.Contains(section, "| Samsung | All Galaxy Watch models"+productNameMarker+" | Pattern |") ||
		strings.Contains(section, "| Samsung | All models") {
		t.Fatal("Samsung aggregate entry should keep the all-model scope in the table")
	}
	if !strings.Contains(section, "**Recording device notes:**") ||
		strings.Contains(section, "[^1]:") ||
		strings.Contains(section, "[^2]:") ||
		strings.Contains(section, "All models: Series 1\u201311, SE, Ultra.") {
		t.Fatal("Recording Devices notes should be local to the section and describe detection only")
	}
}

func requireRecordingNoteMarker(t *testing.T, section, note string) string {
	t.Helper()
	for i := 1; i <= 9; i++ {
		marker := fmt.Sprintf("<sup>%d</sup>", i)
		if strings.Contains(section, marker+" "+note) {
			return marker
		}
	}
	t.Fatalf("Recording Devices notes should contain note %q", note)
	return ""
}

func TestSupportedDevicesGenericSupportStaysDeviceFocused(t *testing.T) {
	generated := generateSupportedDevicesMD()
	sectionStart := strings.Index(generated, "\n## Generic Support\n")
	if sectionStart < 0 {
		t.Fatal("Generic Support section not found")
	}
	sectionEnd := strings.Index(generated[sectionStart:], "\n## Recording Devices\n")
	if sectionEnd < 0 {
		t.Fatal("Recording Devices section not found after Generic Support")
	}
	section := generated[sectionStart : sectionStart+sectionEnd]

	for _, featureText := range []string{
		"Feature highlights",
		"Garmin Descent",
		"Radar events",
		"Running dynamics",
		"Developer fields: custom data",
	} {
		if strings.Contains(section, featureText) {
			t.Fatalf("Generic Support should not list feature-level support item %q", featureText)
		}
	}
	if !strings.Contains(section, "This catalog lists recognized recording devices, sensors, sync apps, developer apps, smart trainers, and virtual platforms.") {
		t.Fatal("Generic Support should explain that this document is a source/device catalog")
	}
	if !strings.Contains(section, "FIT Developer Field values are imported when present in activity streams; recognized developer apps are listed below.") {
		t.Fatal("Generic Support should mention developer-field import without turning into a feature catalog")
	}
	inventorySentence := "Detected recording devices and most sensors with serial numbers appear on activity pages and are also listed under Gear (`/hardware/recorder` and `/hardware/sensor`). They can also be used in activity filters and automation rules."
	if !strings.Contains(section, inventorySentence) {
		t.Fatal("Generic Support should mention the web UI hardware inventory and rule/filter usage")
	}
	if !strings.Contains(section, "\n## Generic Support\n\n"+inventorySentence+"\n\nFIT files from any device are imported.") {
		t.Fatal("Generic Support should introduce the activity/Gear inventory note before import behavior")
	}
	if strings.Contains(section, "**Skadia imports FIT files") ||
		strings.Contains(section, "**FIT files from any device are imported.") {
		t.Fatal("Generic Support import statement should not be bold or repeat the product name")
	}
	formatParagraph := "FIT files from any device are imported. All ANT+ and Bluetooth Low Energy sensors are automatically recognized. Data is fully imported even if the specific model name is not listed below. GPX and TCX files can also be imported, but they usually contain less device and sensor metadata."
	if !strings.Contains(section, formatParagraph) {
		t.Fatal("Generic Support should clarify that non-FIT imports are supported with less metadata")
	}
	if strings.Contains(section, "—") {
		t.Fatal("Generic Support should avoid em dashes in prose")
	}
	if strings.Contains(section, "not listed below.\n\nGPX and TCX files") {
		t.Fatal("Generic Support should keep FIT and GPX/TCX import notes in one paragraph")
	}
	if strings.Contains(section, "- Detected recording devices") {
		t.Fatal("Generic Support should not bury the activity/Gear inventory note as a list item")
	}
}

func TestSupportedDevicesDeveloperAppsPresentation(t *testing.T) {
	generated := generateSupportedDevicesMD()
	sectionStart := strings.Index(generated, "\n## Developer Apps\n")
	if sectionStart < 0 {
		t.Fatal("Developer Apps section not found")
	}
	sectionEnd := strings.Index(generated[sectionStart:], "\n## Smart Trainers & Virtual Platforms\n")
	if sectionEnd < 0 {
		t.Fatal("Smart Trainers section not found after Developer Apps")
	}
	section := generated[sectionStart : sectionStart+sectionEnd]

	if strings.Contains(section, "| app_") {
		t.Fatal("Developer Apps section should not expose raw app_* categories")
	}
	for _, row := range []string{
		"| CORE | Body Temperature | Registry |",
		"| Intervals CIQ | Training App | Registry |",
		"| SuuntoFitExport1 | Sport Watch | Registry |",
		"| Varia Radar | Radar | Registry |",
		"| Stryd | Running Power | Registry |",
	} {
		if !strings.Contains(section, row) {
			t.Fatalf("Developer Apps section missing expected row %q", row)
		}
	}
	if strings.Contains(section, "Stryd has 3 UUIDs") {
		t.Fatal("Developer Apps section should not contain a stale static Stryd UUID count")
	}
	for _, text := range []string{
		"| [`services/api/internal/fitimport/developer_app_registry.go`](services/api/internal/fitimport/developer_app_registry.go) | Third-party apps with known Application ID | Application ID hex lookup |",
		"Developer Apps use FIT Developer Fields to embed custom data and are detected by Application ID. Multiple application IDs can resolve to the same app entry.",
		"- **Registry** — Resolved via Application ID in [developer_app_registry.go](services/api/internal/fitimport/developer_app_registry.go)",
	} {
		if !strings.Contains(generated, text) {
			t.Fatalf("Supported devices docs should use Application ID terminology, missing %q", text)
		}
	}
	if strings.Contains(generated, "Application UUID") || strings.Contains(generated, "UUIDs") {
		t.Fatal("Supported devices docs should not describe FIT application IDs as UUIDs")
	}
}

func TestSupportedDevicesGeneratedMarkdownHasNoTrailingWhitespace(t *testing.T) {
	for lineNo, line := range strings.Split(generateSupportedDevicesMD(), "\n") {
		if strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t") {
			t.Fatalf("Generated markdown line %d has trailing whitespace: %q", lineNo+1, line)
		}
	}
}

func TestSupportedDevicesDeveloperFieldSensorsAreDiscoverable(t *testing.T) {
	generated := generateSupportedDevicesMD()
	sectionStart := strings.Index(generated, "\n## Sensors\n")
	if sectionStart < 0 {
		t.Fatal("Sensors section not found")
	}
	sectionEnd := strings.Index(generated[sectionStart:], "\n## Developer Apps\n")
	if sectionEnd < 0 {
		t.Fatal("Developer Apps section not found after Sensors")
	}
	section := generated[sectionStart : sectionStart+sectionEnd]

	if !strings.Contains(section, "Some sensor-like devices and apps, including Stryd, use FIT Developer Fields instead of standard ANT+/BLE sensor records. If a sensor is not listed here, also check [Developer Apps](#developer-apps).") {
		t.Fatal("Sensors section should point users to Developer Apps for developer-field sensors")
	}
	for _, notSensor := range []string{
		"| Stryd | Developer Field | Running Power | Registry |",
		"| CORE | Developer Field | Body Temperature | Registry |",
		"| FORM | Developer Field | Swim Goggle | Registry |",
		"| Varia Radar | Developer Field | Radar | Registry |",
		"| Zepp | Developer Field | Wearable | Registry |",
		"| COROS | Developer Field | Running Power | Registry |",
		"| Daefore | Developer Field | Bike Power | Registry |",
		"| Intervals CIQ | Developer Field | Training App | Registry |",
		"| RideWithPower | Developer Field | Bike Power | Registry |",
		"| SuuntoFitExport1 | Developer Field | Sport Watch | Registry |",
	} {
		if strings.Contains(section, notSensor) {
			t.Fatalf("Sensors section should not include non-sensor developer app row %q", notSensor)
		}
	}
	if strings.Contains(section, "Registry rows in this table are detected from FIT Developer Fields") {
		t.Fatal("Sensors section should not describe developer-field rows that are no longer listed in the table")
	}
}

func TestSupportedDevicesPolarSensorsStayMapBacked(t *testing.T) {
	generated := generateSupportedDevicesMD()
	sectionStart := strings.Index(generated, "\n## Sensors\n")
	if sectionStart < 0 {
		t.Fatal("Sensors section not found")
	}
	sectionEnd := strings.Index(generated[sectionStart:], "\n## Developer Apps\n")
	if sectionEnd < 0 {
		t.Fatal("Developer Apps section not found after Sensors")
	}
	section := generated[sectionStart : sectionStart+sectionEnd]

	for _, row := range []string{
		"| Polar | H10 | Heart Rate | Map |",
		"| Polar | H9 | Heart Rate | Map |",
		"| Polar | OH1 | Heart Rate | Map |",
		"| Polar | Verity Sense | Heart Rate | Map |",
	} {
		if !strings.Contains(section, row) {
			t.Fatalf("Sensors table should include Polar map capability row %q", row)
		}
	}
	if strings.Contains(section, "| Polar | H10 | Heart Rate | Product name |") {
		t.Fatal("Polar H10 should be listed through the static map capability, not a generator-only product-name side list")
	}
}

func TestSupportedDevicesUpToDate(t *testing.T) {
	generated := generateSupportedDevicesMD()
	path := filepath.Join(repoRoot(), "SUPPORTED_DEVICES.md")
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("SUPPORTED_DEVICES.md not found at %s: %v", path, err)
	}
	if generated != string(committed) {
		t.Fatal("SUPPORTED_DEVICES.md is out of date. Run:\n  go test ./internal/fitimport/ -run TestUpdateSupportedDevices -update")
	}
}

func TestUpdateSupportedDevices(t *testing.T) {
	if !*update {
		t.Skip("use -update flag to regenerate SUPPORTED_DEVICES.md")
	}
	generated := generateSupportedDevicesMD()
	path := filepath.Join(repoRoot(), "SUPPORTED_DEVICES.md")
	if err := os.WriteFile(path, []byte(generated), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
	t.Logf("SUPPORTED_DEVICES.md updated at %s", path)
}
