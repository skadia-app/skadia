package fitimport

import (
	"math"
	"testing"
	"time"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/proto"

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

// TestExtractStreams_BatterySocScale tests BatterySoc Scale: /2
func TestExtractStreams_BatterySocScale(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	records := []*mesgdef.Record{
		{
			Timestamp:  now,
			BatterySoc: 200, // /2 = 100.0%
			// Set other fields to invalid to avoid noise
			PositionLat:  invalidSint32,
			PositionLong: invalidSint32,
		},
	}

	result := ExtractStreams(records, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 stream point, got %d", len(result))
	}

	if result[0].BatterySocPct == nil {
		t.Fatal("BatterySocPct: expected non-nil")
	}
	if math.Abs(*result[0].BatterySocPct-100.0) > 0.01 {
		t.Errorf("BatterySocPct: got %f, want 100.0", *result[0].BatterySocPct)
	}
}

// TestExtractStreams_Po2Scale tests Po2 Scale: /100
func TestExtractStreams_Po2Scale(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	records := []*mesgdef.Record{
		{
			Timestamp:    now,
			Po2:          21, // /100 = 0.21
			PositionLat:  invalidSint32,
			PositionLong: invalidSint32,
		},
	}

	result := ExtractStreams(records, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 stream point, got %d", len(result))
	}

	if result[0].Po2 == nil {
		t.Fatal("Po2: expected non-nil")
	}
	if math.Abs(*result[0].Po2-0.21) > 0.001 {
		t.Errorf("Po2: got %f, want 0.21", *result[0].Po2)
	}
}

// TestExtractStreams_LeftPowerPhase tests Power Phase: raw / 0.7111111 = degrees
func TestExtractStreams_LeftPowerPhase(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	records := []*mesgdef.Record{
		{
			Timestamp:      now,
			LeftPowerPhase: []uint8{0, 128}, // [0/0.7111111, 128/0.7111111] = [0.0, ~180.0]
			PositionLat:    invalidSint32,
			PositionLong:   invalidSint32,
		},
	}

	result := ExtractStreams(records, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 stream point, got %d", len(result))
	}

	if result[0].LeftPowerPhaseStartDeg == nil {
		t.Fatal("LeftPowerPhaseStartDeg: expected non-nil")
	}
	if math.Abs(*result[0].LeftPowerPhaseStartDeg-0.0) > 0.01 {
		t.Errorf("LeftPowerPhaseStartDeg: got %f, want 0.0", *result[0].LeftPowerPhaseStartDeg)
	}

	if result[0].LeftPowerPhaseEndDeg == nil {
		t.Fatal("LeftPowerPhaseEndDeg: expected non-nil")
	}
	// 128 / 0.7111111 = ~180.0
	if math.Abs(*result[0].LeftPowerPhaseEndDeg-180.0) > 0.1 {
		t.Errorf("LeftPowerPhaseEndDeg: got %f, want ~180.0", *result[0].LeftPowerPhaseEndDeg)
	}
}

// TestExtractStreams_CycleLength16Priority tests CycleLength16 > CycleLength
func TestExtractStreams_CycleLength16Priority(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	records := []*mesgdef.Record{
		{
			Timestamp:     now,
			CycleLength16: 250, // /100 = 2.5 m (preferred)
			CycleLength:   150, // /100 = 1.5 m (should be ignored)
			PositionLat:   invalidSint32,
			PositionLong:  invalidSint32,
		},
	}

	result := ExtractStreams(records, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 stream point, got %d", len(result))
	}

	if result[0].CycleLengthM == nil {
		t.Fatal("CycleLengthM: expected non-nil")
	}
	if math.Abs(*result[0].CycleLengthM-2.5) > 0.01 {
		t.Errorf("CycleLengthM: got %f, want 2.5 (CycleLength16 preferred)", *result[0].CycleLengthM)
	}
}

// TestExtractStreams_CycleLengthFallback tests fallback to CycleLength when CycleLength16 is invalid
func TestExtractStreams_CycleLengthFallback(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	records := []*mesgdef.Record{
		{
			Timestamp:     now,
			CycleLength16: basetype.Uint16Invalid, // invalid → fallback
			CycleLength:   150,                    // /100 = 1.5 m
			PositionLat:   invalidSint32,
			PositionLong:  invalidSint32,
		},
	}

	result := ExtractStreams(records, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 stream point, got %d", len(result))
	}

	if result[0].CycleLengthM == nil {
		t.Fatal("CycleLengthM: expected non-nil")
	}
	if math.Abs(*result[0].CycleLengthM-1.5) > 0.01 {
		t.Errorf("CycleLengthM: got %f, want 1.5 (CycleLength fallback)", *result[0].CycleLengthM)
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

func TestExtractStreams_DeveloperFieldClearTextKeys(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	fieldDescs := []*mesgdef.FieldDescription{
		{
			DeveloperDataIndex:    0,
			FieldDefinitionNumber: 8,
			FieldName:             []string{"Form Power"},
			Units:                 []string{"Watts"},
		},
		{
			DeveloperDataIndex:    0,
			FieldDefinitionNumber: 9,
			FieldName:             []string{"Leg Spring Stiffness"},
			Units:                 []string{"kN/m"},
		},
	}

	records := []*mesgdef.Record{
		{
			Timestamp: now,
			DeveloperFields: []proto.DeveloperField{
				{DeveloperDataIndex: 0, Num: 8, Value: proto.Float64(42.5)},
				{DeveloperDataIndex: 0, Num: 9, Value: proto.Float64(9.8)},
			},
		},
	}

	streams := ExtractStreams(records, fieldDescs)
	if len(streams) != 1 {
		t.Fatalf("expected 1 stream point, got %d", len(streams))
	}

	df := streams[0].DeveloperFields
	if df == nil {
		t.Fatal("expected developer fields, got nil")
	}

	if _, ok := df["Form Power"]; !ok {
		t.Errorf("expected key 'Form Power', got keys: %v", df)
	}
	if _, ok := df["Leg Spring Stiffness"]; !ok {
		t.Errorf("expected key 'Leg Spring Stiffness', got keys: %v", df)
	}
	if _, ok := df["8"]; ok {
		t.Error("numeric key '8' should not exist when clear-text name available")
	}
}

func TestExtractStreams_DeveloperFieldFallbackNumeric(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	records := []*mesgdef.Record{
		{
			Timestamp: now,
			DeveloperFields: []proto.DeveloperField{
				{DeveloperDataIndex: 0, Num: 5, Value: proto.Float64(1.23)},
			},
		},
	}

	streams := ExtractStreams(records, nil)
	if len(streams) != 1 {
		t.Fatalf("expected 1 stream point, got %d", len(streams))
	}

	df := streams[0].DeveloperFields
	if _, ok := df["5"]; !ok {
		t.Errorf("expected numeric fallback key '5', got keys: %v", df)
	}
}
