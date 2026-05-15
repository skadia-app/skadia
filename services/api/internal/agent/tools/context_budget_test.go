package tools

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
)

func TestPointToPointRoutingToolDefinitionsFitEightKContext(t *testing.T) {
	profile := agent.GetProfile("small", "", map[string]any{
		"max_context_tokens":  8000,
		"max_response_tokens": 2000,
	})
	query := "Wie weit ist es von Beispielort A nach Beispielort B? Kannst du mir eine MTB-Route planen?"
	route := agent.RouteQuery(query, &profile)
	toolNames := agent.GetToolsForQuery(query, route.Domains, &profile)

	reg := NewRegistry()
	RegisterAll(reg)
	defs := reg.Definitions(toolNames, &agent.ChatPreferences{})
	if len(defs) == 0 {
		t.Fatal("routing point-to-point query produced no tool definitions")
	}

	systemPrompt := agent.AssembleSystemPrompt(profile, nil, time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC))
	tokens := estimateChatRequestTokensForTest([]llmwire.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: query},
	}, defs, 2000)
	if tokens > 8000 {
		t.Fatalf("estimated request tokens = %d, want <= 8000; tools = %#v", tokens, toolNames)
	}
}

func TestPointToPointRoutingToolDefinitionsFitFourKWithActiveRoutingSkill(t *testing.T) {
	profile := agent.GetProfile("small", "", map[string]any{
		"max_context_tokens":  4000,
		"max_response_tokens": 1000,
	})
	query := "Wie weit ist Beispielort A von Beispielort B entfernt?"
	route := agent.RouteQuery(query, &profile)
	toolNames := agent.GetToolsForQuery(query, route.Domains, &profile)

	reg := NewRegistry()
	RegisterAll(reg)
	defs := reg.Definitions(toolNames, &agent.ChatPreferences{})
	if len(defs) == 0 {
		t.Fatal("routing point-to-point query produced no tool definitions")
	}

	skills, err := agent.LoadSkills("")
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}
	var activeSkills []*agent.Skill
	for _, domain := range route.Domains {
		if skill := skills[domain]; skill != nil {
			activeSkills = append(activeSkills, skill)
		}
	}

	systemPrompt := agent.AssembleSystemPrompt(profile, activeSkills, time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC))
	tokens := estimateChatRequestTokensForTest([]llmwire.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: query},
	}, defs, 750)
	if tokens > 4000 {
		t.Fatalf("estimated request tokens = %d, want <= 4000; domains = %#v tools = %#v", tokens, route.Domains, toolNames)
	}
}

func estimateChatRequestTokensForTest(messages []llmwire.ChatMessage, tools []llmwire.ToolDefinition, maxTokens int) int {
	raw, _ := json.Marshal(struct {
		Messages []llmwire.ChatMessage    `json:"messages"`
		Tools    []llmwire.ToolDefinition `json:"tools,omitempty"`
	}{
		Messages: messages,
		Tools:    tools,
	})
	return (len(raw)+2)/3 + maxTokens
}
