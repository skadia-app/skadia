package fitimport

import (
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func fptr(v float64) *float64 { return &v }

// TestFixDistanceResets_SingleLap — keine Änderung bei normalem Verlauf
func TestFixDistanceResets_SingleLap(t *testing.T) {
	now := time.Now()
	points := []model.StreamPoint{
		{Time: now, DistanceM: fptr(0.0)},
		{Time: now.Add(1 * time.Second), DistanceM: fptr(200.0)},
		{Time: now.Add(2 * time.Second), DistanceM: fptr(500.0)},
		{Time: now.Add(3 * time.Second), DistanceM: fptr(800.0)},
	}
	got := fixDistanceResets(points)
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
		{Time: now, DistanceM: fptr(200.0)},
		{Time: now.Add(1 * time.Second), DistanceM: fptr(500.0)},
		// GOTOES Reset: dist springt auf nahe 0
		{Time: now.Add(2 * time.Second), DistanceM: fptr(0.12)},
		{Time: now.Add(3 * time.Second), DistanceM: fptr(300.0)},
		{Time: now.Add(4 * time.Second), DistanceM: fptr(700.0)},
	}
	got := fixDistanceResets(points)
	// Nach Reset: offset=500, also: 0.12+500=500.12, 300+500=800, 700+500=1200
	want := []float64{200.0, 500.0, 500.12, 800.0, 1200.0}
	for i, w := range want {
		if got[i].DistanceM == nil {
			t.Fatalf("point %d: DistanceM is nil, want %f", i, w)
		}
		if absF(*got[i].DistanceM-w) > 0.01 {
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
		{Time: now, DistanceM: fptr(200.0)},
		{Time: now.Add(1 * time.Second), DistanceM: nil},
		{Time: now.Add(2 * time.Second), DistanceM: fptr(0.5)}, // Reset nach 200m
		{Time: now.Add(3 * time.Second), DistanceM: fptr(300.0)},
	}
	got := fixDistanceResets(points)
	if got[1].DistanceM != nil {
		t.Error("point 1: DistanceM should remain nil")
	}
	if got[2].DistanceM == nil {
		t.Fatal("point 2: DistanceM is nil")
	}
	if absF(*got[2].DistanceM-200.5) > 0.01 {
		t.Errorf("point 2: got %f, want 200.5", *got[2].DistanceM)
	}
}

// TestFixDistanceResets_SmallDrop — kleiner Rückwärtssprung (<100m) ist kein Reset
// The threshold is 100m: drops smaller than 100m are NOT treated as resets.
func TestFixDistanceResets_SmallDrop(t *testing.T) {
	now := time.Now()
	points := []model.StreamPoint{
		{Time: now, DistanceM: fptr(500.0)},
		{Time: now.Add(1 * time.Second), DistanceM: fptr(498.0)}, // 2m zurück, kein Reset
		{Time: now.Add(2 * time.Second), DistanceM: fptr(600.0)},
	}
	got := fixDistanceResets(points)
	// Kein Offset — 498 < 500 aber < 100m Differenz
	if absF(*got[1].DistanceM-498.0) > 0.01 {
		t.Errorf("point 1: got %f, want 498.0 (small drop should not trigger reset)", *got[1].DistanceM)
	}
	if absF(*got[2].DistanceM-600.0) > 0.01 {
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
		{Time: now, DistanceM: fptr(0.0)},
		{Time: now.Add(1 * time.Second), DistanceM: fptr(500.0)},
		// Reset 1: raw → 0.1 (offset becomes 500)
		{Time: now.Add(2 * time.Second), DistanceM: fptr(0.1)},
		{Time: now.Add(3 * time.Second), DistanceM: fptr(400.0)},
		// Reset 2: raw → 0.2 (offset becomes 500+400=900)
		{Time: now.Add(4 * time.Second), DistanceM: fptr(0.2)},
		{Time: now.Add(5 * time.Second), DistanceM: fptr(300.0)},
	}
	got := fixDistanceResets(points)
	// Segment 1: 0→500 (no offset)
	// After Reset 1: 0.1+500=500.1, 400+500=900
	// After Reset 2: 0.2+900=900.2, 300+900=1200
	want := []float64{0.0, 500.0, 500.1, 900.0, 900.2, 1200.0}
	for i, w := range want {
		if got[i].DistanceM == nil {
			t.Fatalf("point %d: DistanceM is nil, want %f", i, w)
		}
		if absF(*got[i].DistanceM-w) > 0.01 {
			t.Errorf("point %d: got %f, want %f", i, *got[i].DistanceM, w)
		}
	}
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
