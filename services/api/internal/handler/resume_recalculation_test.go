package handler

import (
	"os"
	"strings"
	"testing"
)

func TestSegmentRecalculationWorkerStartIsBoundedPerProcess(t *testing.T) {
	src, err := os.ReadFile("resume.go")
	if err != nil {
		t.Fatalf("read resume.go: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "segmentRecalculationWorkerRunning.CompareAndSwap(false, true)") {
		t.Fatalf("startSegmentRecalculationRuns does not guard duplicate workers")
	}
	if !strings.Contains(body, "defer segmentRecalculationWorkerRunning.Store(false)") {
		t.Fatalf("startSegmentRecalculationRuns does not release the worker guard")
	}
}
