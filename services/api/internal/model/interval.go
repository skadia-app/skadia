package model

import "time"

type ActivityInterval struct {
	ID           string    `json:"id"`
	ActivityID   string    `json:"activity_id"`
	SegmentNum   int       `json:"segment_num"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	DurationS    int       `json:"duration_s"`
	AvgPowerW    *float64  `json:"avg_power_w,omitempty"`
	AvgHeartRate *int      `json:"avg_heart_rate,omitempty"`
	AvgSpeedMS   *float64  `json:"avg_speed_ms,omitempty"`
	AvgCadence   *float64  `json:"avg_cadence,omitempty"`
	Intensity    string    `json:"intensity"`
}
