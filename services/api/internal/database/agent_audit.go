package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type AgentAuditRow struct {
	ID           int64
	UserID       string
	SessionID    string
	ToolCallID   string
	ToolName     string
	Args         json.RawMessage
	ResultStatus string
	ResultDetail json.RawMessage
	RequestedAt  time.Time
	CompletedAt  *time.Time
}

func (db *DB) ClaimAuditSlot(ctx context.Context, tx pgx.Tx, row AgentAuditRow) (bool, *AgentAuditRow, error) {
	if tx == nil {
		return false, nil, fmt.Errorf("ClaimAuditSlot requires tx")
	}
	if len(row.Args) == 0 {
		row.Args = json.RawMessage(`{}`)
	}
	if row.ResultStatus == "" {
		row.ResultStatus = "pending"
	}

	inserted, err := scanAgentAuditRow(tx.QueryRow(ctx, `
		INSERT INTO agent_audit_log (user_id, session_id, tool_call_id, tool_name, args, result_status)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		ON CONFLICT (user_id, session_id, tool_call_id) DO NOTHING
		RETURNING id, user_id::text, session_id::text, tool_call_id, tool_name, args, result_status,
		          result_detail, requested_at, completed_at
	`, row.UserID, row.SessionID, row.ToolCallID, row.ToolName, row.Args, row.ResultStatus))
	if err == nil {
		return true, inserted, nil
	}
	if err != pgx.ErrNoRows {
		return false, nil, err
	}

	existing, err := scanAgentAuditRow(tx.QueryRow(ctx, `
		SELECT id, user_id::text, session_id::text, tool_call_id, tool_name, args, result_status,
		       result_detail, requested_at, completed_at
		FROM agent_audit_log
		WHERE user_id = $1 AND session_id = $2 AND tool_call_id = $3
	`, row.UserID, row.SessionID, row.ToolCallID))
	if err != nil {
		return false, nil, err
	}
	return false, existing, nil
}

func (db *DB) CompleteAudit(ctx context.Context, tx pgx.Tx, id int64, status string, detail json.RawMessage) error {
	if tx == nil {
		return fmt.Errorf("CompleteAudit requires tx")
	}
	var detailArg any
	if len(detail) > 0 {
		detailArg = detail
	}
	tag, err := tx.Exec(ctx, `
		UPDATE agent_audit_log
		SET result_status = $2,
		    result_detail = $3::jsonb,
		    completed_at = now()
		WHERE id = $1
	`, id, status, detailArg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func scanAgentAuditRow(row pgx.Row) (*AgentAuditRow, error) {
	var r AgentAuditRow
	if err := row.Scan(
		&r.ID,
		&r.UserID,
		&r.SessionID,
		&r.ToolCallID,
		&r.ToolName,
		&r.Args,
		&r.ResultStatus,
		&r.ResultDetail,
		&r.RequestedAt,
		&r.CompletedAt,
	); err != nil {
		return nil, err
	}
	return &r, nil
}
