package enrichment

import (
	"context"
	"fmt"
	"sort"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

const (
	AutoTitleStrategyNone         = "none"
	AutoTitleStrategySummit       = "summit"
	AutoTitleStrategyPointToPoint = "point_to_point"
	AutoTitleStrategySportPlace   = "sport_place"
	AutoTitleStrategySportOnly    = "sport_only"

	autoTitlePointToPointMinM              = 5000.0
	autoTitleSummitEndpointRadiusM         = 1000.0
	autoTitleSummitEndpointStrictRadiusM   = 250.0
	autoTitleSummitEndpointElevationDeltaM = 150.0
)

type AutoTitleInput struct {
	SportType string
	SubType   string

	StartCity string
	EndCity   string

	StartLat, StartLng *float64
	EndLat, EndLng     *float64

	StartElevationM *float64
	EndElevationM   *float64
	MinElevationM   *float64
	MaxElevationM   *float64

	WeatherPrefs model.WeatherPrefs
	GeoTags      []model.GeoTag
}

type AutoTitleResult struct {
	Title    string
	Strategy string
}

// GenerateAutoTitle creates a German auto-title like "Radfahrt in München".
// Delegates to the centralized sport taxonomy for label resolution.
func GenerateAutoTitle(sportType, city string) string {
	return GenerateAutoTitleWithSubType(sportType, "", city)
}

// GenerateAutoTitleWithSubType creates a German auto-title using both sport type
// and sub-type for more specific naming (e.g. "MTB-Tour in München").
func GenerateAutoTitleWithSubType(sportType, subType, city string) string {
	label := sport.AutoTitle(sportType, subType)

	if city != "" {
		return fmt.Sprintf("%s in %s", label, city)
	}
	return label
}

func ResolveAutoTitle(in AutoTitleInput) AutoTitleResult {
	if result := resolveSummitAutoTitle(in); result.Title != "" {
		return result
	}
	if result := resolvePointToPointAutoTitle(in); result.Title != "" {
		return result
	}
	return resolveSportFallbackAutoTitle(in)
}

func AutoTitleInputFromDBInput(activity *database.AutoTitleActivityInput, prefs model.WeatherPrefs, tags []model.GeoTag) AutoTitleInput {
	if activity == nil {
		return AutoTitleInput{WeatherPrefs: prefs, GeoTags: tags}
	}
	return AutoTitleInput{
		SportType:       activity.SportType,
		SubType:         activity.SubType,
		StartCity:       activity.StartCity,
		EndCity:         activity.EndCity,
		StartLat:        activity.StartLat,
		StartLng:        activity.StartLng,
		EndLat:          activity.EndLat,
		EndLng:          activity.EndLng,
		StartElevationM: activity.StartElevationM,
		EndElevationM:   activity.EndElevationM,
		MinElevationM:   activity.MinElevationM,
		MaxElevationM:   activity.MaxElevationM,
		WeatherPrefs:    prefs,
		GeoTags:         tags,
	}
}

func RunAutoTitle(ctx context.Context, db *database.DB, userID, activityID string) error {
	settings, err := db.GetEnrichmentSettings(ctx, userID)
	if err != nil {
		return fmt.Errorf("auto title settings: %w", err)
	}
	if settings != nil && !settings.EnableAutoNaming {
		return nil
	}

	prefs, err := db.GetWeatherPrefs(ctx, userID)
	if err != nil {
		prefs = model.DefaultWeatherPrefs()
	}
	activity, err := db.GetAutoTitleActivityInput(ctx, userID, activityID)
	if err != nil {
		return err
	}
	tags, err := db.GetGeoTags(ctx, activityID)
	if err != nil {
		return err
	}
	result := ResolveAutoTitle(AutoTitleInputFromDBInput(activity, prefs, tags))
	if result.Title == "" {
		return nil
	}
	return db.UpdateActivityAutoTitle(ctx, userID, activityID, result.Title)
}

func resolvePointToPointAutoTitle(in AutoTitleInput) AutoTitleResult {
	if in.StartCity == "" || in.EndCity == "" || in.StartCity == in.EndCity {
		return AutoTitleResult{}
	}
	if !hasStartEndCoords(in) || startEndDistanceM(in) < autoTitlePointToPointMinM {
		return AutoTitleResult{}
	}
	return AutoTitleResult{
		Title:    fmt.Sprintf("%s nach %s", in.StartCity, in.EndCity),
		Strategy: AutoTitleStrategyPointToPoint,
	}
}

func resolveSummitAutoTitle(in AutoTitleInput) AutoTitleResult {
	if !mountainWeatherGate(in) {
		return AutoTitleResult{}
	}
	anchor, ok := chooseSummitAnchor(in.GeoTags)
	if !ok || anchor.Name == "" {
		return AutoTitleResult{}
	}

	startOnSummit := endpointOnSummit(in.StartLat, in.StartLng, in.StartElevationM, anchor)
	endOnSummit := endpointOnSummit(in.EndLat, in.EndLng, in.EndElevationM, anchor)
	farApart := hasStartEndCoords(in) && startEndDistanceM(in) >= autoTitlePointToPointMinM

	switch {
	case startOnSummit && endOnSummit:
		label := sport.AutoTitle(in.SportType, in.SubType)
		if label == "" || label == "Training" {
			return AutoTitleResult{Title: anchor.Name, Strategy: AutoTitleStrategySummit}
		}
		return AutoTitleResult{Title: fmt.Sprintf("%s am %s", label, anchor.Name), Strategy: AutoTitleStrategySummit}
	case startOnSummit && in.EndCity != "":
		return AutoTitleResult{Title: fmt.Sprintf("%s nach %s", anchor.Name, in.EndCity), Strategy: AutoTitleStrategySummit}
	case endOnSummit && in.StartCity != "":
		return AutoTitleResult{Title: fmt.Sprintf("%s ab %s", anchor.Name, in.StartCity), Strategy: AutoTitleStrategySummit}
	case farApart && in.StartCity != "" && in.EndCity != "":
		return AutoTitleResult{Title: fmt.Sprintf("%s ab %s nach %s", anchor.Name, in.StartCity, in.EndCity), Strategy: AutoTitleStrategySummit}
	case in.StartCity != "":
		return AutoTitleResult{Title: fmt.Sprintf("%s ab %s", anchor.Name, in.StartCity), Strategy: AutoTitleStrategySummit}
	default:
		return AutoTitleResult{Title: anchor.Name, Strategy: AutoTitleStrategySummit}
	}
}

func resolveSportFallbackAutoTitle(in AutoTitleInput) AutoTitleResult {
	label := sport.AutoTitle(in.SportType, in.SubType)
	if in.StartCity != "" {
		return AutoTitleResult{
			Title:    fmt.Sprintf("%s ab %s", label, in.StartCity),
			Strategy: AutoTitleStrategySportPlace,
		}
	}
	return AutoTitleResult{Title: label, Strategy: AutoTitleStrategySportOnly}
}

func mountainWeatherGate(in AutoTitleInput) bool {
	if in.MinElevationM == nil || in.MaxElevationM == nil {
		return false
	}
	threshold := float64(in.WeatherPrefs.MountainThresholdM)
	if threshold <= 0 {
		threshold = float64(model.DefaultWeatherPrefs().MountainThresholdM)
	}
	return *in.MaxElevationM-*in.MinElevationM >= threshold
}

func chooseSummitAnchor(tags []model.GeoTag) (model.GeoTag, bool) {
	candidates := make([]model.GeoTag, 0, len(tags))
	for _, tag := range tags {
		if tag.Name == "" {
			continue
		}
		if tag.Category != "summit" && tag.Category != "pass" {
			continue
		}
		candidates = append(candidates, tag)
	}
	if len(candidates) == 0 {
		return model.GeoTag{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		ie, je := f64Value(candidates[i].EleM), f64Value(candidates[j].EleM)
		if ie != je {
			return ie > je
		}
		id, jd := f64Value(candidates[i].DistM), f64Value(candidates[j].DistM)
		if id != jd {
			return id < jd
		}
		return candidates[i].Name < candidates[j].Name
	})
	return candidates[0], true
}

func endpointOnSummit(lat, lng, ele *float64, summit model.GeoTag) bool {
	if lat == nil || lng == nil || summit.Lat == nil || summit.Lng == nil {
		return false
	}
	distanceM := geo.DistanceHaversine(*lat, *lng, *summit.Lat, *summit.Lng)
	if distanceM > autoTitleSummitEndpointRadiusM {
		return false
	}
	if ele == nil || summit.EleM == nil {
		return distanceM <= autoTitleSummitEndpointStrictRadiusM
	}
	diff := *ele - *summit.EleM
	if diff < 0 {
		diff = -diff
	}
	return diff <= autoTitleSummitEndpointElevationDeltaM
}

func hasStartEndCoords(in AutoTitleInput) bool {
	return in.StartLat != nil && in.StartLng != nil && in.EndLat != nil && in.EndLng != nil
}

func startEndDistanceM(in AutoTitleInput) float64 {
	return geo.DistanceHaversine(*in.StartLat, *in.StartLng, *in.EndLat, *in.EndLng)
}

func f64Value(v *float64) float64 {
	if v == nil {
		return -1
	}
	return *v
}
