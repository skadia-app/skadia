package systemstatus

import (
	"context"
	"errors"
	"testing"
)

func TestStorageStatusBlocksLocalMultiReplicaWithoutSharedVolume(t *testing.T) {
	got := StorageStatusFromSnapshot(StorageSnapshot{
		Backend:       "local",
		APIReplicas:   2,
		SharedStorage: "false",
	})
	if got.State != StatusBlocked || got.ReasonCode != "local_storage_not_replica_safe" {
		t.Fatalf("status = %+v", got)
	}
}

func TestStorageStatusUnknownWhenLocalReplicaInputsMissing(t *testing.T) {
	got := StorageStatusFromSnapshot(StorageSnapshot{Backend: "local", SharedStorage: "unknown"})
	if got.State != StatusUnknown || got.ReasonCode != "replica_count_unknown" {
		t.Fatalf("status = %+v", got)
	}
}

func TestStorageStatusRequiresS3WriteDeleteProbe(t *testing.T) {
	got := StorageStatusFromSnapshot(StorageSnapshot{
		Backend:          "s3",
		HeadBucketOK:     true,
		PutGetDeleteOK:   false,
		ListPrefixOK:     true,
		MultipartAbortOK: true,
	})
	if got.State != StatusDegraded || got.ReasonCode != "s3_write_delete_probe_failed" {
		t.Fatalf("status = %+v", got)
	}
}

func TestStorageStatusRequiresS3BucketReachability(t *testing.T) {
	got := StorageStatusFromSnapshot(StorageSnapshot{
		Backend:          "s3",
		HeadBucketOK:     false,
		PutGetDeleteOK:   true,
		ListPrefixOK:     true,
		MultipartAbortOK: true,
	})
	if got.State != StatusUnavailable || got.ReasonCode != "s3_bucket_unreachable" {
		t.Fatalf("status = %+v, want unavailable s3_bucket_unreachable", got)
	}
}

func TestStorageStatusReportsS3LifecycleProbeFailure(t *testing.T) {
	got := StorageStatusFromSnapshot(StorageSnapshot{
		Backend:          "s3",
		HeadBucketOK:     true,
		PutGetDeleteOK:   true,
		ListPrefixOK:     true,
		MultipartAbortOK: false,
	})
	if got.State != StatusDegraded || got.ReasonCode != "s3_multipart_abort_probe_failed" {
		t.Fatalf("status = %+v, want degraded s3_multipart_abort_probe_failed", got)
	}
}

type fakeStorageProbe struct {
	headOK           bool
	writeDeleteOK    bool
	listOK           bool
	multipartAbortOK bool
}

func (f fakeStorageProbe) HeadBucket(context.Context) error {
	if f.headOK {
		return nil
	}
	return errors.New("head bucket failed")
}

func (f fakeStorageProbe) PutGetDelete(context.Context) error {
	if f.writeDeleteOK {
		return nil
	}
	return errors.New("write delete failed")
}

func (f fakeStorageProbe) ListPrefix(context.Context) error {
	if f.listOK {
		return nil
	}
	return errors.New("list prefix failed")
}

func (f fakeStorageProbe) MultipartAbortDryRun(context.Context) error {
	if f.multipartAbortOK {
		return nil
	}
	return errors.New("multipart abort failed")
}

func TestCollectStorageComponentUsesAllS3ProbeResults(t *testing.T) {
	got := StorageSnapshotFromProbe(context.Background(), "s3", "s3-local-garage", fakeStorageProbe{
		headOK:           true,
		writeDeleteOK:    true,
		listOK:           false,
		multipartAbortOK: true,
	})
	st := StorageStatusFromSnapshot(got)
	if st.State != StatusDegraded || st.ReasonCode != "s3_list_prefix_probe_failed" {
		t.Fatalf("status = %+v, want degraded s3_list_prefix_probe_failed", st)
	}
}
