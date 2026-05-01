package event

// Compile-time assertions: both *Publisher and *FakeInvalidator must satisfy
// the AnalyticsInvalidator interface so handlers can hold either one.
var _ AnalyticsInvalidator = (*Publisher)(nil)
var _ AnalyticsInvalidator = (*FakeInvalidator)(nil)
