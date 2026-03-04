package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// CheckTier1 prüft ob eine Aktivität bereits vorhanden ist (hohe Konfidenz).
// Gibt nil zurück wenn kein Duplikat erkannt wurde.
// fileBytes: Rohdaten der Datei (für SHA256-Hash)
// data: geparste Aktivitätsdaten (kann nil sein)
// garminID, stravaID: Plattform-IDs (0 = nicht vorhanden)
func CheckTier1(ctx context.Context, db *database.DB, userID string, fileBytes []byte, data *model.ActivityData, garminID, stravaID int64) (*DuplicateMatch, error) {
	// 1. File-Hash (SHA256)
	hash := sha256.Sum256(fileBytes)
	hashStr := hex.EncodeToString(hash[:])
	existing, err := db.CheckDuplicate(ctx, userID, hashStr)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return &DuplicateMatch{
			ExistingActivityID: existing.ID,
			Confidence:         "hash",
			MissingFields:      missingFields(existing),
		}, nil
	}

	// 2. Garmin Activity ID
	if garminID > 0 {
		exists, err := db.ActivityExistsByGarminID(ctx, userID, garminID)
		if err != nil {
			return nil, err
		}
		if exists {
			id, _ := db.GetActivityIDByGarminID(ctx, userID, garminID)
			return &DuplicateMatch{ExistingActivityID: id, Confidence: "platform_id"}, nil
		}
	}

	// 3. Strava Activity ID
	if stravaID > 0 {
		exists, err := db.ActivityExistsByStravaID(ctx, userID, stravaID)
		if err != nil {
			return nil, err
		}
		if exists {
			id, _ := db.GetActivityIDByStravaID(ctx, userID, stravaID)
			return &DuplicateMatch{ExistingActivityID: id, Confidence: "platform_id"}, nil
		}
	}

	// 4. Temporal Match: Zeitstempel ±60s + Sport + Distanz ±5%
	if data != nil && !data.StartedAt.IsZero() {
		from := data.StartedAt.Add(-60 * time.Second)
		to := data.StartedAt.Add(60 * time.Second)
		candidates, err := db.FindActivitiesByTimeRange(ctx, userID, from, to, data.SportType)
		if err != nil {
			return nil, err
		}
		for _, c := range candidates {
			if data.DistanceM > 0 && c.DistanceM != nil && *c.DistanceM > 0 {
				diff := math.Abs(*c.DistanceM-data.DistanceM) / data.DistanceM
				if diff <= 0.05 {
					return &DuplicateMatch{
						ExistingActivityID: c.ID,
						Confidence:         "temporal",
						MissingFields:      missingFields(&c),
					}, nil
				}
			}
		}
	}

	return nil, nil
}

// missingFields gibt die Namen aller fehlenden Metriken (nil oder 0) zurück.
func missingFields(a *model.Activity) []string {
	var fields []string
	if a.AvgHeartRate == nil || *a.AvgHeartRate == 0 {
		fields = append(fields, "avg_heart_rate")
	}
	if a.MaxHeartRate == nil || *a.MaxHeartRate == 0 {
		fields = append(fields, "max_heart_rate")
	}
	if a.AvgPowerW == nil || *a.AvgPowerW == 0 {
		fields = append(fields, "avg_power_w")
	}
	if a.MaxPowerW == nil || *a.MaxPowerW == 0 {
		fields = append(fields, "max_power_w")
	}
	if a.AvgCadence == nil || *a.AvgCadence == 0 {
		fields = append(fields, "avg_cadence")
	}
	if a.AvgTemperatureC == nil || *a.AvgTemperatureC == 0 {
		fields = append(fields, "avg_temperature_c")
	}
	if a.ElevationGainM == nil || *a.ElevationGainM == 0 {
		fields = append(fields, "elevation_gain_m")
	}
	return fields
}
