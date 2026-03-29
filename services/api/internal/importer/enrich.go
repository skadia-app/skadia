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
	return fields
}

// buildReplaceFields erstellt die Map aller zu überschreibenden Felder für ReplaceActivity.
// Reine Funktion ohne Seiteneffekte — direkt testbar.
func buildReplaceFields(newData *model.ActivityData) map[string]any {
	return map[string]any{
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
	}
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
	if len(fields) == 0 {
		return 0, nil
	}
	if err := db.UpdateEnrichmentFieldsSafe(ctx, existingID, fields); err != nil {
		return 0, err
	}
	return len(fields), nil
}

// ReplaceActivity überschreibt alle Felder einer Aktivität mit neuen Daten.
func ReplaceActivity(ctx context.Context, db *database.DB, existingID string, newData *model.ActivityData) error {
	if newData == nil {
		return nil
	}
	return db.UpdateEnrichmentFields(ctx, existingID, buildReplaceFields(newData))
}
