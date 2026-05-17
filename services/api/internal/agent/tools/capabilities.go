package tools

import (
	"context"
	"encoding/json"
)

func AgentCapabilityManifestTool(reg *Registry) *Tool {
	return &Tool{
		Name:        "get_agent_capability_manifest",
		Description: "List the assistant capability areas, representative tools, and relevant limits. Use this for questions about what the assistant can do or which tools are available.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Permission:  PermissionRead,
		AuditMode:   AuditNone,
		Capability: &ToolCapability{
			Area:              "capabilities",
			PlannerSelectable: true,
		},
		ExecuteRead: func(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
			return executeAgentCapabilityManifest(reg), nil
		},
	}
}

func executeAgentCapabilityManifest(reg *Registry) map[string]any {
	capabilities := []map[string]any(nil)
	if reg != nil {
		capabilities = reg.CapabilityManifest()
	}
	return map[string]any{
		"status":       "ok",
		"capabilities": capabilities,
		"tool_loading": map[string]any{
			"default": "Tools are selected per request intent instead of loading every available tool.",
			"writes":  "Write tools are loaded only for clear write intents and enabled permissions.",
		},
	}
}
