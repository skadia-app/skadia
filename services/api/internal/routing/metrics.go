package routing

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ParetoFrontSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "skadia_routing_pareto_front_size",
		Help:    "Number of active Pareto states per node after insertion.",
		Buckets: []float64{1, 2, 3, 4, 5, 6, 8, 12, 20, 50},
	})

	ReconstructionFailureCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "skadia_routing_reconstruction_failures_total",
		Help: "Count of path reconstruction failures.",
	})

	PenaltyRerunCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skadia_routing_penalty_rerun_total",
		Help: "Penalty reruns triggered by the round-trip overlap check.",
	}, []string{"outcome"})

	ParetoTruncatedCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "skadia_routing_pareto_truncated_total",
		Help: "Count of findReachable runs that hit a cap.",
	}, []string{"reason"})

	SuggestRoutesDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "skadia_routing_suggest_routes_duration_seconds",
		Help:    "End-to-end SuggestRoutes handler duration.",
		Buckets: prometheus.ExponentialBuckets(0.5, 1.5, 10),
	}, []string{"outcome"})
)
