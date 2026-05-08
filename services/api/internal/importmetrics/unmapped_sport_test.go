package importmetrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestUnmappedSportRecorderCountsSupportedSource(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := NewUnmappedSportRecorder(registry)

	recorder.RecordUnmappedSport(SourceStrava)
	recorder.RecordUnmappedSport(SourceStrava)

	got := gatherCounterValue(t, registry, "skadia_import_unmapped_sport_total", "strava")
	if got != 2 {
		t.Fatalf("strava counter = %v, want 2", got)
	}
}

func TestUnmappedSportRecorderBucketsUnknownSource(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := NewUnmappedSportRecorder(registry)

	recorder.RecordUnmappedSport(Source("raw-provider-name"))

	if got := gatherCounterValue(t, registry, "skadia_import_unmapped_sport_total", "raw-provider-name"); got != 0 {
		t.Fatalf("raw provider label counter = %v, want 0", got)
	}
	if got := gatherCounterValue(t, registry, "skadia_import_unmapped_sport_total", "unknown"); got != 1 {
		t.Fatalf("unknown counter = %v, want 1", got)
	}
}

func TestNewUnmappedSportRecorderReusesAlreadyRegisteredCounter(t *testing.T) {
	registry := prometheus.NewRegistry()
	first := NewUnmappedSportRecorder(registry)
	second := NewUnmappedSportRecorder(registry)

	first.RecordUnmappedSport(SourceFIT)
	second.RecordUnmappedSport(SourceFIT)

	if got := gatherCounterValue(t, registry, "skadia_import_unmapped_sport_total", "fit"); got != 2 {
		t.Fatalf("fit counter = %v, want 2", got)
	}
}

func gatherCounterValue(t *testing.T, gatherer prometheus.Gatherer, metricName, source string) float64 {
	t.Helper()

	families, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != metricName {
			continue
		}
		for _, metric := range family.GetMetric() {
			if hasSourceLabel(metric, source) {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func hasSourceLabel(metric *dto.Metric, source string) bool {
	for _, label := range metric.GetLabel() {
		if label.GetName() == "source" && label.GetValue() == source {
			return true
		}
	}
	return false
}
