package tools

import (
	"context"
	"encoding/json"
	"fmt"
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
		AuditMode:   AuditNone,
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
