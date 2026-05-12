package tools

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		seconds any
		want    string
	}{
		{nil, "0min"},
		{0, "0min"},
		{59, "0min"},
		{600, "10min"},
		{3661, "1h01min"},
		{7380.0, "2h03min"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.seconds); got != tt.want {
			t.Fatalf("formatDuration(%v) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestFormatActivityCompactRunning(t *testing.T) {
	row := map[string]any{
		"id":               "activity-1",
		"started_at":       time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC),
		"title":            "Trailrunde",
		"sport_type":       "running",
		"sub_type":         "trail",
		"is_virtual":       true,
		"distance_m":       12420.0,
		"elapsed_time_s":   3720,
		"avg_speed_ms":     3.333333333,
		"avg_heart_rate":   144,
		"elevation_gain_m": 456.4,
		"start_city":       "Freiburg",
	}

	got := formatActivityCompact(row)
	if got["date"] != "10.05.2026" || got["sport"] != "running/trail (virtual)" {
		t.Fatalf("unexpected date/sport: %#v", got)
	}
	if got["km"] != 12.4 || got["duration"] != "1h02min" || got["pace"] != "5:00 min/km" {
		t.Fatalf("unexpected compact metrics: %#v", got)
	}
	if got["speed"] != nil {
		t.Fatalf("running activity must not include speed: %#v", got)
	}
	if got["elevation_m"] != 456.0 || got["city"] != "Freiburg" {
		t.Fatalf("unexpected metadata: %#v", got)
	}
}

func TestFormatActivityCompactSpeedSport(t *testing.T) {
	row := map[string]any{
		"id":             "activity-2",
		"title":          "Rad",
		"sport_type":     "cycling",
		"distance_m":     30000,
		"duration_s":     int64(3600),
		"avg_speed_ms":   8.3333333,
		"is_electric":    true,
		"is_indoor":      true,
		"started_at":     time.Time{},
		"avg_heart_rate": nil,
	}

	got := formatActivityCompact(row)
	if got["sport"] != "cycling (electric, indoor)" {
		t.Fatalf("unexpected sport label: %#v", got)
	}
	if got["km"] != 30.0 || got["duration"] != "1h00min" || got["speed"] != "30.0 km/h" {
		t.Fatalf("unexpected compact metrics: %#v", got)
	}
	if got["date"] != "" || got["pace"] != nil {
		t.Fatalf("unexpected date/pace: %#v", got)
	}
}
