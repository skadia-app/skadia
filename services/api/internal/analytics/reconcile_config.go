package analytics

import (
	"os"
	"strconv"
	"time"
)

// ReconcileConfig holds all tuning parameters for the readiness reconcile job.
// Load once at startup via LoadReconcileConfig and pass it through; do not
// re-read env vars on every tick.
type ReconcileConfig struct {
	// PublishDelay is the pause between consecutive NATS publishes to avoid
	// flooding the broker. 0 disables the inter-publish sleep.
	// Default: 10ms (~100 msg/s). Override: RECONCILE_PUBLISH_DELAY_MS (≥0).
	PublishDelay time.Duration

	// MaxPerRun caps the number of invalidations per job tick. Users beyond
	// the cap are processed on the next tick.
	// Default: 1000. Override: RECONCILE_MAX_PER_RUN (>0).
	MaxPerRun int

	// InitialDelay is how long the scheduler waits before the first run.
	// Default: 5 min. Override: RECONCILE_INITIAL_DELAY_SECONDS (≥0).
	InitialDelay time.Duration

	// TickInterval is the time between successive runs.
	// Default: 24 h. Override: RECONCILE_TICK_INTERVAL_HOURS (>0).
	TickInterval time.Duration
}

// LoadReconcileConfig reads all reconcile env vars exactly once and returns a
// validated ReconcileConfig. Invalid or out-of-range values fall back to the
// documented defaults.
func LoadReconcileConfig() ReconcileConfig {
	cfg := ReconcileConfig{
		PublishDelay: 10 * time.Millisecond,
		MaxPerRun:    1000,
		InitialDelay: 5 * time.Minute,
		TickInterval: 24 * time.Hour,
	}

	if raw := os.Getenv("RECONCILE_PUBLISH_DELAY_MS"); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms >= 0 {
			cfg.PublishDelay = time.Duration(ms) * time.Millisecond
		}
	}

	if raw := os.Getenv("RECONCILE_MAX_PER_RUN"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			cfg.MaxPerRun = n
		}
	}

	if raw := os.Getenv("RECONCILE_INITIAL_DELAY_SECONDS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			cfg.InitialDelay = time.Duration(n) * time.Second
		}
	}

	if raw := os.Getenv("RECONCILE_TICK_INTERVAL_HOURS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			cfg.TickInterval = time.Duration(n) * time.Hour
		}
	}

	return cfg
}
