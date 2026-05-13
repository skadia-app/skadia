package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
)

var runSQLInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "sql": {
      "type": "string",
      "description": "Single validated read-only SELECT query. Do not include user_id filters; RLS handles user isolation."
    }
  },
  "required": ["sql"],
  "additionalProperties": false
}`)

func RunSQLTool() *Tool {
	return &Tool{
		Name:        "run_sql",
		Description: "Execute a validated read-only SQL query against Skadia data.",
		InputSchema: runSQLInputSchema,
		Permission:  PermissionRead,
		AuditMode:   AuditRead,
		IsUserOwned: true,
		ExecuteRead: executeRunSQL,
	}
}

func executeRunSQL(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
	if deps == nil || deps.SQLAgent == nil {
		return nil, fmt.Errorf("run_sql requires SQLAgent")
	}
	var req struct {
		SQL string `json:"sql"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("parse run_sql args: %w", err)
	}
	if req.SQL == "" {
		return nil, fmt.Errorf("run_sql requires sql")
	}
	rows, err := deps.SQLAgent.ExecuteSQL(ctx, req.SQL, userID, 100)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"rows":  rows,
		"count": len(rows),
	}, nil
}

func runSQLAuditArgs(args json.RawMessage) (json.RawMessage, []string) {
	var req struct {
		SQL string `json:"sql"`
	}
	_ = json.Unmarshal(args, &req)
	sql := agent.AutofixSQL(req.SQL)
	analysis, _ := agent.AnalyzeSQL(sql)
	tables := append([]string(nil), analysis.Tables...)
	sum := sha256.Sum256([]byte(sql))
	raw, _ := json.Marshal(map[string]any{
		"sql_hash": fmt.Sprintf("sha256:%x", sum[:]),
		"tables":   tables,
	})
	return raw, tables
}

func runSQLAuditDetail(tables []string, rowCount int, err error) json.RawMessage {
	detail := map[string]any{
		"tables":    tables,
		"row_count": rowCount,
		"status":    "executed",
	}
	if err != nil {
		detail["status"] = "error"
		detail["error"] = "execution_failed"
	}
	raw, _ := json.Marshal(detail)
	return raw
}

func runSQLResultRowCount(result any) int {
	out, ok := result.(map[string]any)
	if !ok {
		return 0
	}
	switch count := out["count"].(type) {
	case int:
		return count
	case int64:
		return int(count)
	case float64:
		return int(count)
	default:
		return 0
	}
}

func claimReadAuditSlot(ctx context.Context, audit *agent.AuditWriter, row database.AgentAuditRow) (*database.AgentAuditRow, error) {
	_, existing, err := audit.ClaimPending(ctx, row)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("pending audit row missing")
	}
	return existing, nil
}

func completeReadAudit(ctx context.Context, db *database.DB, userID string, id int64, status string, detail json.RawMessage) error {
	return db.WithUserTx(ctx, userID, func(tx pgx.Tx) error {
		return db.CompleteAudit(ctx, tx, id, status, detail)
	})
}
