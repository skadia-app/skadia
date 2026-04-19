package routing

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "config", "badrouter")); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// TestMain loads built-in profiles before running any tests in the routing package.
// All tests in this package depend on profiles (DefaultCostConfig, BuiltinProfile, etc.).
func TestMain(m *testing.M) {
	root := findProjectRoot()
	if root == "" {
		fmt.Fprintln(os.Stderr, "FATAL: could not find project root (config/badrouter/ not found)")
		os.Exit(1)
	}
	n, err := LoadBuiltinProfiles(filepath.Join(root, "config", "badrouter"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: LoadBuiltinProfiles: %v\n", err)
		os.Exit(1)
	}
	if n == 0 {
		fmt.Fprintln(os.Stderr, "FATAL: no profiles loaded from config/badrouter/")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// --- Helper: build a minimal graph for edge cost tests ---

func testGraph(tags1 uint64, tags2 *uint64) *graph.Graph {
	t2 := uint64(0)
	if tags2 != nil {
		t2 = *tags2
	}
	return &graph.Graph{
		EdgeDist:  []uint32{10000}, // 100m in cm
		EdgeTags1: []uint64{tags1},
		EdgeTags2: []uint64{t2},
	}
}

func encodeTags(hw, sf string) uint64 {
	return graph.EncodeOSMTags(&graph.RawTags{Highway: hw, Surface: sf})
}

func encodeTagsFull(rt *graph.RawTags) uint64 {
	return graph.EncodeOSMTags(rt)
}

// --- Test: all 5 profiles load ---

func TestBuiltinProfile_AllValid(t *testing.T) {
	for _, name := range []string{"trekking", "hiking", "mtb", "gravel", "road"} {
		p, err := BuiltinProfile(name)
		if err != nil {
			t.Errorf("BuiltinProfile(%q) failed: %v", name, err)
		}
		if p.Profile != name {
			t.Errorf("BuiltinProfile(%q).Profile = %q", name, p.Profile)
		}
	}
}

func TestBuiltinProfile_UnknownFails(t *testing.T) {
	_, err := BuiltinProfile("alpine_skiing")
	if err == nil {
		t.Error("expected error for unknown profile")
	}
}

func TestBuildCostConfig_TrekkingDefaults(t *testing.T) {
	cfg, err := BuildCostConfig(ProfileConfig{Profile: "trekking"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UphillCost != 0 {
		t.Errorf("UphillCost = %f, want 0", cfg.UphillCost)
	}
	if cfg.DownhillCost != 60 {
		t.Errorf("DownhillCost = %f, want 60", cfg.DownhillCost)
	}
	if math.Abs(float64(cfg.UphillCutoff)-0.015) > 0.001 {
		t.Errorf("UphillCutoff = %f, want 0.015", cfg.UphillCutoff)
	}
	if cfg.TurnCost != 90 {
		t.Errorf("TurnCost = %f, want 90", cfg.TurnCost)
	}
}

func TestBuildCostConfig_OverrideMerge(t *testing.T) {
	cfg, err := BuildCostConfig(ProfileConfig{
		Profile:    "hiking",
		UphillCost: new(float32(99)),
		TurnCost:   new(float32(42)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UphillCost != 99 {
		t.Errorf("UphillCost = %f, want 99 (override)", cfg.UphillCost)
	}
	if cfg.TurnCost != 42 {
		t.Errorf("TurnCost = %f, want 42 (override)", cfg.TurnCost)
	}
	// DownhillCost should remain at hiking default (7)
	if cfg.DownhillCost != 7 {
		t.Errorf("DownhillCost = %f, want 7 (default)", cfg.DownhillCost)
	}
}

func TestEdgeCost_MotorwayBlocked_AllProfiles(t *testing.T) {
	tags := encodeTags("motorway", "")
	g := testGraph(tags, nil)

	for _, profile := range []string{"trekking", "hiking", "mtb", "gravel", "road"} {
		cfg, err := BuildCostConfig(ProfileConfig{Profile: profile})
		if err != nil {
			t.Fatal(err)
		}
		ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])
		if ecf.Flat < costBlocked {
			t.Errorf("%s: motorway should be blocked, got Flat=%f", profile, ecf.Flat)
		}
	}
}

func TestEdgeCost_PathCheap_ForHiking(t *testing.T) {
	tags := encodeTags("path", "earth")
	g := testGraph(tags, nil)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "hiking"})
	ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])
	// path=1.0 * earth=1.0 + sacPenalty[0] (untagged, preferred=1) → ~1.05
	if ecf.Flat < 0.9 || ecf.Flat > 1.5 {
		t.Errorf("hiking path+earth: Flat=%f, expected ~1.0", ecf.Flat)
	}
}

func TestEdgeCost_PathExpensive_ForRoad(t *testing.T) {
	tags := encodeTags("path", "")
	g := testGraph(tags, nil)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "road"})
	ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])
	// path=10 for road profile
	if ecf.Flat < 9.0 {
		t.Errorf("road path: Flat=%f, expected >=10", ecf.Flat)
	}
}

func TestEdgeCost_SACBlocked_AboveLimit(t *testing.T) {
	// SAC T4 = alpine_hiking (enum 4) with max_sac=3
	tags := encodeTagsFull(&graph.RawTags{Highway: "path", SacScale: "alpine_hiking"})
	g := testGraph(tags, nil)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "hiking"})
	ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])
	if ecf.Flat < costBlocked {
		t.Errorf("SAC T4 with max_sac=3 should be blocked, got Flat=%f", ecf.Flat)
	}
}

func TestEdgeCost_SACPenalty0_NotZero(t *testing.T) {
	// Untagged way (sacScale=0) with sacPreferred=1 → K1 formula → ~0.05
	tags := encodeTags("path", "")
	g := testGraph(tags, nil)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "hiking"})
	ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])

	// The total CF for untagged path includes sacPenalty[0].
	// sacPenalty[0] with sacPref=1: pow(1.05, 1) - 1 = 0.05
	// baseCF = 1.0 * 1.0 + 0.05 = 1.05
	// With forest bonus (0.5 weight, forest_pct=0 → +0.5) but no EdgeTags2 → no enrichment
	// finalCF = 1.05
	if ecf.Flat < 1.04 || ecf.Flat > 1.1 {
		t.Errorf("sacPenalty[0] should produce CF ~1.05, got %f", ecf.Flat)
	}
}

func TestEdgeCost_ForestBonus_Additive(t *testing.T) {
	tags := encodeTags("path", "")
	// forest_pct=100 → forestAdd = (1.0 - 1.0) * weight = 0.0 (no penalty for full forest)
	// forest_pct=1   → forestAdd = (1.0 - 0.01) * weight ≈ weight (almost no forest)
	// Note: forest_pct=0 means "no data" (no penalty), so we use 1 for "no forest"
	enrich1 := graph.EncodeEnrichmentTags(&graph.RawEnrichment{ForestPct: 1})
	enrich100 := graph.EncodeEnrichmentTags(&graph.RawEnrichment{ForestPct: 100})

	g1 := testGraph(tags, &enrich1)
	g100 := testGraph(tags, &enrich100)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "hiking"})
	ecf1 := cfg.EdgeCost(g1.EdgeTags1[0], g1.EdgeTags2[0])
	ecf100 := cfg.EdgeCost(g100.EdgeTags1[0], g100.EdgeTags2[0])

	// forestWeight=0.5: almost-no-forest should be ~0.5 more expensive than full-forest
	diff := ecf1.Flat - ecf100.Flat
	if diff < 0.3 || diff > 0.7 {
		t.Errorf("forest bonus diff = %f, expected ~0.5 (no forest=%f, full forest=%f)",
			diff, ecf1.Flat, ecf100.Flat)
	}
}

func TestEdgeCost_NoisePenalty_Additive(t *testing.T) {
	tags := encodeTags("path", "")
	enrich0 := graph.EncodeEnrichmentTags(&graph.RawEnrichment{NoiseScore: 0})
	enrich100 := graph.EncodeEnrichmentTags(&graph.RawEnrichment{NoiseScore: 100})

	g0 := testGraph(tags, &enrich0)
	g100 := testGraph(tags, &enrich100)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "hiking"})
	ecf0 := cfg.EdgeCost(g0.EdgeTags1[0], g0.EdgeTags2[0])
	ecf100 := cfg.EdgeCost(g100.EdgeTags1[0], g100.EdgeTags2[0])

	// noiseWeight=0.4: noise=100 → +0.4*2.0 = +0.8
	diff := ecf100.Flat - ecf0.Flat
	if diff < 0.5 || diff > 1.2 {
		t.Errorf("noise penalty diff = %f, expected ~0.8 (quiet=%f, noisy=%f)",
			diff, ecf0.Flat, ecf100.Flat)
	}
}

func TestEdgeCost_CyclerouteBoost_Lerp(t *testing.T) {
	// Create expensive road edge (secondary=1.6 for trekking)
	tags := encodeTags("secondary", "asphalt")
	enrichCR := graph.EncodeEdgeTags2(&graph.RawTags{}, &graph.RawEnrichment{IsCycleroute: true})
	enrichNone := graph.EncodeEdgeTags2(&graph.RawTags{}, &graph.RawEnrichment{IsCycleroute: false})

	gCR := testGraph(tags, &enrichCR)
	gNone := testGraph(tags, &enrichNone)

	// Use full cycleroute boost = 1.0 → should LERP to 1.0
	cfg, _ := BuildCostConfig(ProfileConfig{
		Profile:         "trekking",
		CyclerouteBoost: new(float32(1.0)),
	})
	ecfCR := cfg.EdgeCost(gCR.EdgeTags1[0], gCR.EdgeTags2[0])
	ecfNone := cfg.EdgeCost(gNone.EdgeTags1[0], gNone.EdgeTags2[0])

	// With boost=1.0, cycleroute CF = 1.0 regardless of baseCF
	if ecfCR.Flat > 1.5 {
		t.Errorf("cycleroute boost=1.0 should LERP to ~1.0, got %f", ecfCR.Flat)
	}
	if ecfNone.Flat < 1.5 {
		t.Errorf("non-cycleroute secondary should be expensive, got %f", ecfNone.Flat)
	}
}

func TestEdgeCost_WetWeather(t *testing.T) {
	tags := encodeTags("path", "mud")
	g := testGraph(tags, nil)

	cfgDry, _ := BuildCostConfig(ProfileConfig{Profile: "hiking"})
	cfgWet, _ := BuildCostConfig(ProfileConfig{
		Profile:    "hiking",
		WetWeather: new(true),
	})
	ecfDry := cfgDry.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])
	ecfWet := cfgWet.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])

	// Wet mud penalty = +2.0 (additional to base mud cost)
	diff := ecfWet.Flat - ecfDry.Flat
	if diff < 1.5 || diff > 2.5 {
		t.Errorf("wet weather mud penalty diff = %f, expected ~2.0 (dry=%f, wet=%f)",
			diff, ecfDry.Flat, ecfWet.Flat)
	}
}

func TestEdgeCost_3Costfactor_Hiking(t *testing.T) {
	tags := encodeTags("path", "mud")
	g := testGraph(tags, nil)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "hiking"})
	ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])

	// Hiking with mud: downMod=+0.5 → Down > Flat
	if ecf.Down <= ecf.Flat {
		t.Errorf("hiking mud: Down (%f) should be > Flat (%f)", ecf.Down, ecf.Flat)
	}
	// Up should also differ (mud upMod=+0.3)
	if ecf.Up <= ecf.Flat {
		t.Errorf("hiking mud: Up (%f) should be > Flat (%f)", ecf.Up, ecf.Flat)
	}
}

func TestEdgeCost_AccessFootOnly_BikePenalty(t *testing.T) {
	// bicycle=no, foot=yes → +4.0 for bike profiles
	tags := encodeTagsFull(&graph.RawTags{
		Highway: "path",
		Bicycle: "no",
		Foot:    "yes",
	})
	g := testGraph(tags, nil)

	cfgBike, _ := BuildCostConfig(ProfileConfig{Profile: "trekking"})
	cfgFoot, _ := BuildCostConfig(ProfileConfig{Profile: "hiking"})

	ecfBike := cfgBike.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])
	ecfFoot := cfgFoot.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])

	// Bike profile should have +4.0 penalty
	if ecfBike.Flat < ecfFoot.Flat+3.0 {
		t.Errorf("bike penalty for foot-only: bike=%f, foot=%f, expected bike >= foot+3",
			ecfBike.Flat, ecfFoot.Flat)
	}
}

func TestEdgeCost_OnewayReverse_BikePenalty(t *testing.T) {
	// primary + oneway + reverse direction
	tags := encodeTagsFull(&graph.RawTags{
		Highway: "primary",
		Oneway:  true,
	})
	// Set reverse bit (bit 61)
	tags |= 1 << 61

	g := testGraph(tags, nil)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "trekking"})
	ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])

	// Without reverse, primary costfactor is 3.0 for trekking
	tagsNoRev := encodeTagsFull(&graph.RawTags{Highway: "primary", Oneway: true})
	gFwd := testGraph(tagsNoRev, nil)
	ecfFwd := cfg.EdgeCost(gFwd.EdgeTags1[0], gFwd.EdgeTags2[0])

	// Reverse penalty for primary = +50
	diff := ecf.Flat - ecfFwd.Flat
	if diff < 40 || diff > 60 {
		t.Errorf("oneway reverse penalty for primary = %f, expected ~50 (rev=%f, fwd=%f)",
			diff, ecf.Flat, ecfFwd.Flat)
	}
}

func TestEdgeCost_EdgeTags2Nil_NoPanic(t *testing.T) {
	tags := encodeTags("path", "")
	g := testGraph(tags, nil) // no EdgeTags2

	for _, profile := range []string{"trekking", "hiking", "mtb", "gravel", "road"} {
		cfg, _ := BuildCostConfig(ProfileConfig{Profile: profile})
		ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0]) // must not panic
		if ecf.Flat <= 0 {
			t.Errorf("%s: expected positive costfactor, got %f", profile, ecf.Flat)
		}
	}
}

func TestDefaultCostConfig_Works(t *testing.T) {
	cfg := DefaultCostConfig()
	if cfg == nil {
		t.Fatal("DefaultCostConfig returned nil")
	}
	if cfg.EdgeCost == nil {
		t.Fatal("EdgeCost closure is nil")
	}
	if cfg.DownhillCost != 60 {
		t.Errorf("DownhillCost = %f, want 60", cfg.DownhillCost)
	}
	if cfg.MaxLatentCost <= 0 {
		t.Errorf("MaxLatentCost should be positive, got %f", cfg.MaxLatentCost)
	}
}

// --- Additional edge case tests ---

func TestEdgeCost_StepsBlocked_ForMtb(t *testing.T) {
	tags := encodeTags("steps", "")
	g := testGraph(tags, nil)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "mtb"})
	ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])
	if ecf.Flat < costBlocked {
		t.Errorf("mtb: steps should be blocked, got %f", ecf.Flat)
	}
}

func TestEdgeCost_StepsAllowed_ForHiking(t *testing.T) {
	tags := encodeTags("steps", "")
	g := testGraph(tags, nil)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "hiking"})
	ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])
	if ecf.Flat >= costBlocked {
		t.Errorf("hiking: steps should be allowed, got %f", ecf.Flat)
	}
}

func TestEdgeCost_Road_UnpavedTrack_Expensive(t *testing.T) {
	tags := encodeTags("track", "gravel")
	g := testGraph(tags, nil)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "road"})
	ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])
	if ecf.Flat < 9.0 {
		t.Errorf("road: unpaved track should be expensive (>=10), got %f", ecf.Flat)
	}
}

func TestEdgeCost_PrivateBlocked_AllProfiles(t *testing.T) {
	tags := encodeTagsFull(&graph.RawTags{Highway: "residential", Access: "private"})
	g := testGraph(tags, nil)

	for _, profile := range []string{"trekking", "hiking", "mtb", "gravel", "road"} {
		cfg, _ := BuildCostConfig(ProfileConfig{Profile: profile})
		ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])
		if ecf.Flat < costBlocked {
			t.Errorf("%s: private access should be blocked, got %f", profile, ecf.Flat)
		}
	}
}

func TestEdgeCost_3CF_NotForRoad(t *testing.T) {
	// Road profile should NOT produce 3CF (Up/Down should be 0)
	tags := encodeTags("tertiary", "asphalt")
	g := testGraph(tags, nil)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "road"})
	ecf := cfg.EdgeCost(g.EdgeTags1[0], g.EdgeTags2[0])
	if ecf.Up != 0 || ecf.Down != 0 {
		t.Errorf("road should not have 3CF: Up=%f, Down=%f", ecf.Up, ecf.Down)
	}
}

func TestBuildCostConfig_CutoffConversion(t *testing.T) {
	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "hiking"})
	// Hiking has UphillCutoffPct=3 → cutoff=0.03
	if math.Abs(float64(cfg.UphillCutoff)-0.03) > 0.001 {
		t.Errorf("UphillCutoff = %f, want 0.03 (3%%)", cfg.UphillCutoff)
	}
}

func TestMergeProfile_HighwayCostOverride(t *testing.T) {
	override := ProfileConfig{
		PrimaryCost:     new(float32(2.0)),
		CyclewayCost:    new(float32(1.5)),
		TrackGrade1Cost: new(float32(0.8)),
	}

	base, err := BuiltinProfile("gravel")
	if err != nil {
		t.Fatalf("BuiltinProfile(gravel): %v", err)
	}
	merged := MergeProfile(base, override)

	if merged.PrimaryCost == nil || *merged.PrimaryCost != 2.0 {
		t.Errorf("PrimaryCost = %v, want 2.0", merged.PrimaryCost)
	}
	if merged.CyclewayCost == nil || *merged.CyclewayCost != 1.5 {
		t.Errorf("CyclewayCost = %v, want 1.5", merged.CyclewayCost)
	}
	// PathCost should remain at base value (1.6 from gravel.json), not overridden
	if merged.PathCost == nil || *merged.PathCost != *base.PathCost {
		t.Errorf("PathCost should be base value %v, got %v", base.PathCost, merged.PathCost)
	}
}

func TestBuildEdgeCostClosure_DefaultIdentity(t *testing.T) {
	for _, name := range []string{"trekking", "hiking", "mtb", "gravel", "road"} {
		t.Run(name, func(t *testing.T) {
			cfgBase, _ := BuildCostConfig(ProfileConfig{Profile: name})

			p := ProfileConfig{Profile: name}
			setIfNotBlocked := func(dst **float32, cost float32) {
				if cost < costBlocked {
					*dst = new(cost)
				}
			}
			setIfNotBlocked(&p.PathCost, profileHighwayCostDefault(name, 16, 0))
			setIfNotBlocked(&p.CyclewayCost, profileHighwayCostDefault(name, 18, 0))
			setIfNotBlocked(&p.PrimaryCost, profileHighwayCostDefault(name, 5, 0))
			setIfNotBlocked(&p.SecondaryCost, profileHighwayCostDefault(name, 7, 0))
			setIfNotBlocked(&p.TertiaryCost, profileHighwayCostDefault(name, 9, 0))
			setIfNotBlocked(&p.ResidentialCost, profileHighwayCostDefault(name, 11, 0))
			setIfNotBlocked(&p.UnclassifiedCost, profileHighwayCostDefault(name, 13, 0))
			setIfNotBlocked(&p.ServiceCost, profileHighwayCostDefault(name, 14, 0))
			setIfNotBlocked(&p.FootwayCost, profileHighwayCostDefault(name, 17, 0))
			setIfNotBlocked(&p.BridlewayCost, profileHighwayCostDefault(name, 19, 0))
			setIfNotBlocked(&p.PedestrianCost, profileHighwayCostDefault(name, 21, 0))
			setIfNotBlocked(&p.TrackCost, profileHighwayCostDefault(name, 15, 0))
			setIfNotBlocked(&p.TrackGrade1Cost, profileHighwayCostDefault(name, 15, 1))
			setIfNotBlocked(&p.TrackGrade2Cost, profileHighwayCostDefault(name, 15, 2))
			setIfNotBlocked(&p.TrackGrade3Cost, profileHighwayCostDefault(name, 15, 3))

			cfgExplicit, _ := BuildCostConfig(p)

			for _, hw := range []string{"path", "track", "cycleway", "primary", "secondary",
				"tertiary", "residential", "service", "unclassified", "footway",
				"bridleway", "pedestrian"} {
				tags1 := encodeTags(hw, "")
				ecfBase := cfgBase.EdgeCost(tags1, 0)
				ecfExpl := cfgExplicit.EdgeCost(tags1, 0)
				if ecfBase.Flat != ecfExpl.Flat {
					t.Errorf("[%s] %s: base.Flat=%.4f explicit.Flat=%.4f",
						name, hw, ecfBase.Flat, ecfExpl.Flat)
				}
			}

			for _, tt := range []string{"grade1", "grade2", "grade3"} {
				tags1 := encodeTagsFull(&graph.RawTags{Highway: "track", Tracktype: tt})
				ecfBase := cfgBase.EdgeCost(tags1, 0)
				ecfExpl := cfgExplicit.EdgeCost(tags1, 0)
				if ecfBase.Flat != ecfExpl.Flat {
					t.Errorf("[%s] track/%s: base.Flat=%.4f explicit.Flat=%.4f",
						name, tt, ecfBase.Flat, ecfExpl.Flat)
				}
			}
		})
	}
}

func TestEdgeCost_TrafficPenalty(t *testing.T) {
	tags := encodeTags("path", "")
	enrich0 := graph.EncodeEnrichmentTags(&graph.RawEnrichment{TrafficClass: 0})
	enrich6 := graph.EncodeEnrichmentTags(&graph.RawEnrichment{TrafficClass: 6})

	g0 := testGraph(tags, &enrich0)
	g6 := testGraph(tags, &enrich6)

	cfg, _ := BuildCostConfig(ProfileConfig{Profile: "hiking"})
	ecf0 := cfg.EdgeCost(g0.EdgeTags1[0], g0.EdgeTags2[0])
	ecf6 := cfg.EdgeCost(g6.EdgeTags1[0], g6.EdgeTags2[0])

	diff := ecf6.Flat - ecf0.Flat
	// trafficWeight=0.6, class 6 → 6/6 * 0.6 = 0.6
	if diff < 0.4 || diff > 0.8 {
		t.Errorf("traffic penalty diff = %f, expected ~0.6", diff)
	}
}

// --- Loader tests ---

func saveAndRestoreBuiltins(t *testing.T) {
	t.Helper()
	saved := loadedBuiltins.Load()
	t.Cleanup(func() { loadedBuiltins.Store(saved) })
}

func TestLoadBuiltinProfiles_FromConfig(t *testing.T) {
	// TestMain already loaded profiles — verify they're all there
	for _, name := range ValidProfileNames {
		p, err := BuiltinProfile(name)
		if err != nil {
			t.Errorf("BuiltinProfile(%q): %v", name, err)
			continue
		}
		if p.Profile != name {
			t.Errorf("BuiltinProfile(%q).Profile = %q", name, p.Profile)
		}
		if p.Pass0Coefficient == nil {
			t.Errorf("BuiltinProfile(%q).Pass0Coefficient is nil", name)
		}
		if p.UphillCost == nil {
			t.Errorf("BuiltinProfile(%q).UphillCost is nil", name)
		}
		if p.AllowSteps == nil {
			t.Errorf("BuiltinProfile(%q).AllowSteps is nil", name)
		}
		if p.Name == "" {
			t.Errorf("BuiltinProfile(%q).Name is empty", name)
		}
		if len(p.SportTypes) == 0 {
			t.Errorf("BuiltinProfile(%q).SportTypes is empty", name)
		}
	}
}

func TestLoadBuiltinProfiles_TempDirOverride(t *testing.T) {
	saveAndRestoreBuiltins(t)

	dir := t.TempDir()
	override := `{
		"profile": "hiking", "name": "Test Hiking",
		"sport_types": ["hiking"],
		"uphill_cost": 99, "downhill_cost": 7,
		"uphill_cutoff_pct": 3, "downhill_cutoff_pct": 3,
		"elev_penalty_buffer_m": 5, "elev_max_buffer_m": 10, "elev_buffer_reduce": 0.01,
		"allow_steps": true, "turn_cost": 0, "pass0_coefficient": 2.5
	}`
	if err := os.WriteFile(filepath.Join(dir, "hiking.json"), []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := LoadBuiltinProfiles(dir)
	if err != nil {
		t.Fatalf("LoadBuiltinProfiles: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}

	p, err := BuiltinProfile("hiking")
	if err != nil {
		t.Fatalf("BuiltinProfile: %v", err)
	}
	if p.UphillCost == nil || *p.UphillCost != 99 {
		t.Errorf("UphillCost = %v, want 99", p.UphillCost)
	}
	if p.Name != "Test Hiking" {
		t.Errorf("Name = %q, want 'Test Hiking'", p.Name)
	}
}

func TestLoadBuiltinProfiles_EmptyDir(t *testing.T) {
	saveAndRestoreBuiltins(t)
	n, err := LoadBuiltinProfiles(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
	_, err = BuiltinProfile("hiking")
	if err == nil {
		t.Error("expected error for unknown profile in empty map")
	}
	if err != nil && strings.Contains(err.Error(), "not loaded") {
		t.Errorf("expected 'unknown profile' error, got 'not loaded': %v", err)
	}
}

func TestLoadBuiltinProfiles_ProfileMismatch(t *testing.T) {
	saveAndRestoreBuiltins(t)
	dir := t.TempDir()
	// Filename is mtb.json but content says profile="hiking"
	bad := `{"profile":"hiking","uphill_cost":0,"downhill_cost":0,"uphill_cutoff_pct":1.5,"downhill_cutoff_pct":1.5,"elev_penalty_buffer_m":5,"elev_max_buffer_m":10,"elev_buffer_reduce":0,"allow_steps":false,"turn_cost":30,"pass0_coefficient":1.5}`
	os.WriteFile(filepath.Join(dir, "mtb.json"), []byte(bad), 0o644)

	n, err := LoadBuiltinProfiles(dir)
	if n != 0 {
		t.Fatalf("expected 0 (mismatch), got %d", n)
	}
	if err == nil {
		t.Error("expected aggregated error")
	}
}

func TestLoadBuiltinProfiles_IncompleteProfile(t *testing.T) {
	saveAndRestoreBuiltins(t)
	dir := t.TempDir()
	// Missing most required fields
	incomplete := `{"profile":"hiking","uphill_cost":7,"downhill_cost":7}`
	os.WriteFile(filepath.Join(dir, "hiking.json"), []byte(incomplete), 0o644)

	n, err := LoadBuiltinProfiles(dir)
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
	if err == nil {
		t.Error("expected error for incomplete profile")
	}
}

func TestBuiltinProfile_NotLoaded(t *testing.T) {
	saveAndRestoreBuiltins(t)
	loadedBuiltins.Store(nil)

	_, err := BuiltinProfile("hiking")
	if err == nil || !strings.Contains(err.Error(), "not loaded") {
		t.Errorf("expected 'not loaded' error, got: %v", err)
	}
}

func TestDefaultSportTypes(t *testing.T) {
	if st := DefaultSportTypes("hiking"); len(st) == 0 {
		t.Error("hiking should have sport types")
	}
	if st := DefaultSportTypes("mtb"); len(st) == 0 {
		t.Error("mtb should have sport types")
	}
	if st := DefaultSportTypes("unknown"); len(st) != 0 {
		t.Errorf("unknown should have empty sport types, got %v", st)
	}
}

func intPtr(v int) *int             { return &v }
func float32Ptr(v float32) *float32 { return &v }

func TestMergeProfile_TrailVisibilityOverrideWins(t *testing.T) {
	base := ProfileConfig{Profile: "hiking", MaxTrailVisibility: intPtr(4), TrailVisibilityPenalty: float32Ptr(0.3)}
	override := ProfileConfig{Profile: "hiking", MaxTrailVisibility: intPtr(3), TrailVisibilityPenalty: float32Ptr(0.8)}
	merged := MergeProfile(base, override)
	if *merged.MaxTrailVisibility != 3 {
		t.Errorf("MaxTrailVisibility: want 3, got %d", *merged.MaxTrailVisibility)
	}
	if *merged.TrailVisibilityPenalty != 0.8 {
		t.Errorf("TrailVisibilityPenalty: want 0.8, got %f", *merged.TrailVisibilityPenalty)
	}
}

func TestMergeProfile_TrailVisibilityNilOverrideKeepsBase(t *testing.T) {
	base := ProfileConfig{Profile: "hiking", MaxTrailVisibility: intPtr(4), TrailVisibilityPenalty: float32Ptr(0.3)}
	override := ProfileConfig{Profile: "hiking"}
	merged := MergeProfile(base, override)
	if *merged.MaxTrailVisibility != 4 {
		t.Errorf("want 4 (base)")
	}
	if *merged.TrailVisibilityPenalty != 0.3 {
		t.Errorf("want 0.3 (base)")
	}
}

func TestMergeProfile_TrailVisibilityBothNilKeepsNil(t *testing.T) {
	base := ProfileConfig{Profile: "hiking"}
	override := ProfileConfig{Profile: "hiking"}
	merged := MergeProfile(base, override)
	if merged.MaxTrailVisibility != nil {
		t.Errorf("MaxTrailVisibility: want nil, got %v", merged.MaxTrailVisibility)
	}
	// Verify Fast-Path (spec §3.2.4): derefInt(nil) == 0 → closure guards skip decode.
	maxTv := derefInt(merged.MaxTrailVisibility)
	tvPenalty := deref32(merged.TrailVisibilityPenalty)
	if maxTv != 0 || tvPenalty != 0.0 {
		t.Errorf("nil-defaults must dereference to 0; got maxTv=%d tvPenalty=%f", maxTv, tvPenalty)
	}
}

func TestOnewayReverseEdgeCostClosure(t *testing.T) {
	// Encode a residential edge with oneway=yes, oneway_bicycle=true
	residentialTags := &graph.RawTags{
		Highway:       "residential",
		Oneway:        true,
		OnewayBicycle: true,
	}
	tags1 := graph.EncodeOSMTags(residentialTags)
	tags2 := graph.EncodeEdgeTags2(residentialTags, &graph.RawEnrichment{})
	reverseTags1 := tags1 | (1 << 61) // reverse-edge marker

	// Hiking profile: reverse edge on vehicle-oneway should be ALLOWED
	hikingCfg, _ := BuildCostConfig(ProfileConfig{Profile: "hiking"})
	hikingCost := hikingCfg.EdgeCost(reverseTags1, tags2)
	if hikingCost.Flat >= costBlocked {
		t.Errorf("hiking profile blocked on reverse vehicle-oneway residential, want allowed")
	}

	// Trekking (bike) profile: reverse edge should be BLOCKED (oneway_bicycle=true)
	trekkingCfg, _ := BuildCostConfig(ProfileConfig{Profile: "trekking"})
	trekkingCost := trekkingCfg.EdgeCost(reverseTags1, tags2)
	if trekkingCost.Flat < costBlocked {
		t.Errorf("trekking profile allowed on reverse vehicle-oneway residential, want blocked")
	}

	// Bike profile with oneway_bicycle=false: should be ALLOWED (with penalty, not blocked)
	residentialTagsNoBike := &graph.RawTags{
		Highway:       "residential",
		Oneway:        true,
		OnewayBicycle: false,
	}
	tags1NoBike := graph.EncodeOSMTags(residentialTagsNoBike)
	reverseTags1NoBike := tags1NoBike | (1 << 61)
	tags2NoBike := graph.EncodeEdgeTags2(residentialTagsNoBike, &graph.RawEnrichment{})
	trekkingCostNoBike := trekkingCfg.EdgeCost(reverseTags1NoBike, tags2NoBike)
	if trekkingCostNoBike.Flat >= costBlocked {
		t.Errorf("trekking profile blocked on reverse residential with oneway_bicycle=false, want allowed (penalized)")
	}
}

// tv2tags packs trail_visibility (0-6) into Tags2 bits 36-38.
func tv2tags(tv uint8) uint64 { return uint64(tv&0x7) << 36 }

// buildWorstCaseTags2 — spec §3.2.6 worst case: noise_score=100, traffic_class=6,
// trail_visibility=5 (horrible). Bit positions verified graph/tags.go:213-224.
func buildWorstCaseTags2() uint64 {
	var t uint64
	t |= uint64(100)     // bits 0-6: noise_score (7-bit, OSM clamp 100)
	t |= uint64(6) << 14 // bits 14-16: traffic_class (max 6)
	t |= uint64(5) << 36 // bits 36-38: trail_visibility=horrible
	return t
}

// mergeWithDefaults builds a merged config using BuiltinProfile. Fatals if builtins unloaded.
func mergeWithDefaults(t *testing.T, p ProfileConfig) ProfileConfig {
	t.Helper()
	base, err := BuiltinProfile(p.Profile)
	if err != nil {
		t.Fatalf("BuiltinProfile(%q): %v", p.Profile, err)
	}
	return MergeProfile(base, p)
}

func TestBuildEdgeCostClosure_TrailVisibilityBlock(t *testing.T) {
	p := mergeWithDefaults(t, ProfileConfig{Profile: "hiking", MaxTrailVisibility: intPtr(3)})
	closure := buildEdgeCostClosure(p)
	tags1 := uint64(16) // highway=path
	if closure(tags1, tv2tags(2)).Flat >= costBlocked {
		t.Errorf("tv=2 ≤ maxTv=3 must not block")
	}
	if closure(tags1, tv2tags(5)).Flat < costBlocked {
		t.Errorf("tv=5 > maxTv=3 must block")
	}
}

func TestBuildEdgeCostClosure_TrailVisibilityPenaltyAdditive(t *testing.T) {
	p := mergeWithDefaults(t, ProfileConfig{Profile: "hiking", TrailVisibilityPenalty: float32Ptr(0.5)})
	closure := buildEdgeCostClosure(p)
	tags1 := uint64(16)
	baseCf := closure(tags1, 0).Flat
	tv3Cf := closure(tags1, tv2tags(3)).Flat
	delta := tv3Cf - baseCf
	if delta < 0.49 || delta > 0.51 {
		t.Errorf("tv=3 expected delta≈0.5; got %f", delta)
	}
}

func TestBuildEdgeCostClosure_TrailVisibilitySeverityScale(t *testing.T) {
	p := mergeWithDefaults(t, ProfileConfig{Profile: "hiking", TrailVisibilityPenalty: float32Ptr(0.5)})
	closure := buildEdgeCostClosure(p)
	tags1 := uint64(16)
	baseCf := closure(tags1, 0).Flat
	tv4Cf := closure(tags1, tv2tags(4)).Flat
	if delta := tv4Cf - baseCf; delta < 0.99 || delta > 1.01 {
		t.Errorf("tv=4 expected delta≈1.0; got %f", delta)
	}
}

func TestBuildEdgeCostClosure_TrailVisibilityUnknownNoChange(t *testing.T) {
	p := mergeWithDefaults(t, ProfileConfig{Profile: "hiking", TrailVisibilityPenalty: float32Ptr(0.5), MaxTrailVisibility: intPtr(3)})
	closure := buildEdgeCostClosure(p)
	tags1 := uint64(16)
	tv0Cf := closure(tags1, tv2tags(0)).Flat
	untagged := closure(tags1, 0).Flat
	if tv0Cf != untagged {
		t.Errorf("tv=0 must equal untagged; %f vs %f", tv0Cf, untagged)
	}
}

func TestBuildEdgeCostClosure_TrailVisibilityFastPathDisabled(t *testing.T) {
	// Both nil → cost identical regardless of Tags2 trail_visibility bits.
	p := mergeWithDefaults(t, ProfileConfig{Profile: "hiking"})
	closure := buildEdgeCostClosure(p)
	tags1 := uint64(16)
	if closure(tags1, tv2tags(4)).Flat != closure(tags1, 0).Flat {
		t.Errorf("TV disabled: cost must not change with tv bits")
	}
}

func TestEdgeCostInteractionCombinatorics(t *testing.T) {
	// Spec §3.2.6: worst-case finalCF ≤ 4× baseCF.
	// Seeds halved vs rev2 (0.4/0.4/0.25) so worst-case stays under the cap:
	//   noiseAdd ≤ 2×0.4=0.8, trafficAdd ≤ 1×0.4=0.4, tvAdd ≤ 3×0.25=0.75 → +1.95
	//   vs baseCF=1.2 (hiking, highway=path, surface=unknown): finalCF=3.15, ratio 2.63 ≤ 4.0.
	p := mergeWithDefaults(t, ProfileConfig{
		Profile:                "hiking",
		NoisePenalty:           float32Ptr(0.4),
		TrafficPenalty:         float32Ptr(0.4),
		TrailVisibilityPenalty: float32Ptr(0.25),
		MaxTrailVisibility:     intPtr(6),
	})
	closure := buildEdgeCostClosure(p)
	tags1 := uint64(16)
	baseCf := closure(tags1, 0).Flat
	worstCf := closure(tags1, buildWorstCaseTags2()).Flat
	if worstCf > 4.0*baseCf {
		t.Errorf("combinatorics: worst=%f > 4×base=%f — seeds or cap revision needed", worstCf, baseCf)
	}
}

// TestEdgeCostInteraction_PerProfileSeeds verifies that all PR2 seed calibrations
// remain below the 4× baseCF cap under worst-case tag combinatorics.
//
// noise_penalty / traffic_penalty are loaded LIVE from BuiltinProfile (config/badrouter/*.json)
// so any JSON change is automatically reflected — no silent drift.
//
// trail_visibility_penalty seeds are intentional literals from spec §3.4 PR2-Seeds-Table:
//   | Profile  | max_trail_visibility | trail_visibility_penalty (seed) |
//   | hiking   | 3                    | 0.5                             |
//   | mtb      | 4                    | 0.4                             |
//   | gravel   | 3                    | 0.8  ← critical case (Opus #7)  |
//   | trekking | 3                    | 0.5                             |
//   | road     | 6 / 0.0 (unchanged)  | —                               |
//
// MaxTrailVisibility is forced to 6 for ALL profiles so tv=5 (horrible, severity=3)
// is never blocked — this isolates the additive-penalty cap from the block gate.
// The block-gate path (tv > maxTv → costBlocked) is tested separately in
// TestEdgeCostInteraction_BlockGate_PerProfile.
//
// Worst-case tags2: noise_score=100, traffic_class=6, trail_visibility=5 (horrible).
// tvSeverity[5]=3, so tvAdd = tvPenalty * 3.
// noiseAdd_max = 2 * noisePenalty, trafficAdd_max = trafficPenalty.
//
// This test is the canary that MUST fail when PR2 lands if any profile exceeds the cap —
// it forces seed recalibration or a hard cap before merge.
func TestEdgeCostInteraction_PerProfileSeeds(t *testing.T) {
	// PR2 target trail_visibility seeds (spec §3.4 — intentional literals, not from JSON yet).
	// noise_penalty / traffic_penalty are loaded live from BuiltinProfile below.
	pr2TvSeeds := map[string]float32{
		"hiking":   0.5,
		"mtb":      0.4,
		"gravel":   0.8, // Opus #7: critical case — verify gravel base avoids cap breach
		"trekking": 0.5,
		"road":     0.0,
	}

	for _, profileName := range []string{"hiking", "mtb", "gravel", "trekking", "road"} {
		profileName := profileName
		t.Run(profileName, func(t *testing.T) {
			// Load noise_penalty / traffic_penalty live from JSON — no silent drift.
			base, err := BuiltinProfile(profileName)
			if err != nil {
				t.Fatalf("BuiltinProfile(%q): %v", profileName, err)
			}

			tvPenalty := pr2TvSeeds[profileName]

			p := mergeWithDefaults(t, ProfileConfig{
				Profile: profileName,
				// Live seeds from JSON (Phase-A values; auto-updated when JSONs change):
				NoisePenalty:   base.NoisePenalty,
				TrafficPenalty: base.TrafficPenalty,
				// PR2 target seeds (intentional literals — block-gate tested separately):
				TrailVisibilityPenalty: float32Ptr(tvPenalty),
				MaxTrailVisibility:     intPtr(6), // open gate: no blocking, pure additive cap check
			})
			closure := buildEdgeCostClosure(p)
			tags1 := uint64(16) // highway=path, no surface, no SAC tag
			baseCf := closure(tags1, 0).Flat
			worstCf := closure(tags1, buildWorstCaseTags2()).Flat
			cap4x := 4.0 * baseCf
			ratio := worstCf / baseCf
			if worstCf > cap4x {
				t.Errorf("profile %q: worst=%f > 4×base=%f (ratio=%.3f) — PR2 seeds exceed cap; "+
					"recalibrate seeds or add hard cap before merge",
					profileName, worstCf, cap4x, ratio)
			}
		})
	}
}

// TestEdgeCostInteraction_BlockGate_PerProfile verifies that the max_trail_visibility gate
// returns costBlocked when tv > maxTv for every profile that has max_trail_visibility > 0.
//
// TODAY (Phase-A): all 5 profiles have max_trail_visibility=0 in config/badrouter/*.json,
// so all sub-tests SKIP — this is expected and correct (null-wirkung, spec §Phase-A).
//
// WHEN PR2 LANDS: profiles get max_trail_visibility=3 (hiking/gravel/trekking) or
// max_trail_visibility=4 (mtb). Those sub-tests will activate automatically and catch
// any regression where tv > maxTv fails to return costBlocked.
func TestEdgeCostInteraction_BlockGate_PerProfile(t *testing.T) {
	for _, profileName := range []string{"hiking", "mtb", "gravel", "trekking", "road"} {
		profileName := profileName
		t.Run(profileName, func(t *testing.T) {
			p, err := BuiltinProfile(profileName)
			if err != nil {
				t.Fatalf("BuiltinProfile(%q): %v", profileName, err)
			}

			maxTv := derefInt(p.MaxTrailVisibility)
			if maxTv == 0 {
				t.Skipf("profile %q has max_trail_visibility=0 (Phase-A null-wirkung); block-gate n/a", profileName)
			}

			// Use the raw builtin profile so the gate is live from JSON.
			closure := buildEdgeCostClosure(p)
			tags1 := uint64(16) // highway=path

			overTv := uint8(maxTv + 1)
			if overTv > 6 {
				t.Skipf("profile %q maxTv=%d already at cap (6); cannot encode tv > maxTv", profileName, maxTv)
			}

			result := closure(tags1, tv2tags(overTv)).Flat
			if result < costBlocked {
				t.Errorf("profile %q: tv=%d > maxTv=%d must return costBlocked (≥%.0f), got %f",
					profileName, overTv, maxTv, float32(costBlocked), result)
			}
		})
	}
}

func TestPhaseA_AllProfilesFastPathActive(t *testing.T) {
	// With JSON seeds 0/0.0, no profile should decode trail_visibility.
	profiles := []string{"hiking", "mtb", "trekking", "gravel", "road"}
	tags1Path := uint64(16)
	for _, name := range profiles {
		base, err := BuiltinProfile(name)
		if err != nil {
			t.Fatalf("BuiltinProfile(%q): %v", name, err)
		}
		closure := buildEdgeCostClosure(base)
		cfNoTv := closure(tags1Path, 0).Flat
		cfWithTv := closure(tags1Path, uint64(5)<<36).Flat // tv=horrible in tags2
		if cfNoTv != cfWithTv {
			t.Errorf("profile %q: Phase-A must ignore trail_visibility — %f vs %f",
				name, cfNoTv, cfWithTv)
		}
	}
}
