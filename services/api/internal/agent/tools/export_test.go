package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
)

func TestRegistryExportDefinitionsReadProfilesNeverIncludeWriteTools(t *testing.T) {
	reg := exportTestRegistry()
	prefs := &agent.ChatPreferences{ChatWriteToolsEnabled: true, ChatAllowedWriteTools: []string{"update_activity"}}

	for _, profile := range []ToolExportProfile{ToolExportLocalMCPRead, ToolExportRemoteMCPRead} {
		t.Run(string(profile), func(t *testing.T) {
			defs := reg.ExportDefinitions(profile, nil, prefs)
			if hasExportedTool(defs, "update_activity") {
				t.Fatalf("%s exported write tool: %#v", profile, defs)
			}
			read := exportedTool(defs, "read_activity")
			if read == nil || read.MutationMode != MutationNone {
				t.Fatalf("%s read tool = %#v, want mutation none", profile, read)
			}
		})
	}
}

func TestRegistryExportDefinitionsLocalMCPWritePrepareUsesPrepareOnly(t *testing.T) {
	reg := exportTestRegistry()
	prefs := &agent.ChatPreferences{ChatWriteToolsEnabled: true, ChatAllowedWriteTools: []string{"update_activity"}}

	defs := reg.ExportDefinitions(ToolExportLocalMCPWritePrepare, nil, prefs)
	write := exportedTool(defs, "update_activity")
	if write == nil {
		t.Fatalf("allowed write tool missing from %#v", defs)
	}
	if write.MutationMode != MutationPrepareOnly {
		t.Fatalf("mutation mode = %q, want prepare_only", write.MutationMode)
	}
	if hasExportedTool(defs, "delete_activity") {
		t.Fatalf("disallowed write tool exported: %#v", defs)
	}
}

func TestRegistryExportDefinitionsInternalChatUsesDirectConfirmedMode(t *testing.T) {
	reg := exportTestRegistry()
	prefs := &agent.ChatPreferences{ChatWriteToolsEnabled: true, ChatAllowedWriteTools: []string{"update_activity"}}

	write := exportedTool(reg.ExportDefinitions(ToolExportInternalChat, nil, prefs), "update_activity")
	if write == nil || write.MutationMode != MutationDirectInternalConfirmed {
		t.Fatalf("internal write export = %#v, want direct_internal_confirmed", write)
	}
}

func TestRegistryDefinitionsRespectGlobalWriteDisableAndAllowList(t *testing.T) {
	reg := exportTestRegistry()
	disabled := &agent.ChatPreferences{ChatWriteToolsEnabled: false, ChatAllowedWriteTools: []string{"update_activity"}}

	if hasLLMTool(reg.Definitions(nil, disabled), "update_activity") {
		t.Fatal("disabled write tool reached model definitions")
	}
	gotFilter := reg.AddEnabledWriteTools([]string{"read_activity"}, disabled)
	if len(gotFilter) != 1 || gotFilter[0] != "read_activity" {
		t.Fatalf("disabled AddEnabledWriteTools = %#v, want original filter only", gotFilter)
	}

	enabled := &agent.ChatPreferences{ChatWriteToolsEnabled: true, ChatAllowedWriteTools: []string{"update_activity"}}
	gotFilter = reg.AddEnabledWriteTools([]string{"read_activity"}, enabled)
	if len(gotFilter) != 1 || gotFilter[0] != "read_activity" {
		t.Fatalf("enabled AddEnabledWriteTools = %#v, want original filter only", gotFilter)
	}
	defs := reg.Definitions([]string{"read_activity", "update_activity"}, enabled)
	if len(defs) < 2 || defs[0].Name != "read_activity" || defs[1].Name != "update_activity" {
		t.Fatalf("definitions order = %#v, want filter order", defs)
	}
	if !hasLLMTool(defs, "update_activity") {
		t.Fatalf("allowed write tool missing from model definitions: %#v", defs)
	}
	if hasLLMTool(defs, "delete_activity") {
		t.Fatalf("disallowed write tool reached model definitions: %#v", defs)
	}
}

func TestRegistryExportDefinitionsDoNotLeakMetadataToLLMDefinitions(t *testing.T) {
	reg := exportTestRegistry()
	prefs := &agent.ChatPreferences{ChatWriteToolsEnabled: true, ChatAllowedWriteTools: []string{"update_activity"}}
	raw, err := json.Marshal(reg.Definitions(nil, prefs))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"permission", "audit_mode", "mutation_mode", "requires_confirmation"} {
		if jsonContains(raw, forbidden) {
			t.Fatalf("LLM tool definitions leaked %q in %s", forbidden, raw)
		}
	}
}

func exportTestRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "read_activity",
		Description: "Read activity",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Permission:  PermissionRead,
	})
	reg.Register(&Tool{
		Name:                 "update_activity",
		Description:          "Update activity",
		InputSchema:          json.RawMessage(`{"type":"object"}`),
		Permission:           PermissionWriteSafe,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		ExecuteWrite: func(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
			return map[string]any{"updated": true}, nil
		},
	})
	reg.Register(&Tool{
		Name:                 "delete_activity",
		Description:          "Delete activity",
		InputSchema:          json.RawMessage(`{"type":"object"}`),
		Permission:           PermissionWriteDestructive,
		RequiresConfirmation: true,
		AuditMode:            AuditWrite,
		ExecuteWrite: func(ctx context.Context, deps *WriteExecutionDeps, userID string, args json.RawMessage) (any, error) {
			return map[string]any{"deleted": true}, nil
		},
	})
	return reg
}

func exportedTool(defs []ExportedTool, name string) *ExportedTool {
	for i := range defs {
		if defs[i].Name == name {
			return &defs[i]
		}
	}
	return nil
}

func hasExportedTool(defs []ExportedTool, name string) bool {
	return exportedTool(defs, name) != nil
}

func hasLLMTool(defs []llmwire.ToolDefinition, name string) bool {
	for _, def := range defs {
		if def.Name == name {
			return true
		}
	}
	return false
}

func jsonContains(raw []byte, value string) bool {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return false
	}
	return containsStringValue(decoded, value)
}

func containsStringValue(v any, value string) bool {
	switch x := v.(type) {
	case string:
		return x == value
	case []any:
		for _, item := range x {
			if containsStringValue(item, value) {
				return true
			}
		}
	case map[string]any:
		for key, item := range x {
			if key == value || containsStringValue(item, value) {
				return true
			}
		}
	}
	return false
}
