package migrations_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestMigrationUserLocaleUnitsConstraints(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	userID := uuid.NewString()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"mig_i18n_"+userID[:8],
		"mig_i18n_"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})

	if _, err := pool.Exec(ctx,
		`UPDATE users SET locale = 'es', units = 'imperial' WHERE id = $1`, userID); err != nil {
		t.Fatalf("valid locale/units rejected: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE users SET locale = 'pt' WHERE id = $1`, userID); err == nil {
		t.Fatal("invalid locale accepted, want check constraint failure")
	}
	if _, err := pool.Exec(ctx,
		`UPDATE users SET units = 'nautical' WHERE id = $1`, userID); err == nil {
		t.Fatal("invalid units accepted, want check constraint failure")
	}
}
