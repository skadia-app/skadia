package bestefforts

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

type fakeBestEffortStore struct {
	deletedActivityIDs []string
	batchUpserts       [][]BestEffortRecord
}

func (s *fakeBestEffortStore) UpsertBestEffortRec(context.Context, BestEffortRecord) error {
	return nil
}

func (s *fakeBestEffortStore) BatchUpsertBestEffortRecs(_ context.Context, records []BestEffortRecord) error {
	s.batchUpserts = append(s.batchUpserts, records)
	return nil
}

func (s *fakeBestEffortStore) DeleteAllBestEfforts(context.Context, string) error {
	return nil
}

func (s *fakeBestEffortStore) DeleteBestEffortsForActivity(_ context.Context, activityID string) error {
	s.deletedActivityIDs = append(s.deletedActivityIDs, activityID)
	return nil
}

func (s *fakeBestEffortStore) DeleteOrphanedBestEfforts(context.Context, string, []string) error {
	return nil
}

func (s *fakeBestEffortStore) AcquireBestEffortLock(context.Context, string) (*pgxpool.Conn, error) {
	return nil, nil
}

func (s *fakeBestEffortStore) TryAcquireBestEffortLock(context.Context, string) (*pgxpool.Conn, bool, error) {
	return nil, true, nil
}

func (s *fakeBestEffortStore) GetBestEffortsForActivityRec(context.Context, string, string) ([]BestEffortRankedRecord, error) {
	return []BestEffortRankedRecord{}, nil
}

func (s *fakeBestEffortStore) GetBestEffortRecalcActivities(context.Context, string) ([]ActivityInfo, error) {
	return nil, nil
}

func (s *fakeBestEffortStore) GetBestEffortActivityInfo(context.Context, string) (*ActivityInfo, error) {
	return nil, nil
}

func (s *fakeBestEffortStore) GetStreamPoints(context.Context, string) ([]model.StreamPoint, error) {
	return nil, nil
}

func (s *fakeBestEffortStore) GetStreamPointsLightweight(context.Context, string) ([]model.StreamPointLight, error) {
	return nil, nil
}

func TestComputeForActivityInnerDeletesStaleRowsWhenActivityNoLongerHasRecordGroup(t *testing.T) {
	store := &fakeBestEffortStore{}
	info := ActivityInfo{
		ID:        "activity-unsupported",
		UserID:    "user-1",
		SportType: "alpine_skiing",
	}

	got, err := computeForActivityInner[model.StreamPointLight](context.Background(), store, info, nil)
	if err != nil {
		t.Fatalf("computeForActivityInner: %v", err)
	}
	if got != nil {
		t.Fatalf("computeForActivityInner returned %v, want nil for unsupported activity", got)
	}
	if len(store.deletedActivityIDs) != 1 || store.deletedActivityIDs[0] != info.ID {
		t.Fatalf("deleted activities = %#v, want [%q]", store.deletedActivityIDs, info.ID)
	}
	if len(store.batchUpserts) != 0 {
		t.Fatalf("batch upserts = %#v, want none", store.batchUpserts)
	}
}

func TestComputeBestEffortRecordsDoesNotPersist(t *testing.T) {
	store := &fakeBestEffortStore{}
	info := ActivityInfo{
		ID:             "activity-running",
		UserID:         "user-1",
		SportType:      "running",
		StartedAt:      time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		DistanceM:      10_000,
		ElevationGainM: 200,
		ElapsedTimeS:   3_600,
		BiggestClimbM:  100,
	}

	records := computeBestEffortRecords[model.StreamPointLight](info, nil)

	if len(records) == 0 {
		t.Fatal("computeBestEffortRecords returned no records for running activity")
	}
	if len(store.deletedActivityIDs) != 0 {
		t.Fatalf("deleted activities = %#v, want none", store.deletedActivityIDs)
	}
	if len(store.batchUpserts) != 0 {
		t.Fatalf("batch upserts = %#v, want none", store.batchUpserts)
	}
}

func TestComputeForActivityInnerPersistsComputedRecords(t *testing.T) {
	store := &fakeBestEffortStore{}
	info := ActivityInfo{
		ID:             "activity-running",
		UserID:         "user-1",
		SportType:      "running",
		StartedAt:      time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		DistanceM:      10_000,
		ElevationGainM: 200,
		ElapsedTimeS:   3_600,
		BiggestClimbM:  100,
	}

	if _, err := computeForActivityInner[model.StreamPointLight](context.Background(), store, info, nil); err != nil {
		t.Fatalf("computeForActivityInner: %v", err)
	}
	if len(store.deletedActivityIDs) != 1 || store.deletedActivityIDs[0] != info.ID {
		t.Fatalf("deleted activities = %#v, want [%q]", store.deletedActivityIDs, info.ID)
	}
	if len(store.batchUpserts) != 1 {
		t.Fatalf("batch upserts = %#v, want one call", store.batchUpserts)
	}
	if len(store.batchUpserts[0]) == 0 {
		t.Fatal("computeForActivityInner persisted no records for running activity")
	}
}
