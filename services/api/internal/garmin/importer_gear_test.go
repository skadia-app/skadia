package garmin

import (
	"encoding/json"
	"testing"
)

func TestGarminGearDecodeAppliesActivityTypeDefaults(t *testing.T) {
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
				{"gearPk": 123, "activityType": {"typeKey": "cycling"}},
				{"gearPk": 123, "activityType": {"typeKey": "running"}},
				{"gearPk": 123, "activityType": {"typeKey": "definitely_unknown"}}
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
	if len(wrapper[0].GearActivityTypeDTOS) != 3 {
		t.Fatalf("GearActivityTypeDTOS = %#v, want three raw messages", wrapper[0].GearActivityTypeDTOS)
	}
	if got := string(wrapper[0].GearActivityTypeDTOS[0]); got == "" {
		t.Fatal("GearActivityTypeDTOS raw message is empty")
	}

	defaults := garminDefaultSportsByGear(wrapper[0])
	create := gearCreateFromGarminDTO(wrapper[0].GearDTOS[0], defaults)
	want := []string{"cycling", "running"}
	if len(create.DefaultForSports) != len(want) {
		t.Fatalf("DefaultForSports = %#v, want %#v", create.DefaultForSports, want)
	}
	for i := range want {
		if create.DefaultForSports[i] != want[i] {
			t.Fatalf("DefaultForSports = %#v, want %#v", create.DefaultForSports, want)
		}
	}
}
