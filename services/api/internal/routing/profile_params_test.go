package routing

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestValidateBRouterProfileParamsRejectsUnknownKey(t *testing.T) {
	params := map[string]string{"SAC_scale_limit": "2", "unknown_param": "1"}
	if err := ValidateBRouterProfileParams("hiking-mountain", params); err == nil {
		t.Fatal("expected unknown_param to be rejected")
	}
}

func TestValidateBRouterProfileParamsAllowsKnownSurface(t *testing.T) {
	params := map[string]string{"SAC_scale_limit": "2", "consider_elevation": "true"}
	if err := ValidateBRouterProfileParams("hiking-mountain", params); err != nil {
		t.Fatalf("ValidateBRouterProfileParams returned error: %v", err)
	}
}

func TestBRouterProfileParamSurfaceCoversDocumentedProfileVars(t *testing.T) {
	root := findProjectRoot()
	if root == "" {
		t.Fatal("project root not found")
	}
	re := regexp.MustCompile(`%([A-Za-z0-9_.-]+)%`)
	for profile, surface := range brouterProfileParamSurface {
		path := filepath.Join(root, "data", "brouter", "profiles", profile+".brf")
		data, err := os.ReadFile(path)
		if err != nil {
			if profile == "hiking-sac2" {
				continue
			}
			t.Fatalf("read %s: %v", path, err)
		}
		for _, match := range re.FindAllSubmatch(data, -1) {
			key := string(match[1])
			if !surface[key] {
				t.Fatalf("%s documents profile param %q but validator surface does not allow it", profile, key)
			}
		}
	}
}
