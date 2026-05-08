package importmetrics

import "github.com/prometheus/client_golang/prometheus"

type Source string

const (
	SourceFIT         Source = "fit"
	SourceStrava      Source = "strava"
	SourceKomoot      Source = "komoot"
	SourceKomootRoute Source = "komoot_route"
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
	if err := registerer.Register(counter); err != nil {
		if alreadyRegistered, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := alreadyRegistered.ExistingCollector.(*prometheus.CounterVec); ok {
				counter = existing
			} else {
				panic(err)
			}
		} else {
			panic(err)
		}
	}
	return &prometheusUnmappedSportRecorder{counter: counter}
}

func RecordUnmappedSport(source Source) {
	defaultUnmappedSportRecorder.RecordUnmappedSport(source)
}

func (r *prometheusUnmappedSportRecorder) RecordUnmappedSport(source Source) {
	r.counter.WithLabelValues(sourceLabel(source)).Inc()
}

func sourceLabel(source Source) string {
	switch source {
	case SourceFIT, SourceStrava, SourceKomoot, SourceKomootRoute:
		return string(source)
	default:
		panic("unsupported unmapped sport source: " + string(source))
	}
}
