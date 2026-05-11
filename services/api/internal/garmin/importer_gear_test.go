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
	if create.ApplicableSportPredicates == nil {
		t.Fatal("ApplicableSportPredicates is nil, want three predicates")
	}
	preds := *create.ApplicableSportPredicates
	if len(preds) != 3 {
		t.Fatalf("ApplicableSportPredicates = %#v, want three predicates", preds)
	}
	if preds[1].SportType != "cycling" ||
		preds[1].SubType == nil ||
		*preds[1].SubType != "mountain" {
		t.Fatalf("mountain predicate = %#v", preds[1])
	}
}

func TestParseGearActivityAssignmentUsesActivityAndGearIdentifiers(t *testing.T) {
	activityID, gearIDs := parseGearActivityAssignment(json.RawMessage(`{
		"activityId": 987654321,
		"gearPk": 123,
		"gearUuid": "gear-uuid-123"
	}`))
	if activityID != 987654321 {
		t.Fatalf("activityID = %d, want 987654321", activityID)
	}
	if len(gearIDs) != 2 || gearIDs[0] != "gear-uuid-123" || gearIDs[1] != "pk:123" {
		t.Fatalf("gearIDs = %#v, want uuid and pk fallback", gearIDs)
	}
}
