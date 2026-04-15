package database

import (
	"context"
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
