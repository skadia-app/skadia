package importer

import (
	"context"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// buildEnrichFields erstellt die Map der zu aktualisierenden Felder für EnrichActivity.
// Schreibt nur Felder die in existing nil/0 sind und in newData einen Wert haben.
// Reine Funktion ohne Seiteneffekte — direkt testbar.
//
// Regel: 0 gilt als "nicht gesetzt" für alle numerischen Metriken. Das ist ein
// akzeptabler Trade-off, da legitime Null-Werte (z.B. 0 Höhenmeter) nicht von
// "nie berechnet" unterscheidbar sind.
func buildEnrichFields(existing *model.Activity, newData *model.ActivityData) map[string]any {
	fields := map[string]any{}
	if existing != nil {
		if existing.AvgHeartRate == nil && newData.AvgHeartRate > 0 {
			fields["avg_heart_rate"] = newData.AvgHeartRate
		}
		if existing.MaxHeartRate == nil && newData.MaxHeartRate > 0 {
			fields["max_heart_rate"] = newData.MaxHeartRate
		}
		if existing.AvgPowerW == nil && newData.AvgPowerW > 0 {
			fields["avg_power_w"] = newData.AvgPowerW
		}
		if existing.MaxPowerW == nil && newData.MaxPowerW > 0 {
			fields["max_power_w"] = newData.MaxPowerW
		}
		if existing.AvgCadence == nil && newData.AvgCadence > 0 {
			fields["avg_cadence"] = newData.AvgCadence
		}
		if existing.AvgTemperatureC == nil && newData.AvgTemperatureC > 0 {
			fields["avg_temperature_c"] = newData.AvgTemperatureC
		}
		if existing.ElevationGainM == nil && newData.ElevationGainM > 0 {
			fields["elevation_gain_m"] = newData.ElevationGainM
		}
		if existing.ElevationLossM == nil && newData.ElevationLossM > 0 {
			fields["elevation_loss_m"] = newData.ElevationLossM
		}
		if existing.MaxElevationM == nil && newData.MaxElevationM > 0 {
			fields["max_elevation_m"] = newData.MaxElevationM
		}
		if existing.MinElevationM == nil && newData.MinElevationM > 0 {
			fields["min_elevation_m"] = newData.MinElevationM
		}
		if existing.MovingTimeS == nil && newData.MovingTimeS > 0 {
			fields["moving_time_s"] = newData.MovingTimeS
		}
		if existing.TSS == nil && newData.TSS > 0 {
			fields["tss"] = newData.TSS
		}
		if existing.DistanceM == nil && newData.DistanceM > 0 {
			fields["distance_m"] = newData.DistanceM
		}
	} else {
		// Kein existing übergeben — alle non-zero Felder aus newData schreiben.
		if newData.AvgHeartRate > 0 {
			fields["avg_heart_rate"] = newData.AvgHeartRate
		}
		if newData.MaxHeartRate > 0 {
			fields["max_heart_rate"] = newData.MaxHeartRate
		}
		if newData.AvgPowerW > 0 {
			fields["avg_power_w"] = newData.AvgPowerW
		}
		if newData.MaxPowerW > 0 {
			fields["max_power_w"] = newData.MaxPowerW
		}
		if newData.AvgCadence > 0 {
			fields["avg_cadence"] = newData.AvgCadence
		}
		if newData.AvgTemperatureC > 0 {
			fields["avg_temperature_c"] = newData.AvgTemperatureC
		}
		if newData.ElevationGainM > 0 {
			fields["elevation_gain_m"] = newData.ElevationGainM
		}
		if newData.ElevationLossM > 0 {
			fields["elevation_loss_m"] = newData.ElevationLossM
		}
		if newData.MaxElevationM > 0 {
			fields["max_elevation_m"] = newData.MaxElevationM
		}
		if newData.MinElevationM > 0 {
			fields["min_elevation_m"] = newData.MinElevationM
		}
		if newData.MovingTimeS > 0 {
			fields["moving_time_s"] = newData.MovingTimeS
		}
		if newData.TSS > 0 {
			fields["tss"] = newData.TSS
		}
		if newData.DistanceM > 0 {
			fields["distance_m"] = newData.DistanceM
		}
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
// Schreibt nur Felder die in der bestehenden Aktivität nil/0 sind und in newData vorhanden.
// Gibt die Anzahl gefüllter Felder zurück.
func EnrichActivity(ctx context.Context, db *database.DB, existingID string, existing *model.Activity, newData *model.ActivityData) (int, error) {
	if newData == nil {
		return 0, nil
	}
	fields := buildEnrichFields(existing, newData)
	if len(fields) == 0 {
		return 0, nil
	}
	if err := db.UpdateEnrichmentFields(ctx, existingID, fields); err != nil {
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
