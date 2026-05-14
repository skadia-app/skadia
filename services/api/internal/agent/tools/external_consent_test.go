package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
)

func TestExternalReadConsentRequiresServerIssuedConfirmedToken(t *testing.T) {
	reg := NewRegistry()
	executed := 0
	reg.Register(&Tool{
		Name:        "external_place_lookup",
		Description: "External place lookup",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"query":{"type":"string"}},
			"required":["query"],
			"additionalProperties":false
		}`),
		Permission: PermissionRead,
		ExternalRead: &ExternalReadRequirement{
			Policy:  ExternalReadPolicyManualConfirm,
			Purpose: "external_geocoding",
		},
		ExecuteRead: func(ctx context.Context, deps *Deps, userID string, args json.RawMessage) (any, error) {
			executed++
			var in struct {
				Query        string `json:"query"`
				ConsentToken string `json:"consent_token"`
				Confirmed    bool   `json:"confirmed"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.ConsentToken != "" || in.Confirmed {
				t.Fatalf("executor received consent control fields: %#v", in)
			}
			return map[string]any{"status": "ok", "query": in.Query}, nil
		},
	})
	consent := NewExternalReadConsentManager(time.Minute)
	runtime := NewAgentToolRuntime(reg, &Deps{ExternalConsent: consent}, nil)

	first := executeGuardrailTool(t, runtime, "user-a", "external_place_lookup", json.RawMessage(`{"query":"Synthetic Place"}`))
	assertToolStatus(t, first, "requires_user_confirmation")
	request := consentRequestFromResult(t, first)
	if request.Token == "" {
		t.Fatalf("consent token missing: %#v", request)
	}
	if executed != 0 {
		t.Fatalf("external tool executed before confirmation")
	}

	selfApproved := executeGuardrailTool(t, runtime, "user-a", "external_place_lookup", json.RawMessage(`{
		"query":"Synthetic Place",
		"consent_token":"model-made-token",
		"confirmed":true
	}`))
	assertToolStatus(t, selfApproved, "requires_user_confirmation")
	if executed != 0 {
		t.Fatalf("external tool executed with model-supplied self-approval")
	}

	unconfirmedServerToken := executeGuardrailTool(t, runtime, "user-a", "external_place_lookup", json.RawMessage(`{
		"query":"Synthetic Place",
		"consent_token":"`+request.Token+`"
	}`))
	assertToolStatus(t, unconfirmedServerToken, "requires_user_confirmation")
	if executed != 0 {
		t.Fatalf("external tool executed with unconfirmed server token")
	}

	if !consent.Confirm("user-a", request.Token) {
		t.Fatal("Confirm returned false for server-issued token")
	}
	confirmed := executeGuardrailTool(t, runtime, "user-a", "external_place_lookup", json.RawMessage(`{
		"query":"Synthetic Place",
		"consent_token":"`+request.Token+`"
	}`))
	assertToolStatus(t, confirmed, "ok")
	if executed != 1 {
		t.Fatalf("external tool executed %d times, want 1", executed)
	}

	reused := executeGuardrailTool(t, runtime, "user-a", "external_place_lookup", json.RawMessage(`{
		"query":"Synthetic Place",
		"consent_token":"`+request.Token+`"
	}`))
	assertToolStatus(t, reused, "requires_user_confirmation")
	if executed != 1 {
		t.Fatalf("external tool executed after one-time token reuse")
	}
}

func TestExternalReadConsentRejectsTokenForDifferentUserOrArgs(t *testing.T) {
	reg := NewRegistry()
	executed := 0
	reg.Register(&Tool{
		Name:        "external_weather_lookup",
		Description: "External weather lookup",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"query":{"type":"string"}},
			"required":["query"],
			"additionalProperties":false
		}`),
		Permission: PermissionRead,
		ExternalRead: &ExternalReadRequirement{
			Policy:  ExternalReadPolicyManualConfirm,
			Purpose: "live_weather",
		},
		ExecuteRead: func(context.Context, *Deps, string, json.RawMessage) (any, error) {
			executed++
			return map[string]any{"status": "ok"}, nil
		},
	})
	consent := NewExternalReadConsentManager(time.Minute)
	runtime := NewAgentToolRuntime(reg, &Deps{ExternalConsent: consent}, nil)

	first := executeGuardrailTool(t, runtime, "user-a", "external_weather_lookup", json.RawMessage(`{"query":"synthetic forecast window"}`))
	request := consentRequestFromResult(t, first)
	if !consent.Confirm("user-a", request.Token) {
		t.Fatal("Confirm returned false for server-issued token")
	}

	wrongUser := executeGuardrailTool(t, runtime, "user-b", "external_weather_lookup", json.RawMessage(`{
		"query":"synthetic forecast window",
		"consent_token":"`+request.Token+`"
	}`))
	assertToolStatus(t, wrongUser, "requires_user_confirmation")

	wrongArgs := executeGuardrailTool(t, runtime, "user-a", "external_weather_lookup", json.RawMessage(`{
		"query":"different synthetic forecast window",
		"consent_token":"`+request.Token+`"
	}`))
	assertToolStatus(t, wrongArgs, "requires_user_confirmation")
	if executed != 0 {
		t.Fatalf("external tool executed despite mismatched consent")
	}
}

func consentRequestFromResult(t *testing.T, result agent.ToolRuntimeResult) ExternalReadConsentRequest {
	t.Helper()
	raw, err := json.Marshal(toolResultMap(t, result)["consent_request"])
	if err != nil {
		t.Fatalf("marshal consent_request: %v", err)
	}
	var request ExternalReadConsentRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("unmarshal consent_request: %v; raw=%s", err, raw)
	}
	return request
}
