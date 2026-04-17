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
				pool := generateSyntheticPool(region, targetM, cov == "full_coverage")
				coverageAvailable := cov == "full_coverage"

				labeled := assignLabels(pool, []int{0, 1, 2}, targetM, coverageAvailable)

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

// generateSyntheticPool creates 3 RouteCandidates for a region archetype.
// Route variance: [short, medium, long] relative to target distance.
func generateSyntheticPool(region string, targetM float64, hasCoverage bool) []RouteCandidate {
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
			NodeID:    uint64(i + 1),
			DistanceM: distances[i],
			AscentM:   ascents[i],
			DescentM:  ascents[i],
			AvgFlat:   1.5,
			// Introduce some variance between routes so descriptors can fire:
			MergedForestPct:      forestBase + float32(i)*0.05,
			MergedRiverProximity: riverBase + float32(i)*0.03,
			MergedProtectedArea:  protectedBase + float32(i)*0.08,
			MergedNoiseScore:     noiseBase * (1.0 - float32(i)*0.1),
			MergedTrafficClass:   trafficBase * (1.0 - float32(i)*0.1),
			HasCoverage:          hasCoverage,
		}
	}
	return pool
}
