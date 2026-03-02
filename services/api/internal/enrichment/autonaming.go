package enrichment

import "fmt"

// sportLabels maps sport types to German labels for auto-naming.
var sportLabels = map[string]string{
	"cycling":                    "Radfahrt",
	"running":                    "Lauf",
	"hiking":                     "Wanderung",
	"walking":                    "Spaziergang",
	"swimming":                   "Schwimmen",
	"e_biking":                   "E-Bike-Tour",
	"skiing":                     "Skifahrt",
	"cross_country_skiing":       "Langlauf",
	"paddling":                   "Paddeltour",
	"strength":                   "Krafttraining",
	"strength_training":          "Krafttraining",
	"rock_climbing":              "Klettern",
	"stand_up_paddleboarding":    "SUP-Tour",
	"trail_running":              "Trailrun",
	"mountaineering":             "Bergtour",
	"inline_skating":             "Inline-Skaten",
	"open_water_swimming":        "Freiwasserschwimmen",
	"mountain_biking":            "MTB-Tour",
	"gravel_cycling":             "Gravel-Tour",
	"generic":                    "Training",
}

// GenerateAutoTitle creates a German auto-title like "Radfahrt in Muenchen"
func GenerateAutoTitle(sportType, city string) string {
	label, ok := sportLabels[sportType]
	if !ok {
		label = "Training"
	}

	if city != "" {
		return fmt.Sprintf("%s in %s", label, city)
	}
	return label
}
