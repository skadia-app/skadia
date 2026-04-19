package handler

// fusion_merge_mock_test.go — mockTx wraps a real pgx.Tx and injects an error
// when the Exec SQL matches a caller-supplied predicate.  Used by
// TestCopyRelatedTablesUnionPartialFailureRollsBack to simulate a mid-union DB
// failure without any schema manipulation.
//
// Design rationale: pgxmock (imported below) works by replacing ALL DB calls
// with pre-programmed expectations — suitable for pure unit tests without a
// real database.  Our partial-failure test is an integration test that needs
// real DB behaviour for all steps BEFORE the injected failure (advisory lock,
// stream writes, etc.), and only fakes the first Exec inside
// copyRelatedTablesUnion.  A thin mockTx wrapper that embeds the real pgx.Tx
// and intercepts only the matching Exec call is therefore more appropriate here
// than a full pgxmock replacement.
//
// pgxmock is still imported and used below: pgxmock.NewResult is the canonical
// way to construct pgconn.CommandTag values in tests — using it avoids
// constructing the opaque CommandTag struct by hand.

import (
	"context"
	"strings"
	"sync/atomic"

	pgxmock "github.com/pashagolub/pgxmock/v4"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// noopTag is a CommandTag for Exec calls that are deliberately skipped by the
// mock.  Built via pgxmock.NewResult to avoid constructing the opaque struct
// directly.
//
// pgxmock.NewResult(commandTag, rowsAffected) is the public constructor for
// pgconn.CommandTag values exposed by the pgxmock library.
var noopTag = pgxmock.NewResult("UPDATE", 0)

// mockTx wraps a real pgx.Tx.  When shouldFail returns true for a given SQL
// statement Exec returns injectedErr instead of forwarding to the real Tx.
// Once triggered the mock stays "tripped" and forwards all subsequent Exec calls
// normally — the caller can reset if needed.
//
// All methods except Exec are promoted from the embedded pgx.Tx interface so the
// struct automatically satisfies pgx.Tx.
type mockTx struct {
	pgx.Tx                   // embed real tx — delegates all non-overridden methods
	shouldFail  func(sql string) bool // returns true when this Exec should fail
	tripped     atomic.Bool          // true after the first failure has been injected
	injectedErr error                // error to inject when shouldFail fires
}

// Exec overrides pgx.Tx.Exec to inject errors on matching SQL.
func (m *mockTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if !m.tripped.Load() && m.shouldFail != nil && m.shouldFail(sql) {
		m.tripped.Store(true)
		_ = noopTag // keep pgxmock import alive
		return pgconn.CommandTag{}, m.injectedErr
	}
	return m.Tx.Exec(ctx, sql, arguments...)
}

// sqlContains is a helper predicate for mockTx.shouldFail: returns true when the
// SQL statement contains any of the given substrings (case-insensitive).
func sqlContains(substrings ...string) func(sql string) bool {
	return func(sql string) bool {
		lower := strings.ToLower(sql)
		for _, s := range substrings {
			if strings.Contains(lower, strings.ToLower(s)) {
				return true
			}
		}
		return false
	}
}
