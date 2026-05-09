package database

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// systemModifierTagSyncMetrics holds the Prometheus instruments for the
// WithSystemModifierTagSyncSuspended helper. An isolated instance can be
// injected for test isolation; production code uses the package default.
type systemModifierTagSyncMetrics struct {
	resyncCallsTotal         *prometheus.CounterVec
	resyncActivitiesProcessed prometheus.Histogram
	skipGateActiveSeconds    prometheus.Histogram
	repairsTotal             *prometheus.CounterVec
}

func newSystemModifierTagSyncMetrics(reg prometheus.Registerer) *systemModifierTagSyncMetrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	m := &systemModifierTagSyncMetrics{
		resyncCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "skadia_system_modifier_tag_sync_resync_calls_total",
			Help: "Total successful WithSystemModifierTagSyncSuspended calls, labeled by caller name.",
		}, []string{"caller"}),

		resyncActivitiesProcessed: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "skadia_system_modifier_tag_sync_resync_activities_processed",
			Help:    "Distribution of deduped activity ID counts passed to the resync function per call.",
			Buckets: []float64{1, 10, 100, 1000, 10000},
		}),

		skipGateActiveSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "skadia_system_modifier_tag_sync_skip_gate_active_seconds",
			Help:    "Duration the skadia.skip_system_modifier_tag_sync flag is active (SET LOCAL to commit/rollback).",
			Buckets: prometheus.DefBuckets,
		}),

		repairsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "skadia_system_modifier_tag_sync_repairs_total",
			Help: "Total individual repair operations performed by skadia_resync_system_modifier_tags, labeled by kind.",
		}, []string{"kind"}),
	}
	mustRegister(reg, m.resyncCallsTotal)
	mustRegister(reg, m.resyncActivitiesProcessed)
	mustRegister(reg, m.skipGateActiveSeconds)
	mustRegister(reg, m.repairsTotal)
	return m
}

// mustRegister registers c; if the collector is already registered, reuses
// the existing collector (same metric, different call site). Panics on any
// other error.
func mustRegister(reg prometheus.Registerer, c prometheus.Collector) {
	if err := reg.Register(c); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			// The existing collector is the same metric — reuse it.
			_ = are
			return
		}
		panic(err)
	}
}

var defaultSyncMetrics = sync.OnceValue(func() *systemModifierTagSyncMetrics {
	return newSystemModifierTagSyncMetrics(prometheus.DefaultRegisterer)
})
