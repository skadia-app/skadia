package handler

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

func TestApplyFusionSportSelectionPersistsTupleAndModifiers(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)

	isElectric := true
	isVirtual := true
	if err := applyFusionSportSelection(ctx, db.Pool, activityID, &FusionSportSelection{
		SportType:  "mountain_biking",
		IsElectric: &isElectric,
		IsVirtual:  &isVirtual,
	}); err != nil {
		t.Fatalf("applyFusionSportSelection: %v", err)
	}

	var sportType string
	var subType *string
	var gotElectric, gotVirtual, gotHeatmap bool
	if err := pool.QueryRow(ctx, `
		SELECT sport_type, sub_type, is_electric, is_virtual, exclude_from_heatmap
		FROM activities WHERE id = $1`, activityID,
	).Scan(&sportType, &subType, &gotElectric, &gotVirtual, &gotHeatmap); err != nil {
		t.Fatalf("select activity: %v", err)
	}
	if sportType != "cycling" || subType == nil || *subType != "mountain" {
		t.Fatalf("tuple = %s/%v, want cycling/mountain", sportType, subType)
	}
	if !gotElectric || !gotVirtual || !gotHeatmap {
		t.Fatalf("modifiers = electric %v virtual %v heatmap %v, want true/true/true", gotElectric, gotVirtual, gotHeatmap)
	}
}

func TestFusionRowToEntryCarriesIndoorVirtualModifiers(t *testing.T) {
	entry := fusionRowToEntry(database.FusionCandidateRow{
		ActivityIDA: "a",
		ActivityIDB: "b",
		IsIndoorA:   true,
		IsVirtualB:  true,
	})

	if !entry.IsIndoorA {
		t.Fatal("IsIndoorA was not carried to API entry")
	}
	if !entry.IsVirtualB {
		t.Fatal("IsVirtualB was not carried to API entry")
	}
}
