package garmin

import (
	"bytes"
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// processSportFIT parses a Garmin sport FIT file (type 3) and upserts training zone
// definitions for HR, power and speed zones into the database.
func processSportFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	if _, err := storeHealthFIT(ctx, db, fs, data, userID, "sport", "sport.fit", time.Now().UTC()); err != nil {
		slog.Warn("processSportFIT: storeHealthFIT failed", "error", err)
		// non-fatal — continue with parsing
	}

	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		return nil // undecodable file — skip silently
	}

	type zoneEntry struct {
		index uint16
		name  string
		high  float64
	}

	var (
		sport    typedef.Sport    = typedef.SportInvalid
		subSport typedef.SubSport = typedef.SubSportInvalid

		zonesTarget *mesgdef.ZonesTarget

		hrEntries    []zoneEntry
		powerEntries []zoneEntry
		speedEntries []zoneEntry
	)

	for i := range fit.Messages {
		msg := &fit.Messages[i]
		switch msg.Num {
		case typedef.MesgNumSport:
			s := mesgdef.NewSport(msg)
			if s.Sport != typedef.SportInvalid {
				sport = s.Sport
			}
			if s.SubSport != typedef.SubSportInvalid {
				subSport = s.SubSport
			}

		case typedef.MesgNumZonesTarget:
			zonesTarget = mesgdef.NewZonesTarget(msg)

		case typedef.MesgNumHrZone:
			z := mesgdef.NewHrZone(msg)
			if z.MessageIndex == typedef.MessageIndexInvalid {
				continue
			}
			if z.HighBpm == basetype.Uint8Invalid {
				continue
			}
			hrEntries = append(hrEntries, zoneEntry{
				index: uint16(z.MessageIndex & typedef.MessageIndexMask),
				name:  z.Name,
				high:  float64(z.HighBpm),
			})

		case typedef.MesgNumPowerZone:
			z := mesgdef.NewPowerZone(msg)
			if z.MessageIndex == typedef.MessageIndexInvalid {
				continue
			}
			if z.HighValue == basetype.Uint16Invalid {
				continue
			}
			powerEntries = append(powerEntries, zoneEntry{
				index: uint16(z.MessageIndex & typedef.MessageIndexMask),
				name:  z.Name,
				high:  float64(z.HighValue),
			})

		case typedef.MesgNumSpeedZone:
			z := mesgdef.NewSpeedZone(msg)
			if z.MessageIndex == typedef.MessageIndexInvalid {
				continue
			}
			if z.HighValue == basetype.Uint16Invalid {
				continue
			}
			speedEntries = append(speedEntries, zoneEntry{
				index: uint16(z.MessageIndex & typedef.MessageIndexMask),
				name:  z.Name,
				high:  float64(z.HighValue) / 1000.0, // scale to m/s
			})
		}
	}

	sportI16 := int16(sport)
	subSportI16 := int16(subSport)

	// Build shared ZonesTarget thresholds.
	var (
		maxHR    *int16
		thrHR    *int16
		ftp      *int32
		hrCalc   *int16
		pwrCalc  *int16
	)
	if zonesTarget != nil {
		if zonesTarget.MaxHeartRate != basetype.Uint8Invalid {
			v := int16(zonesTarget.MaxHeartRate)
			maxHR = &v
		}
		if zonesTarget.ThresholdHeartRate != basetype.Uint8Invalid {
			v := int16(zonesTarget.ThresholdHeartRate)
			thrHR = &v
		}
		if zonesTarget.FunctionalThresholdPower != basetype.Uint16Invalid {
			v := int32(zonesTarget.FunctionalThresholdPower)
			ftp = &v
		}
		if zonesTarget.HrCalcType != typedef.HrZoneCalcInvalid {
			v := int16(zonesTarget.HrCalcType)
			hrCalc = &v
		}
		if zonesTarget.PwrCalcType != typedef.PwrZoneCalcInvalid {
			v := int16(zonesTarget.PwrCalcType)
			pwrCalc = &v
		}
	}

	sortEntries := func(entries []zoneEntry) ([]string, []float64) {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].index < entries[j].index
		})
		names := make([]string, len(entries))
		highs := make([]float64, len(entries))
		for i, e := range entries {
			names[i] = e.name
			highs[i] = e.high
		}
		return names, highs
	}

	upsert := func(zoneType string, names []string, highs []float64) {
		zone := &model.TrainingZone{
			UserID:                   userID,
			Sport:                    sportI16,
			SubSport:                 subSportI16,
			ZoneType:                 zoneType,
			MaxHeartRate:             maxHR,
			ThresholdHeartRate:       thrHR,
			FunctionalThresholdPower: ftp,
			HrCalcType:               hrCalc,
			PwrCalcType:              pwrCalc,
			ZoneNames:                names,
			ZoneHighs:                highs,
			SourceType:               "garmin_fit",
		}
		if err := db.UpsertTrainingZones(ctx, zone); err != nil {
			slog.Warn("processSportFIT: upsert training zones failed",
				"zone_type", zoneType,
				"sport", sportI16,
				"sub_sport", subSportI16,
				"error", err,
			)
		}
	}

	if len(hrEntries) > 0 {
		names, highs := sortEntries(hrEntries)
		upsert("hr", names, highs)
	}

	if len(powerEntries) > 0 {
		names, highs := sortEntries(powerEntries)
		upsert("power", names, highs)
	}

	if len(speedEntries) > 0 {
		names, highs := sortEntries(speedEntries)
		upsert("speed", names, highs)
	}

	// If no zone messages were found but ZonesTarget has threshold values, store
	// them as a standalone "thresholds" record so they are not silently discarded.
	if len(hrEntries) == 0 && len(powerEntries) == 0 && len(speedEntries) == 0 &&
		(maxHR != nil || thrHR != nil || ftp != nil) {
		upsert("thresholds", nil, nil)
	}

	return nil
}
