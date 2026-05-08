package tags

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedGoFileMatchesYAML verifies that system_tags_generated.go is current
// with respect to config/tags/system_tags.yaml. Run scripts/generate-tags.sh to regenerate.
func TestGeneratedGoFileMatchesYAML(t *testing.T) {
	repoRoot := findRepoRoot(t)
	yamlPath := filepath.Join(repoRoot, "config", "tags", "system_tags.yaml")
	schemaPath := filepath.Join(repoRoot, "config", "tags", "schema", "system_tags.schema.json")
	goOutPath := filepath.Join(repoRoot, "services", "api", "internal", "tags", "system_tags_generated.go")

	cmd := exec.Command(
		"go", "run",
		filepath.Join(repoRoot, "services", "api", "cmd", "tagsgen"),
		"--source", yamlPath,
		"--schema", schemaPath,
		"--go-out", goOutPath,
		"--check",
	)
	cmd.Dir = filepath.Join(repoRoot, "services", "api")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("system_tags_generated.go is stale; run scripts/generate-tags.sh to regenerate.\n%s", out)
	}
}

// TestGeneratedTSFileMatchesYAML verifies that frontend/src/lib/generatedSystemTags.ts is current.
func TestGeneratedTSFileMatchesYAML(t *testing.T) {
	repoRoot := findRepoRoot(t)
	yamlPath := filepath.Join(repoRoot, "config", "tags", "system_tags.yaml")
	schemaPath := filepath.Join(repoRoot, "config", "tags", "schema", "system_tags.schema.json")
	tsOutPath := filepath.Join(repoRoot, "frontend", "src", "lib", "generatedSystemTags.ts")

	cmd := exec.Command(
		"go", "run",
		filepath.Join(repoRoot, "services", "api", "cmd", "tagsgen"),
		"--source", yamlPath,
		"--schema", schemaPath,
		"--ts-out", tsOutPath,
		"--check",
	)
	cmd.Dir = filepath.Join(repoRoot, "services", "api")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generatedSystemTags.ts is stale; run scripts/generate-tags.sh to regenerate.\n%s", out)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "..", "..")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
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
