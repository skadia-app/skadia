package importmetrics

import "github.com/prometheus/client_golang/prometheus"

type Source string

const (
	SourceFIT         Source = "fit"
	SourceStrava      Source = "strava"
	SourceKomoot      Source = "komoot"
	SourceKomootRoute Source = "komoot_route"
	SourceUnknown     Source = "unknown"
)

type UnmappedSportRecorder interface {
	RecordUnmappedSport(source Source)
}

type prometheusUnmappedSportRecorder struct {
	counter *prometheus.CounterVec
}

var defaultUnmappedSportRecorder = NewUnmappedSportRecorder(prometheus.DefaultRegisterer)

func NewUnmappedSportRecorder(registerer prometheus.Registerer) UnmappedSportRecorder {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "skadia_import_unmapped_sport_total",
		Help: "Number of imported activities whose provider sport could not be mapped to the taxonomy.",
	}, []string{"source"})
	registerer.MustRegister(counter)
	return &prometheusUnmappedSportRecorder{counter: counter}
}

func RecordUnmappedSport(source Source) {
	defaultUnmappedSportRecorder.RecordUnmappedSport(source)
}

func (r *prometheusUnmappedSportRecorder) RecordUnmappedSport(source Source) {
	r.counter.WithLabelValues(string(normalizeSource(source))).Inc()
}

func normalizeSource(source Source) Source {
	switch source {
	case SourceFIT, SourceStrava, SourceKomoot, SourceKomootRoute:
		return source
	default:
		return SourceUnknown
	}
}
