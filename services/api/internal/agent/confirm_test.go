package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestConfirmationManagerRegisterResolve(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := NewConfirmationManager(time.Minute)

	ch, err := manager.Register(ctx, &PendingConfirm{
		SessionID:  "session-1",
		ToolCallID: "call-1",
		Tool:       "update_activity_title",
		Args:       json.RawMessage(`{"title":"x"}`),
		Summary:    "Titel ändern",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !manager.Resolve("session-1", "call-1", ConfirmResponse{Confirmed: true, Source: "user"}) {
		t.Fatal("Resolve returned false")
	}
	resp := <-ch
	if !resp.Confirmed || resp.Source != "user" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if manager.Resolve("session-1", "call-1", ConfirmResponse{Confirmed: false, Source: "user"}) {
		t.Fatal("second Resolve returned true")
	}
}

func TestConfirmationManagerDuplicateRegister(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := NewConfirmationManager(time.Minute)
	p := &PendingConfirm{SessionID: "session-1", ToolCallID: "call-1"}
	if _, err := manager.Register(ctx, p); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if _, err := manager.Register(ctx, &PendingConfirm{SessionID: "session-1", ToolCallID: "call-1"}); err == nil {
		t.Fatal("duplicate Register returned nil error")
	}
}

func TestConfirmationManagerTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := NewConfirmationManager(20 * time.Millisecond)
	ch, err := manager.Register(ctx, &PendingConfirm{SessionID: "session-1", ToolCallID: "call-1"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	select {
	case resp := <-ch:
		if resp.Confirmed || resp.Source != "timeout" {
			t.Fatalf("unexpected timeout response: %#v", resp)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timed out waiting for confirmation timeout")
	}
}

func TestConfirmationManagerConsumeResolvedAction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := NewConfirmationManager(time.Minute)
	ch, err := manager.Register(ctx, &PendingConfirm{
		ActionID:   "audit:1",
		SessionID:  "session-1",
		ToolCallID: "call-1",
		Args:       json.RawMessage(`{"id":"original"}`),
		Metadata:   json.RawMessage(`{"caller":{"transport":"internal_chat"}}`),
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !manager.Resolve("session-1", "call-1", ConfirmResponse{Confirmed: true, Source: "user"}) {
		t.Fatal("Resolve returned false")
	}
	<-ch
	resp, args, ok := manager.ConsumeConfirmedAction("audit:1")
	if !ok || !resp.Confirmed || string(args) != `{"id":"original"}` {
		t.Fatalf("ConsumeConfirmedAction = %#v %s %v, want confirmed original args", resp, args, ok)
	}
	if _, _, ok := manager.ConsumeConfirmedAction("audit:1"); ok {
		t.Fatal("second ConsumeConfirmedAction returned true")
	}
}

func TestConfirmationManagerPrunesExpiredResolvedActions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := NewConfirmationManager(20 * time.Millisecond)
	if _, err := manager.Register(ctx, &PendingConfirm{
		ActionID:   "audit:1",
		SessionID:  "session-1",
		ToolCallID: "call-1",
		ExpiresAt:  time.Now().Add(20 * time.Millisecond),
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !manager.Resolve("session-1", "call-1", ConfirmResponse{Confirmed: true, Source: "user"}) {
		t.Fatal("Resolve returned false")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := manager.ResolvedAction("audit:1"); ok {
		t.Fatal("ResolvedAction returned expired confirmation")
	}
}
