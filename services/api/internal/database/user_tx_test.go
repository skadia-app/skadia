package database

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/jackc/pgx/v5"
)

func TestOpenReadonlyPoolUsesReadonlyRole(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.RequireSafeDSN(t)
	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(db.Close)

	if err := db.OpenReadonly(ctx, readonlyDSNFromTestDSN(t, dsn)); err != nil {
		t.Fatalf("OpenReadonly: %v", err)
	}

	var currentUser string
	if err := db.ReadonlyPool.QueryRow(ctx, `SELECT current_user`).Scan(&currentUser); err != nil {
		t.Fatalf("current_user: %v", err)
	}
	if currentUser != "skadia_readonly" {
		t.Fatalf("current_user = %q, want skadia_readonly", currentUser)
	}
}

func TestWithUserTxSetsLocalRLSContextAndDoesNotLeak(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.RequireSafeDSN(t)
	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(db.Close)

	const userID = "00000000-0000-0000-0000-000000000123"
	err = db.WithUserTx(ctx, userID, func(tx pgx.Tx) error {
		var got string
		if err := tx.QueryRow(ctx, `SELECT current_setting('app.current_user_id', true)`).Scan(&got); err != nil {
			return err
		}
		if got != userID {
			t.Fatalf("current_setting(app.current_user_id) = %q, want %q", got, userID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithUserTx: %v", err)
	}

	var leaked string
	if err := db.Pool.QueryRow(ctx, `SELECT current_setting('app.current_user_id', true)`).Scan(&leaked); err != nil {
		t.Fatalf("check leaked setting: %v", err)
	}
	if leaked != "" {
		t.Fatalf("app.current_user_id leaked after commit: %q", leaked)
	}
}

func TestWithUserTxRejectsMissingUserID(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.RequireSafeDSN(t)
	db, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(db.Close)

	err = db.WithUserTx(ctx, "", func(tx pgx.Tx) error {
		t.Fatal("callback should not run")
		return nil
	})
	if err == nil {
		t.Fatal("WithUserTx returned nil, want error")
	}
	if !strings.Contains(err.Error(), "RLS context not initialized") {
		t.Fatalf("error = %q, want RLS context not initialized", err.Error())
	}
}

func readonlyDSNFromTestDSN(t *testing.T, dsn string) string {
	t.Helper()
	if _, err := pgx.ParseConfig(dsn); err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.User = url.UserPassword("skadia_readonly", "skadia_readonly_password")
	return u.String()
}
