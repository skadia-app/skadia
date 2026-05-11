package gearmetrics

import "testing"

func TestMetricKeysExposeDescent(t *testing.T) {
	if !IsUsageMetric("elevation_loss_m") {
		t.Fatalf("elevation_loss_m must be accepted as a usage metric")
	}
	if IsUsageMetric("DROP TABLE activities") {
		t.Fatalf("unknown metric must not be accepted")
	}
}

func TestActivityAggregateExprsAreWhitelisted(t *testing.T) {
	expr, ok := ActivityAggregateExpr("elevation_loss_m", "a")
	if !ok {
		t.Fatalf("elevation_loss_m aggregate missing")
	}
	if expr != "COALESCE(SUM(COALESCE(a.elevation_loss_m, 0)), 0)" {
		t.Fatalf("unexpected expr: %s", expr)
	}
	if _, ok := ActivityAggregateExpr("elevation_loss_m", "a;DROP TABLE activities"); ok {
		t.Fatalf("unsafe alias must be rejected")
	}
}

func TestLegacySwapIntervalMapsToElevationGain(t *testing.T) {
	key, ok := LegacyIntervalMetric("swap_interval_hm")
	if !ok || key != MetricElevationGainM {
		t.Fatalf("legacy hm interval must map to elevation gain, got %q ok=%v", key, ok)
	}
}

func TestElapsedDaysIsServiceMetricButNotActivityAggregate(t *testing.T) {
	if !IsServiceMetric("elapsed_days") {
		t.Fatalf("elapsed_days must remain valid for service intervals")
	}
	if _, ok := ActivityAggregateExpr("elapsed_days", "a"); ok {
		t.Fatalf("elapsed_days is date-based and must not be an activity SUM aggregate")
	}
}
