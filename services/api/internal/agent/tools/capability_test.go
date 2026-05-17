package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/capabilities"
)

func TestToolDefinitionsFilterMissingCapabilities(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "core_read",
		Description: "Core read",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Permission:  PermissionRead,
		ExecuteRead: func(context.Context, *Deps, string, json.RawMessage) (any, error) {
			return map[string]string{"status": "ok"}, nil
		},
	})
	reg.Register(&Tool{
		Name:                 "search_photos_by_content",
		Description:          "Photo embeddings",
		InputSchema:          json.RawMessage(`{"type":"object"}`),
		Permission:           PermissionRead,
		RequiredCapabilities: []string{capabilities.KeyPhotoEmbeddings},
		ExecuteRead: func(context.Context, *Deps, string, json.RawMessage) (any, error) {
			return map[string]string{"status": "ok"}, nil
		},
	})

	runtime := NewAgentToolRuntime(reg, &Deps{CapabilityProvider: coreCapabilitiesForTest}, nil)
	defs := runtime.Definitions(nil, nil)

	if hasLLMTool(defs, "search_photos_by_content") {
		t.Fatalf("missing-capability tool reached definitions: %#v", defs)
	}
	if !hasLLMTool(defs, "core_read") {
		t.Fatalf("core tool missing from definitions: %#v", defs)
	}
}

func TestToolRuntimeRejectsMissingCapabilityExecution(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:                 "search_photos_by_content",
		Description:          "Photo embeddings",
		InputSchema:          json.RawMessage(`{"type":"object"}`),
		Permission:           PermissionRead,
		RequiredCapabilities: []string{capabilities.KeyPhotoEmbeddings},
		ExecuteRead: func(context.Context, *Deps, string, json.RawMessage) (any, error) {
			return map[string]string{"status": "ok"}, nil
		},
	})
	runtime := NewAgentToolRuntime(reg, &Deps{CapabilityProvider: coreCapabilitiesForTest}, nil)

	_, err := runtime.Execute(context.Background(), agent.ToolRuntimeRequest{
		UserID:     "user-1",
		SessionID:  "session-1",
		ToolCallID: "call-1",
		ToolName:   "search_photos_by_content",
		Args:       json.RawMessage(`{}`),
	}, nil)
	var featureErr FeatureUnavailableError
	if !errors.As(err, &featureErr) {
		t.Fatalf("Execute err = %v, want FeatureUnavailableError", err)
	}
	if featureErr.Capability != capabilities.KeyPhotoEmbeddings || featureErr.Reason != capabilities.ReasonCoreProfile {
		t.Fatalf("FeatureUnavailableError = %#v", featureErr)
	}
}

func TestRoutePreviewToolsRequireRoutingAndBRouterCapabilities(t *testing.T) {
	reg := NewRegistry()
	RegisterRouteTools(reg)
	runtime := NewAgentToolRuntime(reg, &Deps{CapabilityProvider: routingUnavailableCapabilitiesForTest}, nil)

	defs := runtime.Definitions(nil, nil)
	for _, name := range []string{"plan_route", "plan_route_between"} {
		if hasLLMTool(defs, name) {
			t.Fatalf("%s reached definitions without available routing/BRouter: %#v", name, defs)
		}
		_, err := runtime.Execute(context.Background(), agent.ToolRuntimeRequest{
			UserID:     "user-1",
			SessionID:  "session-1",
			ToolCallID: "call-" + name,
			ToolName:   name,
			Args:       json.RawMessage(`{}`),
		}, nil)
		var featureErr FeatureUnavailableError
		if !errors.As(err, &featureErr) {
			t.Fatalf("Execute(%s) err = %v, want FeatureUnavailableError", name, err)
		}
		if featureErr.Capability != capabilities.KeyRouting && featureErr.Capability != capabilities.KeyBRouter {
			t.Fatalf("Execute(%s) unavailable capability = %q, want routing or brouter", name, featureErr.Capability)
		}
	}
}

func TestRoutePreviewToolsAllowBadRouterOnlyRouting(t *testing.T) {
	reg := NewRegistry()
	RegisterAll(reg)
	runtime := NewAgentToolRuntime(reg, &Deps{CapabilityProvider: badRouterOnlyCapabilitiesForTest}, nil)

	defs := runtime.Definitions([]string{"list_routing_profiles", "plan_route", "plan_route_between"}, nil)
	for _, name := range []string{"list_routing_profiles", "plan_route", "plan_route_between"} {
		if !hasLLMTool(defs, name) {
			t.Fatalf("%s missing for BadRouter-only routing: %#v", name, defs)
		}
	}
}

func TestRoutingProfileVariantWriteToolRequiresRoutingCapability(t *testing.T) {
	reg := NewRegistry()
	RegisterAll(reg)
	runtime := NewAgentToolRuntime(reg, &Deps{CapabilityProvider: routingUnavailableCapabilitiesForTest}, nil)

	filter := runtime.AddEnabledWriteTools([]string{"list_routing_profiles"}, &agent.ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"create_routing_profile_variant"},
	})
	if containsRoutingProfileWriteTestString(filter, "create_routing_profile_variant") {
		t.Fatalf("create_routing_profile_variant exposed without routing capability: %#v", filter)
	}
}

func TestToolExportIncludesRequiredCapabilitiesMetadata(t *testing.T) {
	reg := exportTestRegistry()
	tool, _ := reg.Get("read_activity")
	tool.RequiredCapabilities = []string{capabilities.KeyGeoPOIEnrichment}

	def := exportedTool(reg.ExportDefinitions(ToolExportRemoteMCPRead, []string{"read_activity"}, nil), "read_activity")
	if def == nil {
		t.Fatal("read_activity export missing")
	}
	if len(def.RequiredCapabilities) != 1 || def.RequiredCapabilities[0] != capabilities.KeyGeoPOIEnrichment {
		t.Fatalf("required capabilities = %#v", def.RequiredCapabilities)
	}
}

func coreCapabilitiesForTest(context.Context) capabilities.Capabilities {
	return capabilities.BuildFromRuntime(capabilities.DetectInput{
		DeploymentProfile: "core",
		StorageMode:       "local",
		StorageBackend:    "local",
	}, capabilities.RuntimeProbeState{BRouter: true})
}

func routingUnavailableCapabilitiesForTest(context.Context) capabilities.Capabilities {
	return capabilities.BuildFromRuntime(capabilities.DetectInput{
		DeploymentProfile: "core",
		StorageMode:       "local",
		StorageBackend:    "local",
	}, capabilities.RuntimeProbeState{})
}

func badRouterOnlyCapabilitiesForTest(context.Context) capabilities.Capabilities {
	return capabilities.BuildFromRuntime(capabilities.DetectInput{
		DeploymentProfile: "full",
		StorageMode:       "local",
		StorageBackend:    "local",
	}, capabilities.RuntimeProbeState{
		BadRouterTiles:    true,
		BadRouterProfiles: true,
	})
}
