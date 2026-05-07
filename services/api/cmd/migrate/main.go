// Command migrate runs all pending up-migrations against DATABASE_URL.
package main

import (
	"context"
	"log"
	"os"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/migrate"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	ctx := context.Background()
	db, err := database.New(ctx, dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	if err := migrate.Run(ctx, db.Pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
}
