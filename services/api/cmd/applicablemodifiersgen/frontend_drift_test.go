package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/applicablemodifiers"
)

func TestQuickEditFrontendTSMatchesYAML(t *testing.T) {
	// Paths are relative to the package directory (services/api/cmd/applicablemodifiersgen).
	modifiersPath := filepath.Join("..", "..", "..", "..", "config", "sport-taxonomy", "modifiers.yaml")
	tsPath := filepath.Join("..", "..", "..", "..", "frontend", "src", "lib", "generatedQuickEditApplicableModifiers.ts")

	keys, err := applicablemodifiers.LoadForSubsystem(modifiersPath, "quick-edit")
	if err != nil {
		t.Fatalf("load modifiers: %v", err)
	}

	// RenderTS is called with the subsystem name (same value passed via --subsystem to the CLI).
	want, err := applicablemodifiers.RenderTS("quick-edit", keys)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	got, err := os.ReadFile(tsPath)
	if err != nil {
		t.Fatalf("read ts: %v", err)
	}

	if string(got) != want {
		t.Fatal("ts file is stale; run scripts/generate-applicable-modifiers.sh")
	}
}
