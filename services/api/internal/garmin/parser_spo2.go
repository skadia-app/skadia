package garmin

import (
	"bytes"
	"context"
	"fmt"
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

// processSpO2FIT parses a Garmin SpO2 FIT file (type 58) and upserts blood-oxygen
// snapshots into health_spo2_snapshots.
//
// Strategy: opportunistic — only SDK-standard messages 269 (Spo2Data) and 305
// (HsaSpo2Data) are processed. Proprietary msg 318 is silently skipped.
// If neither standard message is found, the function returns nil without error.
func processSpO2FIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	ts := time.Now().UTC()
	storeHealthFIT(ctx, db, fs, data, userID, "spo2", fmt.Sprintf("spo2_%s.fit", ts.Format("20060102_150405")), ts) //nolint:errcheck // non-fatal

	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		slog.Warn("processSpO2FIT: decode failed", "error", err)
		return nil
	}

	count := 0
	for i := range fit.Messages {
		msg := &fit.Messages[i]
		switch msg.Num {

		case typedef.MesgNumSpo2Data: // 269 — single point-in-time measurement
			s := mesgdef.NewSpo2Data(msg)
			if s.Timestamp.IsZero() || s.Timestamp.Before(minSaneTimestamp) {
				continue
			}
			// Skip invalid / blank readings (0 or basetype.Uint8Invalid = 0xFF).
			if s.ReadingSpo2 == 0 || s.ReadingSpo2 == basetype.Uint8Invalid {
				continue
			}
			spo2Val := float64(s.ReadingSpo2)
			var conf *int16
			if s.ReadingConfidence != basetype.Uint8Invalid {
				c := int16(s.ReadingConfidence)
				conf = &c
			}
			snap := &model.HealthSpO2Snapshot{
				UserID:        userID,
				MeasuredAtUTC: s.Timestamp,
				SpO2Pct:       &spo2Val,
				Confidence:    conf,
				SourceType:    "garmin_fit",
			}
			if upsertErr := db.UpsertHealthSpO2Snapshot(ctx, snap); upsertErr != nil {
				slog.Warn("processSpO2FIT: upsert Spo2Data failed", "ts", s.Timestamp, "error", upsertErr)
				continue
			}
			count++

		case typedef.MesgNumHsaSpo2Data: // 305 — HSA array with processing_interval
			h := mesgdef.NewHsaSpo2Data(msg)
			if h.Timestamp.IsZero() || h.Timestamp.Before(minSaneTimestamp) || len(h.ReadingSpo2) == 0 {
				continue
			}
			interval := time.Duration(0)
			if h.ProcessingInterval != basetype.Uint16Invalid && h.ProcessingInterval > 0 {
				interval = time.Duration(h.ProcessingInterval) * time.Second
			}
			for j, raw := range h.ReadingSpo2 {
				// Blank value 240 means "no reading at this slot" — skip.
				if raw == 240 || raw == basetype.Uint8Invalid {
					continue
				}
				slotTS := h.Timestamp.Add(time.Duration(j) * interval)
				spo2Val := float64(raw)
				var conf *int16
				if j < len(h.Confidence) && h.Confidence[j] != basetype.Uint8Invalid {
					c := int16(h.Confidence[j])
					conf = &c
				}
				snap := &model.HealthSpO2Snapshot{
					UserID:        userID,
					MeasuredAtUTC: slotTS,
					SpO2Pct:       &spo2Val,
					Confidence:    conf,
					SourceType:    "garmin_fit",
				}
				if upsertErr := db.UpsertHealthSpO2Snapshot(ctx, snap); upsertErr != nil {
					slog.Warn("processSpO2FIT: upsert HsaSpo2Data slot failed", "ts", slotTS, "error", upsertErr)
					continue
				}
				count++
			}
		// msg 318 (proprietary) and all other unknown messages are silently skipped.
		}
	}

	if count > 0 {
		slog.Info("processSpO2FIT: upserted snapshots", "user_id", userID, "count", count)
	}
	return nil
}
