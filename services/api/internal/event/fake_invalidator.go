package event

import "sync"

// FakeInvalidator is an in-memory AnalyticsInvalidator for tests. It records
// every PublishAnalyticsInvalidate call and optionally returns NextErr instead
// of nil. Lives in the production package because handlers in other packages
// hold an event.AnalyticsInvalidator field — internal/-visibility precludes a
// dedicated event_test helper package.
//
// mu guards Calls so FakeInvalidator is safe to use from concurrent goroutines
// (required for go test -race correctness).
type FakeInvalidator struct {
	mu      sync.Mutex
	Calls   []string // user_ids in call order; read via CallsCopy for race safety
	NextErr error    // if non-nil, returned instead of nil
}

// PublishAnalyticsInvalidate records the call and returns f.NextErr.
func (f *FakeInvalidator) PublishAnalyticsInvalidate(userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, userID)
	return f.NextErr
}

// CallsCopy returns a copy of the recorded call list, safe to read without
// holding the mutex.
func (f *FakeInvalidator) CallsCopy() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.Calls))
	copy(out, f.Calls)
	return out
}
