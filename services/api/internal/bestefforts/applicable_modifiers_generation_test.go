package bestefforts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/applicablemodifiers"
)

func TestGeneratedGoFileMatchesYAML(t *testing.T) {
	modifiersPath := filepath.Join("..", "..", "..", "..", "config", "sport-taxonomy", "modifiers.yaml")
	keys, err := applicablemodifiers.LoadForSubsystem(modifiersPath, "bestefforts")
	if err != nil {
		t.Fatalf("load YAML: %v", err)
	}
	want, err := applicablemodifiers.RenderGo("bestefforts", "Records", keys)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	got, err := os.ReadFile("applicable_modifiers_generated.go")
	if err != nil {
		t.Fatalf("read generated: %v", err)
	}
	if string(got) != want {
		t.Fatalf("generated file is stale. Run scripts/generate-applicable-modifiers.sh.\nFirst diff line:\n%s",
			firstDiffLine(string(got), want))
	}
}

// firstDiffLine returns the first differing line in "got: ... want: ..." form.
func firstDiffLine(got, want string) string {
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
