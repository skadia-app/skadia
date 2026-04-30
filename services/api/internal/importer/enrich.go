package importer

import (
	"context"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// buildEnrichFields erstellt die Map der zu aktualisierenden Felder für EnrichActivity.
// Nur Felder bei denen existing nil/0 ist UND newData non-zero hat werden inkludiert.
// Die COALESCE-Logik im DB-Layer (UpdateEnrichmentFieldsSafe) ist ein zusätzliches
// Safety-Net gegen Race Conditions bei parallelen Imports.
//
// Regel: 0 gilt als "nicht gesetzt" für alle numerischen Metriken.
func buildEnrichFields(existing *model.Activity, newData *model.ActivityData) map[string]any {
	fields := map[string]any{}
	missingF := func(p *float64) bool { return p == nil || *p == 0 }
	missingI := func(p *int) bool { return p == nil || *p == 0 }

	if newData.AvgHeartRate > 0 && (existing == nil || missingF(existing.AvgHeartRate)) {
		fields["avg_heart_rate"] = newData.AvgHeartRate
	}
	if newData.MaxHeartRate > 0 && (existing == nil || missingI(existing.MaxHeartRate)) {
		fields["max_heart_rate"] = newData.MaxHeartRate
	}
	if newData.AvgPowerW > 0 && (existing == nil || missingF(existing.AvgPowerW)) {
		fields["avg_power_w"] = newData.AvgPowerW
	}
	if newData.MaxPowerW > 0 && (existing == nil || missingI(existing.MaxPowerW)) {
		fields["max_power_w"] = newData.MaxPowerW
	}
	if newData.AvgCadence > 0 && (existing == nil || missingF(existing.AvgCadence)) {
		fields["avg_cadence"] = newData.AvgCadence
	}
	if newData.AvgTemperatureC != 0 && (existing == nil || missingF(existing.AvgTemperatureC)) {
		fields["avg_temperature_c"] = newData.AvgTemperatureC
	}
	if newData.ElevationGainM > 0 && (existing == nil || missingF(existing.ElevationGainM)) {
		fields["elevation_gain_m"] = newData.ElevationGainM
	}
	if newData.ElevationLossM > 0 && (existing == nil || missingF(existing.ElevationLossM)) {
		fields["elevation_loss_m"] = newData.ElevationLossM
	}
	if newData.MaxElevationM != 0 && (existing == nil || missingF(existing.MaxElevationM)) {
		fields["max_elevation_m"] = newData.MaxElevationM
	}
	if newData.MinElevationM != 0 && (existing == nil || missingF(existing.MinElevationM)) {
		fields["min_elevation_m"] = newData.MinElevationM
	}
	if newData.MovingTimeS > 0 && (existing == nil || missingI(existing.MovingTimeS)) {
		fields["moving_time_s"] = newData.MovingTimeS
	}
	if newData.TSS > 0 && (existing == nil || missingF(existing.TSS)) {
		fields["tss"] = newData.TSS
	}
	if newData.DistanceM > 0 && (existing == nil || missingF(existing.DistanceM)) {
		fields["distance_m"] = newData.DistanceM
	}
	for col, val := range duplicateSportUpdateFields(existing, newData) {
		fields[col] = val
	}
	return fields
}

// buildReplaceFields returns the fields ReplaceActivity may overwrite.
// Parser snapshots are updated on import, while manual sport decisions keep
// their effective tuple and modifiers.
func buildReplaceFields(existing *model.Activity, newData *model.ActivityData) map[string]any {
	fields := map[string]any{
		"avg_heart_rate":    newData.AvgHeartRate,
		"max_heart_rate":    newData.MaxHeartRate,
		"avg_power_w":       newData.AvgPowerW,
		"max_power_w":       newData.MaxPowerW,
		"avg_cadence":       newData.AvgCadence,
		"avg_temperature_c": newData.AvgTemperatureC,
		"elevation_gain_m":  newData.ElevationGainM,
		"elevation_loss_m":  newData.ElevationLossM,
		"max_elevation_m":   newData.MaxElevationM,
		"min_elevation_m":   newData.MinElevationM,
		"moving_time_s":     newData.MovingTimeS,
		"distance_m":        newData.DistanceM,
		"tss":               newData.TSS,
		"parsed_sport_type": newData.SportType,
		"parsed_sub_type":   nilIfEmptySportField(newData.SubType),
	}

	if existing == nil || !existing.SportTypeIsManual {
		fields["sport_type"] = newData.SportType
		fields["sub_type"] = nilIfEmptySportField(newData.SubType)
		fields["is_indoor"] = newData.IsIndoor
		fields["is_electric"] = newData.IsElectric
		fields["is_virtual"] = newData.IsVirtual
		fields["exclude_from_heatmap"] = newData.IsVirtual
	}
	return fields
}

func duplicateSportUpdateFields(existing *model.Activity, newData *model.ActivityData) map[string]any {
	fields := map[string]any{}
	if existing == nil || newData == nil || newData.SportType == "" {
		return fields
	}
	if existing.SportTypeIsManual {
		return fields
	}

	existingSubType := ""
	if existing.SubType != nil {
		existingSubType = *existing.SubType
	}

	if existing.SportType == "" || existing.SportType == "generic" || existing.SportType == "unknown" {
		fields["sport_type"] = newData.SportType
		fields["sub_type"] = nilIfEmptySportField(newData.SubType)
		fields["parsed_sport_type"] = newData.SportType
		fields["parsed_sub_type"] = nilIfEmptySportField(newData.SubType)
	} else if existing.SportType == newData.SportType && (existingSubType == "" || existingSubType == "generic") && newData.SubType != "" && newData.SubType != "generic" {
		fields["sub_type"] = newData.SubType
		fields["parsed_sub_type"] = newData.SubType
	}

	if newData.IsElectric && !existing.IsElectric {
		fields["is_electric"] = true
	}
	if newData.IsIndoor && !existing.IsIndoor {
		fields["is_indoor"] = true
	}
	if newData.IsVirtual && !existing.IsVirtual {
		fields["is_virtual"] = true
		fields["exclude_from_heatmap"] = true
	}
	return fields
}

func nilIfEmptySportField(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// EnrichActivity füllt fehlende Felder einer bestehenden Aktivität aus neuen Daten.
// Verwendet COALESCE-Semantik im DB-Layer: existierende Werte werden nie überschrieben.
// Parallele Imports sind damit race-condition-frei — kein Locking nötig.
// Gibt die Anzahl übermittelter Felder zurück (nicht alle werden notwendigerweise geschrieben).
func EnrichActivity(ctx context.Context, db *database.DB, existingID string, existing *model.Activity, newData *model.ActivityData) (int, error) {
	if newData == nil {
		return 0, nil
	}
	fields := buildEnrichFields(existing, newData)
	directFields := duplicateSportUpdateFields(existing, newData)
	for col := range directFields {
		delete(fields, col)
	}
	if len(fields) == 0 {
		if len(directFields) == 0 {
			return 0, nil
		}
	} else if err := db.UpdateEnrichmentFieldsSafe(ctx, existingID, fields); err != nil {
		return 0, err
	}
	if len(directFields) > 0 {
		if err := db.UpdateEnrichmentFields(ctx, existingID, directFields); err != nil {
			return 0, err
		}
	}
	return len(fields) + len(directFields), nil
}

// ReplaceActivity überschreibt alle Felder einer Aktivität mit neuen Daten.
func ReplaceActivity(ctx context.Context, db *database.DB, existingID string, newData *model.ActivityData) error {
	if newData == nil {
		return nil
	}
	existing, err := db.GetActivity(ctx, existingID)
	if err != nil {
		return err
	}
	return db.UpdateEnrichmentFields(ctx, existingID, buildReplaceFields(existing, newData))
}
