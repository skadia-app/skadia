package gear

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/applicablemodifiers"
)

func TestGeneratedGoFileMatchesYAML(t *testing.T) {
	yamlPath := filepath.Join("..", "..", "..", "..", "config", "gear", "applicable_modifiers.yaml")
	keys, err := applicablemodifiers.Load(yamlPath)
	if err != nil {
		t.Fatalf("load YAML: %v", err)
	}
	want, err := applicablemodifiers.RenderGo("gear", "Gear", keys)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	got, err := os.ReadFile("applicable_modifiers_generated.go")
	if err != nil {
		t.Fatalf("read generated: %v", err)
	}
	if string(got) != want {
		t.Fatalf("generated file is stale. Run scripts/generate-gear-applicable-modifiers.sh.\nFirst diff line:\n%s",
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
