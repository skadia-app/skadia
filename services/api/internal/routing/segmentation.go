package routing

// ValleySegment defines a sub-route between two valley floor nodes.
type ValleySegment struct {
	From, To uint64
	FromIdx  int
	ToIdx    int
}

// FindValleySegments analyzes the elevation profile of the Pass0 path and
// identifies natural segment boundaries at valley floors.
//
// A valley floor is a local elevation minimum where:
// - The minimum is <80% of the elevation of adjacent passes
// - Relatively flat terrain exists on each side (drain check)
//
// Returns nil if no plausible valley floors found (fallback to unsegmented).
func FindValleySegments(pass0Path []uint64, reader GraphReader,
	minFlatDistM float64) []ValleySegment {

	n := len(pass0Path)
	if n < 10 {
		return nil
	}

	// Extract elevation profile
	eles := make([]int32, n)
	for i, nodeID := range pass0Path {
		_, _, eles[i] = reader.NodeCoord(nodeID)
	}

	// Find local minima with sliding window
	type localMin struct {
		idx int
		ele int32
	}
	var minima []localMin

	windowSize := min(n/4, 20)
	if windowSize < 3 {
		return nil
	}

	for i := windowSize; i < n-windowSize; i++ {
		isMin := true
		for j := i - windowSize; j <= i+windowSize; j++ {
			if j != i && eles[j] < eles[i] {
				isMin = false
				break
			}
		}
		if !isMin {
			continue
		}

		// Plausibility: must be <80% of the higher neighbor peaks
		leftMax := eles[i]
		for j := range i {
			leftMax = max(leftMax, eles[j])
		}
		rightMax := eles[i]
		for j := i + 1; j < n; j++ {
			rightMax = max(rightMax, eles[j])
		}
		neighborPeak := min(leftMax, rightMax)
		if float64(eles[i]) > 0.8*float64(neighborPeak) {
			continue
		}

		// Drain check: verify that the immediate vicinity of the minimum is
		// relatively flat (no steep jump within 3 nodes on each side).
		// We check a small inner window (3 nodes) rather than the full search
		// window, because passes can legitimately lie within windowSize nodes.
		// minFlatDistM is reserved for a future distance-based implementation.
		flatLeft := true
		flatRight := true
		innerWindow := 3
		// Use absolute elevation difference for threshold to handle negative elevations
		absEle := float64(eles[i])
		if absEle < 0 {
			absEle = -absEle
		}
		threshold := eles[i] + int32(absEle*0.2)
		for j := max(0, i-innerWindow); j < i; j++ {
			if eles[j] > threshold {
				flatLeft = false
				break
			}
		}
		for j := i + 1; j <= min(n-1, i+innerWindow); j++ {
			if eles[j] > threshold {
				flatRight = false
				break
			}
		}
		if !flatLeft || !flatRight {
			continue
		}

		minima = append(minima, localMin{idx: i, ele: eles[i]})
	}

	if len(minima) == 0 {
		return nil
	}

	// Build segments
	var segments []ValleySegment
	prevIdx := 0
	for _, m := range minima {
		if m.idx-prevIdx < 5 {
			continue
		}
		segments = append(segments, ValleySegment{
			From: pass0Path[prevIdx], To: pass0Path[m.idx],
			FromIdx: prevIdx, ToIdx: m.idx,
		})
		prevIdx = m.idx
	}

	if prevIdx < n-1 && prevIdx > 0 {
		segments = append(segments, ValleySegment{
			From: pass0Path[prevIdx], To: pass0Path[n-1],
			FromIdx: prevIdx, ToIdx: n - 1,
		})
	}

	if len(segments) < 2 {
		return nil
	}
	return segments
}
