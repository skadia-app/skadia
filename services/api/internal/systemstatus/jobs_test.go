package systemstatus

import "testing"

func TestJobStatusDegradesOnStuckJobs(t *testing.T) {
	got := JobStatusFromSnapshot(JobSnapshot{Running: 2, Stuck: 1, FailedLast24h: 0})
	if got.State != StatusDegraded || got.ReasonCode != "stuck_jobs" {
		t.Fatalf("status = %+v", got)
	}
}

func TestGDPRStatusDegradesOnExpiredArtifactBacklog(t *testing.T) {
	got := GDPRArtifactStatusFromSnapshot(GDPRArtifactSnapshot{ActiveExports: 1, ExpiredPendingCleanup: 3})
	if got.State != StatusDegraded || got.ReasonCode != "expired_artifact_backlog" {
		t.Fatalf("status = %+v", got)
	}
}

func TestGDPRStatusReportsIncompleteMultipartBacklog(t *testing.T) {
	got := GDPRArtifactStatusFromSnapshot(GDPRArtifactSnapshot{IncompleteMultipart: 2})
	if got.State != StatusDegraded || got.ReasonCode != "incomplete_multipart_backlog" || got.Details["incomplete_multipart_count"] != "2" {
		t.Fatalf("status = %+v, want degraded incomplete_multipart_backlog with count", got)
	}
}
