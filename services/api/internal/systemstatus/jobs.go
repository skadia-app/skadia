package systemstatus

import (
	"strconv"
	"time"
)

type JobSnapshot struct {
	Running       int
	Stuck         int
	FailedLast24h int
	OldestAgeSec  int64
	CheckedAt     time.Time
}

type GDPRArtifactSnapshot struct {
	ActiveExports         int
	ActiveImports         int
	ExpiredPendingCleanup int
	IncompleteMultipart   int
	CounterErrors         int
	StagingBytes          int64
	CheckedAt             time.Time
}

func JobStatusFromSnapshot(s JobSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "jobs",
		Label:     "Jobs",
		State:     StatusOK,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details: map[string]string{
			"running_count":              strconv.Itoa(s.Running),
			"stuck_jobs_count":           strconv.Itoa(s.Stuck),
			"failed_last_24h_count":      strconv.Itoa(s.FailedLast24h),
			"oldest_pending_age_seconds": strconv.FormatInt(s.OldestAgeSec, 10),
		},
	}
	if s.Stuck > 0 {
		st.State = StatusDegraded
		st.ReasonCode = "stuck_jobs"
	}
	return st
}

func GDPRArtifactStatusFromSnapshot(s GDPRArtifactSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "gdpr_artifacts",
		Label:     "GDPR artifacts",
		State:     StatusOK,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details: map[string]string{
			"active_exports_count":          strconv.Itoa(s.ActiveExports),
			"active_imports_count":          strconv.Itoa(s.ActiveImports),
			"expired_pending_cleanup_count": strconv.Itoa(s.ExpiredPendingCleanup),
			"incomplete_multipart_count":    strconv.Itoa(s.IncompleteMultipart),
			"artifact_counter_error_count":  strconv.Itoa(s.CounterErrors),
			"staging_bytes":                 strconv.FormatInt(s.StagingBytes, 10),
		},
	}
	if s.CounterErrors > 0 {
		st.State = StatusDegraded
		st.ReasonCode = "artifact_store_probe_failed"
		return st
	}
	if s.ExpiredPendingCleanup > 0 {
		st.State = StatusDegraded
		st.ReasonCode = "expired_artifact_backlog"
		return st
	}
	if s.IncompleteMultipart > 0 {
		st.State = StatusDegraded
		st.ReasonCode = "incomplete_multipart_backlog"
	}
	return st
}
