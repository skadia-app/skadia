package importer

import "testing"

func TestActivityResultShouldPostProcessIsExplicit(t *testing.T) {
	tests := []struct {
		name   string
		result *ActivityResult
		want   bool
	}{
		{
			name:   "nil",
			result: nil,
			want:   false,
		},
		{
			name: "metadata enrichment alone does not require child data writes",
			result: &ActivityResult{
				Status:                "enriched",
				PlatformIDsBackfilled: []string{"komoot_tour_id"},
			},
			want: false,
		},
		{
			name: "replacement requires child data writes",
			result: &ActivityResult{
				Status:              "replaced",
				PostProcessRequired: true,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.ShouldPostProcess(); got != tt.want {
				t.Fatalf("ShouldPostProcess() = %v, want %v", got, tt.want)
			}
		})
	}
}
