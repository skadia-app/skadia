package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

// TestUpdateEnrichmentFieldsModifierColumnConsistencyOnSlotRemoval pins the
// Direction-A trigger contract: a single-row UPDATE that passes both
// is_commute=true and tags=[] (removing the 'Pendeln' reserved-name slot) must
// commit with is_commute=false. The trigger derives the modifier flag from the
// tags array slot change, overriding the caller-supplied flag value.
//
// This contract matters because callers like enrichment stage-1 write
// is_commute and tags in the same UPDATE map. If they both arrive together and
// the tags write removes 'Pendeln', the trigger must win — otherwise the flag
// and the denormalized array diverge.
func TestUpdateEnrichmentFieldsModifierColumnConsistencyOnSlotRemoval(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "enrtest-"+userID[:8], "enrtest-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM activity_tag_removals WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM activity_tags WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM automation_rules WHERE user_id = $1`, userID)
		_, _ = pool.Exec(bg, `UPDATE tags SET is_system = false WHERE user_id = $1`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM tags WHERE user_id = $1`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = $1`, userID)
	})

	db := &DB{Pool: pool}
	if err := db.SeedSystemDefaults(ctx, userID); err != nil {
		t.Fatalf("SeedSystemDefaults: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	act, err := db.InsertActivity(ctx, userID, &model.ActivityData{
		SportType: "cycling",
		StartedAt: now,
		EndedAt:   now.Add(15 * time.Minute),
		FileHash:  uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("InsertActivity: %v", err)
	}

	// Seed: set tags=['Pendeln'] and is_commute=true directly so the initial
	// state is consistent (trigger fires on INSERT too, reconciling both).
	if _, err := pool.Exec(ctx,
		`UPDATE activities SET tags = ARRAY['Pendeln']::text[], is_commute = true WHERE id = $1`,
		act.ID); err != nil {
		t.Fatalf("seed initial state: %v", err)
	}

	// Verify initial state.
	var initCommute bool
	var initTags []string
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(is_commute, false), COALESCE(tags, '{}') FROM activities WHERE id = $1`, act.ID,
	).Scan(&initCommute, &initTags); err != nil {
		t.Fatalf("query initial state: %v", err)
	}
	if !initCommute {
		t.Fatalf("initial is_commute = false, want true (seed may have failed)")
	}

	// Call UpdateEnrichmentFields with is_commute=true AND tags=[] in the same
	// map. The caller asks for is_commute=true but strips the 'Pendeln' slot
	// from the tags array. Direction A (tags→flags) fires and sets is_commute=false.
	if err := db.UpdateEnrichmentFields(ctx, act.ID, map[string]any{
		"is_commute": true,
		"tags":       []string{},
	}); err != nil {
		t.Fatalf("UpdateEnrichmentFields: %v", err)
	}

	var gotCommute bool
	var gotTags []string
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(is_commute, false), COALESCE(tags, '{}') FROM activities WHERE id = $1`, act.ID,
	).Scan(&gotCommute, &gotTags); err != nil {
		t.Fatalf("query after update: %v", err)
	}

	// Direction A must win: slot removal overrides the caller-supplied flag.
	if gotCommute {
		t.Fatalf("is_commute = true after slot removal; Direction A trigger must set it false "+
			"when tags=[] removes the 'Pendeln' reserved-name slot (caller-supplied is_commute=true loses)")
	}
	if len(gotTags) != 0 {
		t.Fatalf("tags = %v after update, want [] (empty)", gotTags)
	}
}
