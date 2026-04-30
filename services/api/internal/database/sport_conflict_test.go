package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestResolveSportConflictAppliesModifierOnlyResolution(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()

	act, err := db.InsertActivity(ctx, userID, &model.ActivityData{
		SportType:            "cycling",
		StartedAt:            time.Now().UTC(),
		SportConflictPending: true,
		IsElectric:           false,
		IsIndoor:             false,
		IsVirtual:            true,
	})
	if err != nil {
		t.Fatalf("InsertActivity: %v", err)
	}

	isElectric := true
	isVirtual := false
	affected, err := db.ResolveSportConflict(ctx, act.ID, userID, &SportConflictResolution{
		IsElectric: &isElectric,
		IsVirtual:  &isVirtual,
	})
	if err != nil {
		t.Fatalf("ResolveSportConflict: %v", err)
	}
	if !affected {
		t.Fatal("ResolveSportConflict affected no rows")
	}

	reloaded, err := db.GetActivity(ctx, act.ID)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if reloaded.SportConflictPending {
		t.Fatal("sport conflict still pending")
	}
	if !reloaded.SportTypeIsManual || !reloaded.IsElectric || reloaded.IsVirtual || reloaded.ExcludeFromHeatmap {
		t.Fatalf("manual/electric/virtual/heatmap = %v/%v/%v/%v, want true/true/false/false",
			reloaded.SportTypeIsManual, reloaded.IsElectric, reloaded.IsVirtual, reloaded.ExcludeFromHeatmap)
	}
}
