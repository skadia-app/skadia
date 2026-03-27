package fusion

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"
)

// FITMergeResult contains the merged FIT file bytes and metadata.
type FITMergeResult struct {
	Data         []byte
	RecordCount  int
	EarliestTime time.Time
	LatestTime   time.Time
}

// MergeFITFiles merges two FIT files at the binary/protocol level.
//
// Strategy:
//  1. Decode both FIT files into proto.FIT structures
//  2. Categorize messages: records (time-series data) vs. metadata (file_id, session, activity, etc.)
//  3. Merge records by timestamp — target records take priority on collision
//  4. For non-record data messages (laps, events, device_info, etc.), keep from both,
//     sorted by timestamp where applicable
//  5. Rebuild file_id, session, and activity messages from the merged data
//  6. Encode into a new valid FIT file
//
// The fieldMap controls which data fields are taken from which source:
//   - "hr" → sourceID means heart_rate values from source override target
//   - "power" → sourceID means power values from source override target
//   - "gps" → sourceID means lat/lng from source override target
//   - etc.
//
// If fieldMap is nil, all non-null fields from source fill gaps in target (no overwrite).
func MergeFITFiles(targetData, sourceData []byte, fieldMap map[string]string, sourceID string) (*FITMergeResult, error) {
	targetFIT, err := decodeFITBytes(targetData)
	if err != nil {
		return nil, fmt.Errorf("decode target FIT: %w", err)
	}
	sourceFIT, err := decodeFITBytes(sourceData)
	if err != nil {
		return nil, fmt.Errorf("decode source FIT: %w", err)
	}

	// Partition messages by type.
	targetRecords, targetMeta := partitionMessages(targetFIT.Messages)
	sourceRecords, _ := partitionMessages(sourceFIT.Messages)

	// Build timestamp-indexed maps for records.
	targetByTime := indexRecordsByTimestamp(targetRecords)
	sourceByTime := indexRecordsByTimestamp(sourceRecords)

	// Collect all unique timestamps.
	allTimes := collectTimestamps(targetByTime, sourceByTime)

	// Merge records: target is primary, source fills gaps or overrides per fieldMap.
	mergedRecords := make([]proto.Message, 0, len(allTimes))
	for _, t := range allTimes {
		tRec, hasTarget := targetByTime[t]
		sRec, hasSource := sourceByTime[t]

		switch {
		case hasTarget && hasSource:
			// Both have a record at this timestamp — merge fields.
			merged := mergeRecordFields(tRec, sRec, fieldMap, sourceID)
			mergedRecords = append(mergedRecords, merged)
		case hasTarget:
			mergedRecords = append(mergedRecords, tRec)
		case hasSource:
			mergedRecords = append(mergedRecords, sRec)
		}
	}

	// Rebuild the FIT file: file_id + metadata + merged records + session + activity.
	var result []proto.Message

	// Keep file_id from target (or build a new one).
	fileID := findMessageByNum(targetMeta, typedef.MesgNumFileId)
	if fileID != nil {
		result = append(result, *fileID)
	} else {
		fid := mesgdef.NewFileId(nil)
		fid.Type = typedef.FileActivity
		fid.Manufacturer = typedef.ManufacturerDevelopment
		if len(allTimes) > 0 {
			fid.TimeCreated = allTimes[0]
		}
		result = append(result, fid.ToMesg(nil))
	}

	// Keep non-record, non-structural metadata from target (device_info, user_profile, etc.).
	for _, m := range targetMeta {
		switch m.Num {
		case typedef.MesgNumFileId, typedef.MesgNumSession, typedef.MesgNumActivity, typedef.MesgNumLap:
			continue // Will be rebuilt or handled separately.
		default:
			result = append(result, m)
		}
	}

	// Append merged records.
	result = append(result, mergedRecords...)

	// Keep original lap messages from target.
	for _, m := range targetMeta {
		if m.Num == typedef.MesgNumLap {
			result = append(result, m)
		}
	}

	// Rebuild session from merged data, preserving sport type from target.
	session := buildMergedSession(mergedRecords, allTimes, targetMeta)
	result = append(result, session)

	// Rebuild activity message.
	act := mesgdef.NewActivity(nil)
	if len(allTimes) > 0 {
		act.Timestamp = allTimes[len(allTimes)-1]
	}
	act.NumSessions = 1
	act.Type = typedef.ActivityManual
	result = append(result, act.ToMesg(nil))

	// Encode.
	buf := new(bytes.Buffer)
	enc := encoder.New(buf)
	if err := enc.Encode(&proto.FIT{Messages: result}); err != nil {
		return nil, fmt.Errorf("encode merged FIT: %w", err)
	}

	var earliest, latest time.Time
	if len(allTimes) > 0 {
		earliest = allTimes[0]
		latest = allTimes[len(allTimes)-1]
	}

	return &FITMergeResult{
		Data:         buf.Bytes(),
		RecordCount:  len(mergedRecords),
		EarliestTime: earliest,
		LatestTime:   latest,
	}, nil
}

// decodeFITBytes decodes raw bytes into a proto.FIT structure.
func decodeFITBytes(data []byte) (*proto.FIT, error) {
	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	if !dec.Next() {
		return nil, fmt.Errorf("no FIT data found")
	}
	fit, err := dec.Decode()
	if err != nil {
		return nil, err
	}
	return fit, nil
}

// partitionMessages splits messages into records and non-records.
func partitionMessages(messages []proto.Message) (records, meta []proto.Message) {
	for _, m := range messages {
		if m.Num == typedef.MesgNumRecord {
			records = append(records, m)
		} else {
			meta = append(meta, m)
		}
	}
	return
}

// indexRecordsByTimestamp builds a map from timestamp to record message.
// On duplicate timestamps, the last record wins (shouldn't happen in valid FIT files).
func indexRecordsByTimestamp(records []proto.Message) map[time.Time]proto.Message {
	result := make(map[time.Time]proto.Message, len(records))
	for _, m := range records {
		rec := mesgdef.NewRecord(&m)
		if !rec.Timestamp.IsZero() {
			result[rec.Timestamp] = m
		}
	}
	return result
}

// collectTimestamps collects and sorts all unique timestamps from both maps.
func collectTimestamps(a, b map[time.Time]proto.Message) []time.Time {
	seen := make(map[time.Time]struct{}, len(a)+len(b))
	for t := range a {
		seen[t] = struct{}{}
	}
	for t := range b {
		seen[t] = struct{}{}
	}
	times := make([]time.Time, 0, len(seen))
	for t := range seen {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	return times
}

// fieldNumMap maps logical field names to FIT field numbers for Record messages.
var fieldNumMap = map[string][]byte{
	"hr":          {3},     // heart_rate
	"power":       {7},     // power
	"cadence":     {4},     // cadence
	"temperature": {13},    // temperature
	"gps":         {0, 1},  // position_lat, position_long
	"altitude":    {2, 78}, // altitude, enhanced_altitude
	"speed":       {6, 73}, // speed, enhanced_speed
}

// mergeRecordFields merges fields from source into target record based on fieldMap.
// If fieldMap specifies a field should come from sourceID, those fields in the source
// record override the target. Otherwise, source fields only fill gaps (nil values).
func mergeRecordFields(target, source proto.Message, fieldMap map[string]string, sourceID string) proto.Message {
	if fieldMap == nil {
		return gapFillRecord(target, source)
	}

	overrideNums := make(map[byte]bool)
	for logicalField, src := range fieldMap {
		if src == sourceID {
			if nums, ok := fieldNumMap[logicalField]; ok {
				for _, n := range nums {
					overrideNums[n] = true
				}
			}
		}
	}

	if len(overrideNums) == 0 {
		return gapFillRecord(target, source)
	}

	// Deep-copy Fields slice to avoid mutating the original target message.
	merged := target
	merged.Fields = make([]proto.Field, len(target.Fields))
	copy(merged.Fields, target.Fields)

	sourceFieldMap := make(map[byte]proto.Field, len(source.Fields))
	for _, f := range source.Fields {
		sourceFieldMap[f.Num] = f
	}

	for i, f := range merged.Fields {
		if overrideNums[f.Num] {
			if sf, ok := sourceFieldMap[f.Num]; ok {
				merged.Fields[i] = sf
			}
		}
	}

	targetFieldSet := make(map[byte]bool, len(merged.Fields))
	for _, f := range merged.Fields {
		targetFieldSet[f.Num] = true
	}
	for num := range overrideNums {
		if !targetFieldSet[num] {
			if sf, ok := sourceFieldMap[num]; ok {
				merged.Fields = append(merged.Fields, sf)
			}
		}
	}

	return merged
}

// gapFillRecord copies fields from source to target only where target lacks the field.
// Returns a new message with a deep-copied Fields slice to avoid mutating the original.
func gapFillRecord(target, source proto.Message) proto.Message {
	merged := target
	merged.Fields = make([]proto.Field, len(target.Fields), len(target.Fields)+len(source.Fields))
	copy(merged.Fields, target.Fields)

	targetNums := make(map[byte]bool, len(merged.Fields))
	for _, f := range merged.Fields {
		targetNums[f.Num] = true
	}
	for _, f := range source.Fields {
		if !targetNums[f.Num] {
			merged.Fields = append(merged.Fields, f)
		}
	}
	return merged
}

// findMessageByNum returns the first message with the given MesgNum, or nil.
func findMessageByNum(messages []proto.Message, num typedef.MesgNum) *proto.Message {
	for i := range messages {
		if messages[i].Num == num {
			return &messages[i]
		}
	}
	return nil
}

// buildMergedSession creates a Session message summarizing the merged records.
// targetMeta is used to preserve Sport/SubSport from the original target session.
func buildMergedSession(records []proto.Message, allTimes []time.Time, targetMeta []proto.Message) proto.Message {
	sess := mesgdef.NewSession(nil)

	// Preserve sport type from original target session.
	if origSession := findMessageByNum(targetMeta, typedef.MesgNumSession); origSession != nil {
		orig := mesgdef.NewSession(origSession)
		sess.Sport = orig.Sport
		sess.SubSport = orig.SubSport
	}

	if len(allTimes) == 0 {
		return sess.ToMesg(nil)
	}

	sess.StartTime = allTimes[0]
	sess.Timestamp = allTimes[len(allTimes)-1]

	elapsed := allTimes[len(allTimes)-1].Sub(allTimes[0])
	sess.TotalElapsedTime = uint32(elapsed.Milliseconds())
	sess.TotalTimerTime = uint32(elapsed.Milliseconds())

	// Aggregate stats from records.
	var (
		totalDist   uint32
		maxHR       uint8
		sumHR       uint32
		hrCount     uint32
		maxPower    uint16
		sumPower    uint32
		powerCount  uint32
		totalAscent float64
		prevAlt     float64
		hasPrevAlt  bool
	)

	for _, m := range records {
		rec := mesgdef.NewRecord(&m)

		if rec.Distance != 0xFFFFFFFF && rec.Distance > totalDist {
			totalDist = rec.Distance
		}
		if rec.HeartRate != 0xFF {
			sumHR += uint32(rec.HeartRate)
			hrCount++
			if rec.HeartRate > maxHR {
				maxHR = rec.HeartRate
			}
		}
		if rec.Power != 0xFFFF {
			sumPower += uint32(rec.Power)
			powerCount++
			if rec.Power > maxPower {
				maxPower = rec.Power
			}
		}

		// Elevation gain from enhanced altitude (scale: 5, offset: 500).
		if rec.EnhancedAltitude != 0xFFFFFFFF {
			alt := float64(rec.EnhancedAltitude)/5.0 - 500.0
			if hasPrevAlt && alt > prevAlt {
				totalAscent += alt - prevAlt
			}
			prevAlt = alt
			hasPrevAlt = true
		}
	}

	if totalDist > 0 {
		sess.TotalDistance = totalDist
	}
	if hrCount > 0 {
		sess.AvgHeartRate = uint8(sumHR / hrCount)
		sess.MaxHeartRate = maxHR
	}
	if powerCount > 0 {
		sess.AvgPower = uint16(sumPower / powerCount)
		sess.MaxPower = maxPower
	}
	if totalAscent > 0 {
		sess.TotalAscent = uint16(totalAscent)
	}

	return sess.ToMesg(nil)
}
