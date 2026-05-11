package lifts

import "github.com/skadia-app/skadia/services/api/internal/model"

// ConvertActivityDataToStream maps the model.ActivityData stream into the
// detector's StreamPoint slice. Fields not present in a given file type
// (e.g. speed/distance in GPX) are left nil — the detector treats nil as
// "no data", never as zero.
func ConvertActivityDataToStream(data *model.ActivityData) []StreamPoint {
	out := make([]StreamPoint, 0, len(data.Streams))
	for _, sp := range data.Streams {
		pt := StreamPoint{
			Time:      sp.Time,
			Lat:       sp.Lat,
			Lng:       sp.Lng,
			AltitudeM: sp.AltitudeM,
			SpeedMs:   sp.SpeedMS,
			DistanceM: sp.DistanceM,
		}
		out = append(out, pt)
	}
	return out
}
