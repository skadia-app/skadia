package handler

import (
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// RouteCandidate is a fully-constructed round-trip with merged metrics.
type RouteCandidate struct {
	NodeID uint64
	ViaLat float64
	ViaLon float64

	Leg1   *routing.RouteResult
	Leg2   *routing.RouteResult
	Merged *routing.RouteResult

	DistanceM float64
	AscentM   float64
	DescentM  float64
	AvgFlat   float32

	// Distance-weighted averages across merged route.
	// 0 means "either no coverage data OR actually zero value" — see HasCoverage.
	MergedForestPct      float32
	MergedRiverProximity float32
	MergedProtectedArea  float32
	MergedNoiseScore     float32
	MergedTrafficClass   float32

	// HasCoverage is true iff at least one coverage layer (forest, water,
	// protected area, noise, or traffic) returned a real measurement for
	// this route. False means all Merged* values are zero only because no
	// coverage data exists — not because the route scored zero on merit.
	HasCoverage bool

	LoopQuality LoopQuality
	OverlapPct  float64

	// F26: Outdoor-descriptor aggregation over merged.EdgeHandles.
	// MaxMtbScale/MaxSacScale/MaxViaFerrataScale hold per-route extrema.
	// MaxMtbScale is the composite: max(mtb_scale, mtb:scale:uphill) per edge,
	// then max across the whole route. See aggregateLegOutdoor below.
	MaxMtbScale          uint8   `json:"-"`
	MaxSacScale          uint8   `json:"-"`
	MaxViaFerrataScale   uint8   `json:"-"`
	HasViaFerrataHighway bool    `json:"-"`
	MaxTrailVisibility   uint8   `json:"-"`
	IntermediatePctTv    float32 `json:"-"`

	TaggedPctMtb float32 `json:"-"`
	TaggedPctSac float32 `json:"-"`
	TaggedPctVia float32 `json:"-"`
	TaggedPctTv  float32 `json:"-"`
}

// VariantDescriptor describes a single distinctive feature of a route.
type VariantDescriptor struct {
	Key          string  `json:"key"`
	Magnitude    float64 `json:"magnitude"`
	DisplayValue string  `json:"display_value,omitempty"`
}

// LabeledRoute is a RouteCandidate plus its assigned variant_id and descriptors.
//
// Produced by assignLabels in the V2 route-suggestion pipeline.
type LabeledRoute struct {
	Candidate       RouteCandidate
	VariantID       string
	VariantLabelKey string
	Descriptors     []VariantDescriptor
}

// leg2Coverage holds distance-weighted coverage sums (and averages after
// aggregateLeg2Coverage normalises them by DistM).
type leg2Coverage struct {
	DistM     float32
	Forest    float32
	Noise     float32
	Traffic   float32
	Protected float32
	River     float32
	HasAnyCov bool // true if at least one edge has non-zero tags2
}

// aggregateLeg2Coverage iterates edge handles along Leg 2, summing
// distance-weighted coverage values. Returns averages (sum / DistM).
func aggregateLeg2Coverage(handles []uint64, reader routing.GraphReader) leg2Coverage {
	var agg leg2Coverage
	if len(handles) == 0 {
		return agg
	}
	for _, h := range handles {
		tags2 := reader.EdgeTags2(h)
		distCm := reader.EdgeDistanceCm(h)
		if distCm == 0 {
			continue
		}
		distM := float32(distCm) / 100.0

		// Defensive clamp: ForestPct is 7-bit (0..127); /100 could exceed 1.0.
		forestPct := float32(min(int(graph.DecodeForestPct(tags2)), 100)) / 100.0
		noiseScore := float32(graph.DecodeNoiseScore(tags2)) / 100.0
		trafficClass := float32(graph.DecodeTrafficClass(tags2)) / 6.0 // raw 0–7 (3 bits); divided by 6 for consistency with reachable.go:162 — may exceed 1.0 (inherited scale bug, see plan §M-rev3/9)
		protected := float32(graph.DecodeProtectedArea(tags2)) / 3.0
		river := float32(graph.DecodeRiverProximity(tags2)) / 7.0

		agg.DistM += distM
		agg.Forest += forestPct * distM
		agg.Noise += noiseScore * distM
		agg.Traffic += trafficClass * distM
		agg.Protected += protected * distM
		agg.River += river * distM
		if graph.DecodeForestPct(tags2) > 0 ||
			graph.DecodeNoiseScore(tags2) > 0 ||
			graph.DecodeTrafficClass(tags2) > 0 ||
			graph.DecodeProtectedArea(tags2) > 0 ||
			graph.DecodeRiverProximity(tags2) > 0 {
			agg.HasAnyCov = true
		}
	}
	if agg.DistM > 0 {
		agg.Forest /= agg.DistM
		agg.Noise /= agg.DistM
		agg.Traffic /= agg.DistM
		agg.Protected /= agg.DistM
		agg.River /= agg.DistM
	}
	return agg
}

// legOutdoor holds aggregated outdoor-tag metrics. Populated by aggregateLegOutdoor.
type legOutdoor struct {
	DistM              float32
	TaggedDistMtb      float32
	TaggedDistSac      float32
	TaggedDistVia      float32
	TaggedDistTv       float32
	MaxMtbScale        uint8 // max(mtb_scale, mtb:scale:uphill)
	MaxSacScale        uint8
	MaxViaFerrataScale uint8
	HasViaFerrataHighway bool
	MaxTrailVisibility uint8
	IntermediateDistTv float32 // sum over edges where tv == 3
}

// aggregateLegOutdoor walks merged edge handles, decodes outdoor tags from
// Tags1 + Tags2, tracks max values and tagged distances (meters).
func aggregateLegOutdoor(handles []uint64, reader routing.GraphReader) legOutdoor {
	var out legOutdoor
	for _, h := range handles {
		tags1 := reader.EdgeTags1(h)
		tags2 := reader.EdgeTags2(h)
		distCm := reader.EdgeDistanceCm(h)
		distM := float32(distCm) / 100.0
		out.DistM += distM

		hw := graph.DecodeHighway(tags1)
		sac := uint8(graph.DecodeField(tags1, 14, 3))
		mtb := uint8(graph.DecodeField(tags1, 17, 3))
		mtbUp := graph.DecodeMtbScaleUphill(tags2)
		vfScale := graph.DecodeViaFerrataScale(tags2)
		tv := graph.DecodeTrailVisibility(tags2)

		effMtb := mtb
		if mtbUp > effMtb {
			effMtb = mtbUp
		}

		if sac > 0 && distM > 0 {
			out.TaggedDistSac += distM
			if sac > out.MaxSacScale {
				out.MaxSacScale = sac
			}
		}
		if effMtb > 0 && distM > 0 {
			out.TaggedDistMtb += distM
			if effMtb > out.MaxMtbScale {
				out.MaxMtbScale = effMtb
			}
		}
		if (hw == 22 || vfScale > 0) && distM > 0 {
			out.TaggedDistVia += distM
			if hw == 22 {
				out.HasViaFerrataHighway = true
			}
			if vfScale > out.MaxViaFerrataScale {
				out.MaxViaFerrataScale = vfScale
			}
		}
		if tv > 0 && distM > 0 {
			out.TaggedDistTv += distM
			if tv > out.MaxTrailVisibility {
				out.MaxTrailVisibility = tv
			}
			if tv == 3 {
				out.IntermediateDistTv += distM
			}
		}
	}
	return out
}

// mergeCoverage returns the distance-weighted average of two coverage values
// from two legs of different lengths.
func mergeCoverage(l1Avg, l1Dist, l2Avg, l2Dist float32) float32 {
	total := l1Dist + l2Dist
	if total == 0 {
		return 0
	}
	return (l1Avg*l1Dist + l2Avg*l2Dist) / total
}
