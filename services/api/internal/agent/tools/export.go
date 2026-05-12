package tools

import (
	"encoding/json"

	"github.com/skadia-app/skadia/services/api/internal/agent"
)

type ToolExportProfile string

const (
	ToolExportInternalChat         ToolExportProfile = "internal_chat"
	ToolExportLocalMCPRead         ToolExportProfile = "local_mcp_read"
	ToolExportLocalMCPWritePrepare ToolExportProfile = "local_mcp_write_prepare"
	ToolExportRemoteMCPRead        ToolExportProfile = "remote_mcp_read"
)

type MutationMode string

const (
	MutationNone                    MutationMode = "none"
	MutationPrepareOnly             MutationMode = "prepare_only"
	MutationDirectInternalConfirmed MutationMode = "direct_internal_confirmed"
)

type ExportedTool struct {
	Name                 string          `json:"name"`
	Description          string          `json:"description"`
	InputSchema          json.RawMessage `json:"input_schema"`
	Permission           Permission      `json:"permission"`
	AuditMode            AuditMode       `json:"audit_mode"`
	RequiresConfirmation bool            `json:"requires_confirmation"`
	MutationMode         MutationMode    `json:"mutation_mode"`
}

func (r *Registry) ExportDefinitions(profile ToolExportProfile, filter []string, prefs *agent.ChatPreferences) []ExportedTool {
	filterSet := namesSet(filter)
	allowedWrites := allowedWriteToolSet(prefs)
	defs := make([]ExportedTool, 0, len(r.tools))
	for _, name := range r.sortedNames() {
		t := r.tools[name]
		if t == nil || !nameAllowed(filterSet, name) {
			continue
		}
		mode, ok := exportMutationMode(profile, t, prefs, allowedWrites)
		if !ok {
			continue
		}
		defs = append(defs, ExportedTool{
			Name:                 t.Name,
			Description:          t.Description,
			InputSchema:          append(json.RawMessage(nil), t.InputSchema...),
			Permission:           t.Permission,
			AuditMode:            t.AuditMode,
			RequiresConfirmation: t.RequiresConfirmation,
			MutationMode:         mode,
		})
	}
	return defs
}

func exportMutationMode(profile ToolExportProfile, t *Tool, prefs *agent.ChatPreferences, allowedWrites map[string]bool) (MutationMode, bool) {
	if t.Permission == PermissionRead {
		return MutationNone, true
	}
	if prefs == nil || !prefs.ChatWriteToolsEnabled || !allowedWrites[t.Name] {
		return "", false
	}
	switch profile {
	case ToolExportInternalChat:
		return MutationDirectInternalConfirmed, true
	case ToolExportLocalMCPWritePrepare:
		return MutationPrepareOnly, true
	default:
		return "", false
	}
}
