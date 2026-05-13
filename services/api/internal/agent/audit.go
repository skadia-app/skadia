package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
)

var ErrAuditInFlight = errors.New("audit slot is already pending")

type AuditWriter struct {
	DB *database.DB
}

func NewAuditWriter(db *database.DB) *AuditWriter {
	return &AuditWriter{DB: db}
}

func (w *AuditWriter) ClaimPending(ctx context.Context, row database.AgentAuditRow) (bool, *database.AgentAuditRow, error) {
	tx, err := w.beginAuditTx(ctx, row.UserID, row.SessionID, row.ToolCallID)
	if err != nil {
		return false, nil, err
	}
	defer tx.Rollback(ctx)

	claimed, auditRow, err := w.DB.ClaimAuditSlot(ctx, tx, row)
	if err != nil {
		return false, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, nil, err
	}
	return claimed, auditRow, nil
}

func (w *AuditWriter) Complete(ctx context.Context, userID string, id int64, status string, detail json.RawMessage) error {
	tx, err := w.beginAuditTx(ctx, userID, "", "")
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := w.DB.CompleteAudit(ctx, tx, id, status, detail); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *AuditWriter) CompletePending(ctx context.Context, userID string, id int64, status string, detail json.RawMessage) error {
	tx, err := w.beginAuditTx(ctx, userID, "", "")
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := w.DB.CompletePendingAudit(ctx, tx, id, status, detail); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	return tx.Commit(ctx)
}

func (w *AuditWriter) WithAuditSlot(ctx context.Context, row database.AgentAuditRow, fn func(context.Context, pgx.Tx) (json.RawMessage, error)) (json.RawMessage, bool, error) {
	if fn == nil {
		return nil, false, fmt.Errorf("audit execution function is nil")
	}
	tx, err := w.beginAuditTx(ctx, row.UserID, row.SessionID, row.ToolCallID)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)

	claimed, auditRow, err := w.DB.ClaimAuditSlot(ctx, tx, row)
	if err != nil {
		return nil, false, err
	}
	if !claimed {
		if auditRow != nil && auditRow.ResultStatus == "pending" {
			return nil, false, ErrAuditInFlight
		}
		if auditRow == nil {
			return nil, false, fmt.Errorf("audit slot exists but row was not returned")
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		return auditRow.ResultDetail, false, nil
	}

	detail, execErr := fn(ctx, tx)
	status := "executed"
	if execErr != nil {
		status = "error"
		if len(detail) == 0 {
			detail, _ = json.Marshal(map[string]string{"error": "operation failed"})
		}
	}
	if len(detail) == 0 {
		detail = json.RawMessage(`{}`)
	}
	if err := w.DB.CompleteAudit(ctx, tx, auditRow.ID, status, detail); err != nil {
		return nil, true, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, true, err
	}
	return detail, true, execErr
}

func (w *AuditWriter) beginAuditTx(ctx context.Context, userID, sessionID, toolCallID string) (pgx.Tx, error) {
	if w == nil || w.DB == nil || w.DB.Pool == nil {
		return nil, fmt.Errorf("audit writer database is not configured")
	}
	if userID == "" {
		return nil, fmt.Errorf("RLS context not initialized: userID is required")
	}
	tx, err := w.DB.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_user_id', $1, true)", userID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("set local rls context: %w", err)
	}
	if sessionID != "" || toolCallID != "" {
		key := userID + ":" + sessionID + ":" + toolCallID
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", key); err != nil {
			_ = tx.Rollback(ctx)
			return nil, fmt.Errorf("lock audit slot: %w", err)
		}
	}
	return tx, nil
}
