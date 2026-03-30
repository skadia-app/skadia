package enrichment

import "testing"

//go:fix inline
func ptr[T any](v T) *T { return new(v) }

// makePoint is a convenience helper that creates a matchedPoint already
// carrying pre-computed SurfaceClass and HighwayClass values (as the SQL
// enrichment loop would set them before calling modeFilter).
func makePoint(idx int, osmID *int64, surface, highway string) matchedPoint {
	p := matchedPoint{
		Idx:          idx,
		OsmID:        osmID,
		SurfaceClass: surface,
		HighwayClass: highway,
	}
	if surface != "offroad" {
		p.DistM = new(5.0) // within tolerance
	}
	if surface != "offroad" && surface != "unknown" {
		p.Surface = &surface
	}
	if highway != "offroad" && highway != "unknown" {
		p.Highway = &highway
	}
	return p
}

// --- modeFilter tests ---

func TestModeFilter_NoChange(t *testing.T) {
	id := int64(42)
	points := make([]matchedPoint, 10)
	for i := range 10 {
		points[i] = makePoint(i, &id, "asphalt", "residential")
	}

	result := modeFilter(points)

	if len(result) != len(points) {
		t.Fatalf("length changed: got %d want %d", len(result), len(points))
	}
	for i, p := range result {
		if p.SurfaceClass != "asphalt" {
			t.Errorf("point %d: SurfaceClass = %q, want \"asphalt\"", i, p.SurfaceClass)
		}
		if p.HighwayClass != "residential" {
			t.Errorf("point %d: HighwayClass = %q, want \"residential\"", i, p.HighwayClass)
		}
	}
}

func TestModeFilter_FixesDrift(t *testing.T) {
	// 9 points on way 1 (asphalt/residential), point index 4 drifts to way 2 (gravel/track).
	id1 := int64(1)
	id2 := int64(2)
	points := make([]matchedPoint, 9)
	for i := range 9 {
		points[i] = makePoint(i, &id1, "asphalt", "residential")
	}
	// Inject drifting point at index 4.
	points[4] = makePoint(4, &id2, "gravel", "track")

	result := modeFilter(points)

	if result[4].SurfaceClass != "asphalt" {
		t.Errorf("drifting point not corrected: SurfaceClass = %q, want \"asphalt\"", result[4].SurfaceClass)
	}
	if result[4].HighwayClass != "residential" {
		t.Errorf("drifting point not corrected: HighwayClass = %q, want \"residential\"", result[4].HighwayClass)
	}
}

func TestModeFilter_KeepsOffroad(t *testing.T) {
	// 3 points: Way 1 (asphalt), nil OsmID (offroad), Way 1 (asphalt).
	id1 := int64(1)
	points := []matchedPoint{
		makePoint(0, &id1, "asphalt", "residential"),
		{Idx: 1, OsmID: nil, SurfaceClass: "offroad", HighwayClass: "offroad"},
		makePoint(2, &id1, "asphalt", "residential"),
	}

	result := modeFilter(points)

	if result[1].SurfaceClass != "offroad" {
		t.Errorf("offroad point was modified: SurfaceClass = %q, want \"offroad\"", result[1].SurfaceClass)
	}
	if result[1].OsmID != nil {
		t.Errorf("offroad point OsmID was set, want nil")
	}
}

func TestModeFilter_ShortInput(t *testing.T) {
	// Fewer than surfaceModeWindowSize (5) points — must not panic.
	id := int64(7)
	points := []matchedPoint{
		makePoint(0, &id, "asphalt", "primary"),
		makePoint(1, &id, "asphalt", "primary"),
	}

	result := modeFilter(points)

	if len(result) != 2 {
		t.Fatalf("length changed: got %d want 2", len(result))
	}
	for i, p := range result {
		if p.SurfaceClass != "asphalt" {
			t.Errorf("point %d: unexpected SurfaceClass %q", i, p.SurfaceClass)
		}
	}
}

// --- aggregateByField tests ---

func TestAggregateByField_Percentages(t *testing.T) {
	// 100 points: 70× asphalt, 20× gravel, 10× offroad.
	points := make([]matchedPoint, 100)
	id := int64(1)
	for i := range 70 {
		points[i] = makePoint(i, &id, "asphalt", "primary")
	}
	for i := 70; i < 90; i++ {
		points[i] = makePoint(i, &id, "gravel", "track")
	}
	for i := 90; i < 100; i++ {
		points[i] = makePoint(i, nil, "offroad", "offroad")
	}

	result := aggregateByField(points, func(p matchedPoint) string { return p.SurfaceClass })

	// Collect into a map for easy lookup.
	pctByType := make(map[string]float64)
	for _, bd := range result {
		pctByType[bd.Type] = bd.Percentage
	}

	// Sum of all percentages must equal 100.
	var sum float64
	for _, bd := range result {
		sum += bd.Percentage
	}
	if sum < 99.99 || sum > 100.01 {
		t.Errorf("percentage sum = %.4f, want 100.0", sum)
	}

	if pctByType["asphalt"] != 70.0 {
		t.Errorf("asphalt percentage = %.1f, want 70.0", pctByType["asphalt"])
	}
	if pctByType["gravel"] != 20.0 {
		t.Errorf("gravel percentage = %.1f, want 20.0", pctByType["gravel"])
	}
	if pctByType["offroad"] != 10.0 {
		t.Errorf("offroad percentage = %.1f, want 10.0", pctByType["offroad"])
	}
}

func TestAggregateByField_SortedDescending(t *testing.T) {
	// 4 points: 1× gravel, 3× asphalt.  First element must be asphalt (75%).
	id := int64(5)
	points := []matchedPoint{
		makePoint(0, &id, "gravel", "track"),
		makePoint(1, &id, "asphalt", "primary"),
		makePoint(2, &id, "asphalt", "primary"),
		makePoint(3, &id, "asphalt", "primary"),
	}

	result := aggregateByField(points, func(p matchedPoint) string { return p.SurfaceClass })

	if len(result) == 0 {
		t.Fatal("empty result")
	}
	if result[0].Type != "asphalt" {
		t.Errorf("first element type = %q, want \"asphalt\"", result[0].Type)
	}
	if result[0].Percentage != 75.0 {
		t.Errorf("asphalt percentage = %.1f, want 75.0", result[0].Percentage)
	}
}

// --- Classification logic tests ---

// TestClassification_UnknownSurface verifies the classification logic inline:
// DistM within tolerance, Surface nil, Highway set → SurfaceClass="unknown", HighwayClass=highway value.
func TestClassification_UnknownSurface(t *testing.T) {
	p := matchedPoint{
		Idx:     0,
		DistM:   new(10.0),
		Surface: nil,
		Highway: new("track"),
	}

	// Reproduce the classification block from enrichSurfacesSQL.
	if p.DistM == nil || *p.DistM > surfaceMatchToleranceM {
		p.SurfaceClass = "offroad"
		p.HighwayClass = "offroad"
	} else if p.Surface == nil {
		p.SurfaceClass = "unknown"
		if p.Highway != nil {
			p.HighwayClass = *p.Highway
		} else {
			p.HighwayClass = "offroad"
		}
	} else {
		p.SurfaceClass = *p.Surface
		if p.Highway != nil {
			p.HighwayClass = *p.Highway
		} else {
			p.HighwayClass = "unknown"
		}
	}

	if p.SurfaceClass != "unknown" {
		t.Errorf("SurfaceClass = %q, want \"unknown\"", p.SurfaceClass)
	}
	if p.HighwayClass != "track" {
		t.Errorf("HighwayClass = %q, want \"track\"", p.HighwayClass)
	}
}

// TestClassification_Offroad verifies that a nil DistM → offroad classification.
func TestClassification_Offroad(t *testing.T) {
	p := matchedPoint{
		Idx:   0,
		DistM: nil,
	}

	// Reproduce the classification block from enrichSurfacesSQL.
	if p.DistM == nil || *p.DistM > surfaceMatchToleranceM {
		p.SurfaceClass = "offroad"
		p.HighwayClass = "offroad"
	} else if p.Surface == nil {
		p.SurfaceClass = "unknown"
		if p.Highway != nil {
			p.HighwayClass = *p.Highway
		} else {
			p.HighwayClass = "offroad"
		}
	} else {
		p.SurfaceClass = *p.Surface
		if p.Highway != nil {
			p.HighwayClass = *p.Highway
		} else {
			p.HighwayClass = "unknown"
		}
	}

	if p.SurfaceClass != "offroad" {
		t.Errorf("SurfaceClass = %q, want \"offroad\"", p.SurfaceClass)
	}
	if p.HighwayClass != "offroad" {
		t.Errorf("HighwayClass = %q, want \"offroad\"", p.HighwayClass)
	}
}
