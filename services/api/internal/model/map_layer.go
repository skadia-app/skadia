package model

import "time"

// MapLayer represents a row in the map_layers table.
type MapLayer struct {
	ID                  int64     `json:"id"`
	Key                 string    `json:"key"`
	Role                string    `json:"role"` // "base" | "overlay"
	Name                string    `json:"name"`
	SourceKind          string    `json:"source_kind"` // "raster_tiles" | "vector_style" | "protomaps_offline"
	URLTemplate         *string   `json:"url_template,omitempty"`
	StyleURL            *string   `json:"style_url,omitempty"`
	Attribution         *string   `json:"attribution,omitempty"`
	MinZoom             *int16    `json:"min_zoom,omitempty"`
	MaxZoom             *int16    `json:"max_zoom,omitempty"`
	DefaultOpacity      int16     `json:"default_opacity"`
	IsSystem            bool      `json:"is_system"`
	Enabled             bool      `json:"enabled"`
	SortOrder           int32     `json:"sort_order"`
	ProxyThroughBackend bool      `json:"proxy_through_backend"`
	RequiresCapability  *string   `json:"requires_capability,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// SystemLayerVisibility represents a row in system_layer_visibility.
type SystemLayerVisibility struct {
	LayerKey                  string    `json:"layer_key"`
	Enabled                   bool      `json:"enabled"`
	DefaultEnabledForNewUsers bool      `json:"default_enabled_for_new_users"`
	DefaultOpacity            *int16    `json:"default_opacity,omitempty"`
	LabelOverride             *string   `json:"label_override,omitempty"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

// UserMapLayer represents a row in user_map_layers.
// UserID is a UUID string matching users.id and middleware.GetUserID return type.
//
// After migration 000338, url_template is nullable (used by raster_tiles source kind only)
// and the row also carries role, source_kind, and style_url — matching the system map_layers
// table so that user layers can be treated uniformly in the frontend.
type UserMapLayer struct {
	ID             int64     `json:"id"`
	UserID         string    `json:"-"` // UUID; never expose in JSON
	Role           string    `json:"role"`        // "base" | "overlay"
	Name           string    `json:"name"`
	SourceKind     string    `json:"source_kind"` // "raster_tiles" | "vector_style"
	URLTemplate    *string   `json:"url_template,omitempty"`
	StyleURL       *string   `json:"style_url,omitempty"`
	Attribution    *string   `json:"attribution,omitempty"`
	MinZoom        *int16    `json:"min_zoom,omitempty"`
	MaxZoom        *int16    `json:"max_zoom,omitempty"`
	DefaultOpacity int16     `json:"default_opacity"`
	Enabled        bool      `json:"enabled"`
	SortOrder      int32     `json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SystemLayerImmutableFields is the set of map_layers columns that may not be
// PATCH-ed when is_system=true. Enforced in the handler before DB write.
var SystemLayerImmutableFields = map[string]struct{}{
	"key":                 {},
	"source_kind":         {},
	"is_system":           {},
	"url_template":        {},
	"style_url":           {},
	"requires_capability": {},
}
