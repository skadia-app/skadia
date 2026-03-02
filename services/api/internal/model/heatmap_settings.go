package model

// HeatmapSettings holds all user-configurable heatmap parameters.
type HeatmapSettings struct {
	SamplingIntervalM      int  `json:"sampling_interval_m"`
	TeleportThresholdM     int  `json:"teleport_threshold_m"`
	TeleportFilterMap      bool `json:"teleport_filter_map"`
	TeleportFilterActivity bool `json:"teleport_filter_activity"`
}
