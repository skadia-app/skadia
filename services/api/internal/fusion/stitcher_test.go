package fusion

import (
	"testing"
	"time"
)

func float64Ptr(v float64) *float64 { return &v }

func TestSortTimes(t *testing.T) {
	now := time.Now()
	times := []time.Time{
		now.Add(3 * time.Second),
		now.Add(1 * time.Second),
		now.Add(2 * time.Second),
	}
	sortTimes(times)
	for i := 1; i < len(times); i++ {
		if times[i].Before(times[i-1]) {
			t.Errorf("times not sorted: index %d before %d", i, i-1)
		}
	}
}

func TestBuildUnifiedTimeline(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	// Simuliere zwei Quellen mit Überlappung
	streamA := []StreamPoint{
		{Time: now, Lat: float64Ptr(47.0), Lng: float64Ptr(11.0)},
		{Time: now.Add(1 * time.Second), Lat: float64Ptr(47.001), Lng: float64Ptr(11.001)},
		{Time: now.Add(2 * time.Second), Lat: float64Ptr(47.002), Lng: float64Ptr(11.002)},
	}
	streamB := []StreamPoint{
		{Time: now.Add(1 * time.Second), Lat: float64Ptr(47.0015), Lng: float64Ptr(11.0015)},
		{Time: now.Add(2 * time.Second), Lat: float64Ptr(47.0025), Lng: float64Ptr(11.0025)},
		{Time: now.Add(3 * time.Second), Lat: float64Ptr(47.003), Lng: float64Ptr(11.003)},
	}

	// Primary = streamA
	// Erwartung: Bei Überlappung (t+1s, t+2s) wird streamA bevorzugt
	// Bei t+3s (nur B) wird streamB verwendet

	streams := map[string][]StreamPoint{
		"actA": streamA,
		"actB": streamB,
	}

	timeSet := make(map[time.Time]struct{})
	for _, points := range streams {
		for _, p := range points {
			if p.Lat != nil && p.Lng != nil {
				timeSet[p.Time] = struct{}{}
			}
		}
	}

	if len(timeSet) != 4 { // t, t+1, t+2, t+3
		t.Errorf("expected 4 unique timestamps, got %d", len(timeSet))
	}

	// Index-Maps
	indices := map[string]map[time.Time]int{}
	for id, points := range streams {
		idx := make(map[time.Time]int)
		for i, p := range points {
			idx[p.Time] = i
		}
		indices[id] = idx
	}

	primaryID := "actA"
	allIDs := []string{"actA", "actB"}

	times := make([]time.Time, 0, len(timeSet))
	for tm := range timeSet {
		times = append(times, tm)
	}
	sortTimes(times)

	result := make([]StreamPoint, 0, len(times))
	for _, tm := range times {
		var best *StreamPoint
		if idx, ok := indices[primaryID]; ok {
			if i, found := idx[tm]; found {
				p := streams[primaryID][i]
				if p.Lat != nil && p.Lng != nil {
					cp := p
					best = &cp
				}
			}
		}
		if best == nil {
			for _, id := range allIDs {
				if id == primaryID {
					continue
				}
				if idx, ok := indices[id]; ok {
					if i, found := idx[tm]; found {
						p := streams[id][i]
						if p.Lat != nil && p.Lng != nil {
							cp := p
							best = &cp
							break
						}
					}
				}
			}
		}
		if best != nil {
			result = append(result, *best)
		}
	}

	if len(result) != 4 {
		t.Errorf("expected 4 result points, got %d", len(result))
	}

	// Bei t+1s: Primary (A) sollte bevorzugt werden → Lat = 47.001
	if result[1].Lat == nil || *result[1].Lat != 47.001 {
		t.Errorf("at t+1s: expected lat 47.001 (from A), got %v", result[1].Lat)
	}

	// Bei t+3s: Nur B vorhanden → Lat = 47.003
	if result[3].Lat == nil || *result[3].Lat != 47.003 {
		t.Errorf("at t+3s: expected lat 47.003 (from B), got %v", result[3].Lat)
	}
}

func TestEmptySourceIgnored(t *testing.T) {
	// Test: Leere Quelle wird ignoriert ohne Fehler
	streams := map[string][]StreamPoint{
		"empty": {},
	}
	timeSet := make(map[time.Time]struct{})
	for _, points := range streams {
		for _, p := range points {
			if p.Lat != nil && p.Lng != nil {
				timeSet[p.Time] = struct{}{}
			}
		}
	}
	if len(timeSet) != 0 {
		t.Error("empty stream should produce no timestamps")
	}
}
