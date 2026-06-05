package garmin

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/kit/datetime"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/profile/untyped/fieldnum"
	"github.com/muktihari/fit/profile/untyped/mesgnum"
	"github.com/muktihari/fit/proto"
)

const (
	syntheticFixtureManufacturer = uint16(typedef.ManufacturerDevelopment)
	syntheticFixtureProduct      = uint16(1)
	syntheticFixtureSerial       = uint32(900000001)
)

var syntheticFixtureTimeMin = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
var syntheticFixtureTimeMax = time.Date(2026, 1, 3, 23, 59, 59, 0, time.UTC)

func TestGarminFITFixturesAreSynthetic(t *testing.T) {
	paths, err := filepath.Glob("testdata/*.fit")
	if err != nil {
		t.Fatalf("glob FIT fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("expected Garmin FIT fixtures")
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			dec := decoder.New(bytes.NewReader(raw), decoder.WithIgnoreChecksum())
			fit, err := dec.Decode()
			if err != nil {
				t.Fatalf("decode fixture: %v", err)
			}

			var sawDeviceMetadata bool
			for i := range fit.Messages {
				msg := &fit.Messages[i]
				assertSyntheticTimestamp(t, msg)
				assertNoPosition(t, msg)

				switch msg.Num {
				case mesgnum.FileId:
					sawDeviceMetadata = true
					assertSyntheticDeviceField(t, path, "file_id serial_number",
						msg.FieldValueByNum(fieldnum.FileIdSerialNumber), basetype.Uint32)
					assertSyntheticDeviceField(t, path, "file_id manufacturer",
						msg.FieldValueByNum(fieldnum.FileIdManufacturer), basetype.Uint16)
					assertSyntheticDeviceField(t, path, "file_id product",
						msg.FieldValueByNum(fieldnum.FileIdProduct), basetype.Uint16)
				case mesgnum.DeviceInfo:
					sawDeviceMetadata = true
					assertSyntheticDeviceField(t, path, "device_info serial_number",
						msg.FieldValueByNum(fieldnum.DeviceInfoSerialNumber), basetype.Uint32)
					assertSyntheticDeviceField(t, path, "device_info manufacturer",
						msg.FieldValueByNum(fieldnum.DeviceInfoManufacturer), basetype.Uint16)
					assertSyntheticDeviceField(t, path, "device_info product",
						msg.FieldValueByNum(fieldnum.DeviceInfoProduct), basetype.Uint16)
				}
			}
			if !sawDeviceMetadata {
				t.Fatalf("%s: expected synthetic file_id or device_info metadata", path)
			}
		})
	}
}

func assertSyntheticDeviceField(t *testing.T, path, label string, value proto.Value, base basetype.BaseType) {
	t.Helper()
	if !value.Valid(base) {
		return
	}

	switch label {
	case "file_id serial_number", "device_info serial_number":
		if got := value.Uint32(); got != syntheticFixtureSerial {
			t.Fatalf("%s: %s = %d, want synthetic serial %d", path, label, got, syntheticFixtureSerial)
		}
	case "file_id manufacturer", "device_info manufacturer":
		if got := value.Uint16(); got != syntheticFixtureManufacturer {
			t.Fatalf("%s: %s = %d, want synthetic manufacturer %d", path, label, got, syntheticFixtureManufacturer)
		}
	case "file_id product", "device_info product":
		if got := value.Uint16(); got != syntheticFixtureProduct {
			t.Fatalf("%s: %s = %d, want synthetic product %d", path, label, got, syntheticFixtureProduct)
		}
	}
}

func assertSyntheticTimestamp(t *testing.T, msg *proto.Message) {
	t.Helper()
	if v := msg.FieldValueByNum(proto.FieldNumTimestamp); v.Valid(basetype.Uint32) {
		ts := datetime.ToTime(v.Uint32())
		if ts.Before(syntheticFixtureTimeMin) || ts.After(syntheticFixtureTimeMax) {
			t.Fatalf("message %d timestamp %s outside synthetic fixture range", msg.Num, ts.Format(time.RFC3339))
		}
	}
}

func assertNoPosition(t *testing.T, msg *proto.Message) {
	t.Helper()
	var nums []byte
	switch msg.Num {
	case mesgnum.Record:
		nums = []byte{fieldnum.RecordPositionLat, fieldnum.RecordPositionLong}
	case mesgnum.Lap:
		nums = []byte{
			fieldnum.LapStartPositionLat,
			fieldnum.LapStartPositionLong,
			fieldnum.LapEndPositionLat,
			fieldnum.LapEndPositionLong,
		}
	case mesgnum.Session:
		nums = []byte{
			fieldnum.SessionStartPositionLat,
			fieldnum.SessionStartPositionLong,
			fieldnum.SessionEndPositionLat,
			fieldnum.SessionEndPositionLong,
		}
	default:
		return
	}
	for _, num := range nums {
		if v := msg.FieldValueByNum(num); v.Valid(basetype.Sint32) {
			t.Fatalf("message %d contains position field %d", msg.Num, num)
		}
	}
}
