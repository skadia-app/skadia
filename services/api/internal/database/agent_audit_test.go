package database

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAgentAuditClaimCompleteAndRLS(t *testing.T) {
	db, userA := agentAuditTestDB(t)
	_, userB := agentAuditTestDB(t)
	ctx := context.Background()

	sessionID := uuid.NewString()
	toolCallID := "call-1"
	var rowID int64
	err := db.WithUserTx(ctx, userA, func(tx pgx.Tx) error {
		claimed, row, err := db.ClaimAuditSlot(ctx, tx, AgentAuditRow{
			UserID:     userA,
			SessionID:  sessionID,
			ToolCallID: toolCallID,
			ToolName:   "update_activity_title",
			Args:       json.RawMessage(`{"activity_id":"a1"}`),
		})
		if err != nil {
			return err
		}
		if !claimed {
			t.Fatalf("first claim returned claimed=false row=%#v", row)
		}
		rowID = row.ID
		return db.CompleteAudit(ctx, tx, row.ID, "executed", json.RawMessage(`{"affected_id":"a1"}`))
	})
	if err != nil {
		t.Fatalf("claim/complete: %v", err)
	}

	err = db.WithUserTx(ctx, userA, func(tx pgx.Tx) error {
		claimed, existing, err := db.ClaimAuditSlot(ctx, tx, AgentAuditRow{
			UserID:     userA,
			SessionID:  sessionID,
			ToolCallID: toolCallID,
			ToolName:   "update_activity_title",
			Args:       json.RawMessage(`{"activity_id":"a1"}`),
		})
		if err != nil {
			return err
		}
		if claimed {
			t.Fatal("second claim returned claimed=true")
		}
		if existing == nil || existing.ID != rowID || existing.ResultStatus != "executed" {
			t.Fatalf("unexpected existing row: %#v", existing)
		}
		var detail map[string]string
		if err := json.Unmarshal(existing.ResultDetail, &detail); err != nil {
			t.Fatalf("unmarshal result detail: %v", err)
		}
		if detail["affected_id"] != "a1" {
			t.Fatalf("unexpected result detail: %s", existing.ResultDetail)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("claim collision: %v", err)
	}

	err = db.WithUserTx(ctx, userB, func(tx pgx.Tx) error {
		claimed, _, err := db.ClaimAuditSlot(ctx, tx, AgentAuditRow{
			UserID:     userB,
			SessionID:  uuid.NewString(),
			ToolCallID: "call-2",
			ToolName:   "update_activity_title",
			Args:       json.RawMessage(`{"activity_id":"b1"}`),
		})
		if err != nil {
			return err
		}
		if !claimed {
			t.Fatal("userB claim returned claimed=false")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("userB claim: %v", err)
	}

	err = db.WithUserTx(ctx, userA, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM agent_audit_log WHERE session_id = $1`, sessionID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("userA visible own audit rows = %d, want 1", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("userA RLS query: %v", err)
	}
	readonly := agentAuditReadonlyPool(t)
	if _, err := readonly.Exec(ctx, "SELECT set_config('app.current_user_id', $1, false)", userA); err != nil {
		t.Fatalf("set readonly RLS context: %v", err)
	}
	var readonlyCount int
	if err := readonly.QueryRow(ctx, `SELECT count(*) FROM agent_audit_log`).Scan(&readonlyCount); err == nil {
		t.Fatal("skadia_readonly raw agent_audit_log query succeeded, want permission denied")
	}
}

func TestAgentAuditAnonymizedOnUserDeletion(t *testing.T) {
	db, userID := agentAuditTestDB(t)
	ctx := context.Background()

	sessionID := uuid.NewString()
	err := db.WithUserTx(ctx, userID, func(tx pgx.Tx) error {
		claimed, _, err := db.ClaimAuditSlot(ctx, tx, AgentAuditRow{
			UserID:       userID,
			SessionID:    sessionID,
			ToolCallID:   "call-delete",
			ToolName:     "update_activity_title",
			Args:         json.RawMessage(`{"title":"private"}`),
			ResultStatus: "pending",
		})
		if err != nil {
			return err
		}
		if !claimed {
			t.Fatal("initial claim returned claimed=false")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("claim audit slot: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE users SET deleted_at = now() WHERE id = $1`, userID); err != nil {
		t.Fatalf("mark deleted: %v", err)
	}
	if err := db.DeleteUserData(ctx, userID, nil); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire conn: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SET row_security = off`); err != nil {
		t.Fatalf("disable row_security for verification: %v", err)
	}
	var args string
	var detail *string
	var userNull bool
	if err := conn.QueryRow(ctx, `
		SELECT args::text, result_detail::text, user_id IS NULL
		FROM agent_audit_log
		WHERE session_id = $1
	`, sessionID).Scan(&args, &detail, &userNull); err != nil {
		t.Fatalf("query anonymized audit row: %v", err)
	}
	if args != "{}" || detail != nil || !userNull {
		t.Fatalf("audit row not anonymized: args=%s detail=%v userNull=%v", args, detail, userNull)
	}
}

func agentAuditTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	pool := testdb.Open(t)
	userID := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "agent-audit-"+userID[:8], "agent-audit-"+userID[:8]+"@example.com")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &DB{Pool: pool}, userID
}

func agentAuditReadonlyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	u, err := url.Parse(testdb.RequireSafeDSN(t))
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	u.User = url.UserPassword("skadia_readonly", "skadia_readonly_password")
	pool, err := pgxpool.New(context.Background(), u.String())
	if err != nil {
		t.Fatalf("connect readonly pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
