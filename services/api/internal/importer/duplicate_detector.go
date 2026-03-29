package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
func CheckTier1(ctx context.Context, db *database.DB, userID string, fileBytes []byte, data *model.ActivityData, garminID, stravaID, komootTourID int64) (*DuplicateMatch, error) {
	// 1. File-Hash (SHA256)
	// If data already carries a pre-computed hash (e.g. multisport child hashes),
	// use it directly so each child is deduplicated by its own hash, not the parent file hash.
	var hashStr string
	if data != nil && data.FileHash != "" {
		hashStr = data.FileHash
	} else {
		h := sha256.Sum256(fileBytes)
		hashStr = hex.EncodeToString(h[:])
	}
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

	// 2. Garmin Activity ID (single query: returns ID or empty string)
	if garminID > 0 {
		if id, err := db.GetActivityIDByGarminID(ctx, userID, garminID); err == nil && id != "" {
			return &DuplicateMatch{ExistingActivityID: id, Confidence: "platform_id"}, nil
		}
	}

	// 3. Strava Activity ID (single query: returns ID or empty string)
	if stravaID > 0 {
		if id, err := db.GetActivityIDByStravaID(ctx, userID, stravaID); err == nil && id != "" {
			return &DuplicateMatch{ExistingActivityID: id, Confidence: "platform_id"}, nil
		}
	}

	// 4. Komoot-ID Check — analog to Garmin/Strava: no MissingFields at platform_id confidence
	if komootTourID > 0 {
		existingID, err := db.GetActivityIDByKomootTourID(ctx, userID, komootTourID)
		if err == nil && existingID != "" {
			return &DuplicateMatch{
				ExistingActivityID: existingID,
				Confidence:         "platform_id",
			}, nil
		}
	}

	// 5. GPS Track Identity: bei ähnlicher Startzeit GPS-Tracks auf Identität prüfen.
	// Erkennt selbes Gerät in verschiedenen Formaten (z.B. FIT + GPX vom selben Garmin).
	// Multi-Device-Aufzeichnungen (verschiedene GPS-Sensoren) haben ≥5-15m Drift
	// und fallen unter 98% bei 10m Radius → werden als neue Aktivität importiert
	// und von Tier 2 (Fusion-Queue) als Fusionskandidat erkannt.
	if data != nil && !data.StartedAt.IsZero() {
		trackNew := extractLatLngFromData(data.Streams)
		if len(trackNew) >= 10 {
			from := data.StartedAt.Add(-60 * time.Second)
			to := data.StartedAt.Add(60 * time.Second)
			// Keine Sport-Filterung: verschiedene Exportformate können verschiedene Sport-Typen melden
			candidates, err := db.FindActivitiesByTimeRange(ctx, userID, from, to, "")
			if err != nil {
				return nil, err
			}
			for _, c := range candidates {
				existingStreams, err := db.GetStreams(ctx, c.ID)
				if err != nil {
					continue
				}
				trackExisting := extractLatLng(existingStreams)
				if len(trackExisting) < 10 {
					continue
				}
				// 10m Radius + 98% Threshold: nur identische Tracks matchen (selbes Gerät).
				overlap := GPSOverlapPercent(trackNew, trackExisting, 10)
				if overlap >= 98 {
					return &DuplicateMatch{
						ExistingActivityID: c.ID,
						Confidence:         "gps_identity",
						MissingFields:      missingFields(&c),
					}, nil
				}
			}
		}
	}

	return nil, nil
}

// CheckTier2 prüft ob zwei bereits importierte Aktivitäten fusioniert werden sollten.
// activityID ist die Referenz-Aktivität; threshold ist der GPS-Schwellwert (0–100, Default 75).
// timeWindowMin ist das Zeitfenster für Kandidaten-Suche (Default 30).
// Gibt FusionCandidates zurück (GPS-Ähnlichkeit >= threshold).
func CheckTier2(ctx context.Context, db *database.DB, userID string, activityID string, threshold float64, timeWindowMin int) ([]FusionCandidate, error) {
	if threshold <= 0 {
		threshold = 75
	}
	if timeWindowMin <= 0 {
		timeWindowMin = 30
	}

	// Zeitfenster-Kandidaten (existierende DB-Abfrage)
	candidateIDs, err := db.FindFusionCandidates(ctx, userID, activityID, timeWindowMin)
	if err != nil || len(candidateIDs) == 0 {
		return nil, err
	}

	// GPS-Streams der Referenz-Aktivität laden
	streamsA, err := db.GetStreams(ctx, activityID)
	if err != nil {
		return nil, fmt.Errorf("load streams A: %w", err)
	}
	trackA := extractLatLng(streamsA)
	if len(trackA) < 10 {
		return nil, nil // kein GPS → kein Tier-2
	}

	var results []FusionCandidate
	for _, candidateID := range candidateIDs {
		streamsB, err := db.GetStreams(ctx, candidateID)
		if err != nil || len(streamsB) < 10 {
			continue
		}
		trackB := extractLatLng(streamsB)
		if len(trackB) < 10 {
			continue
		}

		pct := GPSOverlapPercent(trackA, trackB, 50) // 50m radius
		if pct < threshold {
			continue
		}

		// Zeitdifferenz
		var timeDiff float64
		actA, _ := db.GetActivity(ctx, activityID)
		actB, _ := db.GetActivity(ctx, candidateID)
		if actA != nil && actB != nil {
			timeDiff = math.Abs(actB.StartedAt.Sub(actA.StartedAt).Seconds())
		}

		results = append(results, FusionCandidate{
			ActivityIDA:    activityID,
			ActivityIDB:    candidateID,
			OverlapPercent: pct,
			FieldRichnessA: countFields(actA),
			FieldRichnessB: countFields(actB),
			TimeDiffSec:    timeDiff,
		})
	}
	return results, nil
}

// extractLatLng extrahiert lat/lng aus Streams (filtert nil).
func extractLatLng(streams []model.StreamResponse) []LatLng {
	out := make([]LatLng, 0, len(streams))
	for _, s := range streams {
		if s.Lat != nil && s.Lng != nil {
			out = append(out, LatLng{Lat: *s.Lat, Lng: *s.Lng})
		}
	}
	return out
}

// extractLatLngFromData extrahiert lat/lng aus geparsten StreamPoints (filtert nil).
func extractLatLngFromData(streams []model.StreamPoint) []LatLng {
	out := make([]LatLng, 0, len(streams))
	for _, s := range streams {
		if s.Lat != nil && s.Lng != nil {
			out = append(out, LatLng{Lat: *s.Lat, Lng: *s.Lng})
		}
	}
	return out
}

// countFields zählt wie viele Metrik-Felder einer Aktivität gesetzt sind.
func countFields(a *model.Activity) int {
	if a == nil {
		return 0
	}
	n := 0
	if a.AvgHeartRate != nil && *a.AvgHeartRate > 0 {
		n++
	}
	if a.MaxHeartRate != nil && *a.MaxHeartRate > 0 {
		n++
	}
	if a.AvgPowerW != nil && *a.AvgPowerW > 0 {
		n++
	}
	if a.MaxPowerW != nil && *a.MaxPowerW > 0 {
		n++
	}
	if a.AvgCadence != nil && *a.AvgCadence > 0 {
		n++
	}
	if a.AvgTemperatureC != nil && *a.AvgTemperatureC != 0 {
		n++
	}
	if a.ElevationGainM != nil && *a.ElevationGainM > 0 {
		n++
	}
	if a.DistanceM != nil && *a.DistanceM > 0 {
		n++
	}
	return n
}

// missingFields gibt die Namen aller fehlenden Metriken (nil oder 0) zurück.
func missingFields(a *model.Activity) []string {
	fields := make([]string, 0, 8)
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
