package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent/llmwire"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	defaultMaxHops          = 5
	defaultResultCharFactor = 200
	defaultContextWindow    = 8000
	minResponseTokens       = 256
)

var dataQuestionPattern = regexp.MustCompile(`(?i)\b(wann|when|wie viele|how many|wieviele|wie oft|how often|welche|welcher|welches|which|wo|where|wieviel|how much|was war|what was|was ist|what is|zuletzt|last|latest|recent|letzte|letzten|letztes|höchste|hoechste|highest|schnellste|fastest|längste|laengste|longest|bestzeit|personal record|rekord|record|pr|zeige|show|finde|find|suche|search|trend|trends|zeitreihe|zeitreihen|pro jahr|pro monat|jahresvergleich|bestwert|bestwerte)\b`)

type Config struct {
	DB             *database.DB
	LLMResolver    *LLMResolver
	ProfileStore   ProfileStore
	Preferences    *PreferenceLoader
	Skills         map[string]*Skill
	Tools          ToolRuntime
	DefaultProfile string
	StreamChat     ChatStreamFunc
}

type Agent struct {
	db             *database.DB
	llmResolver    *LLMResolver
	profileStore   ProfileStore
	preferences    *PreferenceLoader
	skills         map[string]*Skill
	tools          ToolRuntime
	defaultProfile string
	streamChat     ChatStreamFunc
}

func New(cfg Config) *Agent {
	a := &Agent{
		db:             cfg.DB,
		llmResolver:    cfg.LLMResolver,
		profileStore:   cfg.ProfileStore,
		preferences:    cfg.Preferences,
		skills:         cfg.Skills,
		tools:          cfg.Tools,
		defaultProfile: cfg.DefaultProfile,
		streamChat:     cfg.StreamChat,
	}
	if a.defaultProfile == "" {
		a.defaultProfile = "small"
	}
	if a.streamChat == nil {
		a.streamChat = func(ctx context.Context, cfg LLMConfig, params llmwire.ChatRequest) ChatStream {
			client := NewLLMClient(cfg)
			return client.Stream(ctx, params)
		}
	}
	return a
}

func (a *Agent) Stream(ctx context.Context, req ChatRequest, sink SSESink) error {
	if sink == nil {
		return fmt.Errorf("sse sink is nil")
	}

	profile, err := LoadProfile(ctx, a.profileStore, req.UserID, a.defaultProfile)
	if err != nil {
		slog.Warn("chat profile load failed", "error", err)
		fallback := GetProfile(a.defaultProfile, "", nil)
		profile = &fallback
	}
	if err := profile.DisabledError(); err != nil {
		_ = sink.Send(SSEEvent{Type: EventError, ID: req.MessageID, Error: "Chat ist deaktiviert."})
		return err
	}

	domains := []string{"activities", "health"}
	route := RouteQuery(req.Message, profile)
	if len(route.Domains) > 0 {
		domains = route.Domains
	}
	activeSkills := a.skillsForDomains(domains)
	systemPrompt := AssembleSystemPrompt(*profile, activeSkills, time.Now())

	prefs, err := a.loadPrefs(ctx, req.UserID)
	if err != nil {
		slog.Warn("chat preferences load failed", "error", err)
		prefs = &ChatPreferences{}
	}

	toolPlan := BuildToolPlan(req.Message, profile, prefs)
	domains = toolPlan.Intent.Domains
	activeSkills = a.skillsForDomains(domains)
	systemPrompt = AssembleSystemPrompt(*profile, activeSkills, time.Now())
	toolNames := toolPlan.Tools
	toolDefs := []llmwire.ToolDefinition(nil)
	if a.tools != nil {
		toolDefs = a.tools.Definitions(toolNames, prefs)
	}
	requireTool := shouldRequireTool(req.Message, domains, toolDefs)

	if matched := MatchTemplate(req.Message); matched != nil {
		if err := sink.Send(SSEEvent{Type: EventThinking, ID: req.MessageID, Step: ThinkingTemplate}); err != nil {
			return err
		}
		if text, err := a.executeTemplate(ctx, req.UserID, matched); err == nil {
			if err := sink.Send(SSEEvent{Type: EventChunk, ID: req.MessageID, Delta: text}); err != nil {
				return err
			}
			return sink.Send(SSEEvent{Type: EventDone, ID: req.MessageID, Sources: []string{"template"}})
		} else {
			slog.Warn("chat template query failed", "template", matched.Name, "error", err)
		}
	}

	cfg, err := a.resolveLLM(ctx, req.UserID)
	if err != nil {
		_ = sink.Send(SSEEvent{Type: EventError, ID: req.MessageID, Error: chatUnavailableMessage(err)})
		return err
	}
	if err := sink.Send(SSEEvent{Type: EventThinking, ID: req.MessageID, Step: ThinkingAnalyze}); err != nil {
		return err
	}

	contextWindow := effectiveContextWindow(profile, cfg)
	messages := buildConversation(systemPrompt, req.History, req.Message, profile.HistoryMessages)
	resultLimit := profile.ToolResultLimit * defaultResultCharFactor
	if resultLimit <= 0 {
		resultLimit = 1000
	}

	var usage TokenUsage
	hasStreamedContent := false
	hops := 0
	for hops < defaultMaxHops {
		allowTools := len(toolDefs) > 0 && hops < defaultMaxHops-1
		activeTools := []llmwire.ToolDefinition(nil)
		if allowTools {
			activeTools = toolDefs
		}
		fit, ok := fitChatRequest(messages, activeTools, profile, contextWindow, toolPlan.RequiredTools)
		if !ok {
			msg := "Chat nicht verfügbar: Anfrage passt nicht in das konfigurierte Kontextfenster."
			_ = sink.Send(SSEEvent{Type: EventError, ID: req.MessageID, Error: msg})
			return fmt.Errorf("chat request exceeds configured context window")
		}
		messages = fit.Messages
		activeTools = fit.Tools
		if hops == 0 && len(activeTools) > 0 {
			if err := sink.Send(toolLoadedEvent(req.MessageID, activeTools, toolPlan)); err != nil {
				return err
			}
		}
		requireToolNow := requireTool && hops == 0 && len(activeTools) > 0
		params := a.chatParams(cfg, profile, messages, activeTools, len(activeTools) > 0, requireToolNow, fit.MaxTokens)
		stream := a.streamChat(ctx, cfg, params)

		toolCalls, finishReason, streamed, err := collectLLMStream(req.MessageID, stream, sink, &usage)
		if err != nil {
			_ = sink.Send(SSEEvent{Type: EventError, ID: req.MessageID, Error: chatUnavailableMessage(err)})
			return err
		}
		hasStreamedContent = hasStreamedContent || streamed

		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
		}
		if finishReason != "tool_calls" || len(toolCalls) == 0 {
			break
		}

		maxCalls := profile.MaxToolsPerTurn
		if maxCalls <= 0 {
			maxCalls = 1
		}
		if len(toolCalls) > maxCalls {
			slog.Warn("chat tool-call cap reached", "count", len(toolCalls), "max", maxCalls)
			toolCalls = toolCalls[:maxCalls]
		}

		messages = append(messages, conversationMessage{Role: "assistant", ToolCalls: toolCalls})
		for _, call := range toolCalls {
			resultText, stop, err := a.executeToolCall(ctx, req, call, sink, activeTools, toolPlan.Intent)
			if err != nil {
				resultText = "Error: " + err.Error()
			}
			if stop {
				return nil
			}
			messages = append(messages, conversationMessage{
				Role:       "tool",
				Content:    truncate(resultText, resultLimit),
				ToolCallID: call.ID,
			})
		}

		if expandedPlan, expandedDefs := a.expandToolPlanAfterResults(toolPlan, prefs, toolDefs, messages); len(expandedDefs) > len(toolDefs) {
			toolPlan = expandedPlan
			toolDefs = expandedDefs
			if err := sink.Send(toolLoadedEvent(req.MessageID, toolDefs, toolPlan)); err != nil {
				return err
			}
		}

		hops++
		compressOldToolResults(messages)
		if err := sink.Send(SSEEvent{
			Type: EventThinking,
			ID:   req.MessageID,
			Step: fmt.Sprintf(ThinkingHopFmt, hops, defaultMaxHops),
		}); err != nil {
			return err
		}
	}

	if !hasStreamedContent && hops > 0 {
		messages = append(messages, conversationMessage{
			Role: "user",
			Content: "Du hast Tool-Ergebnisse erhalten aber noch keine Antwort gegeben. " +
				"Fasse die Ergebnisse jetzt knapp und sachlich zusammen. Wenn keine Daten gefunden wurden, sag das kurz.",
		})
		fit, ok := fitChatRequest(messages, nil, profile, contextWindow, nil)
		if !ok {
			msg := "Chat nicht verfügbar: Anfrage passt nicht in das konfigurierte Kontextfenster."
			_ = sink.Send(SSEEvent{Type: EventError, ID: req.MessageID, Error: msg})
			return fmt.Errorf("chat request exceeds configured context window")
		}
		messages = fit.Messages
		params := a.chatParams(cfg, profile, messages, nil, false, false, fit.MaxTokens)
		stream := a.streamChat(ctx, cfg, params)
		if _, _, _, err := collectLLMStream(req.MessageID, stream, sink, &usage); err != nil {
			_ = sink.Send(SSEEvent{Type: EventError, ID: req.MessageID, Error: chatUnavailableMessage(err)})
			return err
		}
	}

	done := SSEEvent{Type: EventDone, ID: req.MessageID, Sources: []string{"llm"}, Hops: hops, ContextWindow: contextWindow}
	if usage.TotalTokens > 0 {
		done.Usage = &usage
	}
	return sink.Send(done)
}

func (a *Agent) chatParams(cfg LLMConfig, profile *ModelProfile, messages []conversationMessage, tools []llmwire.ToolDefinition, allowTools bool, requireTool bool, maxTokens int) llmwire.ChatRequest {
	if maxTokens <= 0 {
		maxTokens = profile.MaxResponseTokens
	}
	params := llmwire.ChatRequest{
		Model:                    cfg.Model,
		Messages:                 toLLMMessages(messages),
		Temperature:              floatPtr(profile.Temperature),
		TopP:                     floatPtr(profile.TopP),
		PresencePenalty:          floatPtr(profile.PresencePenalty),
		MaxTokens:                intPtr(maxTokens),
		IncludeUsage:             cfg.SupportsStreamingUsage(),
		DisableParallelToolCalls: allowTools && cfg.SupportsParallelToolControl(),
	}
	if allowTools {
		params.Tools = tools
		if requireTool {
			params.ToolChoice = "required"
		}
	}
	return params
}

func effectiveContextWindow(profile *ModelProfile, cfg LLMConfig) int {
	window := defaultContextWindow
	if profile != nil && profile.MaxContextTokens > 0 {
		window = profile.MaxContextTokens
	}
	if cfg.ContextWindow > 0 && cfg.ContextWindow < window {
		window = cfg.ContextWindow
	}
	return window
}

func responseTokenBudget(profile *ModelProfile, contextWindow int) int {
	maxTokens := 1000
	if profile != nil && profile.MaxResponseTokens > 0 {
		maxTokens = profile.MaxResponseTokens
	}
	if contextWindow <= 0 {
		return maxTokens
	}
	capTokens := contextWindow / 4
	if capTokens < minResponseTokens {
		capTokens = minResponseTokens
	}
	if maxTokens > capTokens {
		maxTokens = capTokens
	}
	return maxTokens
}

type fittedChatRequest struct {
	Messages  []conversationMessage
	Tools     []llmwire.ToolDefinition
	MaxTokens int
}

func fitChatRequest(messages []conversationMessage, tools []llmwire.ToolDefinition, profile *ModelProfile, contextWindow int, requiredTools []string) (fittedChatRequest, bool) {
	maxTokens := responseTokenBudget(profile, contextWindow)
	if contextWindow <= 0 {
		return fittedChatRequest{Messages: messages, Tools: tools, MaxTokens: maxTokens}, true
	}
	messages = trimConversationToContext(messages, tools, maxTokens, contextWindow)
	tools = trimToolsToContext(messages, tools, maxTokens, contextWindow, requiredTools)
	messages = trimConversationToContext(messages, tools, maxTokens, contextWindow)
	messages = shrinkToolResultsToContext(messages, tools, maxTokens, contextWindow)
	if estimatedRequestTokens(messages, tools, maxTokens) <= contextWindow {
		return fittedChatRequest{Messages: messages, Tools: tools, MaxTokens: maxTokens}, true
	}
	baseTokens := estimatedRequestTokens(messages, tools, 0)
	available := contextWindow - baseTokens
	if available < minResponseTokens {
		return fittedChatRequest{Messages: messages, Tools: tools, MaxTokens: maxTokens}, false
	}
	if maxTokens > available {
		maxTokens = available
	}
	if estimatedRequestTokens(messages, tools, maxTokens) > contextWindow {
		return fittedChatRequest{Messages: messages, Tools: tools, MaxTokens: maxTokens}, false
	}
	return fittedChatRequest{Messages: messages, Tools: tools, MaxTokens: maxTokens}, true
}

func shrinkToolResultsToContext(messages []conversationMessage, tools []llmwire.ToolDefinition, maxTokens int, contextWindow int) []conversationMessage {
	if contextWindow <= 0 || estimatedRequestTokens(messages, tools, maxTokens) <= contextWindow {
		return messages
	}
	out := append([]conversationMessage(nil), messages...)
	for estimatedRequestTokens(out, tools, maxTokens) > contextWindow {
		longest := -1
		for idx, msg := range out {
			if msg.Role == "tool" && len(msg.Content) > 200 && (longest == -1 || len(msg.Content) > len(out[longest].Content)) {
				longest = idx
			}
		}
		if longest == -1 {
			return out
		}
		excessTokens := estimatedRequestTokens(out, tools, maxTokens) - contextWindow
		cutBytes := excessTokens*3 + 200
		nextLen := len(out[longest].Content) - cutBytes
		if nextLen < 200 {
			nextLen = 200
		}
		if nextLen >= len(out[longest].Content) {
			nextLen = len(out[longest].Content) / 2
		}
		out[longest].Content = truncate(out[longest].Content, nextLen) + "...[gekürzt]"
	}
	return out
}

func trimConversationToContext(messages []conversationMessage, tools []llmwire.ToolDefinition, maxTokens int, contextWindow int) []conversationMessage {
	if contextWindow <= 0 || len(messages) <= 2 {
		return messages
	}
	out := append([]conversationMessage(nil), messages...)
	for len(out) > 2 && estimatedRequestTokens(out, tools, maxTokens) > contextWindow {
		out = append(out[:1], out[2:]...)
	}
	return out
}

func trimToolsToContext(messages []conversationMessage, tools []llmwire.ToolDefinition, maxTokens int, contextWindow int, requiredTools []string) []llmwire.ToolDefinition {
	if contextWindow <= 0 || len(tools) == 0 {
		return tools
	}
	required := map[string]bool{}
	for _, name := range requiredTools {
		required[name] = true
	}
	out := append([]llmwire.ToolDefinition(nil), tools...)
	for len(out) > 0 && estimatedRequestTokens(messages, out, maxTokens) > contextWindow {
		drop := -1
		for idx := len(out) - 1; idx >= 0; idx-- {
			if !required[out[idx].Name] {
				drop = idx
				break
			}
		}
		if drop == -1 {
			return out
		}
		out = append(out[:drop], out[drop+1:]...)
	}
	return out
}

func estimatedRequestTokens(messages []conversationMessage, tools []llmwire.ToolDefinition, maxTokens int) int {
	raw, _ := json.Marshal(struct {
		Messages []llmwire.ChatMessage    `json:"messages"`
		Tools    []llmwire.ToolDefinition `json:"tools,omitempty"`
	}{
		Messages: toLLMMessages(messages),
		Tools:    tools,
	})
	total := (len(raw) + 2) / 3
	if maxTokens > 0 {
		total += maxTokens
	}
	return total
}

func shouldRequireTool(message string, domains []string, toolDefs []llmwire.ToolDefinition) bool {
	if len(toolDefs) == 0 || !dataQuestionPattern.MatchString(message) {
		return false
	}
	for _, domain := range domains {
		switch domain {
		case "activities", "health", "training", "gear", "geo", "segments", "photos", "routing", "documents":
			return true
		}
	}
	return false
}

func toolLoadedEvent(messageID string, tools []llmwire.ToolDefinition, plan ToolPlan) SSEEvent {
	loadedTools := make([]string, 0, len(tools))
	for _, def := range tools {
		loadedTools = append(loadedTools, def.Name)
	}
	expansionLevel := plan.Intent.ExpansionLevel
	return SSEEvent{
		Type:           EventToolsLoaded,
		ID:             messageID,
		Tools:          loadedTools,
		Reason:         plan.Reason,
		Intent:         plan.Intent.DebugLabel(),
		ExpansionLevel: &expansionLevel,
		OmittedTools:   omittedToolsAfterFit(plan.OmittedTools, plan.Tools, loadedTools),
	}
}

func omittedToolsAfterFit(existing []ToolOmission, planned []string, loaded []string) []ToolOmission {
	out := make([]ToolOmission, 0, len(existing)+len(planned))
	for _, name := range planned {
		if containsToolName(loaded, name) || containsOmittedTool(out, name) {
			continue
		}
		out = append(out, ToolOmission{Name: name, Reason: "context_budget"})
	}
	for _, omission := range existing {
		if !containsToolName(loaded, omission.Name) && !containsOmittedTool(out, omission.Name) {
			out = append(out, omission)
		}
	}
	return out
}

func containsOmittedTool(values []ToolOmission, want string) bool {
	for _, value := range values {
		if value.Name == want {
			return true
		}
	}
	return false
}

func (a *Agent) executeToolCall(ctx context.Context, req ChatRequest, call pendingToolCall, sink SSESink, activeTools []llmwire.ToolDefinition, intent AgentIntent) (string, bool, error) {
	args := json.RawMessage(call.Args)
	if len(strings.TrimSpace(string(args))) == 0 {
		args = json.RawMessage(`{}`)
	}
	allowedTools := toolDefinitionNames(activeTools)
	if !containsToolName(allowedTools, call.Name) {
		resultText := fmt.Sprintf("Error: tool %q was not available for this turn", call.Name)
		if err := sink.Send(SSEEvent{Type: EventToolResult, ID: req.MessageID, ToolCallID: call.ID, Tool: call.Name, Summary: resultText}); err != nil {
			return "", false, err
		}
		return resultText, false, fmt.Errorf("tool %q was not planned for this turn", call.Name)
	}
	var publicArgs json.RawMessage
	if gate, ok := a.tools.(interface {
		PublicToolCallArgs(toolName string, args json.RawMessage) (json.RawMessage, bool)
	}); ok {
		if sanitized, include := gate.PublicToolCallArgs(call.Name, args); include {
			publicArgs = sanitized
		}
	}
	if err := sink.Send(SSEEvent{Type: EventToolCall, ID: req.MessageID, ToolCallID: call.ID, Tool: call.Name, Args: publicArgs}); err != nil {
		return "", false, err
	}
	if a.tools == nil {
		return "", false, fmt.Errorf("tool runtime is not configured")
	}
	result, err := a.tools.Execute(ctx, ToolRuntimeRequest{
		UserID:       req.UserID,
		SessionID:    req.SessionID,
		MessageID:    req.MessageID,
		ToolCallID:   call.ID,
		ToolName:     call.Name,
		Args:         args,
		AllowedTools: allowedTools,
		EnforceTools: true,
		WriteIntent:  intent.WriteRequested,
	}, sink)
	if result.AskUser {
		question := result.Question
		if question == "" {
			question = "Soll ich das fehlende Objekt zuerst anlegen?"
		}
		_ = sink.Send(SSEEvent{Type: EventUserQuestion, ID: req.MessageID, ToolCallID: call.ID, Tool: call.Name, Question: question})
		return "", true, nil
	}
	resultText := result.Summary
	if resultText == "" {
		resultText = marshalResult(result.Result)
	}
	if err != nil {
		resultText = "Error: " + err.Error()
	}
	if sendErr := sink.Send(SSEEvent{Type: EventToolResult, ID: req.MessageID, ToolCallID: call.ID, Tool: call.Name, Summary: truncate(resultText, 500)}); sendErr != nil {
		return "", false, sendErr
	}
	return resultText, false, err
}

func toolDefinitionNames(tools []llmwire.ToolDefinition) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Name)
	}
	return out
}

func (a *Agent) expandToolPlanAfterResults(plan ToolPlan, prefs *ChatPreferences, currentDefs []llmwire.ToolDefinition, messages []conversationMessage) (ToolPlan, []llmwire.ToolDefinition) {
	if a.tools == nil {
		return plan, currentDefs
	}
	candidates := toolExpansionCandidates(messages)
	if len(candidates) == 0 {
		return plan, currentDefs
	}
	candidates = filterNewToolNames(candidates, plan.Tools)
	if len(candidates) == 0 {
		return plan, currentDefs
	}
	next := plan
	next.Intent.ExpansionLevel++
	additional := len(candidates)
	if additional > 4 {
		additional = 4
	}
	candidates = candidates[:additional]
	next.RequiredTools = appendUniqueToolNames(next.RequiredTools, candidates...)
	next.Tools = appendUniqueToolNames(next.Tools, candidates...)
	next.OmittedTools = removeToolOmissions(next.OmittedTools, candidates)
	defs := a.tools.Definitions(next.Tools, prefs)
	if len(defs) <= len(currentDefs) {
		return plan, currentDefs
	}
	return next, defs
}

func toolExpansionCandidates(messages []conversationMessage) []string {
	candidateSet := map[string]bool{}
	for idx := len(messages) - 1; idx >= 0; idx-- {
		msg := messages[idx]
		if msg.Role != "tool" {
			continue
		}
		content := strings.ToLower(msg.Content)
		needs := toolResultNeedTokens(msg.Content)
		if len(needs) == 0 && (strings.Contains(content, `"status":"not_found"`) ||
			strings.Contains(content, `"status":"insufficient_capability"`) ||
			strings.Contains(content, `"ambiguous":true`)) {
			needs = append(needs, "resolve", "context")
		}
		for _, need := range needs {
			for _, name := range expansionToolsForNeed(need) {
				candidateSet[name] = true
			}
		}
	}
	out := []string{}
	for _, name := range knownExpansionToolOrder() {
		if candidateSet[name] && !containsToolName(out, name) {
			out = append(out, name)
		}
	}
	return out
}

func filterNewToolNames(candidates, existing []string) []string {
	out := []string{}
	for _, name := range candidates {
		if !containsToolName(existing, name) && !containsToolName(out, name) {
			out = append(out, name)
		}
	}
	return out
}

func removeToolOmissions(values []ToolOmission, names []string) []ToolOmission {
	out := values[:0]
	for _, value := range values {
		if !containsToolName(names, value.Name) {
			out = append(out, value)
		}
	}
	return out
}

func toolResultNeedTokens(content string) []string {
	var doc map[string]any
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return nil
	}
	out := []string{}
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" && !containsToolName(out, value) {
			out = append(out, value)
		}
	}
	for _, key := range []string{"status", "blocking_reason", "next_action"} {
		if raw, ok := doc[key].(string); ok {
			add(raw)
		}
	}
	switch raw := doc["needs"].(type) {
	case string:
		add(raw)
	case []any:
		for _, item := range raw {
			if value, ok := item.(string); ok {
				add(value)
			}
		}
	}
	if ambiguous, ok := doc["ambiguous"].(bool); ok && ambiguous {
		add("ambiguous")
	}
	return out
}

func expansionToolsForNeed(need string) []string {
	switch {
	case strings.Contains(need, "weather"):
		return []string{"query_weather_at_point_time", "get_agent_service_status"}
	case strings.Contains(need, "coverage"):
		return []string{"get_routing_coverage", "query_coverage", "get_agent_service_status"}
	case strings.Contains(need, "profile"):
		return []string{"list_routing_profiles"}
	case strings.Contains(need, "planning") || strings.Contains(need, "plan_route") || strings.Contains(need, "routing_planning"):
		return []string{"plan_route_between", "plan_route", "suggest_route_goal", "list_routing_profiles", "get_routing_coverage"}
	case strings.Contains(need, "route"):
		return []string{"plan_route_between", "plan_route", "suggest_route_goal"}
	case strings.Contains(need, "place") || strings.Contains(need, "geocode") || strings.Contains(need, "resolve") || strings.Contains(need, "ambiguous") || strings.Contains(need, "not_found"):
		return []string{"geocode_place", "resolve_route_place", "resolve_route_feature", "resolve_entity"}
	case strings.Contains(need, "terrain") || strings.Contains(need, "surface") || strings.Contains(need, "forest") || strings.Contains(need, "water"):
		return []string{"query_activity_terrain_summary", "query_geo_features", "search_pois_nearby", "search_trails_by_difficulty"}
	case strings.Contains(need, "component") || strings.Contains(need, "chain_rotation") || strings.Contains(need, "rotation"):
		return []string{"find_gear", "add_gear_component", "create_chain_rotation", "query_gear_maintenance"}
	case strings.Contains(need, "sql") || strings.Contains(need, "schema") || strings.Contains(need, "database"):
		return []string{"run_sql"}
	default:
		return []string{"resolve_entity"}
	}
}

func knownExpansionToolOrder() []string {
	return []string{
		"get_agent_service_status",
		"geocode_place",
		"resolve_user_location",
		"resolve_route_place",
		"resolve_route_feature",
		"analyze_route_points",
		"get_routing_coverage",
		"query_coverage",
		"list_routing_profiles",
		"plan_route_between",
		"suggest_route_goal",
		"plan_route",
		"query_weather_at_point_time",
		"query_activity_terrain_summary",
		"query_geo_features",
		"search_pois_nearby",
		"search_trails_by_difficulty",
		"resolve_entity",
		"find_gear",
		"add_gear_component",
		"create_chain_rotation",
		"query_gear_maintenance",
		"run_sql",
	}
}

func chatUnavailableMessage(err error) string {
	if err == nil {
		return "Chat nicht verfügbar: unbekannter Fehler."
	}
	if errors.Is(err, context.Canceled) {
		return "Chat abgebrochen."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Chat nicht verfügbar: Zeitlimit überschritten."
	}
	return "Chat nicht verfügbar: Das Modell oder ein Tool ist derzeit nicht erreichbar."
}

func (a *Agent) resolveLLM(ctx context.Context, userID string) (LLMConfig, error) {
	if a.llmResolver == nil {
		return LLMConfig{}, fmt.Errorf("LLM resolver is not configured")
	}
	cfg, err := a.llmResolver.Resolve(ctx, userID, "tool_agent")
	if err != nil {
		return LLMConfig{}, err
	}
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Model) == "" {
		return LLMConfig{}, fmt.Errorf("LLM configuration is incomplete")
	}
	return cfg, nil
}

func (a *Agent) loadPrefs(ctx context.Context, userID string) (*ChatPreferences, error) {
	return a.preferences.LoadChatPreferences(ctx, userID)
}

func (a *Agent) skillsForDomains(domains []string) []*Skill {
	out := make([]*Skill, 0, len(domains))
	for _, domain := range domains {
		if skill := a.skills[domain]; skill != nil {
			out = append(out, skill)
		}
	}
	return out
}

func (a *Agent) executeTemplate(ctx context.Context, userID string, match *TemplateMatch) (string, error) {
	if a.db == nil || a.db.ReadonlyPool == nil {
		return "", fmt.Errorf("readonly database pool is not configured")
	}
	tx, err := a.db.ReadonlyPool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_user_id', $1, true)", userID); err != nil {
		return "", err
	}
	args := append([]any{userID}, match.Params...)
	rows, err := tx.Query(ctx, match.Template.SQL, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	data, err := rowsToMaps(rows)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return match.Template.Format + "\n" + formatRows(data), nil
}

type conversationMessage struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []pendingToolCall
}

type pendingToolCall struct {
	Index int64
	ID    string
	Name  string
	Args  string
}

func buildConversation(systemPrompt string, history []ChatHistoryMessage, message string, limit int) []conversationMessage {
	if limit <= 0 {
		limit = 10
	}
	msgs := []conversationMessage{{Role: "system", Content: systemPrompt}}
	if len(history) > limit {
		history = history[len(history)-limit:]
	}
	for _, item := range history {
		if item.Content == "" || (item.Role != "user" && item.Role != "assistant") {
			continue
		}
		msgs = append(msgs, conversationMessage{Role: item.Role, Content: item.Content})
	}
	msgs = append(msgs, conversationMessage{Role: "user", Content: message})
	return msgs
}

func toLLMMessages(messages []conversationMessage) []llmwire.ChatMessage {
	out := make([]llmwire.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			out = append(out, llmwire.ChatMessage{Role: "system", Content: msg.Content})
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				out = append(out, llmwire.ChatMessage{Role: "assistant", ToolCalls: toLLMToolCalls(msg.ToolCalls)})
			} else {
				out = append(out, llmwire.ChatMessage{Role: "assistant", Content: msg.Content})
			}
		case "tool":
			out = append(out, llmwire.ChatMessage{Role: "tool", Content: msg.Content, ToolCallID: msg.ToolCallID})
		default:
			out = append(out, llmwire.ChatMessage{Role: "user", Content: msg.Content})
		}
	}
	return out
}

func toLLMToolCalls(calls []pendingToolCall) []llmwire.ChatToolCall {
	out := make([]llmwire.ChatToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, llmwire.ChatToolCall{ID: call.ID, Name: call.Name, Args: call.Args})
	}
	return out
}

func collectLLMStream(messageID string, stream ChatStream, sink SSESink, usage *TokenUsage) ([]pendingToolCall, string, bool, error) {
	acc := map[int64]*pendingToolCall{}
	contentBuffer := strings.Builder{}
	streamed := false
	finishReason := ""

	for stream.Next() {
		event := stream.Current()
		if event.Error != nil {
			return nil, finishReason, streamed, fmt.Errorf("%s", event.Error.Message)
		}
		if event.Usage != nil && event.Usage.TotalTokens > 0 && usage != nil {
			usage.PromptTokens += event.Usage.PromptTokens
			usage.CompletionTokens += event.Usage.CompletionTokens
			usage.TotalTokens += event.Usage.TotalTokens
		}
		if event.FinishReason != "" {
			finishReason = event.FinishReason
		}
		for _, tc := range event.ToolCalls {
			entry := acc[tc.Index]
			if entry == nil {
				entry = &pendingToolCall{Index: tc.Index}
				acc[tc.Index] = entry
			}
			if tc.ID != "" {
				entry.ID = tc.ID
			}
			if tc.Name != "" {
				if entry.Name == "" || entry.Name != tc.Name && !strings.Contains(entry.Name, tc.Name) {
					entry.Name += tc.Name
				}
			}
			entry.Args += tc.Args
		}
		if event.Delta != "" {
			contentBuffer.WriteString(event.Delta)
			buffered := contentBuffer.String()
			if !strings.Contains(buffered, "<tool_call>") && !strings.Contains(buffered, "<function=") {
				streamed = true
				if err := sink.Send(SSEEvent{Type: EventChunk, ID: messageID, Delta: event.Delta}); err != nil {
					return nil, finishReason, streamed, err
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, finishReason, streamed, err
	}
	calls := sortedToolCalls(acc)
	return calls, finishReason, streamed, nil
}

func sortedToolCalls(acc map[int64]*pendingToolCall) []pendingToolCall {
	if len(acc) == 0 {
		return nil
	}
	keys := make([]int64, 0, len(acc))
	for key := range acc {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make([]pendingToolCall, 0, len(keys))
	for _, key := range keys {
		call := *acc[key]
		if call.ID == "" {
			call.ID = uuid.NewString()
		}
		out = append(out, call)
	}
	return out
}

func floatPtr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func compressOldToolResults(messages []conversationMessage) {
	boundaries := make([]int, 0)
	for idx, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			boundaries = append(boundaries, idx)
		}
	}
	if len(boundaries) < 3 {
		return
	}
	cutoff := boundaries[len(boundaries)-2]
	for idx := 0; idx < cutoff; idx++ {
		if messages[idx].Role == "tool" {
			messages[idx].Content = truncate(messages[idx].Content, 400) + "...[komprimiert]"
		}
	}
}

func rowsToMaps(rows pgx.Rows) ([]map[string]any, error) {
	fields := rows.FieldDescriptions()
	var out []map[string]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(fields))
		for i, field := range fields {
			row[field.Name] = values[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func formatRows(rows []map[string]any) string {
	if len(rows) == 0 {
		return "Keine Daten gefunden."
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		keys := make([]string, 0, len(row))
		for key := range row {
			if key != "id" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s: %v", key, row[key]))
		}
		lines = append(lines, strings.Join(parts, " | "))
		if len(lines) >= 20 {
			break
		}
	}
	return strings.Join(lines, "\n")
}

func marshalResult(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(raw)
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}
