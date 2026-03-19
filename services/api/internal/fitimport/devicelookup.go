package fitimport

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/muktihari/fit/profile/typedef"
)

// nonGarminProducts maps [manufacturerID, productID] to a human-readable product name.
// Covers ~55 known non-Garmin devices from popular sport electronics brands.
var nonGarminProducts = map[[2]uint16]string{
	// Wahoo Fitness (32)
	{32, 0}:  "Wahoo TICKR",
	{32, 1}:  "Wahoo TICKR Run",
	{32, 2}:  "Wahoo KICKR",
	{32, 3}:  "Wahoo ELEMNT GPS",
	{32, 4}:  "Wahoo KICKR SNAP",
	{32, 22}: "Wahoo TICKR X",
	{32, 23}: "Wahoo ELEMNT Bolt",
	{32, 24}: "Wahoo KICKR CORE",
	{32, 25}: "Wahoo TICKR Fit",
	{32, 26}: "Wahoo ELEMNT Roam",
	{32, 27}: "Wahoo KICKR CLIMB",
	{32, 28}: "Wahoo ELEMNT",
	{32, 29}: "Wahoo KICKR HEADWIND",
	{32, 31}: "Wahoo ELEMNT Bolt V2",
	{32, 35}: "Wahoo ELEMNT Roam V2",
	{32, 43}: "Wahoo ELEMNT Ace",

	// Polar Electro (123)
	{123, 1}:  "Polar H7",
	{123, 2}:  "Polar H10",
	{123, 3}:  "Polar OH1",
	{123, 4}:  "Polar Vantage M",
	{123, 5}:  "Polar Vantage V",
	{123, 6}:  "Polar Grit X",
	{123, 7}:  "Polar Ignite",
	{123, 8}:  "Polar Vantage V2",
	{123, 9}:  "Polar Ignite 2",
	{123, 10}: "Polar Pacer",
	{123, 11}: "Polar Pacer Pro",

	// Favero Electronics (263)
	{263, 10}: "Favero Assioma Uno",
	{263, 12}: "Favero Assioma Duo",

	// SRAM (268)
	{268, 1}: "SRAM AXS Power Meter",
	{268, 2}: "SRAM Red AXS",
	{268, 3}: "SRAM Force AXS",
	{268, 4}: "SRAM Rival AXS",

	// Stages Cycling (69)
	{69, 0}: "Stages Power Meter",
	{69, 1}: "Stages L Power Meter",
	{69, 2}: "Stages LR Power Meter",
	{69, 3}: "Stages SC3",
	{69, 4}: "Stages SB20",

	// SRM (6)
	{6, 0}: "SRM PC8",
	{6, 1}: "SRM PC9",
	{6, 2}: "SRM Origin",
	{6, 3}: "SRM PC10",

	// Quarq (7)
	{7, 0}: "Quarq DZero",
	{7, 1}: "Quarq SRAM DZero",
	{7, 2}: "Quarq Dfour",
	{7, 3}: "Quarq Riken",

	// PowerTap (Saris = 9)
	{9, 0}: "PowerTap P1",
	{9, 1}: "PowerTap P2",
	{9, 2}: "PowerTap G3",
	{9, 3}: "PowerTap C1",

	// Scosche (83)
	{83, 0}: "Scosche Rhythm+",
	{83, 1}: "Scosche Rhythm 24",

	// BSX Athletics (98)
	{98, 0}: "BSX Insight",

	// Power2max (o_synce = 38)
	{38, 0}: "Power2max NG",
	{38, 1}: "Power2max NG eco",
	{38, 2}: "Power2max Type S",

	// IpBike (ibike = 8)
	{8, 0}: "iBike Newton",
	{8, 1}: "iBike Arc",

	// Morpheus (development = 255)
	{255, 1}: "Morpheus Sleep Coach",

	// Sigma Sport (70)
	{70, 0}:  "Sigma ROX",
	{70, 1}:  "Sigma BC 14.0",
	{70, 2}:  "Sigma BC 16.12",
	{70, 3}:  "Sigma ROX 4.0",
	{70, 4}:  "Sigma ROX 7.0",
	{70, 5}:  "Sigma ROX 10.0",
	{70, 6}:  "Sigma ROX 11.1 Evo",
	{70, 30}: "Sigma ROX 11.0",
	{70, 40}: "Sigma ROX 11.1",
	{70, 44}: "Sigma ROX 12.0",
	{70, 50}: "Sigma ROX 12.1 Evo",

	// Lezyne (258)
	{258, 1}: "Lezyne Micro GPS",
	{258, 2}: "Lezyne Mini GPS",
	{258, 3}: "Lezyne GPS",
	{258, 4}: "Lezyne Super GPS",
	{258, 5}: "Lezyne Mega C GPS",
	{258, 6}: "Lezyne Mega XL GPS",

	// Tacx (pre-acquisition by Garmin, manufacturer 89)
	{89, 1}: "Tacx NEO",
	{89, 2}: "Tacx Flux",
	{89, 3}: "Tacx Bushido",
	{89, 4}: "Tacx Vortex",
	{89, 5}: "Tacx Genius",
	{89, 6}: "Tacx NEO 2",
	{89, 7}: "Tacx Flux 2",
	{89, 8}: "Tacx NEO 2T",
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
	isUnknownMfr := strings.HasPrefix(mfrStr, "Manufacturer(")

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
			if !strings.HasPrefix(gpStr, "GarminProduct(") {
				return formatDeviceNameFromEnum("Garmin", gpStr)
			}
		}
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
	if s == "" || s == "ManufacturerInvalid" || strings.HasPrefix(s, "Manufacturer(") {
		return ""
	}

	// Special cases for brands with non-standard capitalization
	switch s {
	case "srm":
		return "SRM"
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
