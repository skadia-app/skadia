package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/agent"
)

func TestRegistryExecuteRead(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "read_activity",
		Description: "Read an activity",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
		Permission:  PermissionRead,
		ExecuteRead: func(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
			if userID != "user-1" {
				t.Fatalf("unexpected userID: %s", userID)
			}
			return map[string]string{"ok": "yes"}, nil
		},
	})

	result, err := reg.ExecuteRead(context.Background(), nil, "user-1", "read_activity", json.RawMessage(`{"id":"a1"}`))
	if err != nil {
		t.Fatalf("ExecuteRead returned error: %v", err)
	}
	if result.(map[string]string)["ok"] != "yes" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRegistryExecuteReadUnknownTool(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.ExecuteRead(context.Background(), nil, "user-1", "missing", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error, got %v", err)
	}
}

func TestRegistryExecuteReadRejectsWriteTool(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "update_activity",
		Description: "Update an activity",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Permission:  PermissionWriteSafe,
		ExecuteRead: func(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
			return "should not run", nil
		},
	})
	_, err := reg.ExecuteRead(context.Background(), nil, "user-1", "update_activity", nil)
	if !errors.Is(err, ErrWriteToolRequiresEnvelope) {
		t.Fatalf("expected ErrWriteToolRequiresEnvelope, got %v", err)
	}
}

func TestRegistryValidateSchemas(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "broken",
		Description: "Broken schema",
		InputSchema: json.RawMessage(`{"type": 12}`),
		Permission:  PermissionRead,
	})

	err := reg.ValidateSchemas()
	if err == nil || !strings.Contains(err.Error(), "compile input schema for broken") {
		t.Fatalf("expected schema compile error, got %v", err)
	}
}

func TestRegistryDefinitionsFilterWriteTools(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "read_activity",
		Description: "Read an activity",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
		Permission:  PermissionRead,
	})
	reg.Register(&Tool{
		Name:                 "update_activity",
		Description:          "Update an activity",
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
	})

	defs := reg.Definitions([]string{"read_activity", "update_activity"}, &agent.ChatPreferences{
		ChatWriteToolsEnabled: false,
		ChatAllowedWriteTools: []string{"update_activity"},
	})
	if len(defs) != 1 || defs[0].Name != "read_activity" {
		t.Fatalf("expected only read tool, got %#v", defs)
	}

	defs = reg.Definitions([]string{"read_activity", "update_activity"}, &agent.ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"update_activity"},
	})
	if len(defs) != 2 || defs[0].Name != "read_activity" || defs[1].Name != "update_activity" {
		t.Fatalf("expected read and allowed write tool, got %#v", defs)
	}
}

func TestRegistryAddEnabledWriteTools(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{Name: "read_activity", InputSchema: json.RawMessage(`{"type":"object"}`), Permission: PermissionRead})
	reg.Register(&Tool{Name: "delete_activity", InputSchema: json.RawMessage(`{"type":"object"}`), Permission: PermissionWriteDestructive})
	reg.Register(&Tool{Name: "update_activity", InputSchema: json.RawMessage(`{"type":"object"}`), Permission: PermissionWriteSafe})

	filter := reg.AddEnabledWriteTools([]string{"read_activity"}, &agent.ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"update_activity"},
	})
	if strings.Join(filter, ",") != "read_activity,update_activity" {
		t.Fatalf("unexpected filter: %#v", filter)
	}
}

func TestAgentToolRuntimePublicToolCallArgsFailsClosedForUnknownTools(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{Name: "read_activity", InputSchema: json.RawMessage(`{"type":"object"}`), Permission: PermissionRead})
	runtime := NewAgentToolRuntime(reg, nil, nil)

	if _, include := runtime.PublicToolCallArgs("missing_tool", json.RawMessage(`{"secret":"value"}`)); include {
		t.Fatal("PublicToolCallArgs exposed args for an unknown tool")
	}
	if args, include := runtime.PublicToolCallArgs("read_activity", json.RawMessage(`{"id":"a1"}`)); !include || string(args) != `{"id":"a1"}` {
		t.Fatalf("PublicToolCallArgs for read tool = include %v args %s", include, args)
	}
}

func TestRegistryLocalMCPReadDefinitionsOmitWriteTools(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "read_activity",
		Description: "Read activity",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Permission:  PermissionRead,
	})
	reg.Register(&Tool{
		Name:                 "update_activity",
		Description:          "Update an activity",
		InputSchema:          json.RawMessage(`{"type":"object"}`),
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		RateLimitPerHour:     10,
		IsUserOwned:          true,
	})

	raw, err := json.Marshal(reg.ExportDefinitions(ToolExportLocalMCPRead, nil, &agent.ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"update_activity"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "update_activity") {
		t.Fatalf("read MCP projection exported write tool: %s", raw)
	}
}

func TestCanonicalParamsPorted(t *testing.T) {
	if len(SportTypes) != 79 {
		t.Fatalf("unexpected sport type count: %d", len(SportTypes))
	}
	if SportTypes[0] != "running" || SportTypes[len(SportTypes)-1] != "canoeing" {
		t.Fatalf("sport type order drifted: first=%s last=%s", SportTypes[0], SportTypes[len(SportTypes)-1])
	}
	if ModifierFilterParams["is_virtual"]["description"] == "" {
		t.Fatalf("modifier params missing is_virtual description")
	}
}
