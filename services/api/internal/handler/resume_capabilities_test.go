package handler

import "testing"

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
