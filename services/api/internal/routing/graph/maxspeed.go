package graph

import (
	"math"
	"strconv"
	"strings"
)

var countryDefaults = map[string]uint8{
	"DE:urban": 50, "DE:rural": 100, "DE:motorway": 130,
	"AT:urban": 50, "AT:rural": 100, "AT:motorway": 130,
	"CH:urban": 50, "CH:rural": 80, "CH:motorway": 120,
	"FR:urban": 50, "FR:rural": 80, "FR:motorway": 130,
	"IT:urban": 50, "IT:rural": 90, "IT:motorway": 130,
	"ES:urban": 50, "ES:rural": 90, "ES:motorway": 120,
	"NL:urban": 50, "NL:rural": 80, "NL:motorway": 130,
	"BE:urban": 50, "BE:rural": 70, "BE:motorway": 120,
	"PL:urban": 50, "PL:rural": 90, "PL:motorway": 140,
	"CZ:urban": 50, "CZ:rural": 90, "CZ:motorway": 130,
	"GB:urban": 48, "GB:rural": 97, "GB:motorway": 113, // mph converted
	"US:urban": 40, "US:rural": 89, "US:motorway": 105, // mph converted
}

var specialMaxspeed = map[string]uint8{
	"walk": 5, "none": 0, "signals": 0,
}

// ParseMaxspeed converts an OSM maxspeed string to uint8 km/h.
// Returns 0 for unknown/missing values. Clamps to 255.
func ParseMaxspeed(s string) uint8 {
	if s == "" {
		return 0
	}

	// Special values.
	if v, ok := specialMaxspeed[s]; ok {
		return v
	}

	// Country defaults.
	if v, ok := countryDefaults[s]; ok {
		return v
	}

	// "30 mph" format.
	if strings.HasSuffix(s, " mph") || strings.HasSuffix(s, "mph") {
		numStr := strings.TrimSuffix(strings.TrimSuffix(s, " mph"), "mph")
		numStr = strings.TrimSpace(numStr)
		if n, err := strconv.Atoi(numStr); err == nil {
			kmh := min(int(math.Round(float64(n)*1.609)), 255)
			return uint8(kmh)
		}
	}

	// Plain numeric.
	if n, err := strconv.Atoi(s); err == nil {
		if n > 255 {
			n = 255
		}
		if n < 0 {
			n = 0
		}
		return uint8(n)
	}

	return 0
}
