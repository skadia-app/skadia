package database

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestReadonlyRolePasswordFromDSN(t *testing.T) {
	password, ok, err := ReadonlyRolePasswordFromDSN("postgres://skadia_readonly:secret@postgres:5432/skadia?sslmode=disable")
	if err != nil {
		t.Fatalf("ReadonlyRolePasswordFromDSN: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if password != "secret" {
		t.Fatalf("password = %q, want secret", password)
	}
}

func TestReadonlyRolePasswordFromDSNSkipsNonReadonlyRole(t *testing.T) {
	password, ok, err := ReadonlyRolePasswordFromDSN("postgres://skadia:secret@postgres:5432/skadia?sslmode=disable")
	if err != nil {
		t.Fatalf("ReadonlyRolePasswordFromDSN: %v", err)
	}
	if ok {
		t.Fatalf("ok = true, want false; password=%q", password)
	}
}

func TestReadonlyRolePasswordFromDSNRejectsPlaceholder(t *testing.T) {
	_, ok, err := ReadonlyRolePasswordFromDSN("postgres://skadia_readonly:skadia_readonly_password@postgres:5432/skadia?sslmode=disable")
	if err == nil {
		t.Fatal("err = nil, want placeholder rejection")
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
}

func TestIsReadonlyAuthError(t *testing.T) {
	err := fmt.Errorf("ping readonly database: %w", &pgconn.PgError{Code: "28P01"})
	if !IsReadonlyAuthError(err) {
		t.Fatal("IsReadonlyAuthError = false, want true")
	}
	if IsReadonlyAuthError(fmt.Errorf("ping readonly database: %w", &pgconn.PgError{Code: "3D000"})) {
		t.Fatal("IsReadonlyAuthError = true for non-auth error")
	}
}
