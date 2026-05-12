package database

import (
	"context"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func LoadRuleForPreview(ctx context.Context, readonly *pgxpool.Pool, userID, ruleID string) (*model.AutomationRule, error) {
	if readonly == nil {
		return nil, fmt.Errorf("readonly pool is required")
	}
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}
	tx, err := readonly.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin readonly tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_user_id', $1, true)", userID); err != nil {
		return nil, fmt.Errorf("set local rls context: %w", err)
	}
	rule, err := scanRule(tx.QueryRow(ctx,
		`SELECT `+ruleSelectCols+` FROM automation_rules WHERE id = $1 AND user_id = $2`,
		ruleID, userID,
	))
	if err != nil {
		return nil, fmt.Errorf("load rule for preview: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit readonly tx: %w", err)
	}
	return rule, nil
}
