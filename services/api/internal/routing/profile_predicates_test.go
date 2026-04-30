package routing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestDefaultPredicates(t *testing.T) {
	empty := ""

	tests := []struct {
		profile string
		want    []sport.Predicate
	}{
		{
			profile: "mtb",
			want:    []sport.Predicate{{Sport: "cycling", SubType: strPtr("mountain")}},
		},
		{
			profile: "gravel",
			want:    []sport.Predicate{{Sport: "cycling", SubType: strPtr("gravel_cycling")}},
		},
		{
			profile: "road",
			want: []sport.Predicate{
				{Sport: "cycling", SubType: &empty},
				{Sport: "cycling", SubType: strPtr("road")},
				{Sport: "cycling", SubType: strPtr("gravel_cycling")},
			},
		},
		{
			profile: "trekking",
			want: []sport.Predicate{
				{Sport: "cycling", SubType: &empty},
				{Sport: "cycling", SubType: strPtr("road")},
				{Sport: "cycling", SubType: strPtr("mixed_surface")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.profile, func(t *testing.T) {
			got := DefaultPredicates(tt.profile)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DefaultPredicates(%q) = %#v, want %#v", tt.profile, got, tt.want)
			}
		})
	}
}

func TestLoadProfilesFromDirFillsAndValidatesPredicates(t *testing.T) {
	root := findProjectRoot()
	if root == "" {
		t.Fatal("could not find project root")
	}
	profiles, err := LoadProfilesFromDir(filepath.Join(root, "config", "badrouter"))
	if err != nil {
		t.Fatalf("LoadProfilesFromDir() error = %v", err)
	}

	mtb := profiles["mtb"]
	if !reflect.DeepEqual(mtb.Predicates, []sport.Predicate{{Sport: "cycling", SubType: strPtr("mountain")}}) {
		t.Fatalf("mtb predicates = %#v", mtb.Predicates)
	}
	if !reflect.DeepEqual(mtb.SportTypes, []string{"cycling"}) {
		t.Fatalf("mtb sport types = %#v, want canonical [cycling]", mtb.SportTypes)
	}
}

func TestBuiltinProfileFilesDoNotStoreGenericSubtypeSentinel(t *testing.T) {
	root := findProjectRoot()
	if root == "" {
		t.Fatal("could not find project root")
	}
	matches, err := filepath.Glob(filepath.Join(root, "config", "badrouter", "*.json"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			var cfg ProfileConfig
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			for i, pred := range cfg.Predicates {
				if pred.SubType != nil && *pred.SubType == "generic" {
					t.Fatalf("predicate %d stores sub_type generic; use omitted sub_type for broad match or empty string for exact no-subtype", i)
				}
			}
		})
	}
}

func strPtr(v string) *string {
	return &v
}

func TestValidateProfileConfigRejectsInvalidPredicates(t *testing.T) {
	err := ValidateProfileConfig(ProfileConfig{
		Profile:    "mtb",
		Predicates: []sport.Predicate{{Sport: "cycling", SubType: strPtr("does_not_exist")}},
	})
	if err == nil {
		t.Fatal("ValidateProfileConfig() expected invalid predicate error")
	}
}
