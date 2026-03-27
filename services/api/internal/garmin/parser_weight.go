package garmin

import (
	"bytes"
	"context"
	"log/slog"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// minTS is the earliest plausible Garmin timestamp (2000-01-01 UTC).
// Entries before this are considered invalid/unset.
var minTS = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// processWeightFIT parses a Garmin weight-scale FIT file (type 9) and upserts
// body-composition entries into health_weight_entries.
//
// Message 30 (WeightScale) carries up to 12 body-composition fields.
// All numeric fields use Garmin fixed-point encoding; scale factors are applied
// here to convert to SI units stored in the database.
func processWeightFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	ts := time.Now().UTC()
	storeHealthFIT(ctx, db, fs, data, userID, "weight", "weight.fit", ts) //nolint:errcheck // non-fatal

	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		slog.Warn("processWeightFIT: decode failed", "error", err)
		return nil
	}

	count := 0
	for i := range fit.Messages {
		msg := &fit.Messages[i]

		switch msg.Num {
		case typedef.MesgNumWeightScale: // 30
			ws := mesgdef.NewWeightScale(msg)
			if ws.Timestamp.IsZero() || ws.Timestamp.Before(minTS) {
				continue
			}
			if ws.Weight == typedef.WeightInvalid {
				continue
			}

			// Weight: uint16 /100 → kg, *1000 → grams
			entry := &model.HealthWeightEntry{
				UserID:        userID,
				MeasuredAtUTC: ws.Timestamp,
				WeightG:       int(float64(ws.Weight) / 100.0 * 1000.0),
				SourceType:    "garmin_fit",
			}

			// PercentFat: field 1, uint16 /100
			if ws.PercentFat != basetype.Uint16Invalid {
				v := float64(ws.PercentFat) / 100.0
				entry.BodyFatPct = &v
			}
			// PercentHydration: field 2, uint16 /100
			if ws.PercentHydration != basetype.Uint16Invalid {
				v := float64(ws.PercentHydration) / 100.0
				entry.PercentHydration = &v
			}
			// VisceralFatMass: field 3, uint16 /100 (kg) → grams
			if ws.VisceralFatMass != basetype.Uint16Invalid {
				v := int32(float64(ws.VisceralFatMass) / 100.0 * 1000.0)
				entry.VisceralFatMassG = &v
			}
			// BoneMass: field 4, uint16 /100 (kg) → grams
			if ws.BoneMass != basetype.Uint16Invalid {
				v := int(float64(ws.BoneMass) / 100.0 * 1000.0)
				entry.BoneMassG = &v
			}
			// MuscleMass: field 5, uint16 /100 (kg) → grams
			if ws.MuscleMass != basetype.Uint16Invalid {
				v := int(float64(ws.MuscleMass) / 100.0 * 1000.0)
				entry.MuscleMassG = &v
			}
			// BasalMet: field 7, uint16 /4 (kcal/day)
			if ws.BasalMet != basetype.Uint16Invalid {
				v := float64(ws.BasalMet) / 4.0
				entry.BasalMet = &v
			}
			// PhysiqueRating: field 8, uint8 raw
			if ws.PhysiqueRating != basetype.Uint8Invalid {
				v := int16(ws.PhysiqueRating)
				entry.PhysiqueRating = &v
			}
			// ActiveMet: field 9, uint16 /4 (kcal/day)
			if ws.ActiveMet != basetype.Uint16Invalid {
				v := float64(ws.ActiveMet) / 4.0
				entry.ActiveMet = &v
			}
			// MetabolicAge: field 10, uint8 raw (years)
			if ws.MetabolicAge != basetype.Uint8Invalid {
				v := int16(ws.MetabolicAge)
				entry.MetabolicAge = &v
			}
			// VisceralFatRating: field 11, uint8 raw
			if ws.VisceralFatRating != basetype.Uint8Invalid {
				v := int16(ws.VisceralFatRating)
				entry.VisceralFatRating = &v
			}
			// BMI: field 13, uint16 /10
			if ws.Bmi != basetype.Uint16Invalid {
				v := float64(ws.Bmi) / 10.0
				entry.BMI = &v
			}

			if upsertErr := db.UpsertHealthWeightEntry(ctx, entry); upsertErr != nil {
				slog.Warn("processWeightFIT: upsert failed", "ts", ws.Timestamp, "error", upsertErr)
				continue
			}
			count++
		}
	}

	if count > 0 {
		slog.Info("processWeightFIT: upserted entries", "user_id", userID, "count", count)
	}
	return nil
}
