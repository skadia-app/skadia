package fitimport

import (
	"testing"

	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
)

func TestExtractSensorsResolvesNonGarminProductIDs(t *testing.T) {
	sensors := ExtractSensors([]*mesgdef.DeviceInfo{
		{
			DeviceIndex:     1,
			SourceType:      typedef.SourceTypeAntplus,
			DeviceType:      uint8(typedef.AntplusDeviceTypeHeartRate),
			Manufacturer:    typedef.ManufacturerPolarElectro,
			Product:         2,
			AntDeviceNumber: 42,
		},
	})

	if len(sensors) != 1 {
		t.Fatalf("ExtractSensors returned %d sensors, want 1", len(sensors))
	}
	if sensors[0].DeviceUpsert == nil {
		t.Fatal("sensor DeviceUpsert is nil")
	}
	if got := sensors[0].DeviceUpsert.ProductName; got != "Polar H10" {
		t.Fatalf("DeviceUpsert.ProductName = %q, want %q", got, "Polar H10")
	}
	if got := sensors[0].DeviceUpsert.ProductID; got != 2 {
		t.Fatalf("DeviceUpsert.ProductID = %d, want 2", got)
	}
	if got := sensors[0].DeviceUpsert.Type; got != "sensor" {
		t.Fatalf("DeviceUpsert.Type = %q, want sensor", got)
	}
}
