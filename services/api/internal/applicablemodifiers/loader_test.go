package applicablemodifiers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadValidApplicableModifiersFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "applicable_modifiers.yaml")
	content := []byte("applicable_modifiers:\n  - is_electric\n  - is_indoor\n  - is_virtual\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	want := []string{"is_electric", "is_indoor", "is_virtual"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %v, want %v", got, want)
	}
}
