package garmin

import (
	"bytes"
	"context"
	"fmt"
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

// sportZoneEntry holds a single zone boundary decoded from a FIT zone message.
type sportZoneEntry struct {
	index uint16
	name  string
	high  float64
}

// parsedSportData holds the pure decode output of a sport FIT file.
type parsedSportData struct {
	Sport    int16
	SubSport int16

	MaxHeartRate             *int16
	ThresholdHeartRate       *int16
	FunctionalThresholdPower *int32
	HrCalcType               *int16
	PwrCalcType              *int16

	HREntries    []sportZoneEntry
	PowerEntries []sportZoneEntry
	SpeedEntries []sportZoneEntry
}

// decodeSport decodes a raw sport FIT payload into zone data without any I/O.
func decodeSport(raw []byte) (parsedSportData, error) {
	dec := decoder.New(bytes.NewReader(raw), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		return parsedSportData{}, fmt.Errorf("decode sport FIT: %w", err)
	}

	var (
		sport    typedef.Sport    = typedef.SportInvalid
		subSport typedef.SubSport = typedef.SubSportInvalid

		zonesTarget *mesgdef.ZonesTarget

		hrEntries    []sportZoneEntry
		powerEntries []sportZoneEntry
		speedEntries []sportZoneEntry
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
			hrEntries = append(hrEntries, sportZoneEntry{
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
			powerEntries = append(powerEntries, sportZoneEntry{
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
			speedEntries = append(speedEntries, sportZoneEntry{
				index: uint16(z.MessageIndex & typedef.MessageIndexMask),
				name:  z.Name,
				high:  float64(z.HighValue) / 1000.0, // scale to m/s
			})
		}
	}

	out := parsedSportData{
		Sport:        int16(sport),
		SubSport:     int16(subSport),
		HREntries:    hrEntries,
		PowerEntries: powerEntries,
		SpeedEntries: speedEntries,
	}

	if zonesTarget != nil {
		if zonesTarget.MaxHeartRate != basetype.Uint8Invalid {
			v := int16(zonesTarget.MaxHeartRate)
			out.MaxHeartRate = &v
		}
		if zonesTarget.ThresholdHeartRate != basetype.Uint8Invalid {
			v := int16(zonesTarget.ThresholdHeartRate)
			out.ThresholdHeartRate = &v
		}
		if zonesTarget.FunctionalThresholdPower != basetype.Uint16Invalid {
			v := int32(zonesTarget.FunctionalThresholdPower)
			out.FunctionalThresholdPower = &v
		}
		if zonesTarget.HrCalcType != typedef.HrZoneCalcInvalid {
			v := int16(zonesTarget.HrCalcType)
			out.HrCalcType = &v
		}
		if zonesTarget.PwrCalcType != typedef.PwrZoneCalcInvalid {
			v := int16(zonesTarget.PwrCalcType)
			out.PwrCalcType = &v
		}
	}

	return out, nil
}

// processSportFIT parses a Garmin sport FIT file (type 3) and upserts training zone
// definitions for HR, power and speed zones into the database.
func processSportFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	if _, err := storeHealthFIT(ctx, db, fs, data, userID, "sport", "sport.fit", time.Now().UTC()); err != nil {
		slog.Warn("processSportFIT: storeHealthFIT failed", "error", err)
		// non-fatal — continue with parsing
	}

	parsed, err := decodeSport(data)
	if err != nil {
		// undecodable file — skip silently (preserve original behaviour)
		return nil
	}

	sortEntries := func(entries []sportZoneEntry) ([]string, []float64) {
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
			Sport:                    parsed.Sport,
			SubSport:                 parsed.SubSport,
			ZoneType:                 zoneType,
			MaxHeartRate:             parsed.MaxHeartRate,
			ThresholdHeartRate:       parsed.ThresholdHeartRate,
			FunctionalThresholdPower: parsed.FunctionalThresholdPower,
			HrCalcType:               parsed.HrCalcType,
			PwrCalcType:              parsed.PwrCalcType,
			ZoneNames:                names,
			ZoneHighs:                highs,
			SourceType:               "garmin_fit",
		}
		if err := db.UpsertTrainingZones(ctx, zone); err != nil {
			slog.Warn("processSportFIT: upsert training zones failed",
				"zone_type", zoneType,
				"sport", parsed.Sport,
				"sub_sport", parsed.SubSport,
				"error", err,
			)
		}
	}

	if len(parsed.HREntries) > 0 {
		names, highs := sortEntries(parsed.HREntries)
		upsert("hr", names, highs)
	}

	if len(parsed.PowerEntries) > 0 {
		names, highs := sortEntries(parsed.PowerEntries)
		upsert("power", names, highs)
	}

	if len(parsed.SpeedEntries) > 0 {
		names, highs := sortEntries(parsed.SpeedEntries)
		upsert("speed", names, highs)
	}

	// If no zone messages were found but ZonesTarget has threshold values, store
	// them as a standalone "thresholds" record so they are not silently discarded.
	if len(parsed.HREntries) == 0 && len(parsed.PowerEntries) == 0 && len(parsed.SpeedEntries) == 0 &&
		(parsed.MaxHeartRate != nil || parsed.ThresholdHeartRate != nil || parsed.FunctionalThresholdPower != nil) {
		upsert("thresholds", nil, nil)
	}

	return nil
}
