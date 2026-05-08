package bestefforts

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

type fakeBestEffortStore struct {
	deletedActivityIDs      []string
	batchUpserts            [][]BestEffortRecord
	replaceCalls            []replaceBestEffortsCall
	replaceErrAtCall        int
	orphanedKeepActivityIDs []string
	recalcActivities        []ActivityInfo
	streams                 map[string][]model.StreamPointLight
	streamErrForActivity    string
}

type replaceBestEffortsCall struct {
	activityIDs []string
	records     []BestEffortRecord
}

func (s *fakeBestEffortStore) UpsertBestEffortRec(context.Context, BestEffortRecord) error {
	return nil
}

func (s *fakeBestEffortStore) BatchUpsertBestEffortRecs(_ context.Context, records []BestEffortRecord) error {
	s.batchUpserts = append(s.batchUpserts, records)
	return nil
}

func (s *fakeBestEffortStore) ReplaceBestEffortsForActivities(_ context.Context, activityIDs []string, records []BestEffortRecord) error {
	s.replaceCalls = append(s.replaceCalls, replaceBestEffortsCall{
		activityIDs: append([]string(nil), activityIDs...),
		records:     append([]BestEffortRecord(nil), records...),
	})
	if s.replaceErrAtCall > 0 && len(s.replaceCalls) == s.replaceErrAtCall {
		return errors.New("replace failed")
	}
	return nil
}

func (s *fakeBestEffortStore) DeleteAllBestEfforts(context.Context, string) error {
	return nil
}

func (s *fakeBestEffortStore) DeleteBestEffortsForActivity(_ context.Context, activityID string) error {
	s.deletedActivityIDs = append(s.deletedActivityIDs, activityID)
	return nil
}

func (s *fakeBestEffortStore) DeleteOrphanedBestEfforts(_ context.Context, _ string, keepActivityIDs []string) error {
	s.orphanedKeepActivityIDs = append([]string(nil), keepActivityIDs...)
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
	return s.recalcActivities, nil
}

func (s *fakeBestEffortStore) GetBestEffortActivityInfo(context.Context, string) (*ActivityInfo, error) {
	return nil, nil
}

func (s *fakeBestEffortStore) GetStreamPoints(context.Context, string) ([]model.StreamPoint, error) {
	return nil, nil
}

func (s *fakeBestEffortStore) GetStreamPointsLightweight(_ context.Context, activityID string) ([]model.StreamPointLight, error) {
	if s.streamErrForActivity == activityID {
		return nil, errors.New("stream failed")
	}
	if s.streams == nil {
		return nil, nil
	}
	return s.streams[activityID], nil
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

func TestRecalcAllLoadedActivitiesReplacesBatchesAndCleansOrphans(t *testing.T) {
	activities := []ActivityInfo{
		testBestEffortActivity("activity-running-1", "running", 10_000, 3_600),
		testBestEffortActivity("activity-unsupported", "alpine_skiing", 10_000, 3_600),
		testBestEffortActivity("activity-running-zero", "running", 0, 0),
	}
	store := &fakeBestEffortStore{}

	err := recalcAllLoadedActivities(context.Background(), store, "user-1", activities, 1, nil)
	if err != nil {
		t.Fatalf("recalcAllLoadedActivities: %v", err)
	}

	if len(store.deletedActivityIDs) != 0 {
		t.Fatalf("per-activity deletes = %#v, want none", store.deletedActivityIDs)
	}
	if len(store.batchUpserts) != 0 {
		t.Fatalf("direct batch upserts = %#v, want none", store.batchUpserts)
	}
	if got, want := len(store.replaceCalls), 3; got != want {
		t.Fatalf("replace call count = %d, want %d", got, want)
	}
	for i, activity := range activities {
		call := store.replaceCalls[i]
		if !reflect.DeepEqual(call.activityIDs, []string{activity.ID}) {
			t.Fatalf("replace call %d activity IDs = %#v, want [%q]", i, call.activityIDs, activity.ID)
		}
	}
	if len(store.replaceCalls[0].records) == 0 {
		t.Fatal("supported running activity replacement contained no computed records")
	}
	if len(store.replaceCalls[1].records) != 0 {
		t.Fatalf("unsupported activity replacement records = %#v, want none", store.replaceCalls[1].records)
	}
	if len(store.replaceCalls[2].records) != 0 {
		t.Fatalf("zero-record supported activity replacement records = %#v, want none", store.replaceCalls[2].records)
	}
	if !reflect.DeepEqual(store.orphanedKeepActivityIDs, []string{"activity-running-1", "activity-unsupported", "activity-running-zero"}) {
		t.Fatalf("orphan keep IDs = %#v", store.orphanedKeepActivityIDs)
	}
}

func TestRecalcAllLoadedActivitiesAbortsBeforeOrphanCleanupOnStreamError(t *testing.T) {
	activities := []ActivityInfo{testBestEffortActivity("activity-running-1", "running", 10_000, 3_600)}
	store := &fakeBestEffortStore{streamErrForActivity: activities[0].ID}

	err := recalcAllLoadedActivities(context.Background(), store, "user-1", activities, 1, nil)
	if err == nil {
		t.Fatal("recalcAllLoadedActivities returned nil, want stream error")
	}
	if len(store.replaceCalls) != 0 {
		t.Fatalf("replace calls = %#v, want none", store.replaceCalls)
	}
	if store.orphanedKeepActivityIDs != nil {
		t.Fatalf("orphan cleanup keep IDs = %#v, want nil", store.orphanedKeepActivityIDs)
	}
}

func TestRecalcAllLoadedActivitiesAbortsBeforeOrphanCleanupOnReplacementError(t *testing.T) {
	activities := []ActivityInfo{
		testBestEffortActivity("activity-running-1", "running", 10_000, 3_600),
		testBestEffortActivity("activity-running-2", "running", 20_000, 7_200),
	}
	store := &fakeBestEffortStore{replaceErrAtCall: 2}

	err := recalcAllLoadedActivities(context.Background(), store, "user-1", activities, 1, nil)
	if err == nil {
		t.Fatal("recalcAllLoadedActivities returned nil, want replacement error")
	}
	if got, want := len(store.replaceCalls), 2; got != want {
		t.Fatalf("replace call count = %d, want %d", got, want)
	}
	if store.orphanedKeepActivityIDs != nil {
		t.Fatalf("orphan cleanup keep IDs = %#v, want nil", store.orphanedKeepActivityIDs)
	}
}

func testBestEffortActivity(id, sportType string, distanceM float64, elapsedTimeS int) ActivityInfo {
	elevationGainM := 100.0
	biggestClimbM := 50.0
	if distanceM == 0 && elapsedTimeS == 0 {
		elevationGainM = 0
		biggestClimbM = 0
	}
	return ActivityInfo{
		ID:             id,
		UserID:         "user-1",
		SportType:      sportType,
		StartedAt:      time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		DistanceM:      distanceM,
		ElevationGainM: elevationGainM,
		ElapsedTimeS:   elapsedTimeS,
		BiggestClimbM:  biggestClimbM,
	}
}
