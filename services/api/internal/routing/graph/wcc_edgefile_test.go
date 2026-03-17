// wcc_edgefile_test.go
package graph

import (
	"io"
	"os"
	"testing"
)

// writeTestEdgeFile creates a temp edge file with the given ways.
func writeTestEdgeFile(t *testing.T, ways []testWay) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "wcc-test-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	path := tmp.Name()
	tmp.Close()

	w, err := NewEdgeFileWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	var maxNode uint32
	for _, way := range ways {
		if err := w.WriteWay(way.osmID, way.tags1, way.tags2, false, way.segs); err != nil {
			t.Fatal(err)
		}
		for _, s := range way.segs {
			if s.FromID > maxNode {
				maxNode = s.FromID
			}
			if s.ToID > maxNode {
				maxNode = s.ToID
			}
		}
	}
	if err := w.CloseWithNodeCount(maxNode + 1); err != nil {
		t.Fatal(err)
	}
	return path
}

type testWay struct {
	osmID        int64
	tags1, tags2 uint64
	segs         []WaySegment
}

func countSegments(t *testing.T, path string) int {
	t.Helper()
	r, err := NewEdgeFileReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	count := 0
	for {
		_, _, _, _, segs, err := r.ReadWay()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count += len(segs)
	}
	return count
}

func TestFilterWCCEdgeFile(t *testing.T) {
	// Hauptkomponente: Way 1 mit 3 Segments (0→1→2→0)
	// Kleine Komponente: Way 2 mit 1 Segment (3→4)
	path := writeTestEdgeFile(t, []testWay{
		{osmID: 100, tags1: 1, segs: []WaySegment{
			{FromID: 0, ToID: 1, Dist: 100},
			{FromID: 1, ToID: 2, Dist: 100},
			{FromID: 2, ToID: 0, Dist: 100},
		}},
		{osmID: 200, tags1: 1, segs: []WaySegment{
			{FromID: 3, ToID: 4, Dist: 100},
		}},
	})
	defer os.Remove(path)

	outPath := path + ".filtered"
	defer os.Remove(outPath)

	stats, err := FilterWCCEdgeFile(path, outPath, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.WaysKept != 1 {
		t.Errorf("ways kept = %d, want 1", stats.WaysKept)
	}
	if stats.WaysRemoved != 1 {
		t.Errorf("ways removed = %d, want 1", stats.WaysRemoved)
	}

	// Verify: nur Way 1 (3 Segments) bleibt
	segCount := countSegments(t, outPath)
	if segCount != 3 {
		t.Errorf("filtered segment count = %d, want 3", segCount)
	}
}

func TestFilterWCCEdgeFile_BorderNodesExempt(t *testing.T) {
	path := writeTestEdgeFile(t, []testWay{
		{osmID: 100, tags1: 1, segs: []WaySegment{
			{FromID: 0, ToID: 1, Dist: 100},
			{FromID: 1, ToID: 2, Dist: 100},
		}},
		{osmID: 200, tags1: 1, segs: []WaySegment{
			{FromID: 3, ToID: 4, Dist: 100},
		}},
	})
	defer os.Remove(path)

	outPath := path + ".filtered"
	defer os.Remove(outPath)

	// Node 3 ist Border-Node → Komponente wird nicht gepruned
	borderNodes := map[uint32]bool{3: true}
	stats, err := FilterWCCEdgeFile(path, outPath, 3, borderNodes)
	if err != nil {
		t.Fatal(err)
	}
	if stats.WaysRemoved != 0 {
		t.Errorf("ways removed = %d, want 0 (border node exempts component)", stats.WaysRemoved)
	}
}

func TestFilterWCCEdgeFile_Empty(t *testing.T) {
	path := writeTestEdgeFile(t, nil)
	defer os.Remove(path)

	outPath := path + ".filtered"
	defer os.Remove(outPath)

	stats, err := FilterWCCEdgeFile(path, outPath, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.WaysKept != 0 || stats.WaysRemoved != 0 {
		t.Errorf("empty file: kept=%d removed=%d", stats.WaysKept, stats.WaysRemoved)
	}
}
