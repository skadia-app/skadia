package garmin

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/kit/datetime"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"
)

var updateGarminHealthFixtures = flag.Bool("update-garmin-health-fixtures", false, "rewrite synthetic Garmin health FIT fixtures")

func TestSyntheticGarminHealthFixturesAreCurrent(t *testing.T) {
	for _, fixture := range syntheticGarminHealthFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			want, err := encodeSyntheticFIT(fixture.messages)
			if err != nil {
				t.Fatalf("encode synthetic FIT: %v", err)
			}

			path := filepath.Join("testdata", fixture.name)
			if *updateGarminHealthFixtures {
				if err := os.WriteFile(path, want, 0644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
				return
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%s is stale; run go test ./internal/garmin -run TestSyntheticGarminHealthFixturesAreCurrent -update-garmin-health-fixtures", path)
			}
		})
	}
}

type syntheticFixture struct {
	name     string
	messages []proto.Message
}

func syntheticGarminHealthFixtures() []syntheticFixture {
	return []syntheticFixture{
		{name: "sample_type32_monitoring_b.fit", messages: syntheticMonitoringBFIT(false)},
		{name: "sample_type32_monitoring_b_large.fit", messages: syntheticMonitoringBFIT(true)},
		{name: "sample_type44_hrv_night.fit", messages: syntheticHRVNightFIT(false)},
		{name: "sample_type44_hrv_night_large.fit", messages: syntheticHRVNightFIT(true)},
		{name: "sample_type49_sleep.fit", messages: syntheticSleepFIT()},
		{name: "sample_type58_spo2.fit", messages: syntheticSpO2FIT()},
		{name: "sample_type68_monitor_main.fit", messages: syntheticMonitoringMainFIT()},
	}
}

func encodeSyntheticFIT(messages []proto.Message) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := encoder.New(buf)
	if err := enc.Encode(&proto.FIT{Messages: messages}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func syntheticFileID(fileType typedef.File, ts time.Time) proto.Message {
	fileID := mesgdef.NewFileId(nil)
	fileID.Type = fileType
	fileID.Manufacturer = typedef.Manufacturer(syntheticFixtureManufacturer)
	fileID.Product = syntheticFixtureProduct
	fileID.SerialNumber = syntheticFixtureSerial
	fileID.TimeCreated = ts
	return fileID.ToMesg(nil)
}

func syntheticDeviceInfo(ts time.Time) proto.Message {
	device := mesgdef.NewDeviceInfo(nil)
	device.Timestamp = ts
	device.DeviceIndex = 0
	device.Manufacturer = typedef.Manufacturer(syntheticFixtureManufacturer)
	device.Product = syntheticFixtureProduct
	device.SerialNumber = syntheticFixtureSerial
	device.ProductName = "Synthetic Health Fixture"
	return device.ToMesg(nil)
}

func syntheticMonitoringBFIT(expanded bool) []proto.Message {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	messages := []proto.Message{
		syntheticFileID(typedef.FileMonitoringB, start),
		syntheticDeviceInfo(start),
	}
	samples := 2
	if expanded {
		samples = 8
	}
	for i := 0; i < samples; i++ {
		ts := start.Add(time.Duration(i) * time.Minute)
		mon := mesgdef.NewMonitoring(nil)
		mon.Timestamp = ts
		mon.DeviceIndex = 0
		mon.HeartRate = uint8(60 + i)
		messages = append(messages, mon.ToMesg(nil))

		stress := mesgdef.NewStressLevel(nil)
		stress.StressLevelTime = ts
		stress.StressLevelValue = int16(20 + i)
		stress.UnknownFields = []proto.Field{
			syntheticField(2, basetype.Sint8, proto.Int8(int8(i-1))),
			syntheticField(3, basetype.Sint8, proto.Int8(int8(50+i))),
		}
		messages = append(messages, stress.ToMesg(nil))

		resp := mesgdef.NewRespirationRate(nil)
		resp.Timestamp = ts
		resp.RespirationRate = int16(1500 + i*10)
		messages = append(messages, resp.ToMesg(nil))
	}
	return messages
}

func syntheticHRVNightFIT(includeReadiness bool) []proto.Message {
	start := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	messages := []proto.Message{
		syntheticFileID(typedef.File(44), start),
		syntheticDeviceInfo(start),
		syntheticMessage(281,
			syntheticField(253, basetype.Uint32, proto.Uint32(datetime.ToUint32(start))),
			syntheticField(0, basetype.Uint16, proto.Uint16(55)),
		),
	}
	if includeReadiness {
		messages = append(messages, syntheticMessage(369,
			syntheticField(253, basetype.Uint32, proto.Uint32(datetime.ToUint32(start.Add(5*time.Minute)))),
			syntheticField(0, basetype.Sint16, proto.Int16(72)),
			syntheticField(5, basetype.Sint16, proto.Int16(81)),
			syntheticField(8, basetype.Sint16, proto.Int16(95)),
			syntheticField(10, basetype.Sint16, proto.Int16(102)),
			syntheticField(12, basetype.Sint16, proto.Int16(88)),
			syntheticField(14, basetype.Sint16, proto.Int16(76)),
			syntheticField(16, basetype.Sint16, proto.Int16(91)),
		))
	}
	return messages
}

func syntheticSleepFIT() []proto.Message {
	start := time.Date(2026, 1, 2, 22, 0, 0, 0, time.UTC)
	end := start.Add(7 * time.Hour)
	messages := []proto.Message{
		syntheticFileID(typedef.File(49), start),
		syntheticDeviceInfo(start),
		syntheticSleepEvent(start, 0),
	}
	levels := []typedef.SleepLevel{
		typedef.SleepLevelLight,
		typedef.SleepLevelDeep,
		typedef.SleepLevelRem,
		typedef.SleepLevelAwake,
	}
	for i, level := range levels {
		sl := mesgdef.NewSleepLevel(nil)
		sl.Timestamp = start.Add(time.Duration(i+1) * time.Hour)
		sl.SleepLevel = level
		messages = append(messages, sl.ToMesg(nil))
	}
	messages = append(messages, syntheticSleepEvent(end, 1))
	return messages
}

func syntheticSleepEvent(ts time.Time, eventType uint8) proto.Message {
	return syntheticMessage(typedef.MesgNumEvent,
		syntheticField(253, basetype.Uint32, proto.Uint32(datetime.ToUint32(ts))),
		syntheticField(0, basetype.Enum, proto.Uint8(74)),
		syntheticField(1, basetype.Enum, proto.Uint8(eventType)),
	)
}

func syntheticSpO2FIT() []proto.Message {
	start := time.Date(2026, 1, 3, 8, 0, 0, 0, time.UTC)
	spo2 := mesgdef.NewSpo2Data(nil)
	spo2.Timestamp = start
	spo2.ReadingSpo2 = 97
	spo2.ReadingConfidence = 90
	spo2.Mode = typedef.Spo2MeasurementTypeSpotCheck

	hsa := mesgdef.NewHsaSpo2Data(nil)
	hsa.Timestamp = start.Add(time.Minute)
	hsa.ProcessingInterval = 30
	hsa.ReadingSpo2 = []uint8{96, 240, 95}
	hsa.Confidence = []uint8{90, 0, 88}

	return []proto.Message{
		syntheticFileID(typedef.File(58), start),
		syntheticDeviceInfo(start),
		spo2.ToMesg(nil),
		hsa.ToMesg(nil),
		syntheticMessage(318,
			syntheticField(253, basetype.Uint32, proto.Uint32(datetime.ToUint32(start.Add(2*time.Minute)))),
			syntheticField(0, basetype.Uint8, proto.Uint8(1)),
		),
	}
}

func syntheticMonitoringMainFIT() []proto.Message {
	start := time.Date(2026, 1, 3, 21, 0, 0, 0, time.UTC)
	return []proto.Message{
		syntheticFileID(typedef.File(68), start),
		syntheticDeviceInfo(start),
		syntheticMessage(371,
			syntheticField(253, basetype.Uint32, proto.Uint32(datetime.ToUint32(start))),
			syntheticField(0, basetype.Uint8, proto.Uint8(42)),
		),
	}
}

func syntheticMessage(num typedef.MesgNum, fields ...proto.Field) proto.Message {
	return proto.Message{Num: num, Fields: fields}
}

func syntheticField(num byte, base basetype.BaseType, value proto.Value) proto.Field {
	return proto.Field{
		FieldBase: &proto.FieldBase{
			Name:     "synthetic",
			Num:      num,
			BaseType: base,
			Scale:    1,
		},
		Value: value,
	}
}
