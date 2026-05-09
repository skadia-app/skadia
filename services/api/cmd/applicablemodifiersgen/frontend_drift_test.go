package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/applicablemodifiers"
)

// repoRoot returns the absolute path to the repository root, relative to this package.
func repoRoot() string {
	return filepath.Join("..", "..", "..", "..")
}

func TestFrontendTSFilesMatchYAML(t *testing.T) {
	root := repoRoot()
	modifiersPath := filepath.Join(root, "config", "sport-taxonomy", "modifiers.yaml")

	cases := []struct {
		subsystem    string
		exportPrefix string
		tsPath       string
	}{
		{
			subsystem:    "gear",
			exportPrefix: "Gear",
			tsPath:       filepath.Join(root, "frontend", "src", "lib", "generatedGearApplicableModifiers.ts"),
		},
		{
			subsystem:    "heatmap",
			exportPrefix: "Heatmap",
			tsPath:       filepath.Join(root, "frontend", "src", "lib", "generatedHeatmapApplicableModifiers.ts"),
		},
		{
			subsystem:    "quick-edit",
			exportPrefix: "QuickEdit",
			tsPath:       filepath.Join(root, "frontend", "src", "lib", "generatedQuickEditApplicableModifiers.ts"),
		},
		{
			subsystem:    "routefilter",
			exportPrefix: "Routing",
			tsPath:       filepath.Join(root, "frontend", "src", "lib", "generatedRoutingApplicableModifiers.ts"),
		},
		{
			subsystem:    "rules",
			exportPrefix: "Rules",
			tsPath:       filepath.Join(root, "frontend", "src", "lib", "generatedRulesApplicableModifiers.ts"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.subsystem, func(t *testing.T) {
			keys, err := applicablemodifiers.LoadForSubsystem(modifiersPath, tc.subsystem)
			if err != nil {
				t.Fatalf("load modifiers for %s: %v", tc.subsystem, err)
			}

			want, err := applicablemodifiers.RenderTS(tc.exportPrefix, keys)
			if err != nil {
				t.Fatalf("render TS for %s: %v", tc.subsystem, err)
			}

			got, err := os.ReadFile(tc.tsPath)
			if err != nil {
				t.Fatalf("read ts file for %s: %v", tc.subsystem, err)
			}

			if string(got) != want {
				t.Fatalf("ts file for %s is stale; run scripts/generate-applicable-modifiers.sh", tc.subsystem)
			}
		})
	}
}
