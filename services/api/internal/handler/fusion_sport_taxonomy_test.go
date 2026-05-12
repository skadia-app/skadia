package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

func TestApplyFusionSportSelectionPersistsTupleAndModifiers(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)

	isElectric := true
	isVirtual := true
	if err := applyFusionSportSelection(ctx, db.Pool, db, activityID, &FusionSportSelection{
		SportType:  "mountain_biking",
		IsElectric: &isElectric,
		IsVirtual:  &isVirtual,
	}, nil); err != nil {
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

func TestApplyFusionSportSelectionKeepsParsedSnapshotAndClearsVirtualHeatmap(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)
	if _, err := pool.Exec(ctx, `
		UPDATE activities
		SET parsed_sport_type = 'cycling',
		    parsed_sub_type = 'road',
		    is_virtual = true,
		    exclude_from_heatmap = true
		WHERE id = $1`, activityID); err != nil {
		t.Fatalf("seed activity: %v", err)
	}

	isVirtual := false
	if err := applyFusionSportSelection(ctx, db.Pool, db, activityID, &FusionSportSelection{
		SportType: "cycling",
		SubType:   "mountain",
		IsVirtual: &isVirtual,
	}, nil); err != nil {
		t.Fatalf("applyFusionSportSelection: %v", err)
	}

	var parsedSport string
	var parsedSub *string
	var gotVirtual, gotHeatmap bool
	if err := pool.QueryRow(ctx, `
		SELECT parsed_sport_type, parsed_sub_type, is_virtual, exclude_from_heatmap
		FROM activities WHERE id = $1`, activityID,
	).Scan(&parsedSport, &parsedSub, &gotVirtual, &gotHeatmap); err != nil {
		t.Fatalf("select activity: %v", err)
	}
	if parsedSport != "cycling" || parsedSub == nil || *parsedSub != "road" {
		t.Fatalf("parsed tuple = %s/%v, want unchanged cycling/road", parsedSport, parsedSub)
	}
	if gotVirtual || gotHeatmap {
		t.Fatalf("virtual=%v heatmap=%v, want false/false", gotVirtual, gotHeatmap)
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

func TestLegacyFusionCandidateJSONIncludesSportTupleAndModifiers(t *testing.T) {
	subType := "mountain"
	payload, err := json.Marshal(FusionCandidate{
		ID:         "activity-1",
		SportType:  "cycling",
		SubType:    &subType,
		IsElectric: true,
		IsIndoor:   false,
		IsVirtual:  true,
	})
	if err != nil {
		t.Fatalf("marshal fusion candidate: %v", err)
	}

	for _, want := range []string{
		`"sport_type":"cycling"`,
		`"sub_type":"mountain"`,
		`"is_electric":true`,
		`"is_indoor":false`,
		`"is_virtual":true`,
	} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("payload %s missing %s", payload, want)
		}
	}
}

func TestGetFusionCandidatesUsesAutoTitleForEmptySourceUnlessManual(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	targetID := insertMinimalActivity(t, db, userID)
	generatedID := insertMinimalActivity(t, db, userID)
	manualID := insertMinimalActivity(t, db, userID)

	if _, err := pool.Exec(ctx, `
		UPDATE activities
		SET title = '   ', title_auto = 'Generated title', title_is_manual = false
		WHERE id = $1`, generatedID); err != nil {
		t.Fatalf("seed generated candidate: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE activities
		SET title = 'Manual title', title_auto = 'Generated manual', title_is_manual = true
		WHERE id = $1`, manualID); err != nil {
		t.Fatalf("seed manual candidate: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/activities/"+targetID+"/fusion-candidates", nil)
	req = injectChiURLParam(req, "id", targetID)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rr := httptest.NewRecorder()

	(&Handler{DB: db}).GetFusionCandidates(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GetFusionCandidates status = %d body = %s", rr.Code, rr.Body.String())
	}
	var got []FusionCandidate
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	titles := map[string]string{}
	for _, candidate := range got {
		titles[candidate.ID] = candidate.Title
	}
	if titles[generatedID] != "Generated title" {
		t.Fatalf("generated candidate title = %q, want Generated title (all titles: %#v)", titles[generatedID], titles)
	}
	if titles[manualID] != "Manual title" {
		t.Fatalf("manual candidate title = %q, want Manual title (all titles: %#v)", titles[manualID], titles)
	}
}
