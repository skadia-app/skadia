package shadowfs

import (
	"strconv"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

type ActivitySchemaTagsInput struct {
	Format         string
	Source         string
	StartedAt      time.Time
	SportType      string
	SubType        string
	Title          string
	City           string
	Country        string
	DistanceM      float64
	ElapsedTimeS   int
	ElevationGainM float64
}

func ActivitySchemaTags(input ActivitySchemaTagsInput) map[string]string {
	tags := map[string]string{}
	if input.Format != "" {
		tags["format"] = input.Format
	}
	if input.Source != "" {
		tags["source"] = input.Source
	}
	if !input.StartedAt.IsZero() {
		tags["year"] = input.StartedAt.Format("2006")
		tags["month"] = input.StartedAt.Format("01")
		tags["day"] = input.StartedAt.Format("02")
		tags["time"] = input.StartedAt.Format("1504")
	}

	tuple, _, err := sport.NormalizeSportTupleForImport(input.SportType, input.SubType)
	if err != nil {
		tags["sport"] = "unknown"
	} else {
		tags["sport"] = tuple.SportType
		if tuple.SportType != "multisport" && tuple.SubType != "" {
			tags["subCategory"] = tuple.SubType
		}
	}

	if input.Title != "" {
		tags["title"] = input.Title
	}
	if input.City != "" {
		tags["city"] = input.City
	}
	if input.Country != "" {
		tags["country"] = input.Country
	}
	if input.DistanceM > 0 {
		tags["distance_km"] = strconv.FormatFloat(input.DistanceM/1000.0, 'f', 1, 64)
	}
	if input.ElapsedTimeS > 0 {
		h := input.ElapsedTimeS / 3600
		m := (input.ElapsedTimeS % 3600) / 60
		tags["duration_h"] = strconv.Itoa(h) + "-" + strconv.Itoa(m)
	}
	if input.ElevationGainM > 0 {
		tags["elevation_m"] = strconv.Itoa(int(input.ElevationGainM))
	}
	return tags
}
