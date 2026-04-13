package dataimport

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// findByNaturalKey queries for an existing row matching the natural key
// (spec.NaturalKey columns plus the user column) and returns its ID.
// Returns ("", false, nil) when no natural key is defined, when the user
// column is absent ("none"), or when any natural-key value in row is nil.
func findByNaturalKey(ctx context.Context, tx pgx.Tx, spec tableSpec, userID string, row map[string]any) (string, bool, error) {
	if len(spec.NaturalKey) == 0 {
		return "", false, nil
	}

	userCol := getUserCol(spec)
	if userCol == "" {
		return "", false, nil
	}

	// Collect args: $1 = userID, $2...$N = natural-key values.
	args := make([]any, 0, 1+len(spec.NaturalKey))
	args = append(args, userID)

	conditions := make([]string, 0, 1+len(spec.NaturalKey))
	conditions = append(conditions, fmt.Sprintf("%s = $1", pgx.Identifier{userCol}.Sanitize()))

	for i, col := range spec.NaturalKey {
		val, ok := row[col]
		if !ok || val == nil {
			// NULL natural-key value — cannot reliably match.
			return "", false, nil
		}
		conditions = append(conditions, fmt.Sprintf("%s = $%d", pgx.Identifier{col}.Sanitize(), i+2))
		args = append(args, val)
	}

	query := fmt.Sprintf(
		"SELECT id FROM %s WHERE %s LIMIT 1",
		pgx.Identifier{spec.Name}.Sanitize(),
		strings.Join(conditions, " AND "),
	)

	var existingID string
	err := tx.QueryRow(ctx, query, args...).Scan(&existingID)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("findByNaturalKey %s: %w", spec.Name, err)
	}

	return existingID, true, nil
}

// deleteForReplace removes all rows belonging to userID from the table
// described by spec. Returns an error if the table has no user column.
func deleteForReplace(ctx context.Context, tx pgx.Tx, spec tableSpec, userID string) error {
	userCol := getUserCol(spec)
	if userCol == "" {
		return fmt.Errorf("deleteForReplace: table %q has no user column", spec.Name)
	}

	query := fmt.Sprintf(
		"DELETE FROM %s WHERE %s = $1",
		pgx.Identifier{spec.Name}.Sanitize(),
		pgx.Identifier{userCol}.Sanitize(),
	)

	_, err := tx.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("deleteForReplace %s: %w", spec.Name, err)
	}

	return nil
}

// fkTargetExists verifies that the external FK target referenced by row
// exists in its parent table. Tables that are not explicitly handled default
// to (true, nil), meaning the row is assumed importable.
func fkTargetExists(ctx context.Context, tx pgx.Tx, spec tableSpec, row map[string]any) (bool, error) {
	switch spec.Name {
	case "segment_favorites":
		var exists bool
		err := tx.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM segments WHERE id = $1)",
			row["segment_id"],
		).Scan(&exists)
		if err != nil {
			return false, fmt.Errorf("fkTargetExists segment_favorites: %w", err)
		}
		return exists, nil

	default:
		return true, nil
	}
}
