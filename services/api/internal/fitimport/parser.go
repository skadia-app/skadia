package fitimport

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/kit/semicircles"
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

	// Multi-session: check if all non-multisport sessions share the same sport.
	// GOTOES-merged FIT files produce 2 sessions with identical sport type — these
	// must NOT be split into parent/child multisport activities.
	var nonMultisportSports []typedef.Sport
	for i := range sessionMsgs {
		s := mesgdef.NewSession(&sessionMsgs[i])
		if s.Sport != typedef.SportMultisport {
			nonMultisportSports = append(nonMultisportSports, s.Sport)
		}
	}
	if len(nonMultisportSports) >= 2 && allSameSport(nonMultisportSports) {
		// All sessions are the same sport — treat as single activity.
		// Requires at least 2 non-multisport sessions to avoid misclassifying
		// partial multisport recordings (1 multisport summary + 1 discipline).
		// buildActivityData uses sessions[0] for aggregate metrics — we merge
		// the remaining sessions' totals afterwards so distance/time/elevation
		// reflect the complete merged activity.
		ad, err := buildActivityData(fit, filename)
		if err != nil {
			return nil, err
		}
		mergeSessionTotals(ad, sessionMsgs)
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
	end := min(headerSize+dataSize, len(data))

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
			for range numFields {
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

// gpsHeadingPoint holds a GPS heading extracted from a gps_metadata FIT message.
type gpsHeadingPoint struct {
	Time    time.Time
	Heading float64 // degrees (0–360)
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
	var events []*mesgdef.Event
	var sets []*mesgdef.Set
	var jumps []*mesgdef.Jump
	var hrs []*mesgdef.Hr
	var segmentLaps []*mesgdef.SegmentLap
	var workout *mesgdef.Workout
	var workoutSteps []*mesgdef.WorkoutStep
	var devDataIds []*mesgdef.DeveloperDataId
	var fieldDescs []*mesgdef.FieldDescription
	var diveSummary *mesgdef.DiveSummary
	var diveSettingsMsg *mesgdef.DiveSettings
	var diveGasMsgs []*mesgdef.DiveGas
	var maxMetData *mesgdef.MaxMetData

	// GPS heading points from gps_metadata messages (timestamp → heading in degrees)
	var gpsHeadings []gpsHeadingPoint

	// Sport message (MesgNum 12) fallback for files with no sport in session (Polar, WHOOP)
	var sportFallbackSport typedef.Sport
	var sportFallbackSubSport typedef.SubSport
	var sportFallbackName string
	// Activity message (MesgNum 34): NumSessions for multi-sport, LocalTimestamp for TZ fallback
	var activityNumSessions int
	var activityLocalTimestamp time.Time

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
		case typedef.MesgNumEvent:
			events = append(events, mesgdef.NewEvent(&fit.Messages[i]))
		case typedef.MesgNumSet:
			sets = append(sets, mesgdef.NewSet(&fit.Messages[i]))
		case typedef.MesgNumJump:
			jumps = append(jumps, mesgdef.NewJump(&fit.Messages[i]))
		case typedef.MesgNumHr:
			hrs = append(hrs, mesgdef.NewHr(&fit.Messages[i]))
		case typedef.MesgNumSegmentLap:
			segmentLaps = append(segmentLaps, mesgdef.NewSegmentLap(&fit.Messages[i]))
		case typedef.MesgNumWorkout:
			workout = mesgdef.NewWorkout(&fit.Messages[i])
		case typedef.MesgNumWorkoutStep:
			workoutSteps = append(workoutSteps, mesgdef.NewWorkoutStep(&fit.Messages[i]))
		case typedef.MesgNumDeveloperDataId:
			devDataIds = append(devDataIds, mesgdef.NewDeveloperDataId(&fit.Messages[i]))
		case typedef.MesgNumFieldDescription:
			fieldDescs = append(fieldDescs, mesgdef.NewFieldDescription(&fit.Messages[i]))
		case typedef.MesgNumDiveSummary:
			diveSummary = mesgdef.NewDiveSummary(&fit.Messages[i])
		case typedef.MesgNumDiveSettings:
			diveSettingsMsg = mesgdef.NewDiveSettings(&fit.Messages[i])
		case typedef.MesgNumDiveGas:
			diveGasMsgs = append(diveGasMsgs, mesgdef.NewDiveGas(&fit.Messages[i]))
		case typedef.MesgNumMaxMetData:
			maxMetData = mesgdef.NewMaxMetData(&fit.Messages[i])
		case typedef.MesgNumGpsMetadata:
			gm := mesgdef.NewGpsMetadata(&fit.Messages[i])
			if gm.Heading != basetype.Uint16Invalid {
				// Scale: 100, Units: degrees (uint16 / 100.0 = degrees)
				gpsHeadings = append(gpsHeadings, gpsHeadingPoint{
					Time:    gm.Timestamp,
					Heading: float64(gm.Heading) / 100.0,
				})
			}
		case typedef.MesgNumSport:
			sportMsg := mesgdef.NewSport(&fit.Messages[i])
			if sportMsg.Sport != typedef.SportInvalid {
				sportFallbackSport = sportMsg.Sport
				sportFallbackSubSport = sportMsg.SubSport
			}
			if sportMsg.Name != basetype.StringInvalid && sportMsg.Name != "" {
				sportFallbackName = sportMsg.Name
			}
		case typedef.MesgNumActivity:
			actMsg := mesgdef.NewActivity(&fit.Messages[i])
			if actMsg.NumSessions != basetype.Uint16Invalid && actMsg.NumSessions > 0 {
				activityNumSessions = int(actMsg.NumSessions)
			}
			if !actMsg.LocalTimestamp.IsZero() {
				activityLocalTimestamp = actMsg.LocalTimestamp
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

	// Virtual activity detection
	isVirtual := false

	// Path 1: SubSport 58 (VirtualActivity)
	if fitSubSportID == int16(typedef.SubSportVirtualActivity) {
		isVirtual = true
		if subType == "virtual_activity" {
			subType = "generic"
		}
	}

	// Path 2: Known virtual platform manufacturers
	if fileId != nil {
		switch fileId.Manufacturer {
		case typedef.ManufacturerZwift, typedef.ManufacturerZwiftByte,
			typedef.ManufacturerBkool, typedef.ManufacturerMywhoosh,
			typedef.ManufacturerVirtualtraining, typedef.ManufacturerIctrainer,
			typedef.ManufacturerVirtugo,
			typedef.ManufacturerRgtCycling:
			isVirtual = true
		}
	}

	data := &model.ActivityData{
		FileFormat:    "fit",
		OriginalName:  filename,
		SportType:     sportType,
		SubType:       subType,
		FitSportID:    fitSportID,
		FitSubSportID: fitSubSportID,
		IsEbike:       isEbike,
		IsVirtual:     isVirtual,
		StartedAt:     session.StartTime,
		EndedAt:       session.Timestamp,
	}

	// Apply Activity message fallbacks (NumSessions, LocalTimestamp)
	if activityNumSessions > 0 {
		data.NumSessions = activityNumSessions
	}
	if !activityLocalTimestamp.IsZero() {
		data.LocalTimestamp = activityLocalTimestamp
	}

	// Apply Sport message fallback for files where session has no sport (Polar, WHOOP)
	if (data.SportType == "unknown" || data.SportType == "") && sportFallbackSport != typedef.SportInvalid {
		mapped := SportToString(sportFallbackSport)
		if mapped != "" && mapped != "unknown" {
			data.SportType = mapped
			data.SubType = SubSportToString(sportFallbackSubSport)
			data.FitSportID = int16(sportFallbackSport)
			data.FitSubSportID = int16(sportFallbackSubSport)
		}
	}
	if data.SportName == "" && sportFallbackName != "" {
		data.SportName = sportFallbackName
	}

	// Source / Device from FileId and DeviceInfo[0]
	data.Source, data.SourceDevice, data.RecordingDevice = extractFITSource(fileId, deviceInfos)

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
	if session.MaxCadence != basetype.Uint8Invalid && session.MaxCadence != 0 {
		data.MaxCadence = int(session.MaxCadence)
	}

	// Temperature
	if session.AvgTemperature != 0x7F {
		data.AvgTemperatureC = float64(session.AvgTemperature)
	}
	if session.MinTemperature != basetype.Sint8Invalid {
		data.MinTemperatureC = float64(session.MinTemperature)
	}
	if session.MaxTemperature != basetype.Sint8Invalid {
		data.MaxTemperatureC = float64(session.MaxTemperature)
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

	// === FIT SDK — Movement / Cycles ===
	if session.TotalCycles != basetype.Uint32Invalid {
		v := int32(session.TotalCycles)
		data.TotalCycles = &v
	}
	if session.TotalWork != basetype.Uint32Invalid {
		v := int32(session.TotalWork)
		data.TotalWorkJ = &v
	}
	if session.TotalFatCalories != basetype.Uint16Invalid {
		v := int32(session.TotalFatCalories)
		data.TotalFatCalories = &v
	}
	if session.MetabolicCalories != basetype.Uint16Invalid {
		v := int32(session.MetabolicCalories)
		data.MetabolicCalories = &v
	}
	if session.TotalMovingTime != basetype.Uint32Invalid {
		v := float64(session.TotalMovingTime) / 1000.0
		data.TotalMovingTimeS = &v
	}

	// === FIT SDK — Swimming ===
	if session.SwimStroke != typedef.SwimStrokeInvalid {
		v := strings.ToLower(session.SwimStroke.String())
		data.SwimStroke = &v
	}
	if session.PoolLength != basetype.Uint16Invalid {
		v := float64(session.PoolLength) / 100.0
		data.PoolLengthM = &v
	}
	if session.PoolLengthUnit != typedef.DisplayMeasureInvalid {
		v := strings.ToLower(session.PoolLengthUnit.String())
		data.PoolLengthUnit = &v
	}
	if session.NumLengths != basetype.Uint16Invalid {
		v := int32(session.NumLengths)
		data.NumLengths = &v
	}
	if session.NumActiveLengths != basetype.Uint16Invalid {
		v := int32(session.NumActiveLengths)
		data.NumActiveLengths = &v
	}
	if session.AvgStrokeCount != basetype.Uint32Invalid {
		v := float64(session.AvgStrokeCount) / 10.0
		data.AvgStrokeCount = &v
	}
	if session.AvgStrokeDistance != basetype.Uint16Invalid {
		v := float64(session.AvgStrokeDistance) / 100.0
		data.AvgStrokeDistanceM = &v
	}
	// StrokeCount []uint16 → []int32
	if len(session.StrokeCount) > 0 {
		sc := make([]int32, 0, len(session.StrokeCount))
		for _, v := range session.StrokeCount {
			if v != basetype.Uint16Invalid {
				sc = append(sc, int32(v))
			}
		}
		if len(sc) > 0 {
			data.StrokeCount = sc
		}
	}
	// ZoneCount []uint16 → []int32
	if len(session.ZoneCount) > 0 {
		zc := make([]int32, 0, len(session.ZoneCount))
		for _, v := range session.ZoneCount {
			if v != basetype.Uint16Invalid {
				zc = append(zc, int32(v))
			}
		}
		if len(zc) > 0 {
			data.ZoneCount = zc
		}
	}

	// === FIT SDK — Cadence Extended ===
	if session.AvgFractionalCadence != basetype.Uint8Invalid {
		v := float64(session.AvgFractionalCadence) / 128.0
		data.AvgFractionalCadence = &v
	}
	if session.MaxFractionalCadence != basetype.Uint8Invalid {
		v := float64(session.MaxFractionalCadence) / 128.0
		data.MaxFractionalCadence = &v
	}
	if session.TotalFractionalCycles != basetype.Uint8Invalid {
		v := float64(session.TotalFractionalCycles) / 128.0
		data.TotalFractionalCycles = &v
	}

	// === FIT SDK — Running Dynamics ===
	if session.AvgVerticalOscillation != basetype.Uint16Invalid {
		v := float64(session.AvgVerticalOscillation) / 10.0
		data.AvgVerticalOscillationMm = &v
	}
	if session.AvgStanceTime != basetype.Uint16Invalid {
		v := float64(session.AvgStanceTime) / 10.0
		data.AvgStanceTimeMs = &v
	}
	if session.AvgStanceTimePercent != basetype.Uint16Invalid {
		v := float64(session.AvgStanceTimePercent) / 100.0
		data.AvgStanceTimePct = &v
	}
	if session.AvgStanceTimeBalance != basetype.Uint16Invalid {
		v := float64(session.AvgStanceTimeBalance) / 100.0
		data.AvgStanceTimeBalancePct = &v
	}
	if session.AvgStepLength != basetype.Uint16Invalid {
		v := float64(session.AvgStepLength) / 10.0
		data.AvgStepLengthMm = &v
	}
	if session.AvgVerticalRatio != basetype.Uint16Invalid {
		v := float64(session.AvgVerticalRatio) / 100.0
		data.AvgVerticalRatioPct = &v
	}

	// === FIT SDK — Grade ===
	if session.AvgGrade != basetype.Sint16Invalid {
		v := float64(session.AvgGrade) / 100.0
		data.AvgGradePct = &v
	}
	if session.AvgPosGrade != basetype.Sint16Invalid {
		v := float64(session.AvgPosGrade) / 100.0
		data.AvgPosGradePct = &v
	}
	if session.AvgNegGrade != basetype.Sint16Invalid {
		v := float64(session.AvgNegGrade) / 100.0
		data.AvgNegGradePct = &v
	}
	if session.MaxPosGrade != basetype.Sint16Invalid {
		v := float64(session.MaxPosGrade) / 100.0
		data.MaxPosGradePct = &v
	}
	if session.MaxNegGrade != basetype.Sint16Invalid {
		v := float64(session.MaxNegGrade) / 100.0
		data.MaxNegGradePct = &v
	}

	// === FIT SDK — Vertical Speed ===
	if session.AvgPosVerticalSpeed != basetype.Sint16Invalid {
		v := float64(session.AvgPosVerticalSpeed) / 1000.0
		data.AvgPosVerticalSpeedMs = &v
	}
	if session.AvgNegVerticalSpeed != basetype.Sint16Invalid {
		v := float64(session.AvgNegVerticalSpeed) / 1000.0
		data.AvgNegVerticalSpeedMs = &v
	}
	if session.MaxPosVerticalSpeed != basetype.Sint16Invalid {
		v := float64(session.MaxPosVerticalSpeed) / 1000.0
		data.MaxPosVerticalSpeedMs = &v
	}
	if session.MaxNegVerticalSpeed != basetype.Sint16Invalid {
		v := float64(session.MaxNegVerticalSpeed) / 1000.0
		data.MaxNegVerticalSpeedMs = &v
	}
	if session.AvgVam != basetype.Uint16Invalid {
		v := float64(session.AvgVam) / 1000.0
		data.AvgVamMs = &v
	}

	// === FIT SDK — Altitude Extended ===
	if session.EnhancedAvgAltitude != basetype.Uint32Invalid {
		v := float64(session.EnhancedAvgAltitude)/5.0 - 500.0
		data.AvgAltitudeM = &v
	}
	if session.TotalFractionalAscent != basetype.Uint8Invalid {
		v := float64(session.TotalFractionalAscent) / 100.0
		data.TotalFractionalAscentM = &v
	}
	if session.TotalFractionalDescent != basetype.Uint8Invalid {
		v := float64(session.TotalFractionalDescent) / 100.0
		data.TotalFractionalDescentM = &v
	}

	// === FIT SDK — Pedal Analytics ===
	if session.AvgLeftTorqueEffectiveness != basetype.Uint8Invalid {
		v := float64(session.AvgLeftTorqueEffectiveness) / 2.0
		data.AvgLeftTorqueEffectivenessPct = &v
	}
	if session.AvgRightTorqueEffectiveness != basetype.Uint8Invalid {
		v := float64(session.AvgRightTorqueEffectiveness) / 2.0
		data.AvgRightTorqueEffectivenessPct = &v
	}
	if session.AvgLeftPedalSmoothness != basetype.Uint8Invalid {
		v := float64(session.AvgLeftPedalSmoothness) / 2.0
		data.AvgLeftPedalSmoothnessPct = &v
	}
	if session.AvgRightPedalSmoothness != basetype.Uint8Invalid {
		v := float64(session.AvgRightPedalSmoothness) / 2.0
		data.AvgRightPedalSmoothnessPct = &v
	}
	if session.AvgCombinedPedalSmoothness != basetype.Uint8Invalid {
		v := float64(session.AvgCombinedPedalSmoothness) / 2.0
		data.AvgCombinedPedalSmoothnessPct = &v
	}
	if session.AvgLeftPco != basetype.Sint8Invalid {
		v := int16(session.AvgLeftPco)
		data.AvgLeftPcoMm = &v
	}
	if session.AvgRightPco != basetype.Sint8Invalid {
		v := int16(session.AvgRightPco)
		data.AvgRightPcoMm = &v
	}
	// Power Phase arrays: []uint8, Scale: 0.7111111, fixed 2-element [start, end]
	if len(session.AvgLeftPowerPhase) >= 2 {
		if session.AvgLeftPowerPhase[0] != basetype.Uint8Invalid {
			v := float64(session.AvgLeftPowerPhase[0]) / 0.7111111
			data.AvgLeftPowerPhaseStartDeg = &v
		}
		if session.AvgLeftPowerPhase[1] != basetype.Uint8Invalid {
			v := float64(session.AvgLeftPowerPhase[1]) / 0.7111111
			data.AvgLeftPowerPhaseEndDeg = &v
		}
	}
	if len(session.AvgLeftPowerPhasePeak) >= 2 {
		if session.AvgLeftPowerPhasePeak[0] != basetype.Uint8Invalid {
			v := float64(session.AvgLeftPowerPhasePeak[0]) / 0.7111111
			data.AvgLeftPowerPhasePeakStartDeg = &v
		}
		if session.AvgLeftPowerPhasePeak[1] != basetype.Uint8Invalid {
			v := float64(session.AvgLeftPowerPhasePeak[1]) / 0.7111111
			data.AvgLeftPowerPhasePeakEndDeg = &v
		}
	}
	if len(session.AvgRightPowerPhase) >= 2 {
		if session.AvgRightPowerPhase[0] != basetype.Uint8Invalid {
			v := float64(session.AvgRightPowerPhase[0]) / 0.7111111
			data.AvgRightPowerPhaseStartDeg = &v
		}
		if session.AvgRightPowerPhase[1] != basetype.Uint8Invalid {
			v := float64(session.AvgRightPowerPhase[1]) / 0.7111111
			data.AvgRightPowerPhaseEndDeg = &v
		}
	}
	if len(session.AvgRightPowerPhasePeak) >= 2 {
		if session.AvgRightPowerPhasePeak[0] != basetype.Uint8Invalid {
			v := float64(session.AvgRightPowerPhasePeak[0]) / 0.7111111
			data.AvgRightPowerPhasePeakStartDeg = &v
		}
		if session.AvgRightPowerPhasePeak[1] != basetype.Uint8Invalid {
			v := float64(session.AvgRightPowerPhasePeak[1]) / 0.7111111
			data.AvgRightPowerPhasePeakEndDeg = &v
		}
	}

	// === FIT SDK — Power/Cadence by Position ===
	if len(session.AvgPowerPosition) > 0 {
		pp := make([]int32, 0, len(session.AvgPowerPosition))
		for _, v := range session.AvgPowerPosition {
			if v != basetype.Uint16Invalid {
				pp = append(pp, int32(v))
			}
		}
		if len(pp) > 0 {
			data.AvgPowerPositionW = pp
		}
	}
	if len(session.MaxPowerPosition) > 0 {
		pp := make([]int32, 0, len(session.MaxPowerPosition))
		for _, v := range session.MaxPowerPosition {
			if v != basetype.Uint16Invalid {
				pp = append(pp, int32(v))
			}
		}
		if len(pp) > 0 {
			data.MaxPowerPositionW = pp
		}
	}
	if len(session.AvgCadencePosition) > 0 {
		cp := make([]int16, 0, len(session.AvgCadencePosition))
		for _, v := range session.AvgCadencePosition {
			if v != basetype.Uint8Invalid {
				cp = append(cp, int16(v))
			}
		}
		if len(cp) > 0 {
			data.AvgCadencePosition = cp
		}
	}
	if len(session.MaxCadencePosition) > 0 {
		cp := make([]int16, 0, len(session.MaxCadencePosition))
		for _, v := range session.MaxCadencePosition {
			if v != basetype.Uint8Invalid {
				cp = append(cp, int16(v))
			}
		}
		if len(cp) > 0 {
			data.MaxCadencePosition = cp
		}
	}

	// === FIT SDK — Standing/Sitting ===
	if session.TimeStanding != basetype.Uint32Invalid {
		v := float64(session.TimeStanding) / 1000.0
		data.TimeStandingS = &v
	}
	if session.StandCount != basetype.Uint16Invalid {
		v := int32(session.StandCount)
		data.StandCount = &v
	}

	// === FIT SDK — E-Bike ===
	if session.AvgLevMotorPower != basetype.Uint16Invalid {
		v := int32(session.AvgLevMotorPower)
		data.AvgLevMotorPowerW = &v
	}
	if session.MaxLevMotorPower != basetype.Uint16Invalid {
		v := int32(session.MaxLevMotorPower)
		data.MaxLevMotorPowerW = &v
	}
	if session.LevBatteryConsumption != basetype.Uint8Invalid {
		v := float64(session.LevBatteryConsumption) / 2.0
		data.LevBatteryConsumptionPct = &v
	}

	// === FIT SDK — MTB Grit/Flow (float32 → float64) ===
	if !math.IsNaN(float64(session.TotalGrit)) && session.TotalGrit != math.Float32frombits(basetype.Float32Invalid) {
		v := float64(session.TotalGrit)
		data.TotalGrit = &v
	}
	if !math.IsNaN(float64(session.TotalFlow)) && session.TotalFlow != math.Float32frombits(basetype.Float32Invalid) {
		v := float64(session.TotalFlow)
		data.TotalFlow = &v
	}
	if !math.IsNaN(float64(session.AvgGrit)) && session.AvgGrit != math.Float32frombits(basetype.Float32Invalid) {
		v := float64(session.AvgGrit)
		data.AvgGrit = &v
	}
	if !math.IsNaN(float64(session.AvgFlow)) && session.AvgFlow != math.Float32frombits(basetype.Float32Invalid) {
		v := float64(session.AvgFlow)
		data.AvgFlow = &v
	}

	// === FIT SDK — Jumps ===
	if session.JumpCount != basetype.Uint16Invalid {
		v := int32(session.JumpCount)
		data.JumpCount = &v
	}

	// === FIT SDK — Ball Sports ===
	if session.MaxBallSpeed != basetype.Uint16Invalid {
		v := float64(session.MaxBallSpeed) / 100.0
		data.MaxBallSpeedMs = &v
	}
	if session.AvgBallSpeed != basetype.Uint16Invalid {
		v := float64(session.AvgBallSpeed) / 100.0
		data.AvgBallSpeedMs = &v
	}
	if session.PlayerScore != basetype.Uint16Invalid {
		v := int32(session.PlayerScore)
		data.PlayerScore = &v
	}
	if session.OpponentScore != basetype.Uint16Invalid {
		v := int32(session.OpponentScore)
		data.OpponentScore = &v
	}
	if session.OpponentName != basetype.StringInvalid && session.OpponentName != "" {
		data.OpponentName = &session.OpponentName
	}

	// === FIT SDK — Hemoglobin (arrays, multi-sensor) ===
	if len(session.AvgTotalHemoglobinConc) > 0 {
		hb := make([]float64, 0, len(session.AvgTotalHemoglobinConc))
		for _, v := range session.AvgTotalHemoglobinConc {
			if v != basetype.Uint16Invalid {
				hb = append(hb, float64(v)/100.0)
			}
		}
		if len(hb) > 0 {
			data.AvgTotalHemoglobinConc = hb
		}
	}
	if len(session.MinTotalHemoglobinConc) > 0 {
		hb := make([]float64, 0, len(session.MinTotalHemoglobinConc))
		for _, v := range session.MinTotalHemoglobinConc {
			if v != basetype.Uint16Invalid {
				hb = append(hb, float64(v)/100.0)
			}
		}
		if len(hb) > 0 {
			data.MinTotalHemoglobinConc = hb
		}
	}
	if len(session.MaxTotalHemoglobinConc) > 0 {
		hb := make([]float64, 0, len(session.MaxTotalHemoglobinConc))
		for _, v := range session.MaxTotalHemoglobinConc {
			if v != basetype.Uint16Invalid {
				hb = append(hb, float64(v)/100.0)
			}
		}
		if len(hb) > 0 {
			data.MaxTotalHemoglobinConc = hb
		}
	}
	if len(session.AvgSaturatedHemoglobinPercent) > 0 {
		hb := make([]float64, 0, len(session.AvgSaturatedHemoglobinPercent))
		for _, v := range session.AvgSaturatedHemoglobinPercent {
			if v != basetype.Uint16Invalid {
				hb = append(hb, float64(v)/10.0)
			}
		}
		if len(hb) > 0 {
			data.AvgSaturatedHemoglobinPct = hb
		}
	}
	if len(session.MinSaturatedHemoglobinPercent) > 0 {
		hb := make([]float64, 0, len(session.MinSaturatedHemoglobinPercent))
		for _, v := range session.MinSaturatedHemoglobinPercent {
			if v != basetype.Uint16Invalid {
				hb = append(hb, float64(v)/10.0)
			}
		}
		if len(hb) > 0 {
			data.MinSaturatedHemoglobinPct = hb
		}
	}
	if len(session.MaxSaturatedHemoglobinPercent) > 0 {
		hb := make([]float64, 0, len(session.MaxSaturatedHemoglobinPercent))
		for _, v := range session.MaxSaturatedHemoglobinPercent {
			if v != basetype.Uint16Invalid {
				hb = append(hb, float64(v)/10.0)
			}
		}
		if len(hb) > 0 {
			data.MaxSaturatedHemoglobinPct = hb
		}
	}

	// === FIT SDK — Diving ===
	if session.AvgDepth != basetype.Uint32Invalid {
		v := float64(session.AvgDepth) / 1000.0
		data.AvgDepthM = &v
	}
	if session.MaxDepth != basetype.Uint32Invalid {
		v := float64(session.MaxDepth) / 1000.0
		data.MaxDepthM = &v
	}
	if session.SurfaceInterval != basetype.Uint32Invalid {
		v := int32(session.SurfaceInterval)
		data.SurfaceIntervalS = &v
	}
	if session.DiveNumber != basetype.Uint32Invalid {
		v := int32(session.DiveNumber)
		data.DiveNumber = &v
	}
	if session.StartCns != basetype.Uint8Invalid {
		v := int16(session.StartCns)
		data.StartCnsPct = &v
	}
	if session.EndCns != basetype.Uint8Invalid {
		v := int16(session.EndCns)
		data.EndCnsPct = &v
	}
	if session.StartN2 != basetype.Uint16Invalid {
		v := int32(session.StartN2)
		data.StartN2Pct = &v
	}
	if session.EndN2 != basetype.Uint16Invalid {
		v := int32(session.EndN2)
		data.EndN2Pct = &v
	}
	if session.O2Toxicity != basetype.Uint16Invalid {
		v := int32(session.O2Toxicity)
		data.O2ToxicityOtu = &v
	}

	// DiveSummary (MesgNum 268) is the authoritative source for dive metadata on
	// Garmin Descent devices. Many dive FIT files omit the diving fields from the
	// Session message and only populate DiveSummary. Apply DiveSummary values as a
	// fallback (or override) for any field that is still nil after session parsing.
	if diveSummary != nil {
		if data.AvgDepthM == nil && diveSummary.AvgDepth != basetype.Uint32Invalid {
			v := diveSummary.AvgDepthScaled()
			data.AvgDepthM = &v
		}
		if data.MaxDepthM == nil && diveSummary.MaxDepth != basetype.Uint32Invalid {
			v := diveSummary.MaxDepthScaled()
			data.MaxDepthM = &v
		}
		if data.DiveNumber == nil && diveSummary.DiveNumber != basetype.Uint32Invalid {
			v := int32(diveSummary.DiveNumber)
			data.DiveNumber = &v
		}
		if data.SurfaceIntervalS == nil && diveSummary.SurfaceInterval != basetype.Uint32Invalid {
			v := int32(diveSummary.SurfaceInterval)
			data.SurfaceIntervalS = &v
		}
		if data.StartCnsPct == nil && diveSummary.StartCns != basetype.Uint8Invalid {
			v := int16(diveSummary.StartCns)
			data.StartCnsPct = &v
		}
		if data.EndCnsPct == nil && diveSummary.EndCns != basetype.Uint8Invalid {
			v := int16(diveSummary.EndCns)
			data.EndCnsPct = &v
		}
		if data.StartN2Pct == nil && diveSummary.StartN2 != basetype.Uint16Invalid {
			v := int32(diveSummary.StartN2)
			data.StartN2Pct = &v
		}
		if data.EndN2Pct == nil && diveSummary.EndN2 != basetype.Uint16Invalid {
			v := int32(diveSummary.EndN2)
			data.EndN2Pct = &v
		}
		if data.O2ToxicityOtu == nil && diveSummary.O2Toxicity != basetype.Uint16Invalid {
			v := int32(diveSummary.O2Toxicity)
			data.O2ToxicityOtu = &v
		}
	}

	// === FIT SDK — Respiration / HRV Extended ===
	if session.EnhancedMaxRespirationRate != basetype.Uint16Invalid {
		v := float64(session.EnhancedMaxRespirationRate) / 100.0
		data.MaxRespirationRate = &v
	}
	if session.EnhancedMinRespirationRate != basetype.Uint16Invalid {
		v := float64(session.EnhancedMinRespirationRate) / 100.0
		data.MinRespirationRate = &v
	}
	if session.SdrrHrv != basetype.Uint8Invalid {
		v := int16(session.SdrrHrv)
		data.SdrrHrvMs = &v
	}

	// === FIT SDK — Temperature Extended ===
	if session.MinCoreTemperature != basetype.Uint16Invalid {
		v := float64(session.MinCoreTemperature) / 100.0
		data.MinCoreTemperatureC = &v
	}
	if session.MaxCoreTemperature != basetype.Uint16Invalid {
		v := float64(session.MaxCoreTemperature) / 100.0
		data.MaxCoreTemperatureC = &v
	}

	// === FIT SDK — Misc ===
	if session.AvgLapTime != basetype.Uint32Invalid {
		v := float64(session.AvgLapTime) / 1000.0
		data.AvgLapTimeS = &v
	}
	if session.BestLapIndex != basetype.Uint16Invalid {
		v := int16(session.BestLapIndex)
		data.BestLapIndex = &v
	}
	if session.NumLaps != basetype.Uint16Invalid {
		v := int32(session.NumLaps)
		data.NumLaps = &v
	}
	if session.FirstLapIndex != basetype.Uint16Invalid {
		v := int32(session.FirstLapIndex)
		data.FirstLapIndex = &v
	}
	if session.IntensityFactor != basetype.Uint16Invalid {
		v := float64(session.IntensityFactor) / 1000.0
		data.IntensityFactor = &v
	}
	// LeftRightBalance (LeftRightBalance100: uint16, mask 0x3FFF, scale /100)
	if session.LeftRightBalance != typedef.LeftRightBalance100Invalid {
		v := float64(uint16(session.LeftRightBalance)&0x3FFF) / 100.0
		data.LeftRightBalance = &v
	}
	if session.SportProfileName != basetype.StringInvalid && session.SportProfileName != "" {
		data.SportProfileName = &session.SportProfileName
	}
	if session.SportIndex != basetype.Uint8Invalid {
		v := int16(session.SportIndex)
		data.SportIndex = &v
	}
	// Bounding box (semicircles → degrees)
	if session.NecLat != basetype.Sint32Invalid {
		v := semicircles.ToDegrees(session.NecLat)
		data.NecLat = &v
	}
	if session.NecLong != basetype.Sint32Invalid {
		v := semicircles.ToDegrees(session.NecLong)
		data.NecLng = &v
	}
	if session.SwcLat != basetype.Sint32Invalid {
		v := semicircles.ToDegrees(session.SwcLat)
		data.SwcLat = &v
	}
	if session.SwcLong != basetype.Sint32Invalid {
		v := semicircles.ToDegrees(session.SwcLong)
		data.SwcLng = &v
	}

	// TimeInZone messages are processed after lap/segment-lap extraction (below)
	// so we can map ReferenceIndex → lap UUID.

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

	// New FIT SDK record collections
	data.Events = ExtractEvents(events)
	data.Sets = ExtractSets(sets)
	data.Jumps = ExtractJumps(jumps)
	data.HrData = ExtractHrData(hrs)
	data.SegmentLaps = ExtractSegmentLaps(segmentLaps)

	// TimeInZone messages — dispatch by ReferenceMesg to session / lap / segment-lap.
	// Build messageIndex → UUID lookups from already-extracted laps/segment-laps.
	lapUUIDByMsgIdx := make(map[int]string, len(data.Laps))
	for _, l := range data.Laps {
		lapUUIDByMsgIdx[l.MessageIndex] = l.ID
	}
	segLapUUIDByMsgIdx := make(map[int16]string, len(data.SegmentLaps))
	for _, sl := range data.SegmentLaps {
		segLapUUIDByMsgIdx[sl.MessageIndex] = sl.ID
	}

	for _, tiz := range timeInZones {
		zones := extractTimeInZoneEntries(tiz)
		if len(zones) == 0 {
			continue
		}

		switch tiz.ReferenceMesg {
		case typedef.MesgNumLap:
			idx := int(tiz.ReferenceIndex)
			if lapID, ok := lapUUIDByMsgIdx[idx]; ok {
				if data.LapTimeInZones == nil {
					data.LapTimeInZones = make(map[string][]model.TimeInZoneData)
				}
				data.LapTimeInZones[lapID] = append(data.LapTimeInZones[lapID], zones...)
			}
		case typedef.MesgNumSegmentLap:
			idx := int16(tiz.ReferenceIndex)
			if slID, ok := segLapUUIDByMsgIdx[idx]; ok {
				if data.SegmentLapTimeInZones == nil {
					data.SegmentLapTimeInZones = make(map[string][]model.TimeInZoneData)
				}
				data.SegmentLapTimeInZones[slID] = append(data.SegmentLapTimeInZones[slID], zones...)
			}
		default:
			// Session-level (MesgNumSession, MesgNumInvalid, or unset)
			data.TimeInZoneData = append(data.TimeInZoneData, zones...)
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

	if workout != nil {
		data.Workout = ExtractWorkout(workout)
		data.WorkoutSteps = ExtractWorkoutSteps(workoutSteps)
	}
	data.DeveloperFieldDefs = ExtractDeveloperFieldDefs(devDataIds, fieldDescs)

	// For merged files (multi-source GPX exports and FIT-stitching tools) the session's TotalElapsedTime can be
	// the span from first to last timestamp (e.g. 17h) rather than actual recording
	// time (e.g. 1h28). Cap ElapsedTimeS to the recording time if significantly lower.
	if len(data.Streams) >= 2 {
		recordingTime := activityutil.CalcRecordingTime(data.Streams)
		if recordingTime > 0 && recordingTime < data.ElapsedTimeS {
			data.ElapsedTimeS = recordingTime
		}
	}

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

	// Start position from Session (fallback if no trackpoints)
	if session.StartPositionLat != basetype.Sint32Invalid &&
		session.StartPositionLong != basetype.Sint32Invalid {
		data.StartLat = semicircles.ToDegrees(session.StartPositionLat)
		data.StartLng = semicircles.ToDegrees(session.StartPositionLong)
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

	// Fallback: compute max/min elevation from stream data when session summary
	// doesn't provide them (common with some Garmin firmware versions).
	if data.MaxElevationM == 0 || data.MinElevationM == 0 {
		var maxAlt, minAlt float64
		minAlt = math.MaxFloat64
		maxAlt = -math.MaxFloat64
		altCount := 0
		for _, sp := range data.Streams {
			if sp.AltitudeM != nil {
				alt := *sp.AltitudeM
				if alt > maxAlt {
					maxAlt = alt
				}
				if alt < minAlt {
					minAlt = alt
				}
				altCount++
			}
		}
		if altCount > 0 {
			if data.MaxElevationM == 0 {
				data.MaxElevationM = maxAlt
			}
			if data.MinElevationM == 0 {
				data.MinElevationM = minAlt
			}
		}
	}

	// Indoor detection
	data.IsIndoor = !data.HasGPS

	// === FIT SDK — VO2max (max_met_data message) ===
	if maxMetData != nil {
		if maxMetData.Vo2Max != basetype.Uint16Invalid && maxMetData.Vo2Max > 0 {
			v := maxMetData.Vo2MaxScaled()
			data.Vo2MaxMlKgMin = &v
		}
		// CalibratedData: typedef.Bool, valid values are 0 (false) and 1 (true); 2+ invalid
		if maxMetData.CalibratedData < 2 {
			data.Vo2MaxCalibrated = maxMetData.CalibratedData == 1
		}
		if maxMetData.HrSource != typedef.MaxMetHeartRateSourceInvalid {
			s := maxMetData.HrSource.String()
			data.Vo2MaxHrSource = &s
		}
		if maxMetData.SpeedSource != typedef.MaxMetSpeedSourceInvalid {
			s := maxMetData.SpeedSource.String()
			data.Vo2MaxSpeedSource = &s
		}
	}

	// === FIT SDK — DiveSettings (MesgNum 258) ===
	if diveSettingsMsg != nil {
		ds := &model.DiveSettings{}
		if diveSettingsMsg.GfLow != basetype.Uint8Invalid {
			ds.GfLow = int(diveSettingsMsg.GfLow)
		}
		if diveSettingsMsg.GfHigh != basetype.Uint8Invalid {
			ds.GfHigh = int(diveSettingsMsg.GfHigh)
		}
		if diveSettingsMsg.WaterType != typedef.WaterTypeInvalid {
			ds.WaterType = diveSettingsMsg.WaterType.String()
		}
		if math.Float32bits(diveSettingsMsg.WaterDensity) != basetype.Float32Invalid {
			ds.WaterDensity = float64(diveSettingsMsg.WaterDensity)
		}
		if diveSettingsMsg.Model != typedef.TissueModelTypeInvalid {
			ds.DecoModel = diveSettingsMsg.Model.String()
		}
		if diveSettingsMsg.Po2Warn != basetype.Uint8Invalid {
			ds.Po2Warn = diveSettingsMsg.Po2WarnScaled()
		}
		if diveSettingsMsg.Po2Critical != basetype.Uint8Invalid {
			ds.Po2Critical = diveSettingsMsg.Po2CriticalScaled()
		}
		if diveSettingsMsg.Po2Deco != basetype.Uint8Invalid {
			ds.Po2Deco = diveSettingsMsg.Po2DecoScaled()
		}
		if diveSettingsMsg.SafetyStopEnabled < 2 {
			ds.SafetyStopEnabled = diveSettingsMsg.SafetyStopEnabled == 1
		}
		if diveSettingsMsg.SafetyStopTime != basetype.Uint16Invalid {
			ds.SafetyStopTimeS = int(diveSettingsMsg.SafetyStopTime)
		}
		if diveSettingsMsg.RepeatDiveInterval != basetype.Uint16Invalid {
			ds.RepeatDiveIntervalS = int(diveSettingsMsg.RepeatDiveInterval)
		}
		if diveSettingsMsg.CcrLowSetpoint != basetype.Uint8Invalid {
			ds.CcrLowSetpoint = diveSettingsMsg.CcrLowSetpointScaled()
		}
		if diveSettingsMsg.CcrLowSetpointDepth != basetype.Uint32Invalid {
			ds.CcrLowSetpointDepthM = diveSettingsMsg.CcrLowSetpointDepthScaled()
		}
		if diveSettingsMsg.CcrHighSetpoint != basetype.Uint8Invalid {
			ds.CcrHighSetpoint = diveSettingsMsg.CcrHighSetpointScaled()
		}
		if diveSettingsMsg.CcrHighSetpointDepth != basetype.Uint32Invalid {
			ds.CcrHighSetpointDepthM = diveSettingsMsg.CcrHighSetpointDepthScaled()
		}
		data.DiveSettings = ds
	}

	// === FIT SDK — DiveGas (MesgNum 259) ===
	for _, dg := range diveGasMsgs {
		gas := model.DiveGas{
			GasIndex: int(dg.MessageIndex),
		}
		if dg.HeliumContent != basetype.Uint8Invalid {
			gas.HeliumPct = int(dg.HeliumContent)
		}
		if dg.OxygenContent != basetype.Uint8Invalid {
			gas.OxygenPct = int(dg.OxygenContent)
		}
		if dg.Status != typedef.DiveGasStatusInvalid {
			gas.Status = dg.Status.String()
		}
		if dg.Mode != typedef.DiveGasModeInvalid {
			gas.Mode = dg.Mode.String()
		}
		data.DiveGases = append(data.DiveGases, gas)
	}

	// === FIT SDK — GPS Metadata: assign headings to stream points ===
	if len(gpsHeadings) > 0 && len(data.Streams) > 0 {
		assignHeadingToStreams(data.Streams, gpsHeadings)
	}

	// === FIT SDK — Session GPS accuracy and end position fallback ===
	if session.GpsAccuracy != basetype.Uint8Invalid {
		data.GpsAccuracyM = int(session.GpsAccuracy)
	}
	// End position fallback from session (used when no trackpoints available)
	if session.EndPositionLat != basetype.Sint32Invalid &&
		session.EndPositionLong != basetype.Sint32Invalid {
		if data.EndLat == 0 && data.EndLng == 0 {
			data.EndLat = semicircles.ToDegrees(session.EndPositionLat)
			data.EndLng = semicircles.ToDegrees(session.EndPositionLong)
		}
	}

	return data, nil
}

// assignHeadingToStreams assigns GPS heading values from gps_metadata messages to
// stream points by nearest timestamp. A heading is only assigned when the closest
// gps_metadata point is within 2 seconds of the stream point's timestamp.
func assignHeadingToStreams(streams []model.StreamPoint, headings []gpsHeadingPoint) {
	// headings is assumed to be in chronological order (same as messages in FIT file)
	hi := 0
	for i := range streams {
		// Advance heading index to the closest point in time
		for hi < len(headings)-1 && headings[hi+1].Time.Before(streams[i].Time) {
			hi++
		}
		if hi < len(headings) {
			diff := streams[i].Time.Sub(headings[hi].Time)
			if diff < 0 {
				diff = -diff
			}
			if diff <= 2*time.Second {
				h := float32(headings[hi].Heading)
				streams[i].Heading = &h
			}
		}
	}
}

// extractFITSource determines source app and recording device from FIT metadata.
// It also builds a DeviceUpsert for the recording device (from FileId fields).
func extractFITSource(fileId *mesgdef.FileId, deviceInfos []*mesgdef.DeviceInfo) (source, device string, du *model.DeviceUpsert) {
	var mfrID uint16
	var prodID uint16

	// Try FileId first (always present)
	if fileId != nil {
		mfr := fileId.Manufacturer
		mfrStr := mfr.String()
		mfrID = uint16(mfr)

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
		case mfr == typedef.ManufacturerPeaksware:
			source = "TrainingPeaks"
		case mfr == typedef.ManufacturerTimex:
			source = "Timex"
		case mfr == typedef.ManufacturerTomtom:
			source = "TomTom"
		case mfr == typedef.ManufacturerXplova:
			source = "Xplova"
		case mfr == typedef.ManufacturerConcept2:
			source = "Concept2"
		case mfr == typedef.ManufacturerForm:
			source = "FORM"
		case mfr == typedef.ManufacturerMagene:
			source = "Magene"
		case mfr == typedef.ManufacturerWattbike:
			source = "Wattbike"
		case mfr == typedef.ManufacturerIgpsport:
			source = "iGPSport"
		case mfr == typedef.ManufacturerCycplus:
			source = "Cycplus"
		case mfr == typedef.ManufacturerDevelopment:
			// manufacturer_id=255: Apple Watch, Amazfit, Huawei etc.
			// Resolve from ProductName pattern (Watch7,12 → Apple Watch Ultra 3).
			// DeviceInfo.Descriptor contains user-editable device name ("Ray's Apple Watch")
			// and is NOT used for identification — only ProductName is reliable.
			devSource, devName := ResolveDevelopmentManufacturer(fileId.ProductName, "")
			if devSource != "" {
				source = devSource
			}
			if devName != "" {
				device = devName
			}
		default:
			if mapped := MapVirtualPlatform(mfrStr); mapped != "" {
				source = mapped
			} else if !strings.HasPrefix(mfrStr, "Manufacturer") && mfrStr != "" {
				source = mfrStr
			}
		}

		// Try ProductName from FileId
		if fileId.ProductName != "" {
			device = fileId.ProductName
		}

		// Try product via dynamic field; also extract prodID for DeviceUpsert
		prodName, prodVal := fileId.GetProduct()
		if prodName == "garmin_product" {
			if gp, ok := prodVal.(typedef.GarminProduct); ok {
				prodID = uint16(gp)
				if device == "" && gp != typedef.GarminProductInvalid {
					gpStr := gp.String()
					if !strings.HasPrefix(gpStr, "GarminProduct") {
						device = formatDeviceNameFromEnum(source, gpStr)
					}
				}
			}
		} else if prodName == "favero_product" {
			if fp, ok := prodVal.(typedef.FaveroProduct); ok {
				prodID = uint16(fp)
			}
		} else if prodVal != nil {
			if raw, ok := prodVal.(uint16); ok {
				prodID = raw
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
			if !strings.HasPrefix(mfrStr, "Manufacturer") && mfrStr != "" {
				source = mfrStr
			}
		}
		break
	}

	// Build DeviceUpsert for the recording device
	if mfrID != 0 && mfrID != uint16(typedef.ManufacturerInvalid) {
		var serialNumber *int64
		if fileId != nil && fileId.SerialNumber != 0 && fileId.SerialNumber != 0xFFFFFFFF {
			sn := int64(fileId.SerialNumber)
			serialNumber = &sn
		}
		productName := ResolveProductName(mfrID, prodID, "")
		mfrStr := typedef.Manufacturer(mfrID).String()
		mfrDisplay := FormatManufacturer(mfrStr)

		// Override manufacturer display for resolved development devices
		if mfrID == uint16(typedef.ManufacturerDevelopment) && source != "" && source != "development" {
			mfrDisplay = source
			if productName == "" || productName == "Development" {
				productName = device
			}
		}
		// Override for virtual platforms
		if mapped := MapVirtualPlatform(mfrStr); mapped != "" {
			mfrDisplay = mapped
		}

		du = &model.DeviceUpsert{
			Type:           "recorder",
			Manufacturer:   mfrDisplay,
			ManufacturerID: int(mfrID),
			ProductName:    productName,
			ProductID:      int(prodID),
			SerialNumber:   serialNumber,
		}
	}

	return source, device, du
}

// allSameSport returns true when all elements in sports are identical.
// Used to detect GOTOES-merged files where multiple sessions share one sport.
func allSameSport(sports []typedef.Sport) bool {
	if len(sports) == 0 {
		return false
	}
	first := sports[0]
	for _, s := range sports[1:] {
		if s != first {
			return false
		}
	}
	return true
}

// mergeSessionTotals aggregates metrics from sessions[1:] into ad.
// buildActivityData already populated ad from sessions[0]. This function
// sums additive fields (distance, time, elevation, calories, TSS), takes
// max/min for extremes (HR, power, speed, altitude), and extends the time
// range. Used when GOTOES-merged same-sport files are collapsed to one activity.
func mergeSessionTotals(ad *model.ActivityData, sessionMsgs []proto.Message) {
	if len(sessionMsgs) <= 1 {
		return
	}

	for i := 1; i < len(sessionMsgs); i++ {
		s := mesgdef.NewSession(&sessionMsgs[i])
		if s.Sport == typedef.SportMultisport {
			continue
		}

		// --- Additive fields ---
		if s.TotalDistance != 0xFFFFFFFF {
			ad.DistanceM += float64(s.TotalDistance) / 100.0
		}
		if s.TotalElapsedTime != 0xFFFFFFFF {
			ad.ElapsedTimeS += int(s.TotalElapsedTime / 1000)
		}
		if s.TotalTimerTime != 0xFFFFFFFF {
			ad.MovingTimeS += int(s.TotalTimerTime / 1000)
		}
		if s.TotalAscent != 0xFFFF {
			ad.ElevationGainM += float64(s.TotalAscent)
		}
		if s.TotalDescent != 0xFFFF {
			ad.ElevationLossM += float64(s.TotalDescent)
		}
		if s.TotalCalories != basetype.Uint16Invalid {
			ad.TotalCalories += int(s.TotalCalories)
		}
		if s.TrainingStressScore != 0xFFFF {
			ad.TSS += float64(s.TrainingStressScore) / 10.0
		}

		// --- Max fields ---
		if s.MaxHeartRate != 0xFF && s.MaxHeartRate != 0 && int(s.MaxHeartRate) > ad.MaxHeartRate {
			ad.MaxHeartRate = int(s.MaxHeartRate)
		}
		if s.MaxPower != 0xFFFF && s.MaxPower != 0 && int(s.MaxPower) > ad.MaxPowerW {
			ad.MaxPowerW = int(s.MaxPower)
		}
		var maxSpd float64
		if s.EnhancedMaxSpeed != 0xFFFFFFFF && s.EnhancedMaxSpeed != 0 {
			maxSpd = float64(s.EnhancedMaxSpeed) / 1000.0
		} else if s.MaxSpeed != 0xFFFF {
			maxSpd = float64(s.MaxSpeed) / 1000.0
		}
		if maxSpd > ad.MaxSpeedMS {
			ad.MaxSpeedMS = maxSpd
		}
		if s.EnhancedMaxAltitude != 0xFFFFFFFF {
			maxAlt := float64(s.EnhancedMaxAltitude)/5.0 - 500.0
			if maxAlt > ad.MaxElevationM {
				ad.MaxElevationM = maxAlt
			}
		}

		// --- Min fields ---
		if s.MinHeartRate != basetype.Uint8Invalid && s.MinHeartRate != 0 {
			if ad.MinHeartRate == 0 || int(s.MinHeartRate) < ad.MinHeartRate {
				ad.MinHeartRate = int(s.MinHeartRate)
			}
		}
		if s.EnhancedMinAltitude != 0xFFFFFFFF {
			minAlt := float64(s.EnhancedMinAltitude)/5.0 - 500.0
			if ad.MinElevationM == 0 || minAlt < ad.MinElevationM {
				ad.MinElevationM = minAlt
			}
		}

		// --- Time range ---
		if !s.Timestamp.IsZero() && s.Timestamp.After(ad.EndedAt) {
			ad.EndedAt = s.Timestamp
		}
	}

	// Recalculate avg speed from merged totals (more accurate than any single session)
	if ad.MovingTimeS > 0 && ad.DistanceM > 0 {
		ad.AvgSpeedMS = ad.DistanceM / float64(ad.MovingTimeS)
	}

	// Note: AvgHeartRate, AvgPower, AvgCadence, AvgTemperature are left from
	// sessions[0]. Computing accurate weighted averages requires timer-time weights
	// which adds complexity for marginal gain. The stream-based moving time
	// recalculation in buildActivityData already handles MovingTimeS correctly
	// since it operates on ALL records from the merged file.
}

// extractTimeInZoneEntries converts a single FIT TimeInZone message into HR and power zone entries.
func extractTimeInZoneEntries(tiz *mesgdef.TimeInZone) []model.TimeInZoneData {
	var zones []model.TimeInZoneData
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
			zones = append(zones, zd)
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
			zones = append(zones, zd)
		}
	}
	return zones
}
