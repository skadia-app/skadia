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
					{"gearPk": 123, "activityType": {"typeKey": "mountain_biking"}},
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
	if len(wrapper[0].GearActivityTypeDTOS) != 4 {
		t.Fatalf("GearActivityTypeDTOS = %#v, want four raw messages", wrapper[0].GearActivityTypeDTOS)
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
	if len(create.DefaultSportPredicates) != 3 {
		t.Fatalf("DefaultSportPredicates = %#v, want three predicates", create.DefaultSportPredicates)
	}
	if create.DefaultSportPredicates[1].SportType != "cycling" ||
		create.DefaultSportPredicates[1].SubType == nil ||
		*create.DefaultSportPredicates[1].SubType != "mountain" {
		t.Fatalf("mountain predicate = %#v", create.DefaultSportPredicates[1])
	}
}
