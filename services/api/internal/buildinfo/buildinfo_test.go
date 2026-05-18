package buildinfo

import "testing"

func TestSnapshotUsesUnknownDefaults(t *testing.T) {
	oldCommit, oldCommitTime, oldBuildTime := GitCommit, CommitTime, BuildTime
	t.Cleanup(func() { GitCommit, CommitTime, BuildTime = oldCommit, oldCommitTime, oldBuildTime })
	GitCommit = ""
	CommitTime = ""
	BuildTime = ""

	snap := Snapshot()
	if snap.GitCommit != "unknown" || snap.CommitTime != "unknown" || snap.BuildTime != "unknown" {
		t.Fatalf("Snapshot() = %+v, want unknown commit/commit time/build time", snap)
	}
}

func TestSnapshotIncludesInjectedValues(t *testing.T) {
	oldCommit, oldCommitTime, oldBuildTime := GitCommit, CommitTime, BuildTime
	t.Cleanup(func() { GitCommit, CommitTime, BuildTime = oldCommit, oldCommitTime, oldBuildTime })
	GitCommit = "abc123"
	CommitTime = "2026-05-17T09:58:00Z"
	BuildTime = "2026-05-17T10:00:00Z"

	snap := Snapshot()
	if snap.GitCommit != "abc123" || snap.CommitTime != "2026-05-17T09:58:00Z" || snap.BuildTime != "2026-05-17T10:00:00Z" {
		t.Fatalf("Snapshot() = %+v", snap)
	}
}
