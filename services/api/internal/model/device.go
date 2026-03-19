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

// Device represents a hardware device (GPS watch, sensor, bike computer, etc.)
// seen across one or more activities for a user.
type Device struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	Type            string    `json:"type"`
	Manufacturer    *string   `json:"manufacturer,omitempty"`
	ManufacturerID  *int      `json:"manufacturer_id,omitempty"`
	ProductName     *string   `json:"product_name,omitempty"`
	ProductID       *int      `json:"product_id,omitempty"`
	SerialNumber    *int64    `json:"serial_number,omitempty"`
	AntDeviceNumber *int32    `json:"ant_device_number,omitempty"`
	CustomName      *string   `json:"custom_name,omitempty"`
	DeviceTypes     []string  `json:"device_types,omitempty"`
	SourceType      *string   `json:"source_type,omitempty"`
	FirstSeenAt     time.Time `json:"first_seen_at"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	ActivityCount   int       `json:"activity_count"`
	CreatedAt       time.Time `json:"created_at"`
}

// DisplayName returns a human-readable name for the device, preferring
// custom name > product name > manufacturer > fallback.
func (d *Device) DisplayName() string {
	if d.CustomName != nil && *d.CustomName != "" {
		return *d.CustomName
	}
	if d.ProductName != nil && *d.ProductName != "" {
		return *d.ProductName
	}
	if d.Manufacturer != nil {
		return *d.Manufacturer
	}
	return "Unbekanntes Gerät"
}

// DeviceUpsert is the input for upserting a device record (used during import).
type DeviceUpsert struct {
	UserID          string
	Type            string
	Manufacturer    string
	ManufacturerID  int
	ProductName     string
	ProductID       int
	SerialNumber    *int64
	AntDeviceNumber *int32
	DeviceTypes     []string
	SourceType      string
	SeenAt          time.Time
}

// DeviceUpdate is the PATCH request body for updating a device (e.g. custom name).
type DeviceUpdate struct {
	CustomName *string `json:"custom_name"`
}

// DeviceMerge is the request body for merging two device records into one.
type DeviceMerge struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
}
