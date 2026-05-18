package systemstatus

import "testing"

func TestBRouterStatusSeparatesProcessAndSegments(t *testing.T) {
	got := BRouterStatusFromSnapshot(BRouterSnapshot{Reachable: true, Version: "v1.7.8", SegmentCount: 0})
	if got.State != StatusDegraded || got.ReasonCode != "brouter_segments_missing" {
		t.Fatalf("status = %+v", got)
	}
}

func TestBadRouterStatusMarksFilePresenceOnlyAsUnknown(t *testing.T) {
	got := BadRouterStatusFromSnapshot(BadRouterSnapshot{TileFilesPresent: true, VerifiedSampleRoute: false})
	if got.State != StatusUnknown || got.ReasonCode != "file_presence_only" {
		t.Fatalf("status = %+v", got)
	}
}

func TestFullOnlyGeodataStatusCanBeDisabled(t *testing.T) {
	cases := []ComponentStatus{
		DisabledComponent("badrouter", "Skadia routing graph", "core_profile"),
		DisabledComponent("osm", "OSM geodata", "core_profile"),
		DisabledComponent("dem", "DEM", "core_profile"),
		DisabledComponent("coverage", "Coverage", "core_profile"),
		DisabledComponent("tiles", "Map tiles", "core_profile"),
	}
	for _, got := range cases {
		if got.State != StatusDisabled || got.ReasonCode != "core_profile" {
			t.Fatalf("status = %+v, want disabled core_profile", got)
		}
	}
}

func TestOSMStatusMissingWithoutRegions(t *testing.T) {
	got := OSMStatusFromSnapshot(OSMSnapshot{RegionCount: 0})
	if got.State != StatusMissingArtifacts || got.ReasonCode != "missing_artifacts" {
		t.Fatalf("status = %+v", got)
	}
}

func TestOSMStatusDegradesWhenAnyRegionFailed(t *testing.T) {
	got := OSMStatusFromSnapshot(OSMSnapshot{RegionCount: 10, ImportedCount: 9, FailedCount: 1})
	if got.State != StatusDegraded || got.ReasonCode != "osm_regions_failed" {
		t.Fatalf("status = %+v", got)
	}
	if got.Details["failed_region_count"] != "1" {
		t.Fatalf("failed_region_count = %q, want 1", got.Details["failed_region_count"])
	}
}

func TestDEMStatusMissingWithoutTiles(t *testing.T) {
	got := DEMStatusFromSnapshot(DEMSnapshot{TileCount: 0})
	if got.State != StatusMissingArtifacts || got.ReasonCode != "dem_tiles_missing" {
		t.Fatalf("status = %+v", got)
	}
}

func TestCoverageStatusReportsCorruptCells(t *testing.T) {
	got := CoverageStatusFromSnapshot(CoverageSnapshot{LayerCount: 4, CorruptCellCount: 1})
	if got.State != StatusDegraded || got.ReasonCode != "coverage_corrupt_cells" {
		t.Fatalf("status = %+v", got)
	}
}

func TestPMTilesStatusMissingWithoutFiles(t *testing.T) {
	got := PMTilesStatusFromSnapshot(PMTilesSnapshot{DirPresent: true, FileCount: 0})
	if got.State != StatusMissingArtifacts || got.ReasonCode != "pmtiles_missing" {
		t.Fatalf("status = %+v", got)
	}
}

func TestPMTilesStatusDistinguishesMissingDirectory(t *testing.T) {
	got := PMTilesStatusFromSnapshot(PMTilesSnapshot{DirPresent: false})
	if got.State != StatusMissingArtifacts || got.ReasonCode != "pmtiles_dir_missing" {
		t.Fatalf("status = %+v", got)
	}
}
