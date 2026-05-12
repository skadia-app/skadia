package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestRegistryRejectsWriteToolDirectReadExecution(t *testing.T) {
	reg := NewRegistry()
	reg.Register(testWriteTool("update_activity_title", nil))
	_, err := reg.ExecuteRead(context.Background(), nil, "user-1", "update_activity_title", json.RawMessage(`{}`))
	if !errors.Is(err, ErrWriteToolRequiresEnvelope) {
		t.Fatalf("ExecuteRead err = %v, want ErrWriteToolRequiresEnvelope", err)
	}
}

func TestExecutionEnvelopeWritePreferencesGate(t *testing.T) {
	reg := NewRegistry()
	reg.Register(testWriteTool("update_activity_title", nil))
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(reg, &Deps{
		DB:      db,
		Audit:   agent.NewAuditWriter(db),
		Confirm: agent.NewConfirmationManager(time.Minute),
		Preferences: &agent.PreferenceLoader{Defaults: agent.ChatPreferences{
			ChatWriteToolsEnabled: false,
		}},
	})
	_, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-1",
		ToolName:   "update_activity_title",
		Args:       json.RawMessage(`{"id":"a1"}`),
	})
	if err == nil {
		t.Fatal("PrepareConfirmation returned nil error for disabled write tools")
	}
}

func TestExecutionEnvelopePrepareAndExecuteConfirmed(t *testing.T) {
	calls := 0
	reg := NewRegistry()
	reg.Register(testWriteTool("update_activity_title", func(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
		calls++
		return map[string]any{"updated": true}, nil
	}))
	db, userID := executionEnvelopeTestDB(t)
	confirm := agent.NewConfirmationManager(time.Minute)
	envelope := NewExecutionEnvelope(reg, &Deps{
		DB:      db,
		Audit:   agent.NewAuditWriter(db),
		Confirm: confirm,
		Preferences: &agent.PreferenceLoader{Defaults: agent.ChatPreferences{
			ChatWriteToolsEnabled: true,
			ChatAllowedWriteTools: []string{"update_activity_title"},
		}},
	})
	req := ToolExecutionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-1",
		ToolName:   "update_activity_title",
		Args:       json.RawMessage(`{"id":"a1"}`),
	}
	prepared, err := envelope.PrepareConfirmation(context.Background(), req)
	if err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	if prepared.Summary == "" || prepared.Cached != nil {
		t.Fatalf("unexpected prepared confirmation: %#v", prepared)
	}
	confirmEnvelopeTool(t, envelope, req.SessionID, req.ToolCallID)
	result, err := envelope.ExecuteConfirmed(context.Background(), req)
	if err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("write calls = %d, want 1", calls)
	}
	if result.(map[string]any)["updated"] != true {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestExecutionEnvelopeExecuteConfirmedRequiresSkadiaConfirmation(t *testing.T) {
	calls := 0
	reg := NewRegistry()
	reg.Register(testWriteTool("update_activity_title", func(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
		calls++
		return map[string]any{"updated": true}, nil
	}))
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(reg, &Deps{
		DB:      db,
		Audit:   agent.NewAuditWriter(db),
		Confirm: agent.NewConfirmationManager(time.Minute),
		Preferences: &agent.PreferenceLoader{Defaults: agent.ChatPreferences{
			ChatWriteToolsEnabled: true,
			ChatAllowedWriteTools: []string{"update_activity_title"},
		}},
	})
	req := ToolExecutionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-forged-confirm",
		ToolName:   "update_activity_title",
		Args:       json.RawMessage(`{"id":"a1"}`),
		Confirmed:  true,
	}
	if _, err := envelope.PrepareConfirmation(context.Background(), req); err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	if _, err := envelope.ExecuteConfirmed(context.Background(), req); err == nil {
		t.Fatal("ExecuteConfirmed accepted request confirmed flag without Skadia confirmation")
	}
	if calls != 0 {
		t.Fatalf("write calls = %d, want 0", calls)
	}
}

func TestExecutionEnvelopeExecuteConfirmedIgnoresConfirmationArgsOverrides(t *testing.T) {
	var executedArgs string
	reg := NewRegistry()
	reg.Register(testWriteTool("update_activity_title", func(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
		executedArgs = string(args)
		return map[string]any{"updated": true}, nil
	}))
	db, userID := executionEnvelopeTestDB(t)
	confirm := agent.NewConfirmationManager(time.Minute)
	envelope := NewExecutionEnvelope(reg, &Deps{
		DB:      db,
		Audit:   agent.NewAuditWriter(db),
		Confirm: confirm,
		Preferences: &agent.PreferenceLoader{Defaults: agent.ChatPreferences{
			ChatWriteToolsEnabled: true,
			ChatAllowedWriteTools: []string{"update_activity_title"},
		}},
	})
	req := ToolExecutionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-override",
		ToolName:   "update_activity_title",
		Args:       json.RawMessage(`{"id":"original"}`),
	}
	if _, err := envelope.PrepareConfirmation(context.Background(), req); err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	if !confirm.Resolve(req.SessionID, req.ToolCallID, agent.ConfirmResponse{
		Confirmed:     true,
		Source:        "user",
		ArgsOverrides: json.RawMessage(`{"id":"forged"}`),
	}) {
		t.Fatal("Resolve returned false")
	}
	if _, err := envelope.ExecuteConfirmed(context.Background(), req); err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}
	var gotArgs map[string]string
	if err := json.Unmarshal([]byte(executedArgs), &gotArgs); err != nil {
		t.Fatalf("parse executed args: %v", err)
	}
	if gotArgs["id"] != "original" {
		t.Fatalf("executed args = %s, want original previewed args", executedArgs)
	}
}

func TestExecutionEnvelopeRecordDeclined(t *testing.T) {
	reg := NewRegistry()
	reg.Register(testWriteTool("update_activity_title", nil))
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(reg, &Deps{
		DB:      db,
		Audit:   agent.NewAuditWriter(db),
		Confirm: agent.NewConfirmationManager(time.Minute),
		Preferences: &agent.PreferenceLoader{Defaults: agent.ChatPreferences{
			ChatWriteToolsEnabled: true,
			ChatAllowedWriteTools: []string{"update_activity_title"},
		}},
	})
	req := ToolExecutionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-decline",
		ToolName:   "update_activity_title",
		Args:       json.RawMessage(`{"id":"a1"}`),
	}
	if _, err := envelope.PrepareConfirmation(context.Background(), req); err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	if err := envelope.RecordDeclined(context.Background(), req, "user"); err != nil {
		t.Fatalf("RecordDeclined: %v", err)
	}
	err := db.WithUserTx(context.Background(), userID, func(tx pgx.Tx) error {
		row, err := loadAuditSlot(context.Background(), tx, userID, req.SessionID, req.ToolCallID)
		if err != nil {
			return err
		}
		if row.ResultStatus != "declined" {
			t.Fatalf("status = %s, want declined", row.ResultStatus)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify declined audit row: %v", err)
	}
}

func TestAgentRuntimeConfirmRequestOmitsRawArgsWhenPreviewExists(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:                 "update_activity_title",
		Description:          "Update an activity title",
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"api_key":{"type":"string"}},"required":["id"],"additionalProperties":false}`),
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		ExecuteWrite: func(context.Context, *WriteExecutionDeps, string, json.RawMessage) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	})
	db, userID := executionEnvelopeTestDB(t)
	confirm := agent.NewConfirmationManager(time.Minute)
	runtime := NewAgentToolRuntime(reg, &Deps{
		DB:      db,
		Audit:   agent.NewAuditWriter(db),
		Confirm: confirm,
		Preferences: &agent.PreferenceLoader{Defaults: agent.ChatPreferences{
			ChatWriteToolsEnabled: true,
			ChatAllowedWriteTools: []string{"update_activity_title"},
		}},
	}, nil)
	sink := &toolsCaptureSink{eventsCh: make(chan agent.SSEEvent, 4)}

	done := make(chan error, 1)
	go func() {
		_, err := runtime.Execute(context.Background(), agent.ToolRuntimeRequest{
			UserID:     userID,
			SessionID:  "00000000-0000-0000-0000-000000000111",
			MessageID:  "msg-1",
			ToolCallID: "call-confirm-sse",
			ToolName:   "update_activity_title",
			Args:       json.RawMessage(`{"id":"a1","api_key":"secret"}`),
		}, sink)
		done <- err
	}()

	var evt agent.SSEEvent
	select {
	case evt = <-sink.eventsCh:
	case err := <-done:
		t.Fatalf("runtime returned before confirmation request: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for confirmation request")
	}
	if !confirm.Resolve("00000000-0000-0000-0000-000000000111", "call-confirm-sse", agent.ConfirmResponse{Confirmed: false, Source: "user"}) {
		t.Fatal("Resolve returned false")
	}
	if err := <-done; err != nil {
		t.Fatalf("runtime Execute: %v", err)
	}

	if evt.Type != agent.EventToolConfirmRequest {
		t.Fatalf("event type = %s, want confirm request", evt.Type)
	}
	if len(evt.Args) > 0 {
		t.Fatalf("confirm request leaked raw args: %s", evt.Args)
	}
	rawPreview, _ := json.Marshal(evt.Preview)
	if !strings.Contains(string(rawPreview), "[redacted]") || strings.Contains(string(rawPreview), "secret") {
		t.Fatalf("preview = %s, want redacted sanitized preview", rawPreview)
	}
}

func TestExecutionEnvelopePreviewAuditsWithoutConfirmation(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "dry_run_rule",
		Description: "Preview rule",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}`),
		Permission:  PermissionRead,
		AuditMode:   AuditPreview,
		ExecutePreview: func(ctx context.Context, deps *PreviewExecutionDeps, userID string, args json.RawMessage) (any, error) {
			return map[string]any{"preview": true}, nil
		},
	})
	db, userID := executionEnvelopeTestDB(t)
	envelope := NewExecutionEnvelope(reg, &Deps{DB: db, Audit: agent.NewAuditWriter(db)})
	result, err := envelope.ExecutePreview(context.Background(), ToolExecutionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-preview",
		ToolName:   "dry_run_rule",
		Args:       json.RawMessage(`{"id":"r1"}`),
	})
	if err != nil {
		t.Fatalf("ExecutePreview: %v", err)
	}
	if result.(map[string]any)["preview"] != true {
		t.Fatalf("unexpected preview result: %#v", result)
	}
}

type toolsCaptureSink struct {
	events   []agent.SSEEvent
	eventsCh chan agent.SSEEvent
	raw      []string
}

func (s *toolsCaptureSink) Send(event agent.SSEEvent) error {
	s.events = append(s.events, event)
	if s.eventsCh != nil {
		s.eventsCh <- event
	}
	return nil
}

func (s *toolsCaptureSink) WriteRaw(frame string) error {
	s.raw = append(s.raw, frame)
	return nil
}

func (s *toolsCaptureSink) OnChunk(func(string)) {}

func TestExecutionEnvelopePreviewCannotWriteThroughReadonlyDeps(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.RequireSafeDSN(t)
	db, err := database.New(ctx, dsn)
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(db.Close)
	if err := db.OpenReadonly(ctx, readonlyExecutionDSN(t, dsn)); err != nil {
		t.Fatalf("OpenReadonly: %v", err)
	}
	userID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "preview-write-"+userID[:8], "preview-write-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID) })

	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "dry_run_forged_write",
		Description: "Preview forged write",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Permission:  PermissionRead,
		AuditMode:   AuditPreview,
		ExecutePreview: func(ctx context.Context, deps *PreviewExecutionDeps, userID string, args json.RawMessage) (any, error) {
			_, err := deps.Readonly.Exec(ctx, `UPDATE users SET username = 'forged-preview' WHERE id = $1`, userID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"forged": true}, nil
		},
	})
	envelope := NewExecutionEnvelope(reg, &Deps{DB: db, Readonly: db.ReadonlyPool, Audit: agent.NewAuditWriter(db)})
	_, err = envelope.ExecutePreview(ctx, ToolExecutionRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-forged-preview",
		ToolName:   "dry_run_forged_write",
		Args:       json.RawMessage(`{}`),
	})
	if err == nil {
		t.Fatal("preview tool wrote through readonly deps")
	}
	var username string
	if err := db.Pool.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&username); err != nil {
		t.Fatalf("select username: %v", err)
	}
	if username == "forged-preview" {
		t.Fatal("preview path mutated user data")
	}
}

func TestAgentRuntimeDispatchesPreviewToolBeforeReadExecutor(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "dry_run_rule",
		Description: "Preview rule",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}`),
		Permission:  PermissionRead,
		AuditMode:   AuditPreview,
		ExecutePreview: func(ctx context.Context, deps *PreviewExecutionDeps, userID string, args json.RawMessage) (any, error) {
			return map[string]any{"preview": true}, nil
		},
	})
	db, userID := executionEnvelopeTestDB(t)
	runtime := NewAgentToolRuntime(reg, &Deps{DB: db, Audit: agent.NewAuditWriter(db)}, nil)
	result, err := runtime.Execute(context.Background(), agent.ToolRuntimeRequest{
		UserID:     userID,
		SessionID:  uuid.NewString(),
		ToolCallID: "call-preview-runtime",
		ToolName:   "dry_run_rule",
		Args:       json.RawMessage(`{"id":"r1"}`),
	}, nil)
	if err != nil {
		t.Fatalf("runtime Execute preview: %v", err)
	}
	if got := result.Result.(map[string]any)["preview"]; got != true {
		t.Fatalf("preview result = %#v, want true", got)
	}
}

func testWriteTool(name string, exec func(context.Context, *WriteExecutionDeps, string, json.RawMessage) (any, error)) *Tool {
	if exec == nil {
		exec = func(context.Context, *WriteExecutionDeps, string, json.RawMessage) (any, error) {
			return map[string]any{"ok": true}, nil
		}
	}
	return &Tool{
		Name:                 name,
		Description:          "Update an activity title",
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}`),
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		ExecuteWrite:         exec,
	}
}

func confirmEnvelopeTool(t *testing.T, envelope *ExecutionEnvelope, sessionID, toolCallID string) {
	t.Helper()
	if envelope == nil || envelope.Deps == nil || envelope.Deps.Confirm == nil {
		t.Fatal("envelope confirmation manager is not configured")
	}
	if !envelope.Deps.Confirm.Resolve(sessionID, toolCallID, agent.ConfirmResponse{Confirmed: true, Source: "user"}) {
		t.Fatalf("Resolve(%s, %s) returned false", sessionID, toolCallID)
	}
}

func readonlyExecutionDSN(t *testing.T, dsn string) string {
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

func executionEnvelopeTestDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	pool := testdb.Open(t)
	userID := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "execution-envelope-"+userID[:8], "execution-envelope-"+userID[:8]+"@example.com")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &database.DB{Pool: pool}, userID
}
