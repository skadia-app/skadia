package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckFileReportsDiffForStaleGeneratedOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated.go")
	if err := os.WriteFile(path, []byte("package old\n"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	err := checkFile(path, []byte("package new\n"))
	if err == nil {
		t.Fatal("nil for stale output")
	}
	for _, want := range []string{"is stale", "+package new", "-package old"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in error: %v", want, err)
		}
	}
}

func TestCheckFilePassesForCurrentOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated.go")
	content := []byte("package current\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := checkFile(path, content); err != nil {
		t.Fatalf("unexpected error for current output: %v", err)
	}
}
