package sporttaxgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/applicablemodifiers"
)

func TestGeneratedKnownSubsystemsMatchesRegistry(t *testing.T) {
	registryPath := filepath.Join("..", "..", "..", "..", "config", "applicable-modifiers", "subsystems.yaml")
	subs, err := applicablemodifiers.LoadSubsystemRegistry(registryPath)
	if err != nil {
		t.Fatalf("LoadSubsystemRegistry: %v", err)
	}

	want, err := RenderKnownSubsystems(subs)
	if err != nil {
		t.Fatalf("RenderKnownSubsystems: %v", err)
	}

	got, err := os.ReadFile("subsystems_generated.go")
	if err != nil {
		t.Fatalf("read subsystems_generated.go: %v", err)
	}

	if string(got) != want {
		t.Fatalf("subsystems_generated.go is stale. Run scripts/generate-applicable-modifiers.sh.\nFirst diff line:\n%s",
			firstDiffLineSubsystems(string(got), want))
	}
}

func firstDiffLineSubsystems(got, want string) string {
	gotLines := strings.Split(got, "\n")
	wantLines := strings.Split(want, "\n")
	for i := 0; i < len(gotLines) && i < len(wantLines); i++ {
		if gotLines[i] != wantLines[i] {
			return "got:  " + gotLines[i] + "\nwant: " + wantLines[i]
		}
	}
	if len(gotLines) != len(wantLines) {
		return "got and want have different line counts"
	}
	return "(no diff found)"
}
