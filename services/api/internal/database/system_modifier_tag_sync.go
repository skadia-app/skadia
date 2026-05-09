package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// systemModifierTagSyncQuerier is the minimal pgx surface tx-aware methods
// need when they want to run inside a WithSystemModifierTagSyncSuspended
// transaction. Both *pgxpool.Pool (single-connection callers) and pgx.Tx
// (helper-wrapped callers) satisfy it. Only methods that legitimately need
// the skip gate should accept this interface; existing single-row paths
// keep using their concrete *DB receivers.
type systemModifierTagSyncQuerier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// WithSystemModifierTagSyncSuspended wraps fn in a transaction that has the
// skadia.skip_system_modifier_tag_sync session variable set, then calls
// skadia_resync_system_modifier_tags() on the deduplicated activityIDs
// before commit. If fn or the resync call returns an error, the entire
// transaction rolls back and no row is changed.
//
// activityIDs must be non-empty: the helper's contract is "you have IDs you
// want resynced; call something else for an empty set". Passing an empty or
// nil slice returns an error without opening a transaction.
//
// Callers must NOT wrap downloads, parsing, stream writes, or enrichment
// file IO inside fn. Long-running work belongs OUTSIDE the helper; only
// the SQL block that needs the skip gate should run here.
func (db *DB) WithSystemModifierTagSyncSuspended(
	ctx context.Context,
	activityIDs []string,
	fn func(ctx context.Context, tx pgx.Tx) error,
) (err error) {
	if fn == nil {
		return fmt.Errorf("WithSystemModifierTagSyncSuspended: fn is nil")
	}
	if len(activityIDs) == 0 {
		return fmt.Errorf("WithSystemModifierTagSyncSuspended: empty activityIDs")
	}

	deduped := dedupeSystemModifierActivityIDs(activityIDs)

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			// A panic from fn or one of the SQL helpers must roll the
			// transaction back; otherwise pgxpool keeps the connection
			// holding an open tx until the runtime tears it down. Re-panic
			// so callers still see the original failure.
			_ = tx.Rollback(ctx)
			panic(r)
		}
		if err != nil {
			// Best-effort rollback; ignore rollback errors so the original
			// failure remains the outer return value.
			_ = tx.Rollback(ctx)
		}
	}()

	if len(deduped) > 0 {
		if _, e := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); e != nil {
			err = fmt.Errorf("set skip flag: %w", e)
			return err
		}
	}

	if e := fn(ctx, tx); e != nil {
		err = fmt.Errorf("fn: %w", e)
		return err
	}

	if len(deduped) > 0 {
		if _, e := tx.Exec(ctx, `SELECT skadia_resync_system_modifier_tags($1::uuid[])`, deduped); e != nil {
			err = fmt.Errorf("resync: %w", e)
			return err
		}
	}

	if e := tx.Commit(ctx); e != nil {
		err = fmt.Errorf("commit: %w", e)
		return err
	}
	return nil
}

// LinkActivityTagTx is the tx-aware variant of LinkActivityTag. It must only
// be called from inside WithSystemModifierTagSyncSuspended (or from a test
// transaction that simulates the same skip-flag context). Production code
// outside the helper continues to call LinkActivityTag, which runs on the
// pool and lets the per-row trigger sync denormalized state immediately.
func (db *DB) LinkActivityTagTx(ctx context.Context, tx pgx.Tx, activityID, tagID, assignedBy string) error {
	return linkActivityTagExec(ctx, tx, activityID, tagID, assignedBy)
}

// UpdateEnrichmentFieldsTx is the tx-aware variant of UpdateEnrichmentFields.
// Same constraint as LinkActivityTagTx: callers must run inside a transaction
// that already has the system modifier tag sync skip flag established (i.e.
// from WithSystemModifierTagSyncSuspended).
func (db *DB) UpdateEnrichmentFieldsTx(ctx context.Context, tx pgx.Tx, activityID string, fields map[string]any) error {
	return updateEnrichmentFieldsExec(ctx, tx, activityID, fields)
}

// dedupeSystemModifierActivityIDs preserves first-seen order and drops empty
// strings. Order preservation keeps test fixtures and operator command-line
// inspection deterministic; the resync function does not depend on order.
func dedupeSystemModifierActivityIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
