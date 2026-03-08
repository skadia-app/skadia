package model

import "time"

// DeviceToken represents a registered mobile device for push notifications.
type DeviceToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Token      string     `json:"token"`
	Platform   string     `json:"platform"`    // "android" or "ios"
	DeviceName *string    `json:"device_name,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// Notification represents a push notification stored for a user.
type Notification struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Category  string     `json:"category"`
	Title     string     `json:"title"`
	Body      *string    `json:"body,omitempty"`
	Read      bool       `json:"read"`
	CreatedAt time.Time  `json:"created_at"`
}
