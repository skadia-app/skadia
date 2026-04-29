package fitimport

import (
	"bytes"
	"time"

	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"
)

// synthesizeMinimalActivityFIT returns a valid activity-FIT byte stream
// containing one Session (Cycling, ~1h, ~25 km), one Lap, and 10 Records.
//
// GPS coordinates are placed in the Atlantic Ocean (lat≈0, lon≈-30) with a
// small per-record drift — not a real city, so these bytes are safe to commit.
func synthesizeMinimalActivityFIT() ([]byte, error) {
	// Anchor time: 2020-01-15 10:00:00 UTC — well past FIT epoch, no DST ambiguity.
	start := time.Date(2020, 1, 15, 10, 0, 0, 0, time.UTC)
	duration := time.Hour

	// FileId
	fileId := mesgdef.NewFileId(nil)
	fileId.Type = typedef.FileActivity
	fileId.Manufacturer = typedef.ManufacturerDevelopment
	fileId.TimeCreated = start

	// 10 records: lat≈0°, lon≈-30°, altitude ~100 m, HR 140 bpm.
	// Semicircle conversion: degrees × (2^31 / 180).
	const (
		baseLat = 0.0   // equator
		baseLon = -30.0 // mid-Atlantic, not near any city
		baseAlt = 100.0 // metres
	)
	const recordCount = 10
	const intervalSec = int(36 * time.Second / time.Second) // 360 s / 10 = 36 s each
	const totalDistM = 25_000.0                             // 25 km total

	var records []proto.Message
	for i := 0; i < recordCount; i++ {
		rec := mesgdef.NewRecord(nil)
		rec.Timestamp = start.Add(time.Duration(i*intervalSec) * time.Second)
		// Small drift: ~0.001° per record ≈ ~111 m — plausible cycling speed.
		rec.PositionLat = degToSemicircles(baseLat + float64(i)*0.001)
		rec.PositionLong = degToSemicircles(baseLon + float64(i)*0.001)
		altRaw := uint16((baseAlt + float64(i)*0.5 + 500) * 5) // FIT scale: (m+500)*5
		rec.Altitude = altRaw
		rec.HeartRate = uint8(140 + i)
		records = append(records, rec.ToMesg(nil))
	}

	// Lap covering all records.
	lap := mesgdef.NewLap(nil)
	lap.Timestamp = start.Add(duration)
	lap.StartTime = start
	lap.TotalElapsedTime = uint32(duration.Milliseconds())
	lap.TotalDistance = uint32(totalDistM * 100) // FIT scale: cm

	// Session.
	sess := mesgdef.NewSession(nil)
	sess.Timestamp = start.Add(duration)
	sess.StartTime = start
	sess.Sport = typedef.SportCycling
	sess.TotalElapsedTime = uint32(duration.Milliseconds())
	sess.TotalTimerTime = uint32(duration.Milliseconds())
	sess.TotalDistance = uint32(totalDistM * 100)

	// Activity message.
	act := mesgdef.NewActivity(nil)
	act.Timestamp = start.Add(duration)
	act.LocalTimestamp = start.Add(duration)
	act.NumSessions = 1
	act.Type = typedef.ActivityManual

	messages := []proto.Message{
		fileId.ToMesg(nil),
	}
	messages = append(messages, records...)
	messages = append(messages,
		lap.ToMesg(nil),
		sess.ToMesg(nil),
		act.ToMesg(nil),
	)

	buf := new(bytes.Buffer)
	enc := encoder.New(buf)
	if err := enc.Encode(&proto.FIT{Messages: messages}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// degToSemicircles converts decimal degrees to FIT semicircle units.
func degToSemicircles(deg float64) int32 {
	return int32(deg * (1 << 31) / 180.0)
}
