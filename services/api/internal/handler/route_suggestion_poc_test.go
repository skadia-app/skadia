//go:build poc
// +build poc

package handler

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"testing"
)

// TestPOC_DescriptorCalibration is a SMOKE test for the descriptor
// assignment logic using synthetic, hand-tuned candidate pools.
//
// LIMITATION: The synthetic fixtures are constructed to exercise every
// descriptor branch (monotonic ascent, monotonic forest, etc.), so the
// resulting "none"-rate is an artifact of the fixture design — NOT an
// empirical measurement of real-world descriptor assignment rates.
//
// Real threshold calibration requires re-running this POC against
// assignment counts from live route-suggestion traffic AFTER a fresh
// tile + coverage rebuild (current tiles are 12+ days old).
//
// Acceptance criterion in current form (global ≤ 50%, per-region ≤ 80%)
// verifies only that the code paths are reachable — not that the
// thresholds are correctly tuned for production data.
func TestPOC_DescriptorCalibration(t *testing.T) {
	regions := []string{"flatland", "hills", "alpine", "urban", "coastal"}
	targetDistances := []float64{5000, 10000, 15000, 20000, 30000}
	coverageSituations := []string{"no_coverage", "full_coverage"}

	type calibRow struct {
		region, coverage, descriptorKey string
		distanceKm                      float64
		assignmentCount                 int
		totalRoutes                     int
	}
	aggregated := make(map[string]*calibRow)

	incRow := func(key, region, cov, descKey string, distKm float64, total int) {
		if r, ok := aggregated[key]; ok {
			r.assignmentCount++
		} else {
			aggregated[key] = &calibRow{
				region: region, coverage: cov, descriptorKey: descKey,
				distanceKm: distKm, assignmentCount: 1, totalRoutes: total,
			}
		}
	}

	for _, region := range regions {
		for _, targetM := range targetDistances {
			for _, cov := range coverageSituations {
				// Generate 3 synthetic RouteCandidates per combo.
				pool := generateSyntheticPoolFull(region, targetM, cov == "full_coverage", false)
				coverageAvailable := cov == "full_coverage"

				labeled := assignLabels(pool, []int{0, 1, 2}, targetM, coverageAvailable, "hiking")

				// Count descriptors per key (including "none" for routes with zero descriptors).
				for _, lr := range labeled {
					if len(lr.Descriptors) == 0 {
						key := fmt.Sprintf("%s|%.0f|%s|none", region, targetM/1000, cov)
						incRow(key, region, cov, "none", targetM/1000, 3)
					} else {
						for _, d := range lr.Descriptors {
							key := fmt.Sprintf("%s|%.0f|%s|%s", region, targetM/1000, cov, d.Key)
							incRow(key, region, cov, d.Key, targetM/1000, 3)
						}
					}
				}
			}
		}
	}

	// Write CSV
	f, err := os.Create("/tmp/variant-threshold-calibration.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"region", "distance_km", "coverage", "descriptor_key", "assignment_count", "assignment_rate"})
	for _, r := range aggregated {
		rate := float64(r.assignmentCount) / float64(r.totalRoutes)
		_ = w.Write([]string{
			r.region, fmt.Sprintf("%.0f", r.distanceKm), r.coverage, r.descriptorKey,
			strconv.Itoa(r.assignmentCount),
			fmt.Sprintf("%.2f", rate),
		})
	}
	w.Flush()

	// Verify acceptance criteria
	globalNone, globalTotal := 0, 0
	perRegionNone := make(map[string]int)
	perRegionTotal := make(map[string]int)
	for _, r := range aggregated {
		perRegionTotal[r.region] += r.totalRoutes
		globalTotal += r.totalRoutes
		if r.descriptorKey == "none" {
			globalNone += r.assignmentCount
			perRegionNone[r.region] += r.assignmentCount
		}
	}
	globalRate := float64(globalNone) / float64(globalTotal)
	t.Logf("Global none-rate: %.2f (threshold 0.50)", globalRate)
	if globalRate > 0.50 {
		t.Errorf("Global none-rate %.2f exceeds 0.50 threshold — thresholds need tuning", globalRate)
	}
	for region, n := range perRegionNone {
		rate := float64(n) / float64(perRegionTotal[region])
		t.Logf("Region %s none-rate: %.2f (threshold 0.80)", region, rate)
		if rate > 0.80 {
			t.Errorf("Region %s none-rate %.2f exceeds 0.80 threshold", region, rate)
		}
	}
	t.Logf("CSV written to /tmp/variant-threshold-calibration.csv")
}

// generateSyntheticPool creates 3 RouteCandidates for a region archetype using
// fixed defaults (targetM=10000, hasCoverage=true) and outdoor tag population.
func generateSyntheticPool(region string) []RouteCandidate {
	return generateSyntheticPoolFull(region, 10000.0, true, true)
}

// generateSyntheticPoolFull creates 3 RouteCandidates for a region archetype.
// Route variance: [short, medium, long] relative to target distance.
// withOutdoor populates region-specific outdoor tags (SAC scale, MTB scale,
// trail visibility, via ferrata) in the same pass — avoiding a second switch
// on the caller side (Opus Finding #19).
func generateSyntheticPoolFull(region string, targetM float64, hasCoverage bool, withOutdoor bool) []RouteCandidate {
	// Region-specific base profile
	var baseAscent float64
	var forestBase, noiseBase, trafficBase, protectedBase, riverBase float32
	switch region {
	case "flatland":
		baseAscent, forestBase, noiseBase, trafficBase, protectedBase, riverBase = 50, 0.30, 0.15, 0.20, 0.10, 0.05
	case "hills":
		baseAscent, forestBase, noiseBase, trafficBase, protectedBase, riverBase = 250, 0.45, 0.20, 0.25, 0.15, 0.08
	case "alpine":
		baseAscent, forestBase, noiseBase, trafficBase, protectedBase, riverBase = 600, 0.60, 0.10, 0.15, 0.40, 0.10
	case "urban":
		baseAscent, forestBase, noiseBase, trafficBase, protectedBase, riverBase = 80, 0.15, 0.55, 0.60, 0.05, 0.08
	case "coastal":
		baseAscent, forestBase, noiseBase, trafficBase, protectedBase, riverBase = 120, 0.35, 0.25, 0.30, 0.20, 0.50
	}

	scale := targetM / 10000.0
	baseAscent *= scale

	distances := []float64{targetM * 0.85, targetM * 1.00, targetM * 1.20}
	ascents := []float64{baseAscent * 0.7, baseAscent * 1.0, baseAscent * 1.5}

	pool := make([]RouteCandidate, 3)
	for i := 0; i < 3; i++ {
		pool[i] = RouteCandidate{
			NodeID:      uint64(i + 1),
			DistanceM:   distances[i],
			AscentM:     ascents[i],
			DescentM:    ascents[i],
			AvgFlat:     1.5,
			HasCoverage: hasCoverage,
		}
		if hasCoverage {
			// Introduce some variance between routes so descriptors can fire:
			pool[i].MergedForestPct      = forestBase + float32(i)*0.05
			pool[i].MergedRiverProximity = riverBase + float32(i)*0.03
			pool[i].MergedProtectedArea  = protectedBase + float32(i)*0.08
			pool[i].MergedNoiseScore     = noiseBase * (1.0 - float32(i)*0.1)
			pool[i].MergedTrafficClass   = trafficBase * (1.0 - float32(i)*0.1)
		}
		// else: all Merged* remain 0 (= semantic "no coverage data")
	}

	if withOutdoor {
		switch region {
		case "alpine":
			for i := range pool {
				pool[i].MaxSacScale = uint8(3 + i%3)
				pool[i].MaxMtbScale = uint8(2 + i%2)
				pool[i].MaxTrailVisibility = uint8(2 + i%3)
				pool[i].TaggedPctSac = 0.40 + float32(i)*0.05
				pool[i].TaggedPctTv = 0.30 + float32(i)*0.05
				pool[i].TaggedPctMtb = 0.25
				if i == 2 {
					pool[i].HasViaFerrataHighway = true
					pool[i].MaxViaFerrataScale = 2
					pool[i].TaggedPctVia = 0.02
				}
			}
		case "hills":
			for i := range pool {
				pool[i].MaxSacScale = uint8(1 + i%2)
				pool[i].MaxMtbScale = uint8(1 + i)
				pool[i].MaxTrailVisibility = uint8(1 + i%3)
				pool[i].TaggedPctSac = 0.15 + float32(i)*0.05
				pool[i].TaggedPctTv = 0.15 + float32(i)*0.05
				pool[i].TaggedPctMtb = 0.10 + float32(i)*0.05
			}
		case "urban", "flatland":
			for i := range pool {
				pool[i].TaggedPctSac = 0.05
				pool[i].TaggedPctTv = 0.05
				pool[i].TaggedPctMtb = 0.05
			}
		case "coastal":
			for i := range pool {
				pool[i].MaxSacScale = uint8(i % 2)
				pool[i].MaxTrailVisibility = uint8(1 + i%2)
				pool[i].TaggedPctSac = 0.20
				pool[i].TaggedPctTv = 0.25
				if i == 1 {
					pool[i].HasViaFerrataHighway = true
					pool[i].TaggedPctVia = 0.03
				}
			}
		}
	}

	return pool
}

func TestPOC_OutdoorDescriptors_Acceptance(t *testing.T) {
	regions := []string{"alpine", "hills", "urban", "flatland", "coastal"}
	var alpineExposed, flatlandExposed, alpineAlpineFires int
	var alpineHillsMtbFires, alpineHillsCandidates int
	var urbanFlatlandTotal, urbanFlatlandDescriptors int

	for _, region := range regions {
		pool := generateSyntheticPool(region)
		for i, cand := range pool {
			others := withoutIdx(pool, i)
			ds := computeDescriptorsFor(cand, others, 0, false, defaultOutdoorThresholds)
			switch region {
			case "alpine", "hills":
				alpineHillsCandidates++
				if hasDescriptor(ds, "mtb_technical") {
					alpineHillsMtbFires++
				}
				if region == "alpine" {
					if hasDescriptor(ds, "alpine_hiking") {
						alpineAlpineFires++
					}
					if hasDescriptor(ds, "exposed") {
						alpineExposed++
					}
				}
			case "urban", "flatland":
				urbanFlatlandTotal++
				urbanFlatlandDescriptors += len(ds)
				if hasDescriptor(ds, "exposed") {
					flatlandExposed++
				}
			}
		}
	}

	if alpineHillsCandidates > 0 && float64(alpineHillsMtbFires)/float64(alpineHillsCandidates) < 0.30 {
		t.Errorf("mtb_technical too rare in alpine+hills: %d/%d", alpineHillsMtbFires, alpineHillsCandidates)
	}
	if alpineAlpineFires == 0 {
		t.Errorf("alpine_hiking must fire ≥ 1 in alpine")
	}
	if flatlandExposed > 0 {
		t.Errorf("exposed must not fire in flatland/urban: %d", flatlandExposed)
	}
	if alpineExposed == 0 {
		t.Errorf("exposed must fire ≥ 1 in alpine")
	}
	// Descriptor-free rate for flatland/urban — expect near-zero fires due to coverage gate.
	if urbanFlatlandTotal > 0 {
		avg := float64(urbanFlatlandDescriptors) / float64(urbanFlatlandTotal)
		if avg > 0.8 {
			t.Errorf("urban/flatland: too many descriptors per candidate (avg %.2f)", avg)
		}
	}
}

func withoutIdx(pool []RouteCandidate, idx int) []RouteCandidate {
	out := make([]RouteCandidate, 0, len(pool)-1)
	for i, c := range pool {
		if i != idx {
			out = append(out, c)
		}
	}
	return out
}
