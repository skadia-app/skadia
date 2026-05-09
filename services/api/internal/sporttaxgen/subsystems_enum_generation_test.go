package sporttaxgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/applicablemodifiers"
)

func TestGeneratedSubsystemsEnumMatchesRegistry(t *testing.T) {
	registryPath := filepath.Join("..", "..", "..", "..", "config", "applicable-modifiers", "subsystems.yaml")
	subs, err := applicablemodifiers.LoadSubsystemRegistry(registryPath)
	if err != nil {
		t.Fatalf("LoadSubsystemRegistry: %v", err)
	}

	want, err := RenderSubsystemsEnumFragment(subs)
	if err != nil {
		t.Fatalf("RenderSubsystemsEnumFragment: %v", err)
	}

	enumFragPath := filepath.Join("..", "..", "..", "..", "config", "sport-taxonomy", "schema", "_subsystems_enum.schema.json")
	got, err := os.ReadFile(enumFragPath)
	if err != nil {
		t.Fatalf("read _subsystems_enum.schema.json: %v", err)
	}

	if string(got) != want {
		t.Fatalf("_subsystems_enum.schema.json is stale. Run scripts/generate-applicable-modifiers.sh.\nFirst diff line:\n%s",
			firstDiffLineSubsystemsEnum(string(got), want))
	}
}

func firstDiffLineSubsystemsEnum(got, want string) string {
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
