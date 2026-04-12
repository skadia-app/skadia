// Package routing — Semantic variant definitions and scoring.
// Each SemanticVariant names a themed way to pick a (node, state)
// pair for a given theme: shortest, calmest, greenest, hilly, balanced.
package routing

import "math"

// SemanticVariant is a named scoring function over Pareto-reachable nodes.
// The Score function returns (score, stateIdx): higher score = better match,
// stateIdx references node.States[] for the specific Pareto state chosen.
// A score of -Inf means "this node is not a candidate for this variant".
type SemanticVariant struct {
	ID            string
	LabelKey      string // i18n key, e.g., "route.variant.shortest"
	FallbackLabel string // English fallback

	Score       func(node ReachableNode, req VariantSelectionContext) (float64, int)
	MinPoolSize int // minimum candidates before this variant is offered
}

// VariantSelectionContext bundles the request-level info needed by Score functions.
type VariantSelectionContext struct {
	TargetDistanceM float64
	TargetAscentM   *float64
	LandscapePrefs  *LandscapePrefsInternal
}

// LandscapePrefsInternal holds integer preference weights (0–10 scale) for
// coverage dimensions used by the balanced scorer.
type LandscapePrefsInternal struct {
	Forest     int
	Water      int
	Protected  int
	LowNoise   int
	LowTraffic int
}

// DefaultVariants is the list of built-in semantic variants.
// MinPoolSize values are empirical — variants with tight scoring needs
// (calmest, greenest, hilly) require at least 3 candidates in the
// reachable pool, while shortest and balanced can work with fewer.
var DefaultVariants = []SemanticVariant{
	{
		ID:            "shortest",
		LabelKey:      "route.variant.shortest",
		FallbackLabel: "shortest",
		Score:         scoreShortest,
		MinPoolSize:   1,
	},
	{
		ID:            "calmest",
		LabelKey:      "route.variant.calmest",
		FallbackLabel: "calmest",
		Score:         scoreCalmest,
		MinPoolSize:   3,
	},
	{
		ID:            "greenest",
		LabelKey:      "route.variant.greenest",
		FallbackLabel: "greenest",
		Score:         scoreGreenest,
		MinPoolSize:   3,
	},
	{
		ID:            "hilly",
		LabelKey:      "route.variant.hilly",
		FallbackLabel: "hilly",
		Score:         scoreHilly,
		MinPoolSize:   3,
	},
	{
		ID:            "balanced",
		LabelKey:      "route.variant.balanced",
		FallbackLabel: "balanced",
		Score:         scoreBalanced,
		MinPoolSize:   1,
	},
}

func scoreShortest(node ReachableNode, req VariantSelectionContext) (float64, int) {
	target := req.TargetDistanceM / 2.0
	if target <= 0 {
		return math.Inf(-1), -1
	}
	bestScore := math.Inf(-1)
	bestIdx := -1
	for i, s := range node.States {
		proximity := 1.0 - math.Min(1.0, math.Abs(float64(s.DistanceM)-target)/target)
		if proximity > bestScore {
			bestScore = proximity
			bestIdx = i
		}
	}
	return bestScore, bestIdx
}

func scoreCalmest(node ReachableNode, req VariantSelectionContext) (float64, int) {
	bestScore := math.Inf(-1)
	bestIdx := -1
	for i, s := range node.States {
		quality := 10.0 - math.Min(10.0, float64(s.AvgFlat))
		traffic := 1.0 - float64(s.TrafficClass)
		noise := 1.0 - float64(s.NoiseScore)
		score := quality*0.5 + traffic*0.25 + noise*0.25
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	return bestScore, bestIdx
}

func scoreGreenest(node ReachableNode, req VariantSelectionContext) (float64, int) {
	bestScore := math.Inf(-1)
	bestIdx := -1
	for i, s := range node.States {
		forest := float64(s.ForestPct)
		river := float64(s.RiverProximity)
		protected := float64(s.ProtectedArea)
		score := forest*0.5 + river*0.25 + protected*0.25
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	return bestScore, bestIdx
}

func scoreHilly(node ReachableNode, req VariantSelectionContext) (float64, int) {
	bestScore := math.Inf(-1)
	bestIdx := -1

	if req.TargetAscentM != nil {
		target := *req.TargetAscentM
		for i, s := range node.States {
			proximity := 1.0 - math.Min(1.0, math.Abs(float64(s.Ascent)-target)/(target+100))
			if proximity > bestScore {
				bestScore = proximity
				bestIdx = i
			}
		}
	} else {
		for i, s := range node.States {
			if float64(s.Ascent) > bestScore {
				bestScore = float64(s.Ascent)
				bestIdx = i
			}
		}
	}
	return bestScore, bestIdx
}

func scoreBalanced(node ReachableNode, req VariantSelectionContext) (float64, int) {
	targetDist := req.TargetDistanceM / 2.0
	if targetDist <= 0 {
		return math.Inf(-1), -1
	}

	bestScore := math.Inf(-1)
	bestIdx := -1
	for i, s := range node.States {
		distProx := 1.0 - math.Min(1.0, math.Abs(float64(s.DistanceM)-targetDist)/targetDist)
		quality := 10.0 - math.Min(10.0, float64(s.AvgFlat))

		coverageScore := 0.0
		if req.LandscapePrefs != nil {
			lp := req.LandscapePrefs
			coverageScore += float64(lp.Forest) / 10.0 * float64(s.ForestPct)
			coverageScore += float64(lp.Water) / 10.0 * float64(s.RiverProximity)
			coverageScore += float64(lp.Protected) / 10.0 * float64(s.ProtectedArea)
			coverageScore += float64(lp.LowNoise) / 10.0 * (1.0 - float64(s.NoiseScore))
			coverageScore += float64(lp.LowTraffic) / 10.0 * (1.0 - float64(s.TrafficClass))
		}

		score := distProx*0.4 + quality*0.3 + coverageScore*0.3
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	return bestScore, bestIdx
}
