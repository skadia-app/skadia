package event

// FakeInvalidator is an in-memory AnalyticsInvalidator for tests. It records
// every PublishAnalyticsInvalidate call and optionally returns NextErr instead
// of nil. Lives in the production package because handlers in other packages
// hold an event.AnalyticsInvalidator field — internal/-visibility precludes a
// dedicated event_test helper package.
type FakeInvalidator struct {
	Calls   []string // user_ids in call order
	NextErr error    // if non-nil, returned instead of nil
}

// PublishAnalyticsInvalidate records the call and returns f.NextErr.
func (f *FakeInvalidator) PublishAnalyticsInvalidate(userID string) error {
	f.Calls = append(f.Calls, userID)
	return f.NextErr
}
