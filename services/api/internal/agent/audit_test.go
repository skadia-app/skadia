package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestAuditWriterIdempotentReplay(t *testing.T) {
	db, userID := auditWriterTestDB(t)
	writer := NewAuditWriter(db)
	ctx := context.Background()
	row := database.AgentAuditRow{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-1",
		ToolName:   "update_activity_title",
		Args:       json.RawMessage(`{"title":"x"}`),
	}
	calls := 0
	detail, executed, err := writer.WithAuditSlot(ctx, row, func(context.Context, pgx.Tx) (json.RawMessage, error) {
		calls++
		return json.RawMessage(`{"ok":true}`), nil
	})
	if err != nil {
		t.Fatalf("WithAuditSlot first: %v", err)
	}
	if !executed || calls != 1 || string(detail) != `{"ok":true}` {
		t.Fatalf("unexpected first result detail=%s executed=%v calls=%d", detail, executed, calls)
	}

	detail, executed, err = writer.WithAuditSlot(ctx, row, func(context.Context, pgx.Tx) (json.RawMessage, error) {
		calls++
		return json.RawMessage(`{"ok":false}`), nil
	})
	if err != nil {
		t.Fatalf("WithAuditSlot replay: %v", err)
	}
	if executed || calls != 1 {
		t.Fatalf("replay executed=%v calls=%d, want cached without execution", executed, calls)
	}
	var cached map[string]bool
	if err := json.Unmarshal(detail, &cached); err != nil {
		t.Fatalf("unmarshal cached detail: %v", err)
	}
	if !cached["ok"] {
		t.Fatalf("unexpected cached detail: %s", detail)
	}
}

func TestAuditWriterRecordsError(t *testing.T) {
	db, userID := auditWriterTestDB(t)
	writer := NewAuditWriter(db)
	ctx := context.Background()
	row := database.AgentAuditRow{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-error",
		ToolName:   "update_activity_title",
		Args:       json.RawMessage(`{"title":"x"}`),
	}
	wantErr := errors.New("side effect failed")
	_, executed, err := writer.WithAuditSlot(ctx, row, func(context.Context, pgx.Tx) (json.RawMessage, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if !executed {
		t.Fatal("error path should have claimed and executed the slot")
	}

	err = db.WithUserTx(ctx, userID, func(tx pgx.Tx) error {
		claimed, existing, err := db.ClaimAuditSlot(ctx, tx, row)
		if err != nil {
			return err
		}
		if claimed {
			t.Fatal("post-error claim returned claimed=true")
		}
		if existing == nil || existing.ResultStatus != "error" {
			t.Fatalf("unexpected existing row: %#v", existing)
		}
		var detail map[string]string
		if err := json.Unmarshal(existing.ResultDetail, &detail); err != nil {
			t.Fatalf("unmarshal error detail: %v", err)
		}
		if detail["error"] != wantErr.Error() {
			t.Fatalf("unexpected error detail: %s", existing.ResultDetail)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify audit error row: %v", err)
	}
}

func auditWriterTestDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	pool := testdb.Open(t)
	userID := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "agent-audit-writer-"+userID[:8], "agent-audit-writer-"+userID[:8]+"@example.com")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &database.DB{Pool: pool}, userID
}
