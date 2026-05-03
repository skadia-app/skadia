package garmin

import (
	"encoding/json"
	"testing"
)

func TestGarminGearDecodeExposesUnsupportedActivityTypeDefaults(t *testing.T) {
	src := []byte(`[
		{
			"gearDTOS": [
				{
					"gearPk": 123,
					"uuid": "gear-123",
					"customMakeModel": "Road Bike",
					"gearTypeName": "BIKE",
					"gearStatusName": "ACTIVE"
				}
			],
			"gearActivityTypeDTOS": [
				{"activityType": "cycling"}
			]
		}
	]`)

	var wrapper []garminGearFile
	if err := json.Unmarshal(src, &wrapper); err != nil {
		t.Fatalf("Unmarshal garmin gear fixture: %v", err)
	}
	if len(wrapper) != 1 || len(wrapper[0].GearDTOS) != 1 {
		t.Fatalf("decoded gearDTOS = %#v", wrapper)
	}
	if len(wrapper[0].GearActivityTypeDTOS) != 1 {
		t.Fatalf("GearActivityTypeDTOS = %#v, want one raw message", wrapper[0].GearActivityTypeDTOS)
	}
	if got := string(wrapper[0].GearActivityTypeDTOS[0]); got == "" {
		t.Fatal("GearActivityTypeDTOS raw message is empty")
	}

	create := gearCreateFromGarminDTO(wrapper[0].GearDTOS[0])
	if len(create.DefaultForSports) != 0 {
		t.Fatalf("DefaultForSports = %#v, want empty unsupported Garmin defaults", create.DefaultForSports)
	}
	if got := logUnsupportedGearActivityTypeDefaults(wrapper[0]); got != 1 {
		t.Fatalf("unsupported default count = %d, want 1", got)
	}
}
