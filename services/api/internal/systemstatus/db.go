package systemstatus

import (
	"strconv"
	"time"
)

type DBSnapshot struct {
	PingOK             bool
	MigrationReadError bool
	MigrationVersion   int
	MigrationDirty     bool
	PostGISVersion     string
	ServerVersion      string
	Extensions         map[string]ExtensionSnapshot
	ExpectedExtensions []string
	OptionalExtensions []string
	Compression        CompressionSnapshot
	PoolAcquired       int32
	PoolConns          int32
	LockWaits          int
	CheckedAt          time.Time
}

type ExtensionSnapshot struct {
	Installed bool
	Version   string
}

type CompressionSnapshot struct {
	SettingEnabled       bool
	HypertableCompressed bool
	PolicyPresent        bool
	SegmentBy            string
	OrderBy              string
	CompressedChunks     int
	UncompressedChunks   int
	ToggleJobsRunning    int
	ToggleJobsFailed24h  int
	CheckedAt            time.Time
}

func DBStatusFromSnapshot(s DBSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "db",
		Label:     "Database",
		State:     StatusOK,
		Required:  true,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details:   map[string]string{},
	}
	if !s.PingOK {
		st.State = StatusUnavailable
		st.ReasonCode = "db_unreachable"
		return st
	}
	if s.MigrationReadError {
		st.State = StatusDegraded
		st.ReasonCode = "migration_status_unreadable"
	}
	if s.MigrationDirty {
		st.State = StatusDegraded
		st.ReasonCode = "migration_dirty"
	}
	if s.PostGISVersion == "" {
		st.State = StatusDegraded
		st.ReasonCode = "postgis_missing"
	}
	for _, name := range s.ExpectedExtensions {
		ext := s.Extensions[name]
		if !ext.Installed {
			st.State = StatusDegraded
			st.ReasonCode = "db_extension_missing"
			st.Details["missing_extension"] = name
			return st
		}
		if ext.Version != "" {
			st.Details["extension_"+name] = ext.Version
		}
	}
	for _, name := range s.OptionalExtensions {
		ext := s.Extensions[name]
		if !ext.Installed {
			st.Details["optional_extension_"+name] = "missing"
		} else if ext.Version != "" {
			st.Details["optional_extension_"+name] = ext.Version
		}
	}
	if s.Compression.SettingEnabled && !s.Compression.PolicyPresent {
		st.State = StatusDegraded
		st.ReasonCode = "timescale_compression_policy_missing"
	}
	if s.Compression.SettingEnabled && (!s.Compression.HypertableCompressed || s.Compression.SegmentBy != "activity_id" || s.Compression.OrderBy != "time DESC") {
		st.State = StatusDegraded
		st.ReasonCode = "timescale_compression_option_drift"
	}
	if !s.Compression.SettingEnabled && (s.Compression.HypertableCompressed || s.Compression.PolicyPresent) {
		st.State = StatusDegraded
		st.ReasonCode = "timescale_compression_setting_drift"
	}
	if s.Compression.ToggleJobsFailed24h > 0 {
		st.State = StatusDegraded
		st.ReasonCode = "timescale_compression_toggle_failed"
	}
	st.Details["migration_version"] = strconv.Itoa(s.MigrationVersion)
	st.Details["postgis"] = s.PostGISVersion
	st.Details["server_version"] = s.ServerVersion
	return st
}

func CompressionStatusFromSnapshot(s CompressionSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "timescale_compression",
		Label:     "TimescaleDB compression",
		State:     StatusOK,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details: map[string]string{
			"compressed_chunks_count":   strconv.Itoa(s.CompressedChunks),
			"uncompressed_chunks_count": strconv.Itoa(s.UncompressedChunks),
		},
	}
	if !s.SettingEnabled && !s.HypertableCompressed && !s.PolicyPresent {
		st.State = StatusDisabled
		st.ReasonCode = "timescale_compression_disabled"
		return st
	}
	if !s.SettingEnabled && (s.HypertableCompressed || s.PolicyPresent) {
		st.State = StatusDegraded
		st.ReasonCode = "timescale_compression_setting_drift"
		return st
	}
	if s.SettingEnabled && !s.PolicyPresent {
		st.State = StatusDegraded
		st.ReasonCode = "timescale_compression_policy_missing"
		return st
	}
	if s.SettingEnabled && (!s.HypertableCompressed || s.SegmentBy != "activity_id" || s.OrderBy != "time DESC") {
		st.State = StatusDegraded
		st.ReasonCode = "timescale_compression_option_drift"
		return st
	}
	if s.ToggleJobsFailed24h > 0 {
		st.State = StatusDegraded
		st.ReasonCode = "timescale_compression_toggle_failed"
	}
	return st
}
