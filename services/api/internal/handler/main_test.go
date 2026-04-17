package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing"
)

// findProjectRoot walks up from the current working directory until it finds
// a directory containing config/badrouter/ — the standard BadRouter profile store.
func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "config", "badrouter")); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// TestMain loads built-in BadRouter profiles before any test in this package.
// Tests that call SuggestRoutes end-to-end require routing.BuildCostConfig,
// which panics unless profiles have been registered via LoadBuiltinProfiles.
func TestMain(m *testing.M) {
	root := findProjectRoot()
	if root == "" {
		fmt.Fprintln(os.Stderr, "FATAL: could not locate project root (config/badrouter/ not found)")
		os.Exit(1)
	}
	n, err := routing.LoadBuiltinProfiles(filepath.Join(root, "config", "badrouter"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: LoadBuiltinProfiles: %v\n", err)
		os.Exit(1)
	}
	if n == 0 {
		fmt.Fprintln(os.Stderr, "FATAL: no profiles loaded from config/badrouter/")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
