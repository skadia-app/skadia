package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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
// caller is a short identifier for the call site (e.g. "bulk_modifier_assign",
// "stage1_enrichment"). It populates the caller label on the resync_calls_total
// Prometheus counter. Pass "" to fall back to "unknown".
//
// Callers must NOT wrap downloads, parsing, stream writes, or enrichment
// file IO inside fn. Long-running work belongs OUTSIDE the helper; only
// the SQL block that needs the skip gate should run here.
func (db *DB) WithSystemModifierTagSyncSuspended(
	ctx context.Context,
	caller string,
	activityIDs []string,
	fn func(ctx context.Context, tx pgx.Tx) error,
) (err error) {
	if fn == nil {
		return fmt.Errorf("WithSystemModifierTagSyncSuspended: fn is nil")
	}
	if len(activityIDs) == 0 {
		return fmt.Errorf("WithSystemModifierTagSyncSuspended: empty activityIDs")
	}
	if caller == "" {
		caller = "unknown"
	}

	// Validate and dedupe before opening a transaction so callers see a
	// structured error (with the offending index) instead of a Postgres
	// cast failure mid-tx, and so the skip-gate invariant holds: if
	// post-dedupe the slice is empty, the contract is violated and we
	// return an error rather than running fn without the gate set.
	deduped, err := validateAndDedupeSystemModifierActivityIDs(activityIDs)
	if err != nil {
		return err
	}

	m := db.syncMetrics
	if m == nil {
		m = defaultSyncMetrics()
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	// commitAttempted is set to true immediately before tx.Commit so the
	// defer knows whether the tx is still open. After Commit returns (with
	// or without an error), pgx marks the connection closed — calling
	// Rollback on it would return pgx.ErrTxClosed, which callers should
	// never see as a spurious error on top of the real commit failure.
	commitAttempted := false
	defer func() {
		if r := recover(); r != nil {
			// A panic from fn or one of the SQL helpers must roll the
			// transaction back; otherwise pgxpool keeps the connection
			// holding an open tx until the runtime tears it down. Re-panic
			// so callers still see the original failure.
			_ = tx.Rollback(ctx)
			panic(r)
		}
		if err != nil && !commitAttempted {
			// Real pre-commit failure: the tx is still open, roll it back.
			_ = tx.Rollback(ctx)
		}
		// If commitAttempted is true, the tx is already closed regardless
		// of whether Commit succeeded or failed — skip the redundant Rollback.
	}()

	// validateAndDedupeSystemModifierActivityIDs returned a non-empty slice or
	// an error; if we got here, deduped is guaranteed to have at least one ID.
	var gateStart time.Time
	// Record skip-gate duration (SET LOCAL through commit/rollback) after the
	// full commit path completes, so the observation includes commit-flush time
	// as stated in the metric help text.
	defer func() {
		if !gateStart.IsZero() && err == nil {
			m.skipGateActiveSeconds.Observe(time.Since(gateStart).Seconds())
		}
	}()

	gateStart = time.Now()
	if _, e := tx.Exec(ctx, `SET LOCAL skadia.skip_system_modifier_tag_sync = 'true'`); e != nil {
		err = fmt.Errorf("set skip flag: %w", e)
		return err
	}

	if e := fn(ctx, tx); e != nil {
		err = fmt.Errorf("fn: %w", e)
		return err
	}

	var junctionsInserted, junctionsDeleted, tagsPromoted, removalsDropped, systemTagsSeeded int
	if e := tx.QueryRow(ctx,
		`SELECT junctions_inserted, junctions_deleted, tags_promoted, removals_dropped, system_tags_seeded
		 FROM skadia_resync_system_modifier_tags($1::uuid[])`,
		deduped,
	).Scan(&junctionsInserted, &junctionsDeleted, &tagsPromoted, &removalsDropped, &systemTagsSeeded); e != nil {
		err = fmt.Errorf("resync: %w", e)
		return err
	}

	commitAttempted = true
	if e := tx.Commit(ctx); e != nil {
		err = fmt.Errorf("commit: %w", e)
		return err
	}

	// Record metrics only after a successful commit so a rollback leaves
	// all counters unchanged.
	m.resyncCallsTotal.WithLabelValues(caller).Inc()
	m.resyncActivitiesProcessed.Observe(float64(len(deduped)))
	// skipGateActiveSeconds is observed by the defer above.
	if junctionsInserted > 0 {
		m.repairsTotal.WithLabelValues("junction_inserted").Add(float64(junctionsInserted))
	}
	if junctionsDeleted > 0 {
		m.repairsTotal.WithLabelValues("junction_deleted").Add(float64(junctionsDeleted))
	}
	if tagsPromoted > 0 {
		m.repairsTotal.WithLabelValues("tag_promoted").Add(float64(tagsPromoted))
	}
	if removalsDropped > 0 {
		m.repairsTotal.WithLabelValues("removal_dropped").Add(float64(removalsDropped))
	}
	if systemTagsSeeded > 0 {
		m.repairsTotal.WithLabelValues("system_tag_seeded").Add(float64(systemTagsSeeded))
	}
	return nil
}

// assertSystemModifierTagSyncSkipFlagSet verifies that the
// skadia.skip_system_modifier_tag_sync session variable is 'true' on the
// given transaction. This guards the Tx-variant methods that are only safe
// to call from inside WithSystemModifierTagSyncSuspended: if a caller passes
// an unflagged tx the resync function will not run at commit, leaving
// denormalised tag state inconsistent.
func assertSystemModifierTagSyncSkipFlagSet(ctx context.Context, tx pgx.Tx, callerName string) error {
	// current_setting with missing_ok=true returns NULL when the variable was
	// never set in this session. COALESCE converts that to the empty string so
	// we can scan into a plain string without a pgx null-scan error.
	var skip string
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(current_setting('skadia.skip_system_modifier_tag_sync', true), '')`).Scan(&skip); err != nil {
		return fmt.Errorf("%s: cannot read skip flag: %w", callerName, err)
	}
	if skip != "true" {
		return fmt.Errorf("%s: skadia.skip_system_modifier_tag_sync is not 'true' on this transaction; call from inside WithSystemModifierTagSyncSuspended", callerName)
	}
	return nil
}

// LinkActivityTagTx is the tx-aware variant of LinkActivityTag. It must only
// be called from inside WithSystemModifierTagSyncSuspended (or from a test
// transaction that simulates the same skip-flag context). Production code
// outside the helper continues to call LinkActivityTag, which runs on the
// pool and lets the per-row trigger sync denormalized state immediately.
func (db *DB) LinkActivityTagTx(ctx context.Context, tx pgx.Tx, activityID, tagID, assignedBy string) error {
	if err := assertSystemModifierTagSyncSkipFlagSet(ctx, tx, "LinkActivityTagTx"); err != nil {
		return err
	}
	return linkActivityTagExec(ctx, tx, activityID, tagID, assignedBy)
}

// UpdateEnrichmentFieldsTx is the tx-aware variant of UpdateEnrichmentFields.
// Same constraint as LinkActivityTagTx: callers must run inside a transaction
// that already has the system modifier tag sync skip flag established (i.e.
// from WithSystemModifierTagSyncSuspended).
func (db *DB) UpdateEnrichmentFieldsTx(ctx context.Context, tx pgx.Tx, activityID string, fields map[string]any) error {
	if err := assertSystemModifierTagSyncSkipFlagSet(ctx, tx, "UpdateEnrichmentFieldsTx"); err != nil {
		return err
	}
	return updateEnrichmentFieldsExec(ctx, tx, activityID, fields)
}

// validateAndDedupeSystemModifierActivityIDs validates UUID shape and
// deduplicates. It returns an error if any element is not a valid UUID or if
// all elements are empty strings (which would violate the caller's contract of
// providing at least one real activity ID). Validation happens before any
// transaction opens so callers see a structured error with the offending index
// rather than a Postgres cast failure mid-tx.
func validateAndDedupeSystemModifierActivityIDs(ids []string) ([]string, error) {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for i, id := range ids {
		if id == "" {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			return nil, fmt.Errorf("WithSystemModifierTagSyncSuspended: invalid activity ID %q at index %d: %w", id, i, err)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("WithSystemModifierTagSyncSuspended: no valid activity IDs after deduplication (input had %d element(s), all empty or duplicate)", len(ids))
	}
	return out, nil
}

// dedupeSystemModifierActivityIDs preserves first-seen order and drops empty
// strings. Order preservation keeps test fixtures and operator command-line
// inspection deterministic; the resync function does not depend on order.
//
// This function does NOT validate UUID shape; use
// validateAndDedupeSystemModifierActivityIDs for production call paths that
// need pre-tx validation.
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
