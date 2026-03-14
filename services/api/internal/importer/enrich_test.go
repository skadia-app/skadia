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
// buildEnrichFields das Feld auch dann einschließt wenn existing einen Wert hat.
// Der Schutz vor Überschreiben erfolgt via SQL COALESCE in UpdateEnrichmentFieldsSafe —
// nicht mehr auf Go-Ebene.
func TestBuildEnrichFields_ElevationLoss_ExistingPresent(t *testing.T) {
	existing := &model.Activity{ElevationLossM: new(200.0)}
	newData := &model.ActivityData{ElevationLossM: 312.5}

	fields := buildEnrichFields(existing, newData)

	// Field must be present — COALESCE in the DB layer keeps the existing 200.0.
	if _, ok := fields["elevation_loss_m"]; !ok {
		t.Error("elevation_loss_m soll in fields enthalten sein (COALESCE-Schutz ist DB-seitig)")
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

	fields := buildReplaceFields(newData)

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

	fields := buildReplaceFields(newData)

	if _, ok := fields["max_elevation_m"]; !ok {
		t.Error("max_elevation_m fehlt in buildReplaceFields")
	}
	if _, ok := fields["min_elevation_m"]; !ok {
		t.Error("min_elevation_m fehlt in buildReplaceFields")
	}
}
