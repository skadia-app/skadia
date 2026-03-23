package fitimport

import (
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractEvents(events []*mesgdef.Event) []model.EventRecord {
	var result []model.EventRecord

	for _, e := range events {
		if e.Event == typedef.EventInvalid {
			continue
		}

		rec := model.EventRecord{
			Timestamp: e.Timestamp,
			Event:     int16(e.Event),
			EventType: int16(e.EventType),
		}

		// EventGroup
		if e.EventGroup != basetype.Uint8Invalid {
			v := int16(e.EventGroup)
			rec.EventGroup = &v
		}

		// Data
		if e.Data != basetype.Uint32Invalid {
			v := int64(e.Data)
			rec.Data = &v
		}

		// Gear fields (uint8z — invalid is 0)
		if e.FrontGearNum != basetype.Uint8zInvalid {
			v := int16(e.FrontGearNum)
			rec.FrontGearNum = &v
		}
		if e.FrontGear != basetype.Uint8zInvalid {
			v := int16(e.FrontGear)
			rec.FrontGear = &v
		}
		if e.RearGearNum != basetype.Uint8zInvalid {
			v := int16(e.RearGearNum)
			rec.RearGearNum = &v
		}
		if e.RearGear != basetype.Uint8zInvalid {
			v := int16(e.RearGear)
			rec.RearGear = &v
		}

		// Radar threat level
		if e.RadarThreatLevelMax != typedef.RadarThreatLevelTypeInvalid {
			v := int16(e.RadarThreatLevelMax)
			rec.RadarThreatLevelMax = &v
		}
		if e.RadarThreatCount != basetype.Uint8Invalid {
			v := int16(e.RadarThreatCount)
			rec.RadarThreatCount = &v
		}

		// Radar approach speeds (Scale: 10 → m/s)
		if e.RadarThreatAvgApproachSpeed != basetype.Uint8Invalid {
			v := float64(e.RadarThreatAvgApproachSpeed) / 10.0
			rec.RadarThreatAvgApproachSpeedMs = &v
		}
		if e.RadarThreatMaxApproachSpeed != basetype.Uint8Invalid {
			v := float64(e.RadarThreatMaxApproachSpeed) / 10.0
			rec.RadarThreatMaxApproachSpeedMs = &v
		}

		// Score
		if e.Score != basetype.Uint16Invalid {
			v := int32(e.Score)
			rec.Score = &v
		}
		if e.OpponentScore != basetype.Uint16Invalid {
			v := int32(e.OpponentScore)
			rec.OpponentScore = &v
		}

		// DeviceIndex
		if e.DeviceIndex != typedef.DeviceIndexInvalid {
			v := int16(e.DeviceIndex)
			rec.DeviceIndex = &v
		}

		// ActivityType
		if e.ActivityType != typedef.ActivityTypeInvalid {
			v := int16(e.ActivityType)
			rec.ActivityType = &v
		}

		// StartTimestamp
		if !e.StartTimestamp.IsZero() {
			ts := e.StartTimestamp
			rec.StartTimestamp = &ts
		}

		result = append(result, rec)
	}

	return result
}
