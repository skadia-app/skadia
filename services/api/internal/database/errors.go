package database

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// Sentinel errors returned by DB functions. Callers should use errors.Is
// instead of string-matching on err.Error().
var (
	ErrNotFound              = errors.New("not found")
	ErrActivityNotFound      = errors.New("activity not found")
	ErrGearNotFound          = errors.New("gear not found")
	ErrComponentNotFound     = errors.New("component not found")
	ErrChainRotationNotFound = errors.New("chain rotation not found")
	ErrServiceEventNotFound  = errors.New("service event not found")
	ErrBikeSetupNotFound     = errors.New("bike setup not found")
	ErrReadOnly              = errors.New("read-only")
)

// isUniqueViolation reports whether err is a Postgres unique-constraint violation (SQLSTATE 23505).
// If constraintName is non-empty, only returns true for that specific constraint.
func isUniqueViolation(err error, constraintName string) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok || pgErr.Code != "23505" {
		return false
	}
	return constraintName == "" || pgErr.ConstraintName == constraintName
}

// isCheckViolation reports whether err is a Postgres check-constraint violation (SQLSTATE 23514).
// If constraintName is non-empty, only returns true for that specific constraint.
func isCheckViolation(err error, constraintName string) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok || pgErr.Code != "23514" {
		return false
	}
	return constraintName == "" || pgErr.ConstraintName == constraintName
}
