package handler

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"
)

func TestResumeJobAllowedByCapabilitiesBlocksFullOnlySegmentJobsInCore(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "core"}}
	for _, jobType := range []string{"segment_recompute", "segment_publish"} {
		if h.resumeJobAllowedByCapabilities(jobType) {
			t.Fatalf("%s allowed in core", jobType)
		}
	}
}

func TestResumeJobAllowedByCapabilitiesKeepsCoreSafeJobs(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "core"}}
	for _, jobType := range []string{"recalc_best_efforts", "recalc_best_efforts_all"} {
		if !h.resumeJobAllowedByCapabilities(jobType) {
			t.Fatalf("%s blocked in core", jobType)
		}
	}
}

func TestResumeJobAllowedByCapabilitiesAllowsSegmentJobsInFull(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "full"}}
	for _, jobType := range []string{"segment_recompute", "segment_publish"} {
		if !h.resumeJobAllowedByCapabilities(jobType) {
			t.Fatalf("%s blocked in full", jobType)
		}
	}
}

func TestSegmentJobResumeUsesSegmentScopedRematch(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "resume.go", nil, 0)
	if err != nil {
		t.Fatalf("parse resume.go: %v", err)
	}

	for _, name := range []string{"resumeSegmentPublish", "resumeSegmentRecompute"} {
		fn := findFunction(file, name)
		if fn == nil {
			t.Fatalf("%s not found", name)
		}

		var buf bytes.Buffer
		if err := printer.Fprint(&buf, fset, fn.Body); err != nil {
			t.Fatalf("print %s body: %v", name, err)
		}
		if strings.Contains(buf.String(), "MatchActivitySegments(") {
			t.Fatalf("%s calls global MatchActivitySegments; resume must only recompute the resumed segment", name)
		}
		if !strings.Contains(buf.String(), "UpdateSegmentEffortRanks(ctx, segmentID)") {
			t.Fatalf("%s does not refresh ranks/counts for the resumed segment", name)
		}
	}
}

func TestSegmentRecomputePathsAvoidPreDelete(t *testing.T) {
	for _, tc := range []struct {
		file string
		fn   string
	}{
		{"segments.go", "recomputeSegmentEfforts"},
		{"segments.go", "RecomputeSegment"},
		{"segments.go", "PublishSegment"},
		{"resume.go", "resumeSegmentPublish"},
		{"resume.go", "resumeSegmentRecompute"},
	} {
		body := functionBodyString(t, tc.file, tc.fn)
		if strings.Contains(body, "DeleteSegmentEffortsBySegment(") || strings.Contains(body, "DeleteAllSegmentEffortsBySegment(") {
			t.Fatalf("%s.%s pre-deletes segment efforts; recompute must replace each segment/activity tuple atomically", tc.file, tc.fn)
		}
	}
}

func TestCreateBackfillRefreshesSegmentRanks(t *testing.T) {
	body := functionBodyString(t, "segments.go", "backfillSegmentEfforts")
	if !strings.Contains(body, "UpdateSegmentEffortRanks(ctx, segmentID)") {
		t.Fatalf("backfillSegmentEfforts does not refresh ranks/counts after backfill")
	}
}

func findFunction(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

func functionBodyString(t *testing.T, filename, name string) string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	fn := findFunction(file, name)
	if fn == nil {
		t.Fatalf("%s not found in %s", name, filename)
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, fn.Body); err != nil {
		t.Fatalf("print %s body: %v", name, err)
	}
	return buf.String()
}
