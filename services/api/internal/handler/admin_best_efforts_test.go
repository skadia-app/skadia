package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/bestefforts"
)

type fakeAdminBestEffortsDB struct {
	users         []string
	forEachErr    error
	progress      []int64
	total         []int64
	metadata      [][]byte
	completed     bool
	completedFail bool
	completedErr  string
}

func (db *fakeAdminBestEffortsDB) ForEachUser(_ context.Context, fn func(userID string) error) error {
	for _, userID := range db.users {
		if err := fn(userID); err != nil {
			return err
		}
	}
	return db.forEachErr
}

func (db *fakeAdminBestEffortsDB) UpdateBackgroundJobProgress(_ context.Context, _ string, current int64) error {
	db.progress = append(db.progress, current)
	return nil
}

func (db *fakeAdminBestEffortsDB) UpdateBackgroundJobTotal(_ context.Context, _ string, total int64) error {
	db.total = append(db.total, total)
	return nil
}

func (db *fakeAdminBestEffortsDB) UpdateBackgroundJobMetadata(_ context.Context, _ string, metadata []byte) error {
	db.metadata = append(db.metadata, append([]byte(nil), metadata...))
	return nil
}

func (db *fakeAdminBestEffortsDB) CompleteBackgroundJob(_ context.Context, _ string, failed bool, errorText string) error {
	db.completed = true
	db.completedFail = failed
	db.completedErr = errorText
	return nil
}

type fakeAdminBestEffortsService struct {
	errByUser          map[string]error
	calls              []string
	checkpointActivity []string
}

func (svc *fakeAdminBestEffortsService) RecalcAll(_ context.Context, userID string, progressFn bestefforts.RecalcProgressFunc) error {
	return svc.RecalcAllFromCheckpoint(context.Background(), userID, "", progressFn)
}

func (svc *fakeAdminBestEffortsService) RecalcAllFromCheckpoint(_ context.Context, userID, afterActivityID string, progressFn bestefforts.RecalcProgressFunc) error {
	svc.calls = append(svc.calls, userID)
	svc.checkpointActivity = append(svc.checkpointActivity, afterActivityID)
	if progressFn != nil {
		progressFn(bestefforts.RecalcProgress{
			Done:                    1,
			Total:                   2,
			LastProcessedActivityID: userID + "-activity",
		})
	}
	return svc.errByUser[userID]
}

func TestRunAllUsersBestEffortsRecalcMarksJobFailedWhenAUserRecalcFails(t *testing.T) {
	db := &fakeAdminBestEffortsDB{users: []string{"user-ok", "user-fail", "user-ok-2"}}
	svc := &fakeAdminBestEffortsService{errByUser: map[string]error{
		"user-fail": errors.New("boom"),
	}}

	runAllUsersBestEffortsRecalc(context.Background(), db, svc, "job-1")

	if !db.completed {
		t.Fatal("background job was not completed")
	}
	if !db.completedFail {
		t.Fatal("background job completed as successful, want failed")
	}
	if !strings.Contains(db.completedErr, "1 user recalc failed") || strings.Contains(db.completedErr, "user-fail") || !strings.Contains(db.completedErr, "boom") {
		t.Fatalf("completed error = %q", db.completedErr)
	}
	if got := strings.Join(svc.calls, ","); got != "user-ok,user-fail,user-ok-2" {
		t.Fatalf("RecalcAll calls = %s", got)
	}
	if len(db.total) != 1 || db.total[0] != 3 {
		t.Fatalf("total updates = %#v, want [3]", db.total)
	}
	if len(db.progress) == 0 || db.progress[len(db.progress)-1] != 3 {
		t.Fatalf("progress updates = %#v, want final attempted-user count 3", db.progress)
	}
	if len(db.metadata) == 0 {
		t.Fatal("metadata updates = none, want user/activity checkpoint")
	}
	var checkpoint map[string]any
	if err := json.Unmarshal(db.metadata[len(db.metadata)-1], &checkpoint); err != nil {
		t.Fatalf("unmarshal checkpoint: %v", err)
	}
	if checkpoint["last_user_id"] != "user-ok-2" || checkpoint["last_activity_id"] != "user-ok-2-activity" {
		t.Fatalf("checkpoint = %#v, want last user/activity", checkpoint)
	}
}

func TestRunAllUsersBestEffortsRecalcReportsUserAndIteratorErrors(t *testing.T) {
	db := &fakeAdminBestEffortsDB{
		users:      []string{"user-fail"},
		forEachErr: errors.New("scan failed"),
	}
	svc := &fakeAdminBestEffortsService{errByUser: map[string]error{
		"user-fail": errors.New("boom"),
	}}

	runAllUsersBestEffortsRecalc(context.Background(), db, svc, "job-1")

	if !db.completedFail {
		t.Fatal("background job completed as successful, want failed")
	}
	for _, want := range []string{"1 user recalc failed", "boom", "scan failed"} {
		if !strings.Contains(db.completedErr, want) {
			t.Fatalf("completed error = %q, want substring %q", db.completedErr, want)
		}
	}
	if strings.Contains(db.completedErr, "user-fail") {
		t.Fatalf("completed error leaks user id: %q", db.completedErr)
	}
}

func TestRunAllUsersBestEffortsRecalcFromCheckpointResumesCurrentUser(t *testing.T) {
	db := &fakeAdminBestEffortsDB{users: []string{"user-done", "user-current", "user-next"}}
	svc := &fakeAdminBestEffortsService{}

	runAllUsersBestEffortsRecalcFromCheckpoint(context.Background(), db, svc, "job-1", allUsersBestEffortsCheckpoint{
		LastUserID:     "user-current",
		LastActivityID: "activity-42",
		UserDone:       1,
		UserTotal:      3,
		DoneUsers:      1,
		TotalUsers:     3,
	})

	if got := strings.Join(svc.calls, ","); got != "user-current,user-next" {
		t.Fatalf("RecalcAll calls = %s, want user-current,user-next", got)
	}
	if len(svc.checkpointActivity) != 2 || svc.checkpointActivity[0] != "activity-42" || svc.checkpointActivity[1] != "" {
		t.Fatalf("checkpoint activities = %#v, want [activity-42 \"\"]", svc.checkpointActivity)
	}
}

func TestRunAllUsersBestEffortsRecalcFromCheckpointDoesNotSkipNewUserBeforeResumeMarker(t *testing.T) {
	db := &fakeAdminBestEffortsDB{users: []string{"user-new-before", "user-done", "user-current", "user-next"}}
	svc := &fakeAdminBestEffortsService{}

	runAllUsersBestEffortsRecalcFromCheckpoint(context.Background(), db, svc, "job-1", allUsersBestEffortsCheckpoint{
		LastUserID:     "user-current",
		LastActivityID: "activity-42",
		UserDone:       1,
		UserTotal:      3,
		DoneUsers:      1,
		TotalUsers:     3,
		DoneUserIDs:    []string{"user-done"},
	})

	if got := strings.Join(svc.calls, ","); got != "user-new-before,user-current,user-next" {
		t.Fatalf("RecalcAll calls = %s, want user-new-before,user-current,user-next", got)
	}
	if len(svc.checkpointActivity) != 3 || svc.checkpointActivity[0] != "" || svc.checkpointActivity[1] != "activity-42" || svc.checkpointActivity[2] != "" {
		t.Fatalf("checkpoint activities = %#v, want [\"\" activity-42 \"\"]", svc.checkpointActivity)
	}
}
