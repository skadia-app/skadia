package bestefforts

import (
	"context"
	"testing"

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
