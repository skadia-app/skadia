package coverage

import (
	"context"
	"path/filepath"
	"testing"

	graphpkg "github.com/skadia-app/skadia/services/api/internal/routing/graph"
	"github.com/paulmach/orb"
)

func setupTestCoverageDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	core := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}

	forest := NewGrid(core, 50)
	for row := range forest.Rows {
		for col := range forest.Cols {
			forest.Data[row*forest.Cols+col] = 1
		}
	}
	WriteFCOV(filepath.Join(dir, "forest", "47_8.bin"), fcovHeaderFromGrid(forest), forest.Data)

	noise := NewGrid(core, 100)
	for row := range noise.Rows {
		for col := range noise.Cols {
			noise.Data[row*noise.Cols+col] = 50
		}
	}
	WriteFCOV(filepath.Join(dir, "noise", "47_8.bin"), fcovHeaderFromGrid(noise), noise.Data)

	pop := NewGrid(core, 100)
	for row := range pop.Rows {
		for col := range pop.Cols {
			pop.Data[row*pop.Cols+col] = 42
		}
	}
	WriteFCOV(filepath.Join(dir, "_traffic_populate", "47_8.bin"), fcovHeaderFromGrid(pop), pop.Data)

	WriteBuildTimestamp(dir)
	return dir
}

func TestDiskCoverageReaderSamplePointByLayer(t *testing.T) {
	dir := setupTestCoverageDir(t)
	reader, err := NewDiskCoverageReader(dir, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	val, err := reader.SamplePointByLayer(context.Background(), "forest", 47.5, 8.5)
	if err != nil {
		t.Fatal(err)
	}
	if val != 1 {
		t.Errorf("expected forest=1, got %d", val)
	}

	nVal, err := reader.SamplePointByLayer(context.Background(), "noise", 47.5, 8.5)
	if err != nil {
		t.Fatal(err)
	}
	if nVal != 50 {
		t.Errorf("expected noise=50, got %d", nVal)
	}

	pVal, err := reader.SamplePointByLayer(context.Background(), "populate", 47.5, 8.5)
	if err != nil {
		t.Fatal(err)
	}
	if pVal != 42 {
		t.Errorf("expected populate=42, got %d", pVal)
	}
}

func TestDiskCoverageReaderSampleWay(t *testing.T) {
	dir := setupTestCoverageDir(t)
	reader, err := NewDiskCoverageReader(dir, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	tid := graphpkg.TileIDFromCoord(47.5, 8.5)
	if err := reader.PreloadTile(context.Background(), tid); err != nil {
		t.Fatal(err)
	}

	line := orb.LineString{{8.3, 47.3}, {8.5, 47.5}, {8.7, 47.7}}
	result := reader.SampleWay(line)

	if result.ForestPct < 90 {
		t.Errorf("expected ~100%% forest, got %d%%", result.ForestPct)
	}
	if result.TrafficPopulate != 42 {
		t.Errorf("expected TrafficPopulate=42, got %f", result.TrafficPopulate)
	}
}

func TestDiskCoverageReaderHasTile(t *testing.T) {
	dir := setupTestCoverageDir(t)
	reader, err := NewDiskCoverageReader(dir, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	tid := graphpkg.TileIDFromCoord(47.5, 8.5)
	if !reader.HasTile("forest", tid) {
		t.Error("expected HasTile=true for forest")
	}
	if reader.HasTile("forest", graphpkg.TileIDFromCoord(99.5, 99.5)) {
		t.Error("expected HasTile=false for non-existent tile")
	}
}

func TestMappedGridExportedAccessors(t *testing.T) {
	dir := setupTestCoverageDir(t)
	reader, err := NewDiskCoverageReader(dir, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	tid := graphpkg.TileIDFromCoord(47.5, 8.5)
	mg, err := reader.LoadLayer("forest", tid)
	if err != nil {
		t.Fatal(err)
	}

	if mg.Bound().Min[1] != 47.0 || mg.Bound().Max[1] != 48.0 {
		t.Errorf("unexpected bound: %v", mg.Bound())
	}
	if mg.Rows() <= 0 || mg.Cols() <= 0 {
		t.Errorf("unexpected dimensions: rows=%d cols=%d", mg.Rows(), mg.Cols())
	}
	if mg.ResM() != 50 {
		t.Errorf("expected ResM=50, got %f", mg.ResM())
	}
	if len(mg.Data()) != mg.Rows()*mg.Cols() {
		t.Error("data length mismatch")
	}
}
