package systemstatus

import (
	"context"
	"strconv"
	"time"
)

type StorageSnapshot struct {
	Backend          string
	StorageMode      string
	APIReplicas      int
	SharedStorage    string
	ReplicaSource    string
	HeadBucketOK     bool
	PutGetDeleteOK   bool
	ListPrefixOK     bool
	MultipartAbortOK bool
	CheckedAt        time.Time
}

type StorageProbe interface {
	HeadBucket(context.Context) error
	PutGetDelete(context.Context) error
	ListPrefix(context.Context) error
	MultipartAbortDryRun(context.Context) error
}

func StorageSnapshotFromProbe(ctx context.Context, backend, mode string, probe StorageProbe) StorageSnapshot {
	snap := StorageSnapshot{Backend: backend, StorageMode: mode, CheckedAt: time.Now().UTC()}
	if backend != "s3" || probe == nil {
		return snap
	}
	snap.HeadBucketOK = probe.HeadBucket(ctx) == nil
	snap.PutGetDeleteOK = probe.PutGetDelete(ctx) == nil
	snap.ListPrefixOK = probe.ListPrefix(ctx) == nil
	snap.MultipartAbortOK = probe.MultipartAbortDryRun(ctx) == nil
	return snap
}

func StorageStatusFromSnapshot(s StorageSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "storage",
		Label:     "Storage",
		State:     StatusOK,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details: map[string]string{
			"backend":       s.Backend,
			"mode":          s.StorageMode,
			"replica_count": strconv.Itoa(s.APIReplicas),
		},
	}
	if s.Backend == "local" && (s.APIReplicas == 0 || s.SharedStorage == "unknown") && s.ReplicaSource == "" {
		st.State = StatusUnknown
		st.ReasonCode = "replica_count_unknown"
		return st
	}
	if s.Backend == "local" && s.APIReplicas > 1 && s.SharedStorage != "true" {
		st.State = StatusBlocked
		st.ReasonCode = "local_storage_not_replica_safe"
		return st
	}
	if s.Backend == "s3" && !s.HeadBucketOK {
		st.State = StatusUnavailable
		st.ReasonCode = "s3_bucket_unreachable"
		return st
	}
	if s.Backend == "s3" && !s.PutGetDeleteOK {
		st.State = StatusDegraded
		st.ReasonCode = "s3_write_delete_probe_failed"
		return st
	}
	if s.Backend == "s3" && !s.ListPrefixOK {
		st.State = StatusDegraded
		st.ReasonCode = "s3_list_prefix_probe_failed"
		return st
	}
	if s.Backend == "s3" && !s.MultipartAbortOK {
		st.State = StatusDegraded
		st.ReasonCode = "s3_multipart_abort_probe_failed"
		return st
	}
	return st
}
