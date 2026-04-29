package routing

import (
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
				{Sport: "cycling", SubType: strPtr("generic")},
				{Sport: "cycling", SubType: strPtr("road")},
				{Sport: "cycling", SubType: strPtr("gravel_cycling")},
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
