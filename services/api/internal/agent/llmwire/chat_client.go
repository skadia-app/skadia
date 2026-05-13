package llmwire

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ChatCompletionsClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client

	MaxRetries     int
	DefaultBackoff time.Duration
}

func NewChatCompletionsClient(baseURL, apiKey string, httpClient *http.Client) *ChatCompletionsClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &ChatCompletionsClient{
		BaseURL:        strings.TrimRight(baseURL, "/"),
		APIKey:         apiKey,
		HTTPClient:     httpClient,
		MaxRetries:     2,
		DefaultBackoff: 10 * time.Millisecond,
	}
}

func (c *ChatCompletionsClient) Stream(ctx context.Context, req ChatRequest) ChatStream {
	maxAttempts := 1
	if c.MaxRetries > 0 {
		maxAttempts += c.MaxRetries
	}
	return &chatCompletionsStream{ctx: ctx, client: c, req: req, maxAttempts: maxAttempts}
}

type chatCompletionsStream struct {
	ctx         context.Context
	client      *ChatCompletionsClient
	req         ChatRequest
	sse         *SSEStream
	body        io.Closer
	current     ChatStreamEvent
	err         error
	opened      bool
	seenEvent   bool
	attempts    int
	maxAttempts int
}

func (s *chatCompletionsStream) Next() bool {
	if s.err != nil {
		return false
	}
	for {
		if !s.opened {
			s.opened = true
			if !s.open() {
				return false
			}
		}
		if s.sse == nil {
			return false
		}
		if s.sse.Next() {
			s.current = s.sse.Current()
			if s.current.Error != nil {
				s.err = chatStreamProviderError(s.current.Error)
				s.closeBody()
				return false
			}
			s.seenEvent = true
			return true
		}
		err := s.sse.Err()
		s.closeBody()
		if err != nil {
			s.err = err
			return false
		}
		if !s.seenEvent && s.canRetryAfterEOF() {
			continue
		}
		return false
	}
}

func chatStreamProviderError(chatErr *ChatError) error {
	if chatErr == nil {
		return nil
	}
	msg := strings.TrimSpace(chatErr.Message)
	if msg == "" {
		msg = "provider stream error"
	}
	if chatErr.Code != "" || chatErr.Type != "" {
		return fmt.Errorf("chat completions stream error: %s (type=%s code=%s)", msg, chatErr.Type, chatErr.Code)
	}
	return fmt.Errorf("chat completions stream error: %s", msg)
}

func (s *chatCompletionsStream) Current() ChatStreamEvent {
	return s.current
}

func (s *chatCompletionsStream) Err() error {
	if s.err != nil {
		return s.err
	}
	if s.ctx != nil && s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	return nil
}

func (s *chatCompletionsStream) open() bool {
	for s.attempts < s.maxAttempts {
		s.attempts++
		attempt := s.attempts
		resp, err := s.client.doRequest(s.ctx, s.req)
		if err != nil {
			if !s.retryBeforeFirstEvent(attempt, s.maxAttempts, 0) {
				if s.ctx.Err() != nil {
					err = s.ctx.Err()
				}
				s.err = err
				return false
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			retry := retryableStatus(resp.StatusCode)
			delay := retryAfter(resp.Header.Get("Retry-After"), s.client.DefaultBackoff)
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			err := fmt.Errorf("chat completions returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			if retry && s.retryBeforeFirstEvent(attempt, s.maxAttempts, delay) {
				continue
			}
			if s.ctx.Err() != nil {
				err = s.ctx.Err()
			}
			s.err = err
			return false
		}
		s.body = resp.Body
		s.sse = NewSSEStream(resp.Body)
		return true
	}
	return false
}

func (s *chatCompletionsStream) retryBeforeFirstEvent(attempt, maxAttempts int, delay time.Duration) bool {
	if s.seenEvent || attempt >= maxAttempts {
		return false
	}
	if delay <= 0 {
		delay = s.client.DefaultBackoff
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *chatCompletionsStream) closeBody() {
	if s.body != nil {
		_ = s.body.Close()
		s.body = nil
	}
}

func (s *chatCompletionsStream) canRetryAfterEOF() bool {
	s.sse = nil
	s.opened = false
	return s.retryBeforeFirstEvent(s.attempts, s.maxAttempts, 0)
}

func (c *ChatCompletionsClient) doRequest(ctx context.Context, req ChatRequest) (*http.Response, error) {
	raw, err := json.Marshal(chatCompletionRequestBody(req))
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	return c.HTTPClient.Do(httpReq)
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func retryAfter(header string, fallback time.Duration) time.Duration {
	if header == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(header); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(header); err == nil {
		if delay := time.Until(at); delay > 0 {
			return delay
		}
	}
	return fallback
}

type chatCompletionBody struct {
	Model             string             `json:"model"`
	Messages          []chatMessageBody  `json:"messages"`
	Stream            bool               `json:"stream"`
	StreamOptions     *streamOptionsBody `json:"stream_options,omitempty"`
	ParallelToolCalls *bool              `json:"parallel_tool_calls,omitempty"`
	Tools             []toolBody         `json:"tools,omitempty"`
	Temperature       *float64           `json:"temperature,omitempty"`
	TopP              *float64           `json:"top_p,omitempty"`
	PresencePenalty   *float64           `json:"presence_penalty,omitempty"`
	MaxTokens         *int               `json:"max_tokens,omitempty"`
}

type streamOptionsBody struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessageBody struct {
	Role       string         `json:"role"`
	Content    any            `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []toolCallBody `json:"tool_calls,omitempty"`
}

type toolCallBody struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function toolFunctionBody `json:"function"`
}

type toolFunctionBody struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolBody struct {
	Type     string             `json:"type"`
	Function toolDefinitionBody `json:"function"`
}

type toolDefinitionBody struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

func chatCompletionRequestBody(req ChatRequest) chatCompletionBody {
	body := chatCompletionBody{
		Model:           req.Model,
		Messages:        make([]chatMessageBody, 0, len(req.Messages)),
		Stream:          true,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		PresencePenalty: req.PresencePenalty,
		MaxTokens:       req.MaxTokens,
	}
	if req.IncludeUsage {
		body.StreamOptions = &streamOptionsBody{IncludeUsage: true}
	}
	if req.DisableParallelToolCalls {
		parallelToolCalls := false
		body.ParallelToolCalls = &parallelToolCalls
	}
	for _, msg := range req.Messages {
		body.Messages = append(body.Messages, chatMessageRequestBody(msg))
	}
	for _, tool := range req.Tools {
		body.Tools = append(body.Tools, toolBody{
			Type: "function",
			Function: toolDefinitionBody{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}
	return body
}

func chatMessageRequestBody(msg ChatMessage) chatMessageBody {
	out := chatMessageBody{
		Role:       msg.Role,
		ToolCallID: msg.ToolCallID,
	}
	if len(msg.ContentParts) > 0 {
		out.Content = msg.ContentParts
	} else {
		out.Content = msg.Content
	}
	for _, call := range msg.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, toolCallBody{
			ID:   call.ID,
			Type: "function",
			Function: toolFunctionBody{
				Name:      call.Name,
				Arguments: call.Args,
			},
		})
	}
	return out
}
