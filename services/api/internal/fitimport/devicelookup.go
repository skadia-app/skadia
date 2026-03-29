package fitimport

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/muktihari/fit/profile/typedef"
)

// nonGarminProducts maps [manufacturerID, productID] to a human-readable product name.
// IDs are verified from the FIT SDK where available and device FIT files.
// Garmin-Produkte (manufacturer_id=1) sind NICHT enthalten — die kommen via GarminProduct.String().
// Tacx-Produkte post-Akquisition sind ebenfalls unter GarminProduct (IDs 4265–4276, mfr=1).
var nonGarminProducts = map[[2]uint16]string{
	// Wahoo Fitness (32) — verifiziert: ELEMNT prod=28 aus echtem FIT-File
	{32, 8}:  "Wahoo TICKR",
	{32, 28}: "Wahoo ELEMNT",
	{32, 31}: "Wahoo ELEMNT BOLT",
	{32, 33}: "Wahoo ELEMNT Rival",
	{32, 37}: "Wahoo ELEMNT ROAM",
	{32, 41}: "Wahoo PowrLink",
	{32, 43}: "Wahoo ELEMNT BOLT v2",
	{32, 47}: "Wahoo ELEMNT ROAM v2",

	// Polar Electro (123)
	{123, 1}:  "Polar V800",
	{123, 2}:  "Polar H10",
	{123, 3}:  "Polar H9",
	{123, 4}:  "Polar Vantage V",
	{123, 5}:  "Polar Vantage V Titan",
	{123, 6}:  "Polar Vantage M",
	{123, 7}:  "Polar Grit X",
	{123, 8}:  "Polar Ignite",
	{123, 9}:  "Polar Ignite 2",
	{123, 10}: "Polar Vantage V2",
	{123, 11}: "Polar Grit X Pro",
	{123, 12}: "Polar Pacer",
	{123, 13}: "Polar Pacer Pro",
	{123, 14}: "Polar Vantage V3",
	{123, 15}: "Polar Grit X2 Pro",
	{123, 16}: "Polar Ignite 3",
	{123, 17}: "Polar Unite",
	{123, 18}: "Polar Vantage M2",
	{123, 20}: "Polar OH1",

	// Favero Electronics (263) — Assioma Uno/Duo im FIT SDK als FaveroProduct definiert
	{263, 10}: "Favero Assioma Uno",
	{263, 12}: "Favero Assioma Duo",
	{263, 22}: "Favero Assioma Pro MX",

	// SRAM (268)
	{268, 1037}: "SRAM Rival eTap AXS",
	{268, 1052}: "SRAM Rival AXS",

	// Stages Cycling (69)
	{69, 1}: "Stages Power Gen 1",
	{69, 2}: "Stages Power Gen 2",
	{69, 3}: "Stages Power Gen 3",

	// SRM (6)
	{6, 6}: "SRM PC6",
	{6, 7}: "SRM PC7",
	{6, 8}: "SRM PC8",

	// Quarq (7)
	{7, 1}:    "Quarq Cinqo",
	{7, 9479}: "Quarq DZero",

	// Powertap / Saris (9)
	{9, 14}:   "Powertap Joule 2.0",
	{9, 18}:   "Powertap Joule",
	{9, 19}:   "Powertap Joule GPS",
	{9, 22}:   "Powertap Joule GPS+",
	{9, 272}:  "Powertap C1",
	{9, 288}:  "Powertap P1",
	{9, 4096}: "Powertap G3",
	{9, 4353}: "Powertap Powercal",

	// Scosche (83)
	{83, 3}: "Scosche Rhythm+",

	// BSX Athletics (98)
	{98, 2}: "BSX Insight 2",

	// Saxonar / Power2max (29) — NICHT 38 (o_synce)!
	{29, 1031}: "Power2max S",

	// o_synce (38)
	{38, 1}: "o_synce navi2coach",

	// IpBike / Ifor Powell (54) — NICHT 8 (iBike)!
	{54, 1}: "IpBike",

	// Fitcare / Morpheus (106)
	{106, 5}: "Morpheus M7",

	// Sigma Sport (70)
	{70, 15}: "Sigma ROX 10.0",
	{70, 18}: "Sigma ROX 7.0",
	{70, 41}: "Sigma ROX 11.0",
	{70, 42}: "Sigma iD.RUN",
	{70, 43}: "Sigma iD.RUN HR",
	{70, 44}: "Sigma ROX 12.0",
	{70, 45}: "Sigma iD.FREE",
	{70, 46}: "Sigma iD.TRI",
	{70, 47}: "Sigma PURE GPS",
	{70, 49}: "Sigma ROX 11.1",
	{70, 51}: "Sigma ROX 2.0",
	{70, 52}: "Sigma ROX 4.0",
	{70, 54}: "Sigma ROX 12.1 EVO",
	{70, 69}: "Sigma ROX 4.0 SE",

	// Lezyne (258)
	{258, 1}:  "Lezyne Super GPS",
	{258, 2}:  "Lezyne Macro GPS",
	{258, 3}:  "Lezyne Micro C GPS",
	{258, 4}:  "Lezyne Super GPS",
	{258, 5}:  "Lezyne Super Pro GPS",
	{258, 6}:  "Lezyne Micro GPS",
	{258, 11}: "Lezyne Mega XL GPS",

	// Suunto (23)
	{23, 19}: "Suunto Ambit3 Peak",
	{23, 20}: "Suunto 3 Fitness",
	{23, 22}: "Suunto Ambit3 Run",
	{23, 23}: "Suunto Ambit3 Sport",
	{23, 24}: "Suunto Ambit3 Vertical",
	{23, 25}: "Suunto Spartan Sport",
	{23, 26}: "Suunto Spartan Sport Wrist HR",
	{23, 27}: "Suunto Spartan Ultra",
	{23, 28}: "Suunto Spartan Trainer",
	{23, 29}: "Suunto 5",
	{23, 34}: "Suunto 7",
	{23, 35}: "Suunto 9 Baro",
	{23, 36}: "Suunto 9 Peak",
	{23, 37}: "Suunto 9 Peak Pro",
	{23, 38}: "Suunto Race",
	{23, 39}: "Suunto Vertical",
	{23, 40}: "Suunto Race S",
	{23, 41}: "Suunto Ocean",

	// COROS (294)
	{294, 1}:  "COROS PACE",
	{294, 2}:  "COROS PACE 2",
	{294, 3}:  "COROS PACE 3",
	{294, 4}:  "COROS APEX",
	{294, 5}:  "COROS APEX Pro",
	{294, 6}:  "COROS APEX 2",
	{294, 7}:  "COROS APEX 2 Pro",
	{294, 8}:  "COROS VERTIX",
	{294, 9}:  "COROS VERTIX 2",
	{294, 10}: "COROS VERTIX 2S",
	{294, 11}: "COROS POD",
	{294, 12}: "COROS POD 2",
	{294, 14}: "COROS DURA",
	{294, 15}: "COROS PACE Pro",

	// Magene (107)
	{107, 20}: "Magene C406",
	{107, 30}: "Magene C606",
	{107, 40}: "Magene L508",

	// Wattbike (73)
	{73, 26}: "Wattbike Atom",
	{73, 29}: "Wattbike Icon",
	{73, 30}: "Wattbike Atom X",

	// Hammerhead (289)
	{289, 1}: "Hammerhead Karoo",
	{289, 2}: "Hammerhead Karoo 2",

	// TomTom (71)
	{71, 2015}: "TomTom Runner 3 Cardio",

	// Timex (16)
	{16, 87}: "Timex Ironman GPS",

	// Xplova (45)
	{45, 22582}: "Xplova X5 EVO",
	{45, 20051}: "Xplova Noza S",

	// Cycplus (132)
	{132, 1711}: "Cycplus M1",
	{132, 2104}: "Cycplus M2",

	// Dabuziduo/XOSS (292)
	{292, 10}: "XOSS G+N4",
	{292, 11}: "XOSS G+N3",

	// Coospo (135)
	{135, 1107}: "Coospo BC107",
	{135, 1200}: "Coospo BC200",

	// Tacx (89) — Pre-Akquisition IDs, für historische FIT-Dateien
	{89, 2800}: "Tacx Neo Smart",
	{89, 2850}: "Tacx Neo 2 Smart",
	{89, 3111}: "Tacx Breakaway",
	{89, 4274}: "Tacx Rouvy",
}

// ResolveProductName resolves a human-readable device product name from FIT device info fields.
// Priority order:
//  1. productName string from FIT (explicit device name) — prepend manufacturer if not already present
//  2. GarminProduct enum string from SDK (for Garmin/Dynastream/Tacx manufacturers)
//  3. Lookup in nonGarminProducts map by [manufacturerID, productID]
//  4. Manufacturer name as fallback
//
// Returns "" for ManufacturerInvalid (65535).
func ResolveProductName(manufacturerID, productID uint16, productName string) string {
	mfr := typedef.Manufacturer(manufacturerID)

	// Invalid manufacturer → no useful name
	if mfr == typedef.ManufacturerInvalid {
		return ""
	}

	mfrStr := mfr.String()
	isUnknownMfr := strings.HasPrefix(mfrStr, "Manufacturer")

	// 1. Explicit product_name from FIT message
	if productName != "" {
		mfrDisplay := FormatManufacturer(mfrStr)
		if mfrDisplay != "" {
			// Use only the first word of the manufacturer name as prefix (e.g. "Wahoo" from "Wahoo Fitness")
			brandPrefix := strings.Fields(mfrDisplay)[0]
			if !strings.Contains(strings.ToLower(productName), strings.ToLower(brandPrefix)) {
				return fmt.Sprintf("%s %s", brandPrefix, productName)
			}
		}
		return productName
	}

	// 2. GarminProduct SDK resolution (Garmin, Dynastream, DynastreamOem, Tacx)
	isGarminFamily := mfr == typedef.ManufacturerGarmin ||
		mfr == typedef.ManufacturerDynastream ||
		mfr == typedef.ManufacturerDynastreamOem ||
		mfr == typedef.ManufacturerTacx

	if isGarminFamily {
		gp := typedef.GarminProduct(productID)
		if gp != typedef.GarminProductInvalid {
			gpStr := gp.String()
			if !strings.HasPrefix(gpStr, "GarminProduct") {
				return formatDeviceNameFromEnum("Garmin", gpStr)
			}
		}
		// Unknown Garmin product — no useful name available
		return ""
	}

	// For Favero, use FaveroProduct enum
	if mfr == typedef.ManufacturerFaveroElectronics {
		fp := typedef.FaveroProduct(productID)
		if fp != typedef.FaveroProductInvalid {
			fpStr := fp.String()
			if !strings.HasPrefix(fpStr, "FaveroProduct") {
				return formatDeviceNameFromEnum("Favero", fpStr)
			}
		}
	}

	// 3. Lookup in static non-Garmin product map
	if name, ok := nonGarminProducts[[2]uint16{manufacturerID, productID}]; ok {
		return name
	}

	// 4. Manufacturer name fallback
	if isUnknownMfr {
		return ""
	}
	mfrDisplay := FormatManufacturer(mfrStr)
	return mfrDisplay
}

// FormatManufacturer converts a manufacturer enum string (snake_case) to a human-readable name.
// Returns "" for empty strings or "ManufacturerInvalid".
// Examples: "wahoo_fitness" → "Wahoo Fitness", "garmin" → "Garmin", "srm" → "SRM"
func FormatManufacturer(s string) string {
	if s == "" || strings.HasPrefix(s, "Manufacturer") {
		return ""
	}

	// Special cases for brands with non-standard capitalization
	switch s {
	case "srm":
		return "SRM"
	case "whoop":
		return "WHOOP"
	case "sigmasport":
		return "Sigmasport"
	case "osynce":
		return "o_synce"
	}

	words := strings.FieldsFunc(s, func(r rune) bool { return r == '_' })
	for i, w := range words {
		if w == "" {
			continue
		}
		runes := []rune(w)
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

// formatDeviceNameFromEnum converts a FIT SDK enum string to a human-readable device name.
// Prepends the brand prefix if not already present.
// Examples: "edge_1040" → "Garmin Edge 1040", "assioma_duo" → "Favero Assioma Duo"
func formatDeviceNameFromEnum(brand, enumStr string) string {
	enumStr = strings.ReplaceAll(enumStr, "_", " ")
	words := strings.Fields(enumStr)
	for i, w := range words {
		if len(w) <= 2 {
			words[i] = strings.ToUpper(w) // "2x" → "2X", "xr" → "XR"
		} else {
			runes := []rune(w)
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	name := strings.Join(words, " ")
	if brand != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(brand)) {
		name = brand + " " + name
	}
	return name
}
