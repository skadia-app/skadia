package database

import (
	"context"
	"testing"

	"github.com/google/uuid"

	gearpkg "github.com/skadia-app/skadia/services/api/internal/gear"
)

func TestGetEffectiveGearSchemaForUser(t *testing.T) {
	db, userA := gearTestDB(t)
	ctx := context.Background()

	// Create a second user without any overrides.
	userB := uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userB, "schematest-"+userB[:8], "schematest-"+userB[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user B: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userB)
	})

	// Insert a user-schema override for User A: add "kayak" to helmet.subcategory.
	defs := []gearpkg.UserSchemaDefinition{
		{
			TypeKey: "helmet",
			ExtraOptionValues: map[string][]gearpkg.OptionValue{
				"helmet.subcategory": {{Key: "kayak", LabelDE: "Kajak"}},
			},
		},
	}
	if err := db.UpsertGearSchemaDefinition(ctx, userA, "helmet", defs[0]); err != nil {
		t.Fatalf("UpsertGearSchemaDefinition: %v", err)
	}

	// Effective schema for User A must contain "kayak".
	schemaA, err := db.GetEffectiveGearSchema(ctx, userA)
	if err != nil {
		t.Fatalf("GetEffectiveGearSchema(userA): %v", err)
	}
	var foundA bool
	for _, os := range schemaA.OptionSets {
		if os.Key == "helmet.subcategory" {
			for _, v := range os.Options {
				if v.Key == "kayak" {
					foundA = true
				}
			}
		}
	}
	if !foundA {
		t.Error("expected 'kayak' option in User A's effective schema")
	}

	// Effective schema for User B must NOT contain "kayak".
	schemaB, err := db.GetEffectiveGearSchema(ctx, userB)
	if err != nil {
		t.Fatalf("GetEffectiveGearSchema(userB): %v", err)
	}
	for _, os := range schemaB.OptionSets {
		if os.Key == "helmet.subcategory" {
			for _, v := range os.Options {
				if v.Key == "kayak" {
					t.Error("'kayak' option leaked into User B's effective schema")
				}
			}
		}
	}
}
