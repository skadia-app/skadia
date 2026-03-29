package model

// DiveSettings holds equipment and algorithm settings from a FIT DiveSettings
// message (MesgNum 258). Stored 1:1 per activity.
type DiveSettings struct {
	ActivityID              string  `json:"activity_id"`
	GfLow                   int     `json:"gf_low"`
	GfHigh                  int     `json:"gf_high"`
	WaterType               string  `json:"water_type"`
	WaterDensity            float64 `json:"water_density"`
	DecoModel               string  `json:"deco_model"`
	Po2Warn                 float64 `json:"po2_warn"`
	Po2Critical             float64 `json:"po2_critical"`
	Po2Deco                 float64 `json:"po2_deco"`
	SafetyStopEnabled       bool    `json:"safety_stop_enabled"`
	SafetyStopTimeS         int     `json:"safety_stop_time_s"`
	RepeatDiveIntervalS     int     `json:"repeat_dive_interval_s"`
	CcrLowSetpoint          float64 `json:"ccr_low_setpoint"`
	CcrLowSetpointDepthM    float64 `json:"ccr_low_setpoint_depth_m"`
	CcrHighSetpoint         float64 `json:"ccr_high_setpoint"`
	CcrHighSetpointDepthM   float64 `json:"ccr_high_setpoint_depth_m"`
}

// DiveGas holds breathing gas configuration from a FIT DiveGas message
// (MesgNum 259). Stored 1:N per activity (2–4 gases per dive).
type DiveGas struct {
	ActivityID string `json:"activity_id"`
	GasIndex   int    `json:"gas_index"`
	HeliumPct  int    `json:"helium_pct"`
	OxygenPct  int    `json:"oxygen_pct"`
	Status     string `json:"status"`
	Mode       string `json:"mode"`
}
