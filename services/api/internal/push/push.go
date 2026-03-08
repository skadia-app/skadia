// Package push provides the interface and placeholder implementation for
// mobile push notification delivery.
package push

import (
	"context"
	"log/slog"
)

// Notification categories used throughout the application.
const (
	CategoryEnrichment = "enrichment"
	CategoryAnomaly    = "anomaly"
	CategoryGear       = "gear"
	CategoryChat       = "chat"
)

// Notifier is the interface for push notification delivery.
// Current implementation: NoopNotifier (logs only).
// Phase 9 will add FirebaseNotifier (FCM) and APNsNotifier.
type Notifier interface {
	// Send delivers a push notification to all devices registered by userID.
	Send(ctx context.Context, userID, category, title, body string) error
}

// NoopNotifier is a placeholder that only logs — actual push delivery in Phase 9.
type NoopNotifier struct{}

// NewNoopNotifier creates a new NoopNotifier.
func NewNoopNotifier() *NoopNotifier {
	return &NoopNotifier{}
}

// Send logs the notification intent but does not deliver it.
// TODO Phase 9: replace with Firebase (FCM) and APNs delivery.
func (n *NoopNotifier) Send(ctx context.Context, userID, category, title, body string) error {
	slog.Debug("push notification (noop)",
		"user_id", userID,
		"category", category,
		"title", title,
	)
	return nil
}
