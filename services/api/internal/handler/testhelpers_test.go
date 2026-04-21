package handler

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// insertTestUser creates a random user for integration tests.
// cleanup is handled via t.Cleanup.
func insertTestUser(t *testing.T, pool *pgxpool.Pool, nameEmail ...string) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.New().String()

	username := "test_" + userID[:8]
	email := "test_" + userID[:8] + "@example.com"
	if len(nameEmail) > 0 {
		username = nameEmail[0]
	}
	if len(nameEmail) > 1 {
		email = nameEmail[1]
	}

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, username, email,
	)
	if err != nil {
		t.Fatalf("insertTestUser: %v", err)
	}

	t.Cleanup(func() {
		// Clean up activities (cascades efforts/versions), then the user
		pool.Exec(context.Background(), "DELETE FROM activities WHERE user_id = $1", userID)
		pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})

	return userID
}
