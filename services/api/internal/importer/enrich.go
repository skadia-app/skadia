package importer

import (
	"context"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// EnrichActivity füllt fehlende Felder einer bestehenden Aktivität aus neuen Daten.
// Schreibt nur Felder die in der bestehenden Aktivität nil/0 sind und in newData vorhanden.
// Gibt die Anzahl gefüllter Felder zurück.
func EnrichActivity(ctx context.Context, db *database.DB, existingID string, existing *model.Activity, newData *model.ActivityData) (int, error) {
	if newData == nil {
		return 0, nil
	}

	fields := map[string]any{}

	if newData != nil {
		if existing != nil {
			// model.Activity uses pointer types for optional fields; nil means missing.
			// model.ActivityData uses plain value types; 0 means absent.
			// Note: we treat 0 as "not set" for all numeric metrics.
			// This means a legitimately zero elevation gain will be overwritten if the new source
			// has data. This is an acceptable trade-off as zero elevation gains from GPS noise
			// are indistinguishable from "never computed".
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
			// Note: we treat 0 as "not set" for all numeric metrics.
			// This means a legitimately zero elevation gain will be overwritten if the new source
			// has data. This is an acceptable trade-off as zero elevation gains from GPS noise
			// are indistinguishable from "never computed".
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
	}

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
	fields := map[string]any{
		"avg_heart_rate":    newData.AvgHeartRate,
		"max_heart_rate":    newData.MaxHeartRate,
		"avg_power_w":       newData.AvgPowerW,
		"max_power_w":       newData.MaxPowerW,
		"avg_cadence":       newData.AvgCadence,
		"avg_temperature_c": newData.AvgTemperatureC,
		"elevation_gain_m":  newData.ElevationGainM,
		"moving_time_s":     newData.MovingTimeS,
		"distance_m":        newData.DistanceM,
		"tss":               newData.TSS,
	}
	return db.UpdateEnrichmentFields(ctx, existingID, fields)
}
