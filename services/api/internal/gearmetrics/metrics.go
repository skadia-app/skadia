package gearmetrics

type UsageMetric string

const (
	MetricDistanceM      UsageMetric = "distance_m"
	MetricMovingTimeS    UsageMetric = "moving_time_s"
	MetricElapsedTimeS   UsageMetric = "elapsed_time_s"
	MetricActivityCount  UsageMetric = "activity_count"
	MetricElapsedDays    UsageMetric = "elapsed_days"
	MetricElevationGainM UsageMetric = "elevation_gain_m"
	MetricElevationLossM UsageMetric = "elevation_loss_m"
	MetricNetAscentM     UsageMetric = "net_ascent_m"
	MetricNetDescentM    UsageMetric = "net_descent_m"
	MetricLiftAscentM    UsageMetric = "lift_ascent_m"
	MetricLiftDescentM   UsageMetric = "lift_descent_m"
)

var usageMetrics = map[UsageMetric]struct{}{
	MetricDistanceM:      {},
	MetricMovingTimeS:    {},
	MetricElapsedTimeS:   {},
	MetricActivityCount:  {},
	MetricElevationGainM: {},
	MetricElevationLossM: {},
	MetricNetAscentM:     {},
	MetricNetDescentM:    {},
	MetricLiftAscentM:    {},
	MetricLiftDescentM:   {},
}

func IsUsageMetric(metric string) bool {
	_, ok := usageMetrics[UsageMetric(metric)]
	return ok
}

func ProductiveServiceMetrics() []UsageMetric {
	return []UsageMetric{
		MetricDistanceM,
		MetricMovingTimeS,
		MetricElapsedTimeS,
		MetricActivityCount,
		MetricElapsedDays,
		MetricElevationGainM,
		MetricElevationLossM,
	}
}

func IsServiceMetric(metric string) bool {
	for _, m := range ProductiveServiceMetrics() {
		if string(m) == metric {
			return true
		}
	}
	return false
}

func ActivityAggregateExpr(metric string, alias string) (string, bool) {
	prefix := ""
	if alias != "" {
		if alias != "a" {
			return "", false
		}
		prefix = alias + "."
	}
	switch UsageMetric(metric) {
	case MetricDistanceM:
		return "COALESCE(SUM(COALESCE(" + prefix + "distance_m, 0)), 0)", true
	case MetricMovingTimeS:
		return "COALESCE(SUM(COALESCE(" + prefix + "moving_time_s, 0)), 0)", true
	case MetricElapsedTimeS:
		return "COALESCE(SUM(COALESCE(" + prefix + "elapsed_time_s, 0)), 0)", true
	case MetricActivityCount:
		return "COUNT(*)", true
	case MetricElevationGainM:
		return "COALESCE(SUM(COALESCE(" + prefix + "elevation_gain_m, 0)), 0)", true
	case MetricElevationLossM:
		return "COALESCE(SUM(COALESCE(" + prefix + "elevation_loss_m, 0)), 0)", true
	case MetricNetAscentM:
		return "COALESCE(SUM(COALESCE(" + prefix + "net_ascent_m, 0)), 0)", true
	case MetricNetDescentM:
		return "COALESCE(SUM(COALESCE(" + prefix + "net_descent_m, 0)), 0)", true
	case MetricLiftAscentM:
		return "COALESCE(SUM(COALESCE(" + prefix + "lift_ascent_m, 0)), 0)", true
	case MetricLiftDescentM:
		return "COALESCE(SUM(COALESCE(" + prefix + "lift_descent_m, 0)), 0)", true
	default:
		return "", false
	}
}

func LegacyIntervalMetric(name string) (UsageMetric, bool) {
	switch name {
	case "swap_interval_km":
		return MetricDistanceM, true
	case "swap_interval_hm":
		return MetricElevationGainM, true
	default:
		return "", false
	}
}
