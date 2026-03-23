// wcc_edgefile.go — WCC filter operating on the way-based edge temp file.
package graph

import (
	"fmt"
	"io"
	"log/slog"
)

// WCCFilterStats holds the results of edge file WCC filtering.
type WCCFilterStats struct {
	WaysKept    int
	WaysRemoved int
	NodesTotal  int
	NodesPruned int
}

// FilterWCCEdgeFile reads ways from inputPath, runs Union-Find on all segments
// to identify weakly connected components, and writes only ways whose segments
// belong to components >= minSize to outputPath.
//
// Border nodes (if non-nil) exempt their component from pruning regardless of size.
//
// The output file preserves the original nodeCount in the header (node IDs are
// unchanged — only ways are filtered, BuildCSRFromFile handles sparse IDs).
//
// Memory: ~24 bytes/node (parent+rank+size) + ~8 bytes/segment (from/to pairs).
// For a tile with ~3M nodes and ~8M segments: ~136 MB.
func FilterWCCEdgeFile(inputPath, outputPath string, minSize int,
	borderNodes map[uint32]bool) (WCCFilterStats, error) {

	var stats WCCFilterStats

	// --- Pass 1: Read all segments, build Union-Find ---
	r, err := NewEdgeFileReader(inputPath)
	if err != nil {
		return stats, fmt.Errorf("wcc pass1 open: %w", err)
	}
	nodeCount := r.NodeCount()

	type edgePair struct{ from, to uint32 }
	var pairs []edgePair

	for {
		_, _, _, _, segs, err := r.ReadWay()
		if err == io.EOF {
			break
		}
		if err != nil {
			r.Close()
			return stats, fmt.Errorf("wcc pass1 read: %w", err)
		}
		for _, s := range segs {
			pairs = append(pairs, edgePair{s.FromID, s.ToID})
		}
	}
	r.Close()

	if len(pairs) == 0 {
		// Empty file — write empty output.
		w, err := NewEdgeFileWriter(outputPath)
		if err != nil {
			return stats, err
		}
		return stats, w.CloseWithNodeCount(nodeCount)
	}

	// Find actual max node ID from pairs (nodeCount from header may be larger).
	var maxNode uint32
	for _, p := range pairs {
		if p.from > maxNode {
			maxNode = p.from
		}
		if p.to > maxNode {
			maxNode = p.to
		}
	}

	n := int(maxNode + 1)
	stats.NodesTotal = n
	uf := newUnionFind(n)

	for _, p := range pairs {
		uf.union(int(p.from), int(p.to))
	}
	pairs = nil // free

	// Determine which component roots to keep.
	keepRoot := make(map[int]bool)
	maxSize := 0
	for i := range n {
		root := uf.find(i)
		if uf.size[root] > maxSize {
			maxSize = uf.size[root]
		}
	}
	for i := range n {
		root := uf.find(i)
		if uf.size[root] >= minSize || uf.size[root] == maxSize {
			keepRoot[root] = true
		}
	}
	if borderNodes != nil {
		for nodeID := range borderNodes {
			if int(nodeID) < n {
				keepRoot[uf.find(int(nodeID))] = true
			}
		}
	}

	// Build keep-set for nodes.
	keepNode := make([]bool, n)
	for i := range n {
		if keepRoot[uf.find(i)] {
			keepNode[i] = true
		} else {
			stats.NodesPruned++
		}
	}

	// --- Pass 2: Filter ways ---
	r2, err := NewEdgeFileReader(inputPath)
	if err != nil {
		return stats, fmt.Errorf("wcc pass2 open: %w", err)
	}
	defer r2.Close()

	w, err := NewEdgeFileWriter(outputPath)
	if err != nil {
		return stats, fmt.Errorf("wcc write open: %w", err)
	}

	for {
		osmID, tags1, tags2, isOneway, segs, err := r2.ReadWay()
		if err == io.EOF {
			break
		}
		if err != nil {
			return stats, fmt.Errorf("wcc pass2 read: %w", err)
		}

		// Keep way only if ALL its segments' nodes are in kept components.
		allKept := true
		for _, s := range segs {
			if !keepNode[s.FromID] || !keepNode[s.ToID] {
				allKept = false
				break
			}
		}

		if allKept {
			if err := w.WriteWay(osmID, tags1, tags2, isOneway, segs); err != nil {
				return stats, fmt.Errorf("wcc write way: %w", err)
			}
			stats.WaysKept++
		} else {
			stats.WaysRemoved++
		}
	}

	if err := w.CloseWithNodeCount(nodeCount); err != nil {
		return stats, fmt.Errorf("wcc close: %w", err)
	}

	slog.Info("WCC edge filter",
		"ways_kept", stats.WaysKept,
		"ways_removed", stats.WaysRemoved,
		"nodes_pruned", stats.NodesPruned,
	)
	return stats, nil
}
