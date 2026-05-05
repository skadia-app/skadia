package shadowfs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestActivityImportPathsUseCentralSchemaTagsAndPostProcessing(t *testing.T) {
	apiRoot := testAPIRoot(t)
	cases := []struct {
		name       string
		path       string
		contains   []string
		afterPairs [][2]string
	}{
		{
			name: "upload handler",
			path: "internal/handler/upload.go",
			contains: []string{
				"shadowfs.ActivitySchemaTags(shadowfs.ActivitySchemaTagsInput",
				"ImportService.PostProcess(",
			},
		},
		{
			name: "URL import handler",
			path: "internal/handler/import_url.go",
			contains: []string{
				"shadowfs.ActivitySchemaTags(shadowfs.ActivitySchemaTagsInput",
				"ImportService.PostProcess(",
			},
		},
		{
			name: "hotfolder handler",
			path: "internal/handler/hotfolder.go",
			contains: []string{
				"shadowfs.ActivitySchemaTags(shadowfs.ActivitySchemaTagsInput",
				"ImportService.PostProcess(",
			},
		},
		{
			name: "manual activity handler",
			path: "internal/handler/manual_activity.go",
			contains: []string{
				"shadowfs.ActivitySchemaTags(shadowfs.ActivitySchemaTagsInput",
				"sidecar.WriteMetaCompanion(",
				"refreshActivityFilePathBestEffort(",
			},
			afterPairs: [][2]string{
				{"sidecar.WriteMetaCompanion(", "refreshActivityFilePathBestEffort("},
			},
		},
		{
			name: "Strava importer",
			path: "internal/strava/importer_activities.go",
			contains: []string{
				"shadowfs.ActivitySchemaTags(shadowfs.ActivitySchemaTagsInput",
				"ImportService.PostProcess(",
			},
		},
		{
			name: "Garmin importer",
			path: "internal/garmin/importer_activities.go",
			contains: []string{
				"shadowfs.ActivitySchemaTags(shadowfs.ActivitySchemaTagsInput",
				"ImportService.PostProcess(",
			},
		},
		{
			name: "Komoot importer",
			path: "internal/komoot/importer_activities.go",
			contains: []string{
				"shadowfs.ActivitySchemaTags(shadowfs.ActivitySchemaTagsInput",
				"ImportService.PostProcess(",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := readAPISource(t, apiRoot, tc.path)
			for _, fragment := range tc.contains {
				if !strings.Contains(src, fragment) {
					t.Fatalf("%s missing %q", tc.path, fragment)
				}
			}
			for _, pair := range tc.afterPairs {
				before := strings.Index(src, pair[0])
				after := strings.Index(src, pair[1])
				if before < 0 || after < 0 {
					t.Fatalf("%s missing ordered fragments %q -> %q", tc.path, pair[0], pair[1])
				}
				if after <= before {
					t.Fatalf("%s has %q before %q; refresh must happen after schema-relevant DB/sidecar updates", tc.path, pair[1], pair[0])
				}
			}
		})
	}
}

func testAPIRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readAPISource(t *testing.T, apiRoot, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(apiRoot, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}
