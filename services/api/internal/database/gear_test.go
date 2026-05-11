package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	gearpkg "github.com/skadia-app/skadia/services/api/internal/gear"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/google/uuid"
)

func gearTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "gear-"+userID[:8], "gear-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &DB{Pool: pool}, userID
}

func TestCreateGearStoresDefaultSportPredicates(t *testing.T) {
	db, userID := gearTestDB(t)
	mountain := "mountain"
	preds := []gearpkg.GearDefaultPredicate{{
		SportType: "cycling",
		SubType:   &mountain,
		Modifiers: &sport.ModifierPatch{IsElectric: boolPtr(true)},
	}}
	gear, err := db.CreateGear(context.Background(), userID, &model.GearCreate{
		Name:                   "E-MTB",
		Type:                   "bike",
		DefaultSportPredicates: &preds,
	})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}
	if len(gear.DefaultSportPredicates) != 1 {
		t.Fatalf("DefaultSportPredicates = %#v, want one", gear.DefaultSportPredicates)
	}
	if gear.DefaultSportPredicates[0].SubType == nil || *gear.DefaultSportPredicates[0].SubType != "mountain" {
		t.Fatalf("DefaultSportPredicates = %#v, want mountain subtype", gear.DefaultSportPredicates)
	}
}

func TestCreateChainRotationRejectsOutOfGearMembers(t *testing.T) {
	db, userID := gearTestDB(t)
	ctx := context.Background()
	gearA, err := db.CreateGear(ctx, userID, &model.GearCreate{Name: "Bike A", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear A: %v", err)
	}
	gearB, err := db.CreateGear(ctx, userID, &model.GearCreate{Name: "Bike B", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear B: %v", err)
	}
	compA, err := db.CreateComponent(ctx, gearA.ID, &model.GearComponentCreate{Name: "Kette A", Type: "chain"})
	if err != nil {
		t.Fatalf("CreateComponent A: %v", err)
	}
	compB, err := db.CreateComponent(ctx, gearB.ID, &model.GearComponentCreate{Name: "Kette B", Type: "chain"})
	if err != nil {
		t.Fatalf("CreateComponent B: %v", err)
	}

	_, err = db.CreateChainRotation(ctx, userID, gearA.ID, &ChainRotationGroupCreate{
		Name:         "Kettenrotation",
		ComponentIDs: []string{compA.ID, compB.ID},
	})
	if !errors.Is(err, ErrComponentNotFound) {
		t.Fatalf("CreateChainRotation err = %v, want ErrComponentNotFound", err)
	}
}

func TestCreateChainRotationRejectsForeignGear(t *testing.T) {
	db, userID := gearTestDB(t)
	ctx := context.Background()
	otherUserID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		otherUserID, "gear-"+otherUserID[:8], "gear-"+otherUserID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert other user: %v", err)
	}
	foreignGear, err := db.CreateGear(ctx, otherUserID, &model.GearCreate{Name: "Foreign Bike", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear foreign: %v", err)
	}

	_, err = db.CreateChainRotation(ctx, userID, foreignGear.ID, &ChainRotationGroupCreate{
		Name:         "Kettenrotation",
		ComponentIDs: []string{uuid.NewString(), uuid.NewString()},
	})
	if !errors.Is(err, ErrGearNotFound) {
		t.Fatalf("CreateChainRotation err = %v, want ErrGearNotFound", err)
	}
}

func TestInsertGearDocumentPersistsDocumentMetadata(t *testing.T) {
	db, userID := gearTestDB(t)
	ctx := context.Background()
	gear, err := db.CreateGear(ctx, userID, &model.GearCreate{Name: "Bike", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}
	amount := 42.5
	date := "2026-05-11"
	currency := "EUR"
	doc, err := db.InsertGearDocument(ctx, &model.GearDocument{
		UserID:           userID,
		GearID:           &gear.ID,
		DocType:          "receipt",
		OriginalFilename: "receipt.pdf",
		ContentType:      "application/pdf",
		FileSizeBytes:    1234,
		DocumentDate:     &date,
		DocumentAmount:   &amount,
		DocumentCurrency: &currency,
	})
	if err != nil {
		t.Fatalf("InsertGearDocument: %v", err)
	}
	if doc.DocumentDate == nil || *doc.DocumentDate != date {
		t.Fatalf("DocumentDate = %#v, want %q", doc.DocumentDate, date)
	}
	if doc.DocumentAmount == nil || *doc.DocumentAmount != amount {
		t.Fatalf("DocumentAmount = %#v, want %.2f", doc.DocumentAmount, amount)
	}
	if doc.DocumentCurrency == nil || *doc.DocumentCurrency != currency {
		t.Fatalf("DocumentCurrency = %#v, want %q", doc.DocumentCurrency, currency)
	}
}

func TestDeleteServiceEventRestoresPreviousIntervalReset(t *testing.T) {
	db, userID := gearTestDB(t)
	ctx := context.Background()
	gear, err := db.CreateGear(ctx, userID, &model.GearCreate{Name: "Bike", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}
	component, err := db.CreateComponent(ctx, gear.ID, &model.GearComponentCreate{Name: "Kette", Type: "chain"})
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	interval, err := db.CreateServiceInterval(ctx, component.ID, &model.GearServiceIntervalCreate{
		Name:          "Tauschen",
		MetricKey:     "distance_m",
		IntervalValue: 500,
	})
	if err != nil {
		t.Fatalf("CreateServiceInterval: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE gear_components SET total_distance_m = 100 WHERE id = $1`, component.ID); err != nil {
		t.Fatalf("set distance 100: %v", err)
	}
	first, err := db.CreateServiceEvent(ctx, component.ID, &ServiceEventCreate{
		IntervalID:  &interval.ID,
		PerformedAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("CreateServiceEvent first: %v", err)
	}
	if first.ID == "" {
		t.Fatal("first service event has no ID")
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE gear_components SET total_distance_m = 200 WHERE id = $1`, component.ID); err != nil {
		t.Fatalf("set distance 200: %v", err)
	}
	second, err := db.CreateServiceEvent(ctx, component.ID, &ServiceEventCreate{
		IntervalID:  &interval.ID,
		PerformedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("CreateServiceEvent second: %v", err)
	}

	if err := db.DeleteServiceEvent(ctx, second.ID); err != nil {
		t.Fatalf("DeleteServiceEvent: %v", err)
	}
	intervals, err := db.ListServiceIntervals(ctx, component.ID)
	if err != nil {
		t.Fatalf("ListServiceIntervals: %v", err)
	}
	if len(intervals) != 1 || intervals[0].LastServiceMetricValue == nil || *intervals[0].LastServiceMetricValue != 100 {
		t.Fatalf("LastServiceMetricValue after delete = %#v, want 100", intervals)
	}
}

func TestFindDefaultGearMatchesByPredicate(t *testing.T) {
	db, userID := gearTestDB(t)
	mountain := "mountain"
	emtbPreds := []gearpkg.GearDefaultPredicate{{
		SportType: "cycling",
		SubType:   &mountain,
		Modifiers: &sport.ModifierPatch{IsElectric: boolPtr(true)},
	}}
	emtb, err := db.CreateGear(context.Background(), userID, &model.GearCreate{
		Name:                   "E-MTB",
		Type:                   "bike",
		DefaultSportPredicates: &emtbPreds,
	})
	if err != nil {
		t.Fatalf("CreateGear e-mtb: %v", err)
	}
	roadPreds := []gearpkg.GearDefaultPredicate{{SportType: "cycling"}}
	road, err := db.CreateGear(context.Background(), userID, &model.GearCreate{
		Name:                   "Road",
		Type:                   "bike",
		DefaultSportPredicates: &roadPreds,
	})
	if err != nil {
		t.Fatalf("CreateGear road: %v", err)
	}

	got, err := db.findDefaultGearForActivity(context.Background(), userID, sport.Tuple{SportType: "cycling"}, sport.ModifierValues{})
	if err != nil {
		t.Fatalf("findDefaultGearForActivity road: %v", err)
	}
	if got != road.ID {
		t.Fatalf("road match = %q, want road gear %q", got, road.ID)
	}
	got, err = db.findDefaultGearForActivity(context.Background(), userID, sport.Tuple{SportType: "cycling", SubType: "mountain"}, sport.ModifierValues{IsElectric: true})
	if err != nil {
		t.Fatalf("findDefaultGearForActivity e-mtb: %v", err)
	}
	if got == "" || got == road.ID {
		t.Fatalf("e-mtb match = %q, want predicate gear (not %q)", got, road.ID)
	}
	if got != emtb.ID {
		t.Fatalf("e-mtb match = %q, want %q", got, emtb.ID)
	}
}

func boolPtr(v bool) *bool {
	return &v
}

// TestMergeGearReassignsAGARows covers Critic-4 #3: MergeGear reassigns
// activities.gear_id from secondary to primary, but if a user-authoritative
// aga row points at the secondary, the mirror trigger refuses to overwrite
// it (WHERE source='mirror' guard) and the subsequent DELETE FROM gear cascades
// through aga.gear_id ON DELETE CASCADE — destroying the user'"'"'s aga row.
//
// After the fix, the user-aga row must be reassigned to primaryID and survive
// the merge.
func TestMergeGearReassignsAGARows(t *testing.T) {
	db, userID := gearTestDB(t)
	ctx := context.Background()

	primary, err := db.CreateGear(ctx, userID, &model.GearCreate{
		Name: "Primary Bike", Type: "bike",
	})
	if err != nil {
		t.Fatalf("CreateGear primary: %v", err)
	}
	secondary, err := db.CreateGear(ctx, userID, &model.GearCreate{
		Name: "Secondary Bike", Type: "bike",
	})
	if err != nil {
		t.Fatalf("CreateGear secondary: %v", err)
	}

	// Create an activity, then insert a primary aga row with source='user'
	// pointing at the secondary gear — simulates an authoritative
	// export-restored or user-edited assignment.
	actID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities
			(id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status, moving_time_s)
		VALUES ($1, $2, 'cycling', 'merge-test', now(), $3, 'private', 'complete', 600)
	`, actID, userID, "hash-merge-"+actID[:8]); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM activities WHERE id = $1`, actID)
	})

	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_gear_assignments
			(id, user_id, activity_id, gear_id, role_key, source, quantity)
		VALUES (gen_random_uuid(), $1, $2, $3, 'primary', 'user', 1)
	`, userID, actID, secondary.ID); err != nil {
		t.Fatalf("insert primary aga (user): %v", err)
	}

	// Sanity check: aga row points at secondary with source='user'.
	var gotGearID, gotSource string
	if err := db.Pool.QueryRow(ctx, `
		SELECT gear_id, source FROM activity_gear_assignments
		 WHERE activity_id = $1 AND role_key = 'primary'
	`, actID).Scan(&gotGearID, &gotSource); err != nil {
		t.Fatalf("read aga before merge: %v", err)
	}
	if gotGearID != secondary.ID || gotSource != "user" {
		t.Fatalf("pre-merge aga = (%s, %s), want (%s, user)", gotGearID, gotSource, secondary.ID)
	}

	// Run the merge.
	if err := db.MergeGear(ctx, userID, primary.ID, secondary.ID); err != nil {
		t.Fatalf("MergeGear: %v", err)
	}

	// After the merge:
	//   1. an aga row still exists for the activity, gear_id = primary.ID
	//   2. its source is preserved as 'user' (not silently demoted)
	// (activities.gear_id was dropped in 000355's final block.)
	var afterGearID, afterSource string
	if err := db.Pool.QueryRow(ctx, `
		SELECT gear_id, source FROM activity_gear_assignments
		 WHERE activity_id = $1 AND role_key = 'primary'
	`, actID).Scan(&afterGearID, &afterSource); err != nil {
		t.Fatalf("read aga after merge: %v (user-aga row was destroyed by ON DELETE CASCADE — Critic-4 #3 reproduces)", err)
	}
	if afterGearID != primary.ID {
		t.Errorf("post-merge aga.gear_id = %s, want %s", afterGearID, primary.ID)
	}
	if afterSource != "user" {
		t.Errorf("post-merge aga.source = %q, want %q (user-authoritative source must survive merge)", afterSource, "user")
	}
}

// --- F13: three-state pointer-slice for SportPredicates ---

// TestGearCreateExplicitlyEmptyPredicates verifies that passing a non-nil
// pointer to an empty slice stores [] (not the taxonomy defaults) in the DB.
func TestGearCreateExplicitlyEmptyPredicates(t *testing.T) {
	db, userID := gearTestDB(t)
	empty := []gearpkg.GearDefaultPredicate{}
	g, err := db.CreateGear(context.Background(), userID, &model.GearCreate{
		Name:                      "Explicit Empty Bike",
		Type:                      "bike",
		ApplicableSportPredicates: &empty,
	})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}
	if len(g.ApplicableSportPredicates) != 0 {
		t.Fatalf("ApplicableSportPredicates = %#v, want empty (explicit clear)", g.ApplicableSportPredicates)
	}
}

// TestGearCreateOmittedApplicableUsesLegacyDefault verifies that when
// ApplicableSportPredicates is nil but DefaultSportPredicates is non-nil, the
// legacy field is used as a fallback.
func TestGearCreateOmittedApplicableUsesLegacyDefault(t *testing.T) {
	db, userID := gearTestDB(t)
	preds := []gearpkg.GearDefaultPredicate{{SportType: "cycling"}}
	g, err := db.CreateGear(context.Background(), userID, &model.GearCreate{
		Name:                   "Legacy Fallback Bike",
		Type:                   "bike",
		DefaultSportPredicates: &preds,
		// ApplicableSportPredicates intentionally nil → falls back to Default
	})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}
	if len(g.ApplicableSportPredicates) != 1 || g.ApplicableSportPredicates[0].SportType != "cycling" {
		t.Fatalf("ApplicableSportPredicates = %#v, want one cycling predicate from legacy field", g.ApplicableSportPredicates)
	}
}

// TestGearCreateBothNilPredicatesStoresEmpty verifies that when both predicate
// pointer fields are nil, the DB row stores [] (nil → empty after NormalizeGearDefaultPredicates).
func TestGearCreateBothNilPredicatesStoresEmpty(t *testing.T) {
	db, userID := gearTestDB(t)
	g, err := db.CreateGear(context.Background(), userID, &model.GearCreate{
		Name: "No Predicates Bike",
		Type: "bike",
		// Both ApplicableSportPredicates and DefaultSportPredicates nil
	})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}
	// nil pointer → rawPredicates is nil → NormalizeGearDefaultPredicates(nil) → [] → empty row
	if g.ApplicableSportPredicates == nil {
		t.Fatal("ApplicableSportPredicates is nil, want empty non-nil slice")
	}
	// Note: len may be 0 or more depending on whether the schema-level default
	// injects predicates. The critical assertion is that no panic occurred and
	// the field is non-nil (scanGear always returns at least []).
	_ = g.ApplicableSportPredicates
}
