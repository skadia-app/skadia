package llmwire

import (
	"strings"
	"testing"
)

func TestSSEStreamParsesCRLFCommentsMultilineDataAndDone(t *testing.T) {
	stream := NewSSEStream(strings.NewReader(": ignored\r\n"+
		"data: {\"choices\":[{\"index\":0,\r\n"+
		"data: \"delta\":{\"content\":\"Hallo\"},\"finish_reason\":null}]}\r\n"+
		"\r\n"+
		"data: [DONE]\r\n"+
		"\r\n"))

	if !stream.Next() {
		t.Fatalf("Next() = false, err=%v", stream.Err())
	}
	if got := stream.Current().Delta; got != "Hallo" {
		t.Fatalf("Delta = %q, want Hallo", got)
	}
	if stream.Next() {
		t.Fatal("Next() after [DONE] = true")
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
}

func TestSSEStreamParsesFinalUsageChunkWithEmptyChoices(t *testing.T) {
	stream := NewSSEStream(strings.NewReader("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n"))

	if !stream.Next() {
		t.Fatalf("Next() = false, err=%v", stream.Err())
	}
	got := stream.Current().Usage
	if got == nil {
		t.Fatal("Usage is nil")
	}
	if got.PromptTokens != 2 || got.CompletionTokens != 3 || got.TotalTokens != 5 {
		t.Fatalf("Usage = %#v, want 2/3/5", got)
	}
}

func TestSSEStreamMalformedJSONReturnsError(t *testing.T) {
	stream := NewSSEStream(strings.NewReader("data: {not-json}\n\n"))

	if stream.Next() {
		t.Fatal("Next() = true for malformed JSON")
	}
	if err := stream.Err(); err == nil || !strings.Contains(err.Error(), "parse chat stream event") {
		t.Fatalf("Err() = %v, want parse chat stream event error", err)
	}
}

func TestSSEStreamParsesStreamErrorObject(t *testing.T) {
	stream := NewSSEStream(strings.NewReader("data: {\"error\":{\"message\":\"quota exceeded\",\"type\":\"rate_limit\",\"code\":\"429\"}}\n\n"))

	if !stream.Next() {
		t.Fatalf("Next() = false, err=%v", stream.Err())
	}
	got := stream.Current().Error
	if got == nil {
		t.Fatal("Error is nil")
	}
	if got.Message != "quota exceeded" || got.Type != "rate_limit" || got.Code != "429" {
		t.Fatalf("Error = %#v", got)
	}
}
