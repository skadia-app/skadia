package importer

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

//go:fix inline
func ptr[T any](v T) *T { return new(v) }

// TestBuildEnrichFields_ElevationLoss stellt sicher, dass elevation_loss_m
// propagiert wird, wenn die bestehende Aktivität keinen Wert hat.
func TestBuildEnrichFields_ElevationLoss(t *testing.T) {
	existing := &model.Activity{} // alle Felder nil
	newData := &model.ActivityData{ElevationLossM: 312.5}

	fields := buildEnrichFields(existing, newData)

	if _, ok := fields["elevation_loss_m"]; !ok {
		t.Error("elevation_loss_m fehlt in fields, obwohl existing nil und newData > 0")
	}
	if fields["elevation_loss_m"] != 312.5 {
		t.Errorf("elevation_loss_m = %v, want 312.5", fields["elevation_loss_m"])
	}
}

// TestBuildEnrichFields_ElevationLoss_ExistingPresent stellt sicher, dass
// buildEnrichFields das Feld NICHT einschließt wenn existing bereits einen Wert hat.
// Nur fehlende Felder (nil/0) werden enriched.
func TestBuildEnrichFields_ElevationLoss_ExistingPresent(t *testing.T) {
	existing := &model.Activity{ElevationLossM: new(200.0)}
	newData := &model.ActivityData{ElevationLossM: 312.5}

	fields := buildEnrichFields(existing, newData)

	if _, ok := fields["elevation_loss_m"]; ok {
		t.Error("elevation_loss_m darf NICHT in fields sein wenn existing bereits einen Wert hat")
	}
}

// TestBuildEnrichFields_MaxMinElevation stellt sicher, dass max/min_elevation_m
// propagiert werden wenn fehlend.
func TestBuildEnrichFields_MaxMinElevation(t *testing.T) {
	existing := &model.Activity{}
	newData := &model.ActivityData{MaxElevationM: 2500.0, MinElevationM: 800.0}

	fields := buildEnrichFields(existing, newData)

	if _, ok := fields["max_elevation_m"]; !ok {
		t.Error("max_elevation_m fehlt in fields")
	}
	if _, ok := fields["min_elevation_m"]; !ok {
		t.Error("min_elevation_m fehlt in fields")
	}
}

// TestBuildReplaceFields_ElevationLoss stellt sicher, dass ReplaceActivity
// immer elevation_loss_m enthält.
func TestBuildReplaceFields_ElevationLoss(t *testing.T) {
	newData := &model.ActivityData{ElevationLossM: 450.0}

	fields := buildReplaceFields(nil, newData)

	if _, ok := fields["elevation_loss_m"]; !ok {
		t.Error("elevation_loss_m fehlt in buildReplaceFields")
	}
	if fields["elevation_loss_m"] != 450.0 {
		t.Errorf("elevation_loss_m = %v, want 450.0", fields["elevation_loss_m"])
	}
}

// TestBuildReplaceFields_MaxMinElevation stellt sicher, dass max/min_elevation_m
// in ReplaceActivity enthalten sind.
func TestBuildReplaceFields_MaxMinElevation(t *testing.T) {
	newData := &model.ActivityData{MaxElevationM: 3000.0, MinElevationM: 500.0}

	fields := buildReplaceFields(nil, newData)

	if _, ok := fields["max_elevation_m"]; !ok {
		t.Error("max_elevation_m fehlt in buildReplaceFields")
	}
	if _, ok := fields["min_elevation_m"]; !ok {
		t.Error("min_elevation_m fehlt in buildReplaceFields")
	}
}

func TestBuildReplaceFieldsIncludesSportTupleAndModifiers(t *testing.T) {
	newData := &model.ActivityData{
		SportType:  "cycling",
		SubType:    "mountain",
		IsElectric: true,
		IsIndoor:   true,
		IsVirtual:  true,
	}

	fields := buildReplaceFields(nil, newData)

	for _, key := range []string{"sport_type", "sub_type", "parsed_sport_type", "parsed_sub_type", "is_electric", "is_indoor", "is_virtual", "exclude_from_heatmap"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("%s fehlt in buildReplaceFields", key)
		}
	}
}

func TestBuildReplaceFieldsPreservesManualSportTupleAndModifiers(t *testing.T) {
	existing := &model.Activity{
		SportTypeIsManual: true,
	}
	newData := &model.ActivityData{
		SportType:  "cycling",
		SubType:    "mountain",
		IsElectric: true,
		IsIndoor:   true,
		IsVirtual:  true,
	}

	fields := buildReplaceFields(existing, newData)

	for _, key := range []string{"sport_type", "sub_type", "is_electric", "is_indoor", "is_virtual", "exclude_from_heatmap"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("%s must not be overwritten for manual sport decisions", key)
		}
	}
	if fields["parsed_sport_type"] != "cycling" || fields["parsed_sub_type"] != "mountain" {
		t.Fatalf("parsed tuple = %#v/%#v, want cycling/mountain import snapshot", fields["parsed_sport_type"], fields["parsed_sub_type"])
	}
}

func TestDuplicateSportUpdateFieldsRefinesTupleAndAdditiveModifiers(t *testing.T) {
	existing := &model.Activity{SportType: "cycling"}
	newData := &model.ActivityData{
		SportType:  "cycling",
		SubType:    "mountain",
		IsElectric: true,
		IsVirtual:  true,
	}

	fields := duplicateSportUpdateFields(existing, newData)

	if fields["sub_type"] != "mountain" {
		t.Fatalf("sub_type update = %#v, want mountain", fields["sub_type"])
	}
	if fields["is_electric"] != true {
		t.Fatalf("is_electric update = %#v, want true", fields["is_electric"])
	}
	if fields["is_virtual"] != true {
		t.Fatalf("is_virtual update = %#v, want true", fields["is_virtual"])
	}
	if fields["exclude_from_heatmap"] != true {
		t.Fatalf("exclude_from_heatmap update = %#v, want true", fields["exclude_from_heatmap"])
	}
}

func TestDuplicateSportUpdateFieldsPreservesManualSportDecision(t *testing.T) {
	existing := &model.Activity{SportType: "cycling", SportTypeIsManual: true}
	newData := &model.ActivityData{
		SportType:  "cycling",
		SubType:    "mountain",
		IsElectric: true,
		IsVirtual:  true,
	}

	fields := duplicateSportUpdateFields(existing, newData)

	if len(fields) != 0 {
		t.Fatalf("duplicate sport update fields = %#v, want none for manual sport decision", fields)
	}
}
