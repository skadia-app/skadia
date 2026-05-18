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

func TestOSMStatusMissingWithoutRegions(t *testing.T) {
	got := OSMStatusFromSnapshot(OSMSnapshot{RegionCount: 0})
	if got.State != StatusMissingArtifacts || got.ReasonCode != "missing_artifacts" {
		t.Fatalf("status = %+v", got)
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
	got := PMTilesStatusFromSnapshot(PMTilesSnapshot{FileCount: 0})
	if got.State != StatusMissingArtifacts || got.ReasonCode != "pmtiles_missing" {
		t.Fatalf("status = %+v", got)
	}
}
