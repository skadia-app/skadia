package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// FusionMembership is a single (merged_id, source_id, field_key, user_id) row.
// This is the DATABASE-layer type. The API-response type (handler.FusionMembershipResponse)
// only carries source_id + field_key and lives in the handler package.
type FusionMembership struct {
	MergedID string
	SourceID string
	FieldKey string
	UserID   string
}

// InsertFusionMemberships writes memberships transactionally.
// tx MUST be an active pgx.Tx — passing *pgxpool.Pool would send the batch on an
// auto-commit connection and break atomicity with the surrounding write.
// The restrictive type guarantees the caller is inside a transaction.
//
// pgx batch-mode note: if op N fails with FK-violation, iteration stops; remaining
// queued ops are not executed but the connection is cleanly reset by br.Close().
// The surrounding tx-rollback reverts everything.
func InsertFusionMemberships(ctx context.Context, tx pgx.Tx, memberships []FusionMembership) error {
	if len(memberships) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, m := range memberships {
		batch.Queue(
			`INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT DO NOTHING`,
			m.MergedID, m.SourceID, m.FieldKey, m.UserID,
		)
	}
	br := tx.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(memberships); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert membership %d: %w", i, err)
		}
	}
	return nil
}

// DeleteFusionMembershipsForMerged removes all memberships for a merged activity.
// Called from RevertFusion BEFORE DeleteActivity to keep Dual-Write state consistent
// (CASCADE would eventually clean them, but we need consistency during the revert).
func DeleteFusionMembershipsForMerged(ctx context.Context, tx pgx.Tx, mergedID string) error {
	_, err := tx.Exec(ctx,
		`DELETE FROM activity_fusion_memberships WHERE merged_id = $1`, mergedID)
	return err
}

// GetFusionMembershipsForMerged returns all memberships for a merged activity.
func (db *DB) GetFusionMembershipsForMerged(ctx context.Context, mergedID string) ([]FusionMembership, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT merged_id, source_id, field_key, user_id
		 FROM activity_fusion_memberships
		 WHERE merged_id = $1
		 ORDER BY field_key, source_id`, mergedID)
	if err != nil {
		return nil, fmt.Errorf("query memberships for merged: %w", err)
	}
	defer rows.Close()
	var out []FusionMembership
	for rows.Next() {
		var m FusionMembership
		if err := rows.Scan(&m.MergedID, &m.SourceID, &m.FieldKey, &m.UserID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetFusionMembershipsAsSource returns memberships where activityID is a source.
// Used by tests to assert "this activity is a fusion source" and by RevertFusion
// cross-fusion-check.
func (db *DB) GetFusionMembershipsAsSource(ctx context.Context, activityID string) ([]FusionMembership, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT merged_id, source_id, field_key, user_id
		 FROM activity_fusion_memberships
		 WHERE source_id = $1
		 ORDER BY merged_id, field_key`, activityID)
	if err != nil {
		return nil, fmt.Errorf("query memberships as source: %w", err)
	}
	defer rows.Close()
	var out []FusionMembership
	for rows.Next() {
		var m FusionMembership
		if err := rows.Scan(&m.MergedID, &m.SourceID, &m.FieldKey, &m.UserID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountFusionMembershipsAsSource returns how many distinct merged activities
// reference activityID as a source. Drives the Cross-Fusion badge.
func (db *DB) CountFusionMembershipsAsSource(ctx context.Context, activityID string) (int, error) {
	var n int
	err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT merged_id) FROM activity_fusion_memberships WHERE source_id = $1`,
		activityID).Scan(&n)
	return n, err
}

// GetFusionMembershipsForActivities returns every membership row whose merged_id is in
// the given slice, plus the source-count per activity. One roundtrip instead of N+1
// per-row queries on list endpoints.
//
// Returns:
//   - memberships: map[mergedID] -> []FusionMembership  (only merged activities are keys)
//   - sourceCounts: map[sourceID] -> int                (only source activities are keys)
func (db *DB) GetFusionMembershipsForActivities(
	ctx context.Context,
	activityIDs []string,
) (memberships map[string][]FusionMembership, sourceCounts map[string]int, err error) {
	if len(activityIDs) == 0 {
		return map[string][]FusionMembership{}, map[string]int{}, nil
	}

	memberships = make(map[string][]FusionMembership)
	sourceCounts = make(map[string]int)

	// Memberships where any given id is the merged_id.
	rows, err := db.Pool.Query(ctx,
		`SELECT merged_id, source_id, field_key, user_id
		 FROM activity_fusion_memberships
		 WHERE merged_id = ANY($1)
		 ORDER BY merged_id, field_key, source_id`,
		activityIDs,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("query memberships for batch: %w", err)
	}
	for rows.Next() {
		var m FusionMembership
		if err := rows.Scan(&m.MergedID, &m.SourceID, &m.FieldKey, &m.UserID); err != nil {
			rows.Close()
			return nil, nil, err
		}
		memberships[m.MergedID] = append(memberships[m.MergedID], m)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	// Source counts where any given id is the source_id.
	countRows, err := db.Pool.Query(ctx,
		`SELECT source_id, COUNT(DISTINCT merged_id)
		 FROM activity_fusion_memberships
		 WHERE source_id = ANY($1)
		 GROUP BY source_id`,
		activityIDs,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("query source counts for batch: %w", err)
	}
	defer countRows.Close()
	for countRows.Next() {
		var sid string
		var cnt int
		if err := countRows.Scan(&sid, &cnt); err != nil {
			return nil, nil, err
		}
		sourceCounts[sid] = cnt
	}
	if err := countRows.Err(); err != nil {
		return nil, nil, err
	}
	return memberships, sourceCounts, nil
}

// GetLatestEditData returns the edit_data JSONB of the most recent activity_edits
// row for (activity_id, edit_type). Returns nil (no error) if no row exists.
func (db *DB) GetLatestEditData(ctx context.Context, activityID, editType string) (json.RawMessage, error) {
	var raw json.RawMessage
	err := db.Pool.QueryRow(ctx,
		`SELECT edit_data FROM activity_edits
		 WHERE activity_id = $1 AND edit_type = $2
		 ORDER BY created_at DESC LIMIT 1`,
		activityID, editType).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetLatestEditData(%s, %s): %w", activityID, editType, err)
	}
	return raw, nil
}

// GetActivityTitle returns the display title of an activity (COALESCE(title, '')).
// Used by the sidecar writer to embed a human-readable source title in the
// FusionMemberSnapshot.
func (db *DB) GetActivityTitle(ctx context.Context, activityID string) (string, error) {
	var title string
	err := db.Pool.QueryRow(ctx,
		`SELECT COALESCE(title, '') FROM activities WHERE id = $1`,
		activityID).Scan(&title)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return title, err
}

// IsFusionSourceViaMemberships returns true iff activityID appears as source in any membership.
// Replaces the is_fusion_source column read — used by list/queue eligibility filters.
func (db *DB) IsFusionSourceViaMemberships(ctx context.Context, activityID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM activity_fusion_memberships WHERE source_id = $1)`,
		activityID).Scan(&exists)
	return exists, err
}

// ListSourcesForMerged returns distinct source_ids of a merged activity.
// Used by RevertFusion to restore sources.
func (db *DB) ListSourcesForMerged(ctx context.Context, mergedID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT DISTINCT source_id FROM activity_fusion_memberships WHERE merged_id = $1`,
		mergedID)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
