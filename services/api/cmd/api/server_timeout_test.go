package main

import (
	"testing"
	"time"
)

func TestAPIWriteTimeoutDefaultSupportsLongLocalLLMStreams(t *testing.T) {
	if got := apiWriteTimeout(); got != 10*time.Minute {
		t.Fatalf("apiWriteTimeout() = %s, want 10m", got)
	}
}
