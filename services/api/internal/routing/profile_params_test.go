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

func TestValidateBRouterProfileParamsRejectsInvalidProcessUnusedTagsValue(t *testing.T) {
	for _, value := range []string{"maybe", "2"} {
		params := map[string]string{"processUnusedTags": value}
		if err := ValidateBRouterProfileParams("hiking-mountain", params); err == nil {
			t.Fatalf("processUnusedTags=%q accepted, want rejection", value)
		}
	}
}

func TestValidateBRouterProfileParamsAllowsNumericProcessUnusedTagsValues(t *testing.T) {
	for _, value := range []string{"0", "1"} {
		params := map[string]string{"processUnusedTags": value}
		if err := ValidateBRouterProfileParams("hiking-mountain", params); err != nil {
			t.Fatalf("processUnusedTags=%q rejected: %v", value, err)
		}
	}
}

func TestValidateCustomBRouterProfileParamsAllowsProcessUnusedTagsOnly(t *testing.T) {
	if err := ValidateCustomBRouterProfileParams("custom-trail", map[string]string{"processUnusedTags": "1"}); err != nil {
		t.Fatalf("ValidateCustomBRouterProfileParams returned error: %v", err)
	}
	if err := ValidateCustomBRouterProfileParams("custom-trail", map[string]string{"unknown_param": "1"}); err == nil {
		t.Fatal("expected custom profile unknown param to be rejected")
	}
}

func TestBRouterProfileParamSurfaceCoversDocumentedProfileVars(t *testing.T) {
	root := findProjectRoot()
	if root == "" {
		t.Fatal("project root not found")
	}
	re := regexp.MustCompile(`%([A-Za-z0-9_.-]+)%`)
	for profile, surface := range brouterProfileParamSurface {
		path := filepath.Join(root, "config", "brouter", "profiles", profile+".brf")
		data, err := os.ReadFile(path)
		if err != nil {
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
