package model

import "time"

type CustomMap struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Color       string    `json:"color"`
	IsVisible   bool      `json:"is_visible"`
	SortOrder   int       `json:"sort_order"`
	MarkerCount int       `json:"marker_count,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CustomMapMarker struct {
	ID          string    `json:"id"`
	MapID       string    `json:"map_id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Lat         float64   `json:"lat"`
	Lng         float64   `json:"lng"`
	Color       *string   `json:"color"`
	ImageURL    *string   `json:"image_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateCustomMapRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Color       string  `json:"color"`
}

type UpdateCustomMapRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Color       *string `json:"color"`
	IsVisible   *bool   `json:"is_visible"`
}

type CreateMarkerRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	Color       *string `json:"color"`
}

type UpdateMarkerRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
	Color       *string  `json:"color"`
	ImageURL    *string  `json:"image_url"`
}
