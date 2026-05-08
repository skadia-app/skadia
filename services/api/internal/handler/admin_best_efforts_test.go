package handler

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeAdminBestEffortsDB struct {
	users         []string
	forEachErr    error
	progress      []int64
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

func (db *fakeAdminBestEffortsDB) CompleteBackgroundJob(_ context.Context, _ string, failed bool, errorText string) error {
	db.completed = true
	db.completedFail = failed
	db.completedErr = errorText
	return nil
}

type fakeAdminBestEffortsService struct {
	errByUser map[string]error
	calls     []string
}

func (svc *fakeAdminBestEffortsService) RecalcAll(_ context.Context, userID string, _ func(done, total int)) error {
	svc.calls = append(svc.calls, userID)
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
	if !strings.Contains(db.completedErr, "1 user recalc failed") || !strings.Contains(db.completedErr, "user-fail: boom") {
		t.Fatalf("completed error = %q", db.completedErr)
	}
	if got := strings.Join(svc.calls, ","); got != "user-ok,user-fail,user-ok-2" {
		t.Fatalf("RecalcAll calls = %s", got)
	}
	if len(db.progress) != 2 || db.progress[0] != 1 || db.progress[1] != 2 {
		t.Fatalf("progress updates = %#v, want [1 2]", db.progress)
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
	for _, want := range []string{"1 user recalc failed", "user-fail: boom", "scan failed"} {
		if !strings.Contains(db.completedErr, want) {
			t.Fatalf("completed error = %q, want substring %q", db.completedErr, want)
		}
	}
}
