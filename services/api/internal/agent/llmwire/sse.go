package llmwire

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type SSEStream struct {
	reader  *bufio.Reader
	current ChatStreamEvent
	err     error
	done    bool
}

func NewSSEStream(r io.Reader) *SSEStream {
	return &SSEStream{reader: bufio.NewReader(r)}
}

func (s *SSEStream) Next() bool {
	if s == nil || s.done || s.err != nil {
		return false
	}

	var data []string
	for {
		line, readErr := s.reader.ReadString('\n')
		if readErr != nil && len(line) == 0 {
			if readErr != io.EOF {
				s.err = readErr
			}
			return false
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(data) == 0 {
				if readErr == io.EOF {
					return false
				}
				continue
			}
			return s.parseEvent(strings.Join(data, "\n"))
		}
		if strings.HasPrefix(line, ":") {
			if readErr == io.EOF && len(data) == 0 {
				return false
			}
			continue
		}
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			value = strings.TrimPrefix(value, " ")
			data = append(data, value)
		}

		if readErr == io.EOF {
			if len(data) == 0 {
				return false
			}
			return s.parseEvent(strings.Join(data, "\n"))
		}
	}
}

func (s *SSEStream) Current() ChatStreamEvent {
	if s == nil {
		return ChatStreamEvent{}
	}
	return s.current
}

func (s *SSEStream) Err() error {
	if s == nil {
		return nil
	}
	return s.err
}

func (s *SSEStream) parseEvent(data string) bool {
	if strings.TrimSpace(data) == "[DONE]" {
		s.done = true
		return false
	}
	event, err := parseChatStreamEvent([]byte(data))
	if err != nil {
		s.err = err
		return false
	}
	s.current = event
	return true
}

func parseChatStreamEvent(raw []byte) (ChatStreamEvent, error) {
	var chunk chatCompletionChunk
	if err := json.Unmarshal(raw, &chunk); err != nil {
		return ChatStreamEvent{}, fmt.Errorf("parse chat stream event: %w", err)
	}
	if chunk.Error != nil {
		return ChatStreamEvent{Error: chunk.Error}, nil
	}

	event := ChatStreamEvent{Usage: chunk.Usage}
	if len(chunk.Choices) == 0 {
		return event, nil
	}
	choice := chunk.Choices[0]
	event.Delta = choice.Delta.Content
	event.FinishReason = choice.FinishReason
	for _, tc := range choice.Delta.ToolCalls {
		event.ToolCalls = append(event.ToolCalls, ToolCallDelta{
			Index: tc.Index,
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Args:  tc.Function.Arguments,
		})
	}
	return event, nil
}

type chatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int64  `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *ChatUsage `json:"usage"`
	Error *ChatError `json:"error"`
}
