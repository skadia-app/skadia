package fitimport

import (
	"math"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/activityutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

//go:fix inline
func fptr(v float64) *float64 { return new(v) }

// TestFixDistanceResets_SingleLap — keine Änderung bei normalem Verlauf
func TestFixDistanceResets_SingleLap(t *testing.T) {
	now := time.Now()
	points := []model.StreamPoint{
		{Time: now, DistanceM: new(0.0)},
		{Time: now.Add(1 * time.Second), DistanceM: new(200.0)},
		{Time: now.Add(2 * time.Second), DistanceM: new(500.0)},
		{Time: now.Add(3 * time.Second), DistanceM: new(800.0)},
	}
	got := activityutil.FixDistanceResets(points)
	want := []float64{0.0, 200.0, 500.0, 800.0}
	for i, w := range want {
		if got[i].DistanceM == nil {
			t.Fatalf("point %d: DistanceM is nil, want %f", i, w)
		}
		if *got[i].DistanceM != w {
			t.Errorf("point %d: got %f, want %f", i, *got[i].DistanceM, w)
		}
	}
}

// TestFixDistanceResets_TwoLapReset — zweite Runde setzt Distanz zurück auf 0
func TestFixDistanceResets_TwoLapReset(t *testing.T) {
	now := time.Now()
	points := []model.StreamPoint{
		{Time: now, DistanceM: new(200.0)},
		{Time: now.Add(1 * time.Second), DistanceM: new(500.0)},
		// GOTOES Reset: dist springt auf nahe 0
		{Time: now.Add(2 * time.Second), DistanceM: new(0.12)},
		{Time: now.Add(3 * time.Second), DistanceM: new(300.0)},
		{Time: now.Add(4 * time.Second), DistanceM: new(700.0)},
	}
	got := activityutil.FixDistanceResets(points)
	// Nach Reset: offset=500, also: 0.12+500=500.12, 300+500=800, 700+500=1200
	want := []float64{200.0, 500.0, 500.12, 800.0, 1200.0}
	for i, w := range want {
		if got[i].DistanceM == nil {
			t.Fatalf("point %d: DistanceM is nil, want %f", i, w)
		}
		if math.Abs(*got[i].DistanceM-w) > 0.01 {
			t.Errorf("point %d: got %f, want %f", i, *got[i].DistanceM, w)
		}
	}
}

// TestFixDistanceResets_NilDistances — nil-Punkte (kein GPS/keine Distanz) bleiben nil.
// Design: prevDist wird über nil-Punkte hinweg beibehalten (keine Rücksetzung auf 0).
// Wenn nach einem nil-Punkt die Distanz wieder klein ist (0.5 < 200-100=100),
// gilt das als Reset — der Distanzzähler hat sich zurückgesetzt (GPS-Dropout + Reset).
// Das ist das korrekte Verhalten für GOTOES-merged FITs.
func TestFixDistanceResets_NilDistances(t *testing.T) {
	now := time.Now()
	points := []model.StreamPoint{
		{Time: now, DistanceM: new(200.0)},
		{Time: now.Add(1 * time.Second), DistanceM: nil},
		{Time: now.Add(2 * time.Second), DistanceM: new(0.5)}, // Reset nach 200m
		{Time: now.Add(3 * time.Second), DistanceM: new(300.0)},
	}
	got := activityutil.FixDistanceResets(points)
	if got[1].DistanceM != nil {
		t.Error("point 1: DistanceM should remain nil")
	}
	if got[2].DistanceM == nil {
		t.Fatal("point 2: DistanceM is nil")
	}
	if math.Abs(*got[2].DistanceM-200.5) > 0.01 {
		t.Errorf("point 2: got %f, want 200.5", *got[2].DistanceM)
	}
}

// TestFixDistanceResets_SmallDrop — kleiner Rückwärtssprung (<100m) ist kein Reset
// The threshold is 100m: drops smaller than 100m are NOT treated as resets.
func TestFixDistanceResets_SmallDrop(t *testing.T) {
	now := time.Now()
	points := []model.StreamPoint{
		{Time: now, DistanceM: new(500.0)},
		{Time: now.Add(1 * time.Second), DistanceM: new(498.0)}, // 2m zurück, kein Reset
		{Time: now.Add(2 * time.Second), DistanceM: new(600.0)},
	}
	got := activityutil.FixDistanceResets(points)
	// Kein Offset — 498 < 500 aber < 100m Differenz
	if math.Abs(*got[1].DistanceM-498.0) > 0.01 {
		t.Errorf("point 1: got %f, want 498.0 (small drop should not trigger reset)", *got[1].DistanceM)
	}
	if math.Abs(*got[2].DistanceM-600.0) > 0.01 {
		t.Errorf("point 2: got %f, want 600.0", *got[2].DistanceM)
	}
}

// TestFixDistanceResets_ThreeSegments — drei gemergde Segmente (dreifacher Reset).
// Prüft, dass offset korrekt als Summe der Raw-Maxima akkumuliert wird:
// prevDist verfolgt den Raw-Wert (nicht den akkumulierten), damit offset beim
// zweiten Reset nur den Raw-Peak des zweiten Segments addiert (nicht den
// bereits akkumulierten Wert). Ergebnis: 500+400+300 = 1200m Total.
func TestFixDistanceResets_ThreeSegments(t *testing.T) {
	now := time.Now()
	points := []model.StreamPoint{
		// Segment 1: raw 0→500
		{Time: now, DistanceM: new(0.0)},
		{Time: now.Add(1 * time.Second), DistanceM: new(500.0)},
		// Reset 1: raw → 0.1 (offset becomes 500)
		{Time: now.Add(2 * time.Second), DistanceM: new(0.1)},
		{Time: now.Add(3 * time.Second), DistanceM: new(400.0)},
		// Reset 2: raw → 0.2 (offset becomes 500+400=900)
		{Time: now.Add(4 * time.Second), DistanceM: new(0.2)},
		{Time: now.Add(5 * time.Second), DistanceM: new(300.0)},
	}
	got := activityutil.FixDistanceResets(points)
	// Segment 1: 0→500 (no offset)
	// After Reset 1: 0.1+500=500.1, 400+500=900
	// After Reset 2: 0.2+900=900.2, 300+900=1200
	want := []float64{0.0, 500.0, 500.1, 900.0, 900.2, 1200.0}
	for i, w := range want {
		if got[i].DistanceM == nil {
			t.Fatalf("point %d: DistanceM is nil, want %f", i, w)
		}
		if math.Abs(*got[i].DistanceM-w) > 0.01 {
			t.Errorf("point %d: got %f, want %f", i, *got[i].DistanceM, w)
		}
	}
}


// TestDeriveGPSMetrics_MeringScenario — 82 points, 0 with distance,
// 3 clusters separated by multi-hour gaps. Should derive distance for all GPS points.
func TestDeriveGPSMetrics_MeringScenario(t *testing.T) {
	base := time.Date(2014, 6, 21, 14, 19, 22, 0, time.UTC)
	lat, lng := 48.35, 10.90

	// Simulate 3 clusters: 30min, 10h gap, 15min, 5h gap, 37min
	var points []model.StreamPoint
	// Cluster 1: 30 points over 30min
	for i := 0; i < 30; i++ {
		points = append(points, model.StreamPoint{
			Time: base.Add(time.Duration(i) * time.Minute),
			Lat:  fptr(lat + float64(i)*0.001),
			Lng:  fptr(lng),
		})
	}
	// 10.4h gap
	gap1 := base.Add(10*time.Hour + 24*time.Minute)
	for i := 0; i < 15; i++ {
		points = append(points, model.StreamPoint{
			Time: gap1.Add(time.Duration(i) * time.Minute),
			Lat:  fptr(lat + 0.03 + float64(i)*0.001),
			Lng:  fptr(lng + 0.01),
		})
	}
	// 5.1h gap
	gap2 := gap1.Add(5*time.Hour + 6*time.Minute)
	for i := 0; i < 37; i++ {
		points = append(points, model.StreamPoint{
			Time: gap2.Add(time.Duration(i) * time.Minute),
			Lat:  fptr(lat + 0.05 + float64(i)*0.001),
			Lng:  fptr(lng + 0.02),
		})
	}

	got := activityutil.FillMissingDistanceAndSpeed(activityutil.FixDistanceResets(points))

	// Every GPS point should now have distance
	nullCount := 0
	for _, p := range got {
		if p.Lat != nil && p.DistanceM == nil {
			nullCount++
		}
	}
	if nullCount > 0 {
		t.Errorf("%d GPS points still have nil distance", nullCount)
	}

	// Distance should be monotonically increasing
	var prev float64
	for i, p := range got {
		if p.DistanceM == nil {
			continue
		}
		if *p.DistanceM < prev {
			t.Errorf("point %d: distance decreased from %f to %f", i, prev, *p.DistanceM)
		}
		prev = *p.DistanceM
	}

	// Total distance should be reasonable (not 0, not millions)
	if prev < 1000 || prev > 100000 {
		t.Errorf("total distance %f seems unreasonable", prev)
	}
}
