package importer

import (
	"context"
	"testing"
	"time"
)

func TestSchedulePostImportHookDebouncesSameUser(t *testing.T) {
	t.Parallel()

	called := make(chan string, 2)
	svc := &Service{
		PostImportDebounce: 50 * time.Millisecond,
		PostImportHook: func(_ context.Context, userID string) {
			called <- userID
		},
	}

	svc.schedulePostImportHook(context.Background(), "user-1")
	time.Sleep(10 * time.Millisecond)
	svc.schedulePostImportHook(context.Background(), "user-1")

	select {
	case got := <-called:
		t.Fatalf("hook fired before debounce elapsed for %s", got)
	case <-time.After(30 * time.Millisecond):
	}

	select {
	case got := <-called:
		if got != "user-1" {
			t.Fatalf("hook user = %q, want user-1", got)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("hook did not fire after debounce elapsed")
	}

	select {
	case got := <-called:
		t.Fatalf("hook fired more than once, second user %s", got)
	case <-time.After(30 * time.Millisecond):
	}
}
