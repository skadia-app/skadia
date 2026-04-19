package handler

import "testing"

func TestTvDisplayName(t *testing.T) {
	if tvDisplayName(4) != "schlecht sichtbar" {
		t.Errorf("tv(4) want 'schlecht sichtbar'")
	}
	if tvDisplayName(0) != "" || tvDisplayName(99) != "" {
		t.Errorf("tv bounds")
	}
}

func mkCand(opts ...func(*RouteCandidate)) RouteCandidate {
	c := RouteCandidate{}
	for _, o := range opts {
		o(&c)
	}
	return c
}

func withMtb(scale uint8, pct float32) func(*RouteCandidate) {
	return func(c *RouteCandidate) { c.MaxMtbScale = scale; c.TaggedPctMtb = pct }
}
func withSac(scale uint8, pct float32) func(*RouteCandidate) {
	return func(c *RouteCandidate) { c.MaxSacScale = scale; c.TaggedPctSac = pct }
}
func withVia(hw bool, scale uint8, pct float32) func(*RouteCandidate) {
	return func(c *RouteCandidate) { c.HasViaFerrataHighway = hw; c.MaxViaFerrataScale = scale; c.TaggedPctVia = pct }
}
func withTv(max uint8, intPct, pct float32) func(*RouteCandidate) {
	return func(c *RouteCandidate) { c.MaxTrailVisibility = max; c.IntermediatePctTv = intPct; c.TaggedPctTv = pct }
}

func getDescriptor(ds []VariantDescriptor, key string) VariantDescriptor {
	for _, d := range ds {
		if d.Key == key {
			return d
		}
	}
	return VariantDescriptor{}
}

func TestComputeDescriptors_MtbTechnical(t *testing.T) {
	th := defaultOutdoorThresholds
	// S2 (encoded=3) vs S0 (encoded=1) strict > → fires
	target := mkCand(withMtb(3, 0.25))
	others := []RouteCandidate{mkCand(withMtb(1, 0.25))}
	ds := computeDescriptorsFor(target, others, 0, false, th)
	if !hasDescriptor(ds, "mtb_technical") {
		t.Errorf("S2 vs S0 should fire")
	}
	if getDescriptor(ds, "mtb_technical").DisplayValue != "bis S2" {
		t.Errorf("want 'bis S2'; got %q", getDescriptor(ds, "mtb_technical").DisplayValue)
	}
	// Below threshold
	target = mkCand(withMtb(2, 0.25))
	if hasDescriptor(computeDescriptorsFor(target, others, 0, false, th), "mtb_technical") {
		t.Errorf("S1 must not fire")
	}
	// Tie — strict > → no fire
	target = mkCand(withMtb(3, 0.25))
	others = []RouteCandidate{mkCand(withMtb(3, 0.25))}
	if hasDescriptor(computeDescriptorsFor(target, others, 0, false, th), "mtb_technical") {
		t.Errorf("tie must not fire")
	}
}

func TestComputeDescriptors_MtbTechnical_AllTie_NoFire(t *testing.T) {
	th := defaultOutdoorThresholds
	// Alle 3 candidates haben MaxMtbScale=3 (≥ trigger=3) + Coverage.
	// Strict > maxOther → niemand feuert.
	target := mkCand(withMtb(3, 0.25))
	others := []RouteCandidate{
		mkCand(withMtb(3, 0.25)),
		mkCand(withMtb(3, 0.25)),
	}
	ds := computeDescriptorsFor(target, others, 0, false, th)
	if hasDescriptor(ds, "mtb_technical") {
		t.Errorf("all-tie must not fire mtb_technical")
	}
}

func TestComputeDescriptors_AlpineHiking_AllTie_NoFire(t *testing.T) {
	th := defaultOutdoorThresholds
	target := mkCand(withSac(4, 0.25))
	others := []RouteCandidate{
		mkCand(withSac(4, 0.25)),
		mkCand(withSac(4, 0.25)),
	}
	ds := computeDescriptorsFor(target, others, 0, false, th)
	if hasDescriptor(ds, "alpine_hiking") {
		t.Errorf("all-tie must not fire alpine_hiking")
	}
}

func TestComputeDescriptors_Exposed_AllHaveVia_NoFire(t *testing.T) {
	th := defaultOutdoorThresholds
	// Alle 3 candidates haben highway=via_ferrata + Coverage ≥ 1%.
	// exposed's early-break kickt in → niemand fires.
	target := mkCand(withVia(true, 2, 0.05))
	others := []RouteCandidate{
		mkCand(withVia(true, 3, 0.06)),
		mkCand(withVia(true, 1, 0.04)),
	}
	ds := computeDescriptorsFor(target, others, 0, false, th)
	if hasDescriptor(ds, "exposed") {
		t.Errorf("all-have-via must not fire exposed (binary descriptor)")
	}
}

func TestComputeDescriptors_LowVisibility_AllTieIntermediate_NoFire(t *testing.T) {
	th := defaultOutdoorThresholds
	// Alle 3 Candidates haben gleichen IntermediatePctTv (30%) + gleiches
	// MaxTrailVisibility=3 (UNTER LowVisMaxTrigger=4 → kein maxTrigger).
	// Δ = me.IntermediatePctTv - maxOther = 0 < LowVisIntDelta (10%) → kein intTrigger.
	target := mkCand(withTv(3, 0.30, 0.30))
	others := []RouteCandidate{
		mkCand(withTv(3, 0.30, 0.30)),
		mkCand(withTv(3, 0.30, 0.30)),
	}
	ds := computeDescriptorsFor(target, others, 0, false, th)
	if hasDescriptor(ds, "low_visibility") {
		t.Errorf("all-tie intermediate + maxTv<trigger must not fire low_visibility")
	}
}

func TestComputeDescriptors_AlpineHiking(t *testing.T) {
	th := defaultOutdoorThresholds
	target := mkCand(withSac(4, 0.25))
	others := []RouteCandidate{mkCand(withSac(3, 0.25))}
	if !hasDescriptor(computeDescriptorsFor(target, others, 0, false, th), "alpine_hiking") {
		t.Errorf("T4 should fire")
	}
}

func TestComputeDescriptors_ExposedHighwayOnly(t *testing.T) {
	th := defaultOutdoorThresholds
	target := mkCand(withVia(true, 0, 0.05))
	others := []RouteCandidate{mkCand(withVia(false, 0, 0))}
	ds := computeDescriptorsFor(target, others, 0, false, th)
	if getDescriptor(ds, "exposed").DisplayValue != "Klettersteig-Abschnitt" {
		t.Errorf("want 'Klettersteig-Abschnitt'")
	}
}

func TestComputeDescriptors_ExposedWithScale(t *testing.T) {
	th := defaultOutdoorThresholds
	target := mkCand(withVia(true, 2, 0.05))
	others := []RouteCandidate{mkCand(withVia(false, 0, 0))}
	if getDescriptor(computeDescriptorsFor(target, others, 0, false, th), "exposed").DisplayValue != "bis K2" {
		t.Errorf("want 'bis K2'")
	}
}

func TestComputeDescriptors_LowVisibilityMaxTrigger(t *testing.T) {
	th := defaultOutdoorThresholds
	target := mkCand(withTv(4, 0, 0.25))
	others := []RouteCandidate{mkCand(withTv(2, 0, 0.25))}
	if !hasDescriptor(computeDescriptorsFor(target, others, 0, false, th), "low_visibility") {
		t.Errorf("tv=4 should fire")
	}
}

func TestComputeDescriptors_LowVisibilityTwoStage(t *testing.T) {
	th := defaultOutdoorThresholds
	target := mkCand(withTv(3, 0.30, 0.30))
	others := []RouteCandidate{mkCand(withTv(2, 0.05, 0.30))}
	ds := computeDescriptorsFor(target, others, 0, false, th)
	if !hasDescriptor(ds, "low_visibility") {
		t.Errorf("two-stage should fire")
	}
	if getDescriptor(ds, "low_visibility").DisplayValue != "30% mäßig sichtbar" {
		t.Errorf("want '30%% mäßig sichtbar'")
	}
}

func TestComputeDescriptors_CoverageGateBoundary(t *testing.T) {
	th := defaultOutdoorThresholds
	target := mkCand(withSac(4, 0.19))
	others := []RouteCandidate{mkCand(withSac(3, 0.19))}
	if hasDescriptor(computeDescriptorsFor(target, others, 0, false, th), "alpine_hiking") {
		t.Errorf("19%% must suppress")
	}
	target = mkCand(withSac(4, 0.20))
	if !hasDescriptor(computeDescriptorsFor(target, others, 0, false, th), "alpine_hiking") {
		t.Errorf("20%% (inclusive) must fire")
	}
}
