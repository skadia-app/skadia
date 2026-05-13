package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestRunSQLToolSchemaValid(t *testing.T) {
	reg := NewRegistry()
	reg.Register(RunSQLTool())
	if err := reg.ValidateSchemas(); err != nil {
		t.Fatalf("ValidateSchemas: %v", err)
	}
	defs := reg.Definitions([]string{"run_sql"}, nil)
	if len(defs) != 1 || defs[0].Name != "run_sql" {
		t.Fatalf("unexpected defs: %#v", defs)
	}
}

func TestRunSQLRequiresSQLAgent(t *testing.T) {
	_, err := executeRunSQL(context.Background(), &Deps{}, "user-1", json.RawMessage(`{"sql":"SELECT 1"}`))
	if err == nil {
		t.Fatal("executeRunSQL returned nil error, want SQLAgent error")
	}
}

func TestRunSQLHappyPathAndTrim(t *testing.T) {
	ctx := context.Background()
	db := openRunSQLTestDB(t)
	sqlAgent := agent.NewSQLAgent(db)

	got, err := executeRunSQL(ctx, &Deps{SQLAgent: sqlAgent}, "00000000-0000-0000-0000-000000000123", json.RawMessage(`{"sql":"SELECT generate_series(1, 120) AS n"}`))
	if err != nil {
		t.Fatalf("executeRunSQL: %v", err)
	}
	result := got.(map[string]any)
	rows := result["rows"].([]map[string]any)
	if result["count"] != 100 || len(rows) != 100 {
		t.Fatalf("expected 100 trimmed rows, got count=%v len=%d", result["count"], len(rows))
	}
}

func TestRunSQLValidatorFailure(t *testing.T) {
	ctx := context.Background()
	db := openRunSQLTestDB(t)
	sqlAgent := agent.NewSQLAgent(db)

	_, err := executeRunSQL(ctx, &Deps{SQLAgent: sqlAgent}, "00000000-0000-0000-0000-000000000123", json.RawMessage(`{"sql":"SELECT * FROM users"}`))
	if err == nil {
		t.Fatal("executeRunSQL returned nil error, want validator failure")
	}
}

func TestRunSQLRuntimeAuditsSensitiveReadWithoutResultRows(t *testing.T) {
	ctx := context.Background()
	db := openRunSQLTestDB(t)
	userID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "run-sql-audit-"+userID[:8], "run-sql-audit-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID) })
	reg := NewRegistry()
	reg.Register(RunSQLTool())
	runtime := NewAgentToolRuntime(reg, &Deps{
		DB:       db,
		SQLAgent: agent.NewSQLAgent(db),
		Audit:    agent.NewAuditWriter(db),
	}, nil)

	result, err := runtime.Execute(ctx, agent.ToolRuntimeRequest{
		UserID:     userID,
		SessionID:  "00000000-0000-0000-0000-000000000456",
		MessageID:  "msg-1",
		ToolCallID: "call-sql-audit",
		ToolName:   "run_sql",
		Args:       json.RawMessage(`{"sql":"SELECT count(*) AS total FROM activities"}`),
	}, nil)
	if err != nil {
		t.Fatalf("runtime Execute run_sql: %v", err)
	}
	if result.Result == nil {
		t.Fatal("runtime Execute returned nil result")
	}

	var toolName, status, argsText, detailText string
	err = db.Pool.QueryRow(ctx, `
		SELECT tool_name, result_status, args::text, COALESCE(result_detail::text, '')
		FROM agent_audit_log
		WHERE user_id = $1 AND tool_call_id = $2
	`, userID, "call-sql-audit").Scan(&toolName, &status, &argsText, &detailText)
	if err != nil {
		t.Fatalf("select audit row: %v", err)
	}
	if toolName != "run_sql" || status != "executed" {
		t.Fatalf("audit tool/status = %s/%s, want run_sql/executed", toolName, status)
	}
	for _, forbidden := range []string{"SELECT count(*)", `"rows"`, `"total"`} {
		if strings.Contains(argsText, forbidden) || strings.Contains(detailText, forbidden) {
			t.Fatalf("audit leaked %q in args=%s detail=%s", forbidden, argsText, detailText)
		}
	}
	for _, required := range []string{"sql_hash", "activities", "row_count"} {
		if !strings.Contains(argsText+detailText, required) {
			t.Fatalf("audit missing %q in args=%s detail=%s", required, argsText, detailText)
		}
	}
}

func TestRunSQLAuditDetailDoesNotPersistRawErrorText(t *testing.T) {
	raw := runSQLAuditDetail([]string{"activities"}, 0, fmt.Errorf("invalid input syntax for type uuid: \"secret-literal\""))

	if strings.Contains(string(raw), "secret-literal") || strings.Contains(string(raw), "invalid input syntax") {
		t.Fatalf("audit detail leaked raw DB error text: %s", raw)
	}
	if !strings.Contains(string(raw), `"status":"error"`) {
		t.Fatalf("audit detail missing error status: %s", raw)
	}
}

func openRunSQLTestDB(t *testing.T) *database.DB {
	t.Helper()
	ctx := context.Background()
	dsn := testdb.RequireSafeDSN(t)
	db, err := database.New(ctx, dsn)
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(db.Close)
	if err := db.OpenReadonly(ctx, readonlyRunSQLDSN(t, dsn)); err != nil {
		t.Fatalf("OpenReadonly: %v", err)
	}
	return db
}

func readonlyRunSQLDSN(t *testing.T, dsn string) string {
	t.Helper()
	if _, err := pgx.ParseConfig(dsn); err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn url: %v", err)
	}
	u.User = url.UserPassword("skadia_readonly", "skadia_readonly_password")
	return u.String()
}
