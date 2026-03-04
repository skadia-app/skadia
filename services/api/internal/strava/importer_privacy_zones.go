package strava

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
)

// PrivacyZonesImporter reads privacy_zones.csv from a Strava export.
//
// privacy_zones.csv columns (German):
//
//	Adresse, Radius, Breitengrad & Längengrad
//
// The "Breitengrad & Längengrad" column contains both lat and lng as a single
// comma-separated value inside one CSV field, e.g. "48.359777, 10.970624".
// The "Radius" column contains values like "0,40 Kilometer" or "1,00 Kilometer".
type PrivacyZonesImporter struct{}

func (p *PrivacyZonesImporter) Name() string { return "privacy_zones" }
func (p *PrivacyZonesImporter) Description() string {
	return "Privatsphären-Zonen (Adresse, Radius)"
}
func (p *PrivacyZonesImporter) Priority() int { return 40 }

func (p *PrivacyZonesImporter) Detect(src ExportSource) (bool, int, error) {
	rows, err := src.ReadCSV("privacy_zones.csv")
	if err != nil || len(rows) < 2 {
		return false, 0, nil
	}
	return true, len(rows) - 1, nil
}

func (p *PrivacyZonesImporter) Import(ctx context.Context, params ImportParams) (*ImportResult, error) {
	result := &ImportResult{}

	rows, err := params.Source.ReadCSV("privacy_zones.csv")
	if err != nil {
		slog.Warn("strava privacy_zones: cannot read privacy_zones.csv", "error", err)
		return result, nil
	}
	if len(rows) < 2 {
		return result, nil
	}

	hdr := csvHeader(rows[0])
	total := len(rows) - 1

	for i, row := range rows[1:] {
		// Column: "Adresse"
		address := colVal(row, hdr["Adresse"])
		// Column: "Radius" — e.g. "0,40 Kilometer" or "1,00 Kilometer"
		radiusStr := colVal(row, hdr["Radius"])
		// Column: "Breitengrad & Längengrad" — e.g. "48.359777, 10.970624"
		latLngStr := colVal(row, hdr["Breitengrad & Längengrad"])

		if params.OnProgress != nil {
			params.OnProgress(i+1, total, "privacy_zones: "+address)
		}

		if address == "" || latLngStr == "" {
			result.Skipped++
			continue
		}

		// Parse lat/lng from the combined field "48.359777, 10.970624"
		parts := strings.SplitN(latLngStr, ",", 2)
		if len(parts) != 2 {
			slog.Warn("strava privacy_zones: cannot parse lat/lng", "value", latLngStr)
			result.Skipped++
			continue
		}
		lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lng, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 != nil || err2 != nil {
			slog.Warn("strava privacy_zones: invalid lat/lng values", "value", latLngStr)
			result.Skipped++
			continue
		}

		// Parse radius — "0,40 Kilometer" → 400 m
		radiusM := parseRadiusToMeters(radiusStr)
		if radiusM <= 0 {
			radiusM = 200 // default 200m when unparseable
		}

		// Use address as the zone name
		zoneName := address

		// Check if zone with same name already exists
		exists, err := params.DB.PrivacyZoneExistsByName(ctx, params.UserID, zoneName)
		if err != nil {
			slog.Warn("strava privacy_zones: check exists failed", "name", zoneName, "error", err)
		}
		if exists {
			result.Skipped++
			continue
		}

		_, err = params.DB.CreatePrivacyZone(ctx, params.UserID, zoneName, lat, lng, radiusM)
		if err != nil {
			slog.Warn("strava privacy_zones: create failed", "name", zoneName, "error", err)
			result.Errors = append(result.Errors, "create zone "+zoneName+": "+err.Error())
			continue
		}

		slog.Info("strava privacy_zones: created zone", "name", zoneName, "lat", lat, "lng", lng, "radius_m", radiusM)
		result.Imported++
	}

	slog.Info("strava privacy_zones import done", "imported", result.Imported, "skipped", result.Skipped)
	return result, nil
}

// parseRadiusToMeters converts Strava radius strings like "0,40 Kilometer" to meters.
func parseRadiusToMeters(s string) int {
	s = strings.TrimSpace(s)
	// Remove unit suffix
	s = strings.ReplaceAll(s, " Kilometer", "")
	s = strings.ReplaceAll(s, " kilometer", "")
	s = strings.ReplaceAll(s, " km", "")
	// Parse German decimal number
	km, ok := parseGermanFloat(s)
	if !ok || km <= 0 {
		return 0
	}
	return int(km * 1000)
}
