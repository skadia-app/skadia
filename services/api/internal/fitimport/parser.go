package fitimport

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"

	"github.com/skadia-app/skadia/services/api/internal/activityutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ParseMultiSessionFIT parses a FIT file and returns one ActivityData per session.
// For single-session files (the common case) it returns a slice with exactly one element.
func ParseMultiSessionFIT(data []byte, filename string) ([]*model.ActivityData, error) {
	fit, err := decodeFIT(data)
	if err != nil && strings.Contains(err.Error(), "invalid basetype") {
		patched := patchDeprecatedBaseTypes(data)
		fit, err = decodeFIT(patched)
	}
	if err != nil {
		return nil, fmt.Errorf("decode FIT (multi-session): %w", err)
	}

	// Collect all session messages
	var sessionMsgs []proto.Message
	for i := range fit.Messages {
		if fit.Messages[i].Num == typedef.MesgNumSession {
			sessionMsgs = append(sessionMsgs, fit.Messages[i])
		}
	}

	if len(sessionMsgs) <= 1 {
		// Normal single-session path
		ad, err := buildActivityData(fit, filename)
		if err != nil {
			return nil, err
		}
		return []*model.ActivityData{ad}, nil
	}

	// Multi-session: build sub-FITs partitioned by session time range
	// Collect all record messages with their timestamps
	var allRecords []proto.Message
	for i := range fit.Messages {
		if fit.Messages[i].Num == typedef.MesgNumRecord {
			allRecords = append(allRecords, fit.Messages[i])
		}
	}

	var results []*model.ActivityData
	for i, sessionMsg := range sessionMsgs {
		session := mesgdef.NewSession(&sessionMsg)

		// Bug 2: Skip the multisport summary session (sport=18). It is a
		// roll-up of all disciplines and must not be imported as a child activity.
		if session.Sport == typedef.SportMultisport {
			continue
		}

		sessionStart := session.StartTime

		// Bug 3: Summary-First format (Garmin firmware ≥2023) writes session
		// messages BEFORE the records. In that layout session.Timestamp is the
		// file-creation time, not the session end. Compute the real end time
		// from StartTime + TotalElapsedTime (Scale 1000 → raw uint32 in ms).
		// Fall back to session.Timestamp only when the computed value is unusable.
		sessionEnd := session.StartTime.Add(
			time.Duration(session.TotalElapsedTime) * time.Millisecond,
		)
		if sessionEnd.IsZero() || !sessionEnd.After(sessionStart) {
			sessionEnd = session.Timestamp
		}

		// Partition records that fall within this session's time range
		var sessionRecords []proto.Message
		for _, rm := range allRecords {
			r := mesgdef.NewRecord(&rm)
			if !r.Timestamp.IsZero() && !r.Timestamp.Before(sessionStart) && !r.Timestamp.After(sessionEnd) {
				sessionRecords = append(sessionRecords, rm)
			}
		}

		sessionFIT := buildSessionFIT(fit, sessionMsg, sessionRecords)
		sessionFilename := fmt.Sprintf("%s_session%d", filename, i+1)

		ad, err := buildActivityData(sessionFIT, sessionFilename)
		if err != nil {
			slog.Warn("multi-session parse failed for session", "index", i, "err", err)
			continue
		}

		// Bug 1: Transition sessions (sport=3, e.g. T1/T2 in triathlon) must be
		// marked explicitly so the importer can handle them differently from real
		// sport disciplines (no Geo/ML enrichment needed).
		if session.Sport == typedef.SportTransition {
			ad.SportType = "transition"
		}

		results = append(results, ad)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no sessions could be parsed from multi-session FIT")
	}
	return results, nil
}

// buildSessionFIT builds a minimal proto.FIT containing only the messages for a single session.
func buildSessionFIT(original *proto.FIT, sessionMsg proto.Message, records []proto.Message) *proto.FIT {
	result := &proto.FIT{
		FileHeader: original.FileHeader,
	}

	// Carry over FileId
	for i := range original.Messages {
		if original.Messages[i].Num == typedef.MesgNumFileId {
			result.Messages = append(result.Messages, original.Messages[i])
			break
		}
	}

	// Records for this session
	result.Messages = append(result.Messages, records...)

	// The session message itself
	result.Messages = append(result.Messages, sessionMsg)

	// DeviceInfo messages (all — they describe the recording device)
	for i := range original.Messages {
		if original.Messages[i].Num == typedef.MesgNumDeviceInfo {
			result.Messages = append(result.Messages, original.Messages[i])
		}
	}

	// UserProfile and ZonesTarget (same for all sessions)
	for i := range original.Messages {
		if original.Messages[i].Num == typedef.MesgNumUserProfile ||
			original.Messages[i].Num == typedef.MesgNumZonesTarget {
			result.Messages = append(result.Messages, original.Messages[i])
		}
	}

	return result
}

// ParseFIT decodes a FIT file and returns the parsed activity data.
func ParseFIT(r io.Reader, filename string) (*model.ActivityData, error) {
	// Read all bytes so we can retry with patched data if needed.
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read FIT: %w", err)
	}

	fit, err := decodeFIT(raw)
	if err != nil && strings.Contains(err.Error(), "invalid basetype") {
		// Old FIT files (protocol v1.0) use deprecated base types without
		// the endian-ability bit: 0x0B (uint16z) and 0x0C (uint32z).
		// Patch them in-memory to the modern equivalents 0x8B / 0x8C.
		patched := patchDeprecatedBaseTypes(raw)
		fit, err = decodeFIT(patched)
	}
	if err != nil {
		return nil, fmt.Errorf("decode FIT: %w", err)
	}

	return buildActivityData(fit, filename)
}

// decodeFIT decodes the first FIT sequence from raw bytes.
func decodeFIT(raw []byte) (*proto.FIT, error) {
	dec := decoder.New(bytes.NewReader(raw), decoder.WithIgnoreChecksum())
	if !dec.Next() {
		return nil, fmt.Errorf("no FIT data found")
	}
	return dec.Decode()
}

// patchDeprecatedBaseTypes fixes old FIT files that use base type numbers
// without the endian-ability bit set. It scans definition messages and
// replaces 0x0B → 0x8B (uint16z) and 0x0C → 0x8C (uint32z).
// Operates on a copy — the original slice is not modified.
func patchDeprecatedBaseTypes(raw []byte) []byte {
	data := make([]byte, len(raw))
	copy(data, raw)

	// Skip file header (first byte = header size)
	if len(data) < 14 {
		return data
	}
	headerSize := int(data[0])
	if headerSize < 12 || headerSize > len(data) {
		return data
	}
	// Data size is at bytes 4-7 (little-endian)
	dataSize := int(data[4]) | int(data[5])<<8 | int(data[6])<<16 | int(data[7])<<24
	pos := headerSize
	end := headerSize + dataSize
	if end > len(data) {
		end = len(data)
	}

	// Local message definitions track field count + sizes for data messages
	type localDef struct {
		fieldSizes []byte
		devFields  int
	}
	locals := make(map[byte]localDef)

	for pos < end {
		header := data[pos]

		if header&0x40 != 0 {
			// Definition message
			localNum := header & 0x0F
			hasDev := header&0x20 != 0
			pos++ // header

			if pos+5 > end {
				break
			}
			// reserved, architecture, global mesg num (2), num fields
			numFields := int(data[pos+4])
			pos += 5

			var sizes []byte
			for i := 0; i < numFields; i++ {
				if pos+3 > end {
					return data
				}
				// field: [num, size, baseType]
				bt := data[pos+2]
				// Patch deprecated base types
				if bt == 0x0B {
					data[pos+2] = 0x8B // uint16z
				} else if bt == 0x0C {
					data[pos+2] = 0x8C // uint32z
				}
				sizes = append(sizes, data[pos+1])
				pos += 3
			}

			devCount := 0
			if hasDev {
				if pos >= end {
					break
				}
				devCount = int(data[pos])
				pos++
				pos += devCount * 3 // each dev field def = 3 bytes
			}
			locals[localNum] = localDef{fieldSizes: sizes, devFields: devCount}
		} else {
			// Data message (normal or compressed timestamp)
			var localNum byte
			if header&0x80 != 0 {
				// Compressed timestamp
				localNum = (header >> 5) & 0x03
			} else {
				localNum = header & 0x0F
			}
			pos++ // header

			def, ok := locals[localNum]
			if !ok {
				break // can't continue without definition
			}
			for _, sz := range def.fieldSizes {
				pos += int(sz)
			}
			// skip dev fields (we don't track their sizes individually,
			// but dev field data follows - for simplicity, we tracked count only)
			// Dev fields in data messages have sizes defined by field_description messages,
			// but for the patching purpose we only care about definition messages.
			// The localDef already accounts for layout.
		}
	}

	return data
}

func buildActivityData(fit *proto.FIT, filename string) (*model.ActivityData, error) {
	var fileId *mesgdef.FileId
	var sessions []*mesgdef.Session
	var records []*mesgdef.Record
	var deviceInfos []*mesgdef.DeviceInfo
	var userProfile *mesgdef.UserProfile
	var zonesTarget *mesgdef.ZonesTarget
	var timeInZones []*mesgdef.TimeInZone
	var laps []*mesgdef.Lap
	var splits []*mesgdef.Split
	var splitSummaries []*mesgdef.SplitSummary
	var lengths []*mesgdef.Length
	var climbPros []*mesgdef.ClimbPro
	var rrIntervals []uint16

	for i := range fit.Messages {
		switch fit.Messages[i].Num {
		case typedef.MesgNumFileId:
			fileId = mesgdef.NewFileId(&fit.Messages[i])
		case typedef.MesgNumSession:
			sessions = append(sessions, mesgdef.NewSession(&fit.Messages[i]))
		case typedef.MesgNumRecord:
			records = append(records, mesgdef.NewRecord(&fit.Messages[i]))
		case typedef.MesgNumDeviceInfo:
			deviceInfos = append(deviceInfos, mesgdef.NewDeviceInfo(&fit.Messages[i]))
		case typedef.MesgNumUserProfile:
			userProfile = mesgdef.NewUserProfile(&fit.Messages[i])
		case typedef.MesgNumZonesTarget:
			zonesTarget = mesgdef.NewZonesTarget(&fit.Messages[i])
		case typedef.MesgNumTimeInZone:
			timeInZones = append(timeInZones, mesgdef.NewTimeInZone(&fit.Messages[i]))
		case typedef.MesgNumLap:
			laps = append(laps, mesgdef.NewLap(&fit.Messages[i]))
		case typedef.MesgNumSplit:
			splits = append(splits, mesgdef.NewSplit(&fit.Messages[i]))
		case typedef.MesgNumSplitSummary:
			splitSummaries = append(splitSummaries, mesgdef.NewSplitSummary(&fit.Messages[i]))
		case typedef.MesgNumLength:
			lengths = append(lengths, mesgdef.NewLength(&fit.Messages[i]))
		case typedef.MesgNumClimbPro:
			climbPros = append(climbPros, mesgdef.NewClimbPro(&fit.Messages[i]))
		case typedef.MesgNumHrv:
			hrv := mesgdef.NewHrv(&fit.Messages[i])
			for _, rrMs := range hrv.Time {
				// HRV Time: Scale 1000, Units s → raw uint16 value = ms
				// Valid range: 1–4999 ms (20–1200 bpm). 0 and 0xFFFF are invalid.
				if rrMs > 0 && rrMs != 0xFFFF && rrMs < 5000 {
					rrIntervals = append(rrIntervals, rrMs)
				}
			}
		}
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no session message found in FIT file")
	}

	session := sessions[0]
	sportType, subType, fitSportID, fitSubSportID := ParseSportFromSession(session)

	// E-Bike normalization: map e_biking sport and e_bike_* sub-sports to
	// their non-electric counterparts and set IsEbike flag instead.
	isEbike := false
	if sportType == "e_biking" {
		sportType = "cycling"
		isEbike = true
	}
	switch subType {
	case "e_bike_fitness":
		subType = "generic"
		isEbike = true
	case "e_bike_mountain":
		subType = "mountain"
		isEbike = true
	}

	data := &model.ActivityData{
		FileFormat:    "fit",
		OriginalName:  filename,
		SportType:     sportType,
		SubType:       subType,
		FitSportID:    fitSportID,
		FitSubSportID: fitSubSportID,
		IsEbike:       isEbike,
		StartedAt:     session.StartTime,
		EndedAt:       session.Timestamp,
	}

	// Source / Device from FileId and DeviceInfo[0]
	data.Source, data.SourceDevice = extractFITSource(fileId, deviceInfos)

	// Time (uint32, Scale: 1000 → seconds)
	if session.TotalElapsedTime != 0xFFFFFFFF {
		data.ElapsedTimeS = int(session.TotalElapsedTime / 1000)
	}
	if session.TotalTimerTime != 0xFFFFFFFF {
		data.MovingTimeS = int(session.TotalTimerTime / 1000)
	}

	// Distance (uint32, Scale: 100 → meters)
	if session.TotalDistance != 0xFFFFFFFF {
		data.DistanceM = float64(session.TotalDistance) / 100.0
	}

	// Elevation
	if session.TotalAscent != 0xFFFF {
		data.ElevationGainM = float64(session.TotalAscent)
	}
	if session.TotalDescent != 0xFFFF {
		data.ElevationLossM = float64(session.TotalDescent)
	}
	// Enhanced altitude: uint32, Scale: 5, Offset: 500
	if session.EnhancedMaxAltitude != 0xFFFFFFFF {
		data.MaxElevationM = float64(session.EnhancedMaxAltitude)/5.0 - 500.0
	}
	if session.EnhancedMinAltitude != 0xFFFFFFFF {
		data.MinElevationM = float64(session.EnhancedMinAltitude)/5.0 - 500.0
	}

	// Speed (uint32, Scale: 1000 → m/s)
	if session.EnhancedAvgSpeed != 0xFFFFFFFF && session.EnhancedAvgSpeed != 0 {
		data.AvgSpeedMS = float64(session.EnhancedAvgSpeed) / 1000.0
	} else if session.AvgSpeed != 0xFFFF {
		data.AvgSpeedMS = float64(session.AvgSpeed) / 1000.0
	}
	if session.EnhancedMaxSpeed != 0xFFFFFFFF && session.EnhancedMaxSpeed != 0 {
		data.MaxSpeedMS = float64(session.EnhancedMaxSpeed) / 1000.0
	} else if session.MaxSpeed != 0xFFFF {
		data.MaxSpeedMS = float64(session.MaxSpeed) / 1000.0
	}

	// Heart rate
	if session.AvgHeartRate != 0xFF && session.AvgHeartRate != 0 {
		data.AvgHeartRate = float64(session.AvgHeartRate)
	}
	if session.MaxHeartRate != 0xFF && session.MaxHeartRate != 0 {
		data.MaxHeartRate = int(session.MaxHeartRate)
	}

	// Power
	if session.AvgPower != 0xFFFF && session.AvgPower != 0 {
		data.AvgPowerW = float64(session.AvgPower)
	}
	if session.MaxPower != 0xFFFF && session.MaxPower != 0 {
		data.MaxPowerW = int(session.MaxPower)
	}
	if session.NormalizedPower != 0xFFFF && session.NormalizedPower != 0 {
		data.NormalizedPowerW = float64(session.NormalizedPower)
	}

	// Cadence
	if session.AvgCadence != 0xFF && session.AvgCadence != 0 {
		data.AvgCadence = float64(session.AvgCadence)
	}

	// Temperature
	if session.AvgTemperature != 0x7F {
		data.AvgTemperatureC = float64(session.AvgTemperature)
	}

	// TSS (uint16, Scale: 10)
	if session.TrainingStressScore != 0xFFFF {
		data.TSS = float64(session.TrainingStressScore) / 10.0
	}

	// Min heart rate
	if session.MinHeartRate != basetype.Uint8Invalid {
		data.MinHeartRate = int(session.MinHeartRate)
	}
	// Total calories
	if session.TotalCalories != basetype.Uint16Invalid {
		data.TotalCalories = int(session.TotalCalories)
	}
	// Training effects (uint8, scale: 10)
	if session.TotalTrainingEffect != basetype.Uint8Invalid {
		data.AerobicTrainingEffect = float64(session.TotalTrainingEffect) / 10.0
	}
	if session.TotalAnaerobicTrainingEffect != basetype.Uint8Invalid {
		data.AnaerobicTrainingEffect = float64(session.TotalAnaerobicTrainingEffect) / 10.0
	}
	// Training load peak (int32, Scale: 65536)
	if session.TrainingLoadPeak != basetype.Sint32Invalid {
		data.TrainingLoadPeak = float64(session.TrainingLoadPeak) / 65536.0
	}
	// SpO2, stress
	if session.AvgSpo2 != basetype.Uint8Invalid {
		data.AvgSpo2Pct = int(session.AvgSpo2)
	}
	if session.AvgStress != basetype.Uint8Invalid {
		data.AvgStressLevel = int(session.AvgStress)
	}
	// Respiration (uint16, Scale: 100)
	if session.EnhancedAvgRespirationRate != basetype.Uint16Invalid {
		data.AvgRespirationRate = float64(session.EnhancedAvgRespirationRate) / 100.0
	}
	// RPE, feel
	if session.WorkoutRpe != basetype.Uint8Invalid {
		data.WorkoutRpe = int(session.WorkoutRpe)
	}
	if session.WorkoutFeel != basetype.Uint8Invalid {
		data.WorkoutFeel = int(session.WorkoutFeel)
	}
	// HRV RMSSD
	if session.RmssdHrv != basetype.Uint8Invalid {
		data.SessionRmssdMs = int(session.RmssdHrv)
	}
	// Threshold power
	if session.ThresholdPower != basetype.Uint16Invalid {
		data.ThresholdPowerW = int(session.ThresholdPower)
	}
	// Core temperature (uint16, Scale: 100)
	if session.AvgCoreTemperature != basetype.Uint16Invalid {
		data.AvgCoreTemperatureC = float64(session.AvgCoreTemperature) / 100.0
	}

	// TimeInZone messages
	for _, tiz := range timeInZones {
		for i, raw := range tiz.TimeInHrZone {
			if raw != basetype.Uint32Invalid {
				zd := model.TimeInZoneData{
					ZoneType:  "hr",
					ZoneIndex: i,
					Seconds:   int(raw / 1000),
				}
				if i < len(tiz.HrZoneHighBoundary) && tiz.HrZoneHighBoundary[i] != basetype.Uint8Invalid {
					zd.ZoneHigh = float64(tiz.HrZoneHighBoundary[i])
				}
				data.TimeInZoneData = append(data.TimeInZoneData, zd)
			}
		}
		for i, raw := range tiz.TimeInPowerZone {
			if raw != basetype.Uint32Invalid {
				zd := model.TimeInZoneData{
					ZoneType:  "power",
					ZoneIndex: i,
					Seconds:   int(raw / 1000),
				}
				if i < len(tiz.PowerZoneHighBoundary) && tiz.PowerZoneHighBoundary[i] != basetype.Uint16Invalid {
					zd.ZoneHigh = float64(tiz.PowerZoneHighBoundary[i])
				}
				data.TimeInZoneData = append(data.TimeInZoneData, zd)
			}
		}
	}

	// Fallback: session-level TimeInHrZone if no dedicated TimeInZone messages
	if len(data.TimeInZoneData) == 0 && len(session.TimeInHrZone) > 0 {
		for i, raw := range session.TimeInHrZone {
			if raw != basetype.Uint32Invalid {
				data.TimeInZoneData = append(data.TimeInZoneData, model.TimeInZoneData{
					ZoneType:  "hr",
					ZoneIndex: i,
					Seconds:   int(raw / 1000),
				})
			}
		}
	}
	if len(data.TimeInZoneData) > 0 {
		// Also check session-level power zones as fallback
		hasPowerZones := false
		for _, z := range data.TimeInZoneData {
			if z.ZoneType == "power" {
				hasPowerZones = true
				break
			}
		}
		if !hasPowerZones && len(session.TimeInPowerZone) > 0 {
			for i, raw := range session.TimeInPowerZone {
				if raw != basetype.Uint32Invalid {
					data.TimeInZoneData = append(data.TimeInZoneData, model.TimeInZoneData{
						ZoneType:  "power",
						ZoneIndex: i,
						Seconds:   int(raw / 1000),
					})
				}
			}
		}
	}

	// UserProfile + ZonesTarget import
	if userProfile != nil || zonesTarget != nil {
		imp := &model.UserProfileImport{}
		if userProfile != nil {
			if userProfile.Gender != typedef.GenderInvalid {
				switch userProfile.Gender {
				case typedef.GenderMale:
					imp.Gender = "M"
				case typedef.GenderFemale:
					imp.Gender = "F"
				}
			}
			if userProfile.Age != basetype.Uint8Invalid {
				imp.AgeYears = int(userProfile.Age)
			}
			if userProfile.Height != basetype.Uint8Invalid {
				// Height is in meters * 100, so raw value IS cm
				imp.HeightCm = int(userProfile.Height)
			}
			if userProfile.Weight != basetype.Uint16Invalid {
				imp.WeightKg = float32(userProfile.Weight) / 10.0
			}
			if userProfile.RestingHeartRate != basetype.Uint8Invalid {
				imp.RestingHR = int(userProfile.RestingHeartRate)
			}
			if userProfile.DefaultMaxHeartRate != basetype.Uint8Invalid {
				imp.MaxHRDetected = int(userProfile.DefaultMaxHeartRate)
			}
		}
		if zonesTarget != nil {
			if zonesTarget.ThresholdHeartRate != basetype.Uint8Invalid {
				imp.LTHR = int(zonesTarget.ThresholdHeartRate)
			}
			if zonesTarget.FunctionalThresholdPower != basetype.Uint16Invalid {
				imp.FTP = int(zonesTarget.FunctionalThresholdPower)
			}
			if zonesTarget.MaxHeartRate != basetype.Uint8Invalid {
				imp.MaxHR = int(zonesTarget.MaxHeartRate)
			}
		}
		data.UserProfileImport = imp
	}

	// Extract streams, track, sensors
	data.Streams = ExtractStreams(records)
	data.RRIntervals = rrIntervals
	data.TrackPoints = ExtractTrackPoints(records)
	data.Sensors = ExtractSensors(deviceInfos)

	// Extract laps, splits, lengths, climb pro events
	data.Laps = ExtractLaps(laps)
	data.Splits = ExtractSplits(splits)
	data.SplitSummaries = ExtractSplitSummaries(splitSummaries)
	data.Lengths = ExtractLengths(lengths)
	data.ClimbProEvents = ExtractClimbProEvents(climbPros)

	// Refine MovingTimeS using stream data when speed or GPS information is
	// available. The device's TotalTimerTime (already set above) is a valid
	// upper bound because the device itself excluded manual/auto-pause periods.
	// Our stream-based calculation may produce a lower value when the device
	// did not pause but the athlete was actually stopped (e.g. red lights,
	// waiting at a summit). We take the minimum of both to stay conservative.
	if activityutil.HasSpeedData(data.Streams) || activityutil.HasGPSData(data.Streams) {
		streamMoving := activityutil.CalcMovingTime(data.Streams, data.SportType)
		if streamMoving > 0 {
			data.MovingTimeS = activityutil.MinMovingTime(data.MovingTimeS, streamMoving)
		}
	}

	// GPS info
	data.HasGPS = len(data.TrackPoints) >= 2
	if data.HasGPS {
		data.StartLat = data.TrackPoints[0].Lat
		data.StartLng = data.TrackPoints[0].Lng
		last := data.TrackPoints[len(data.TrackPoints)-1]
		data.EndLat = last.Lat
		data.EndLng = last.Lng
	}

	// Indoor detection
	data.IsIndoor = !data.HasGPS

	return data, nil
}

// extractFITSource determines source app and recording device from FIT metadata.
func extractFITSource(fileId *mesgdef.FileId, deviceInfos []*mesgdef.DeviceInfo) (source, device string) {
	// Try FileId first (always present)
	if fileId != nil {
		mfr := fileId.Manufacturer
		mfrStr := mfr.String()

		// Determine source brand
		switch {
		case mfr == typedef.ManufacturerGarmin || mfr == typedef.ManufacturerDynastream || mfr == typedef.ManufacturerDynastreamOem:
			source = "Garmin"
		case mfr == typedef.ManufacturerTacx:
			source = "Tacx"
		case mfr == typedef.ManufacturerWahooFitness:
			source = "Wahoo"
		case mfr == typedef.ManufacturerHammerhead:
			source = "Hammerhead"
		case mfr == typedef.ManufacturerSuunto:
			source = "Suunto"
		case mfr == typedef.ManufacturerPolarElectro:
			source = "Polar"
		case mfr == typedef.ManufacturerBryton:
			source = "Bryton"
		case mfr == typedef.ManufacturerSigmasport:
			source = "Sigma Sport"
		case mfr == typedef.ManufacturerCoros:
			source = "Coros"
		case mfr == typedef.ManufacturerSrm:
			source = "SRM"
		case mfr == typedef.ManufacturerStrava:
			source = "Strava"
		case strings.Contains(strings.ToLower(mfrStr), "trainingpeaks"):
			source = "TrainingPeaks"
		case mfr == typedef.ManufacturerZwift:
			source = "Zwift"
		default:
			if !strings.HasPrefix(mfrStr, "Manufacturer(") && mfrStr != "" {
				source = mfrStr
			}
		}

		// Try ProductName from FileId
		if fileId.ProductName != "" {
			device = fileId.ProductName
		}

		// Try GarminProduct via dynamic field
		if device == "" {
			prodName, prodVal := fileId.GetProduct()
			if prodName == "garmin_product" {
				gp := prodVal.(typedef.GarminProduct)
				if gp != typedef.GarminProductInvalid {
					gpStr := gp.String()
					if !strings.HasPrefix(gpStr, "GarminProduct(") {
						device = formatDeviceName(source, gpStr)
					}
				}
			}
		}
	}

	// Enrich device name from DeviceInfo with DeviceIndex=0 (the recording device)
	for _, di := range deviceInfos {
		if di.DeviceIndex != typedef.DeviceIndexCreator {
			continue
		}
		if di.ProductName != "" && (device == "" || len(di.ProductName) > len(device)) {
			device = di.ProductName
		}
		if source == "" {
			mfrStr := di.Manufacturer.String()
			if !strings.HasPrefix(mfrStr, "Manufacturer(") && mfrStr != "" {
				source = mfrStr
			}
		}
		break
	}

	return source, device
}

// formatDeviceName makes GarminProduct enum names human-readable.
// e.g. "instinct_2x_solar" → "Garmin Instinct 2X Solar"
func formatDeviceName(brand, product string) string {
	product = strings.ReplaceAll(product, "_", " ")
	// Capitalize words
	words := strings.Fields(product)
	for i, w := range words {
		if len(w) <= 2 {
			words[i] = strings.ToUpper(w) // "2x" → "2X"
		} else {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	name := strings.Join(words, " ")
	if brand != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(brand)) {
		name = brand + " " + name
	}
	return name
}
