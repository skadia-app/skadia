package auth

import (
	"context"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// ErrEmailCollision indicates a social login email matches an existing account.
// The user should login with their existing credentials and link the social
// account in settings instead.
var ErrEmailCollision = fmt.Errorf("email already registered — please login and link account in settings")

// CheckEmailCollision checks if an email is already registered.
// Used during social login to prevent automatic account merging.
func CheckEmailCollision(ctx context.Context, db *database.DB, email string) error {
	exists, err := db.CheckEmailCollision(ctx, email)
	if err != nil {
		return fmt.Errorf("check email collision: %w", err)
	}
	if exists {
		return ErrEmailCollision
	}
	return nil
}
