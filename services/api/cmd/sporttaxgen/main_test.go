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
		t.Fatalf("write generated fixture: %v", err)
	}

	err := checkFile(path, []byte("package new\n"))
	if err == nil {
		t.Fatal("checkFile returned nil for stale output")
	}
	message := err.Error()
	for _, want := range []string{
		"is stale",
		"--- " + path,
		"+++ expected",
		"-package old",
		"+package new",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("checkFile error missing %q:\n%s", want, message)
		}
	}
}

func TestCheckGeneratedFilesReportsEveryStaleFile(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.go")
	second := filepath.Join(dir, "second.go")
	if err := os.WriteFile(first, []byte("old first\n"), 0o644); err != nil {
		t.Fatalf("write first fixture: %v", err)
	}
	if err := os.WriteFile(second, []byte("old second\n"), 0o644); err != nil {
		t.Fatalf("write second fixture: %v", err)
	}

	err := checkGeneratedFiles([]generatedOutput{
		{path: first, want: []byte("new first\n")},
		{path: second, want: []byte("new second\n")},
	})
	if err == nil {
		t.Fatal("checkGeneratedFiles returned nil for stale outputs")
	}
	message := err.Error()
	for _, want := range []string{
		first + " is stale",
		second + " is stale",
		"-old first",
		"+new first",
		"-old second",
		"+new second",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("checkGeneratedFiles error missing %q:\n%s", want, message)
		}
	}
}
