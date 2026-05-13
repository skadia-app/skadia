package agent

import (
	"context"
	"encoding/json"
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

	toolNames := GetToolsForDomains(domains, profile)
	if a.tools != nil {
		toolNames = a.tools.AddEnabledWriteTools(toolNames, prefs)
	}
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
		_ = sink.Send(SSEEvent{Type: EventError, ID: req.MessageID, Error: "Chat nicht verfügbar: " + err.Error()})
		return err
	}
	if err := sink.Send(SSEEvent{Type: EventThinking, ID: req.MessageID, Step: ThinkingAnalyze}); err != nil {
		return err
	}

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
		params := a.chatParams(cfg, profile, messages, toolDefs, allowTools, requireTool && hops == 0)
		stream := a.streamChat(ctx, cfg, params)

		toolCalls, finishReason, streamed, err := collectLLMStream(req.MessageID, stream, sink, &usage)
		if err != nil {
			_ = sink.Send(SSEEvent{Type: EventError, ID: req.MessageID, Error: "Chat nicht verfügbar: " + err.Error()})
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
			resultText, stop, err := a.executeToolCall(ctx, req, call, sink)
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
		params := a.chatParams(cfg, profile, messages, nil, false, false)
		stream := a.streamChat(ctx, cfg, params)
		if _, _, _, err := collectLLMStream(req.MessageID, stream, sink, &usage); err != nil {
			_ = sink.Send(SSEEvent{Type: EventError, ID: req.MessageID, Error: "Chat nicht verfügbar: " + err.Error()})
			return err
		}
	}

	done := SSEEvent{Type: EventDone, ID: req.MessageID, Sources: []string{"llm"}, Hops: hops, ContextWindow: profile.MaxContextTokens}
	if usage.TotalTokens > 0 {
		done.Usage = &usage
	}
	return sink.Send(done)
}

func (a *Agent) chatParams(cfg LLMConfig, profile *ModelProfile, messages []conversationMessage, tools []llmwire.ToolDefinition, allowTools bool, requireTool bool) llmwire.ChatRequest {
	params := llmwire.ChatRequest{
		Model:                    cfg.Model,
		Messages:                 toLLMMessages(messages),
		Temperature:              floatPtr(profile.Temperature),
		TopP:                     floatPtr(profile.TopP),
		PresencePenalty:          floatPtr(profile.PresencePenalty),
		MaxTokens:                intPtr(profile.MaxResponseTokens),
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

func (a *Agent) executeToolCall(ctx context.Context, req ChatRequest, call pendingToolCall, sink SSESink) (string, bool, error) {
	args := json.RawMessage(call.Args)
	if len(strings.TrimSpace(string(args))) == 0 {
		args = json.RawMessage(`{}`)
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
		UserID:     req.UserID,
		SessionID:  req.SessionID,
		MessageID:  req.MessageID,
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Args:       args,
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
