package handler

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	routingMetricCandidatesBuiltTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skadia_routing_candidates_built_total",
			Help: "Total route candidates built during variant generation, labeled by status.",
		},
		[]string{"status"},
	)
	routingMetricDescriptorAssigned = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skadia_routing_descriptor_assigned_total",
			Help: "Total variant descriptors assigned, labeled by descriptor key.",
		},
		[]string{"key"},
	)
	routingMetricDescriptorCoverageGated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skadia_routing_descriptor_coverage_gated_total",
			Help: "F26 outdoor descriptor fires suppressed by tag-coverage gate.",
		},
		[]string{"key"},
	)
	routingMetricOutdoorCoverageAvailable = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skadia_routing_outdoor_coverage_available_total",
			Help: "V2 SuggestRoutes requests grouped by any-candidate outdoor-coverage availability. Emitted only from suggestRoutesV2Tail (V1 path has no outdoor aggregation — not a data gap, V1 predates F26).",
		},
		[]string{"available"},
	)
	routingMetricDiversityPoolSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "skadia_routing_diversity_pool_size",
			Help:    "Distribution of candidate pool sizes.",
			Buckets: []float64{1, 3, 5, 7},
		},
	)
	routingMetricCoverageAvailable = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "skadia_routing_candidate_coverage_available",
			Help: "Count of route-suggestion requests by coverage availability.",
		},
		[]string{"available"},
	)
)
