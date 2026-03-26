package database

import "errors"

// Sentinel errors returned by DB functions. Callers should use errors.Is
// instead of string-matching on err.Error().
var (
	ErrNotFound        = errors.New("not found")
	ErrActivityNotFound = errors.New("activity not found")
	ErrGearNotFound     = errors.New("gear not found")
	ErrComponentNotFound = errors.New("component not found")
	ErrReadOnly         = errors.New("read-only")
)
