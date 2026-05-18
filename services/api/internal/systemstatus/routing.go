package systemstatus

import (
	"strconv"
	"time"
)

func DisabledComponent(key, label, reason string) ComponentStatus {
	return ComponentStatus{
		Key:        key,
		Label:      label,
		State:      StatusDisabled,
		ReasonCode: reason,
		CheckedAt:  time.Now().UTC(),
	}
}

type BRouterSnapshot struct {
	Reachable     bool
	Version       string
	ProfileCount  int
	SegmentCount  int
	SegmentAgeSec int64
	CheckedAt     time.Time
}

type BadRouterSnapshot struct {
	TileFilesPresent    bool
	VerifiedSampleRoute bool
	ProfileCount        int
	TileCount           int
	LastBuildStatus     string
	CheckedAt           time.Time
}

type OSMSnapshot struct {
	RegionCount      int
	LastImportStatus string
	SourceDate       string
	CheckedAt        time.Time
}

type DEMSnapshot struct {
	TileCount       int
	LastBuildStatus string
	CheckedAt       time.Time
}

type CoverageSnapshot struct {
	LayerCount       int
	CorruptCellCount int
	LastBuildStatus  string
	CheckedAt        time.Time
}

type PMTilesSnapshot struct {
	FileCount       int
	DirPresent      bool
	LastBuildStatus string
	CheckedAt       time.Time
}

func BRouterStatusFromSnapshot(s BRouterSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "brouter",
		Label:     "BRouter",
		State:     StatusOK,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details: map[string]string{
			"version":       s.Version,
			"profile_count": strconv.Itoa(s.ProfileCount),
			"segment_count": strconv.Itoa(s.SegmentCount),
		},
	}
	if !s.Reachable {
		st.State = StatusUnavailable
		st.ReasonCode = "brouter_unreachable"
		return st
	}
	if s.SegmentCount == 0 {
		st.State = StatusDegraded
		st.ReasonCode = "brouter_segments_missing"
	}
	return st
}

func BadRouterStatusFromSnapshot(s BadRouterSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "badrouter",
		Label:     "Skadia routing graph",
		State:     StatusOK,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details: map[string]string{
			"profile_count": strconv.Itoa(s.ProfileCount),
			"tile_count":    strconv.Itoa(s.TileCount),
		},
	}
	if s.LastBuildStatus != "" {
		st.Details["state"] = s.LastBuildStatus
	}
	if !s.TileFilesPresent {
		st.State = StatusMissingArtifacts
		st.ReasonCode = "routing_tiles_missing"
		return st
	}
	if !s.VerifiedSampleRoute {
		st.State = StatusUnknown
		st.ReasonCode = "file_presence_only"
	}
	return st
}

func OSMStatusFromSnapshot(s OSMSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "osm",
		Label:     "OSM geodata",
		State:     StatusOK,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details: map[string]string{
			"region_count": strconv.Itoa(s.RegionCount),
		},
	}
	if s.LastImportStatus != "" {
		st.Details["state"] = s.LastImportStatus
	}
	if s.SourceDate != "" {
		st.Details["source_date"] = s.SourceDate
	}
	if s.RegionCount == 0 {
		st.State = StatusMissingArtifacts
		st.ReasonCode = "missing_artifacts"
	}
	return st
}

func DEMStatusFromSnapshot(s DEMSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "dem",
		Label:     "DEM",
		State:     StatusOK,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details: map[string]string{
			"tile_count": strconv.Itoa(s.TileCount),
		},
	}
	if s.LastBuildStatus != "" {
		st.Details["state"] = s.LastBuildStatus
	}
	if s.TileCount == 0 {
		st.State = StatusMissingArtifacts
		st.ReasonCode = "dem_tiles_missing"
	}
	return st
}

func CoverageStatusFromSnapshot(s CoverageSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "coverage",
		Label:     "Coverage",
		State:     StatusOK,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details: map[string]string{
			"layer_count":        strconv.Itoa(s.LayerCount),
			"corrupt_cell_count": strconv.Itoa(s.CorruptCellCount),
		},
	}
	if s.LastBuildStatus != "" {
		st.Details["state"] = s.LastBuildStatus
	}
	if s.CorruptCellCount > 0 {
		st.State = StatusDegraded
		st.ReasonCode = "coverage_corrupt_cells"
		return st
	}
	if s.LayerCount == 0 {
		st.State = StatusMissingArtifacts
		st.ReasonCode = "coverage_missing"
	}
	return st
}

func PMTilesStatusFromSnapshot(s PMTilesSnapshot) ComponentStatus {
	st := ComponentStatus{
		Key:       "tiles",
		Label:     "Offline map tiles",
		State:     StatusOK,
		CheckedAt: checkedAtOrNow(s.CheckedAt),
		Details: map[string]string{
			"pmtiles_file_count": strconv.Itoa(s.FileCount),
			"directory_present":  strconv.FormatBool(s.DirPresent),
		},
	}
	if s.LastBuildStatus != "" {
		st.Details["state"] = s.LastBuildStatus
	}
	if !s.DirPresent {
		st.State = StatusMissingArtifacts
		st.ReasonCode = "pmtiles_dir_missing"
		return st
	}
	if s.FileCount == 0 {
		st.State = StatusMissingArtifacts
		st.ReasonCode = "pmtiles_missing"
	}
	return st
}
