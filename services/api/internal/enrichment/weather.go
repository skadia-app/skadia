package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// weatherParams are the hourly fields we request from Open-Meteo.
const weatherParams = "temperature_2m,apparent_temperature,relative_humidity_2m," +
	"wind_speed_10m,wind_gusts_10m,precipitation,weather_code,uv_index,freezing_level_height"

// WeatherRequest describes one weather API call.
type WeatherRequest struct {
	ReadingType string  // 'single'|'start'|'summit'|'midpoint'
	Label       *string // display label, nil for single reading
	Lat, Lng    float64
	ElevationM  float64   // actual elevation for lapse rate; 0 = let API use DEM
	At          time.Time // timestamp to look up (picks closest hour)
}

// openMeteoHourly is the JSON structure from both Open-Meteo APIs.
type openMeteoHourly struct {
	Time               []string  `json:"time"`
	Temperature2m      []float64 `json:"temperature_2m"`
	ApparentTemp       []float64 `json:"apparent_temperature"`
	RelativeHumidity2m []int     `json:"relative_humidity_2m"`
	WindSpeed10m       []float64 `json:"wind_speed_10m"`
	WindGusts10m       []float64 `json:"wind_gusts_10m"`
	Precipitation      []float64 `json:"precipitation"`
	WeatherCode        []int     `json:"weather_code"`
	UVIndex            []float64 `json:"uv_index"`
	FreezingLevel      []float64 `json:"freezing_level_height"`
}

type openMeteoResponse struct {
	Hourly openMeteoHourly `json:"hourly"`
}

// FetchWeatherReadings fetches 1–2 weather readings for an activity.
func FetchWeatherReadings(ctx context.Context, reqs []WeatherRequest) ([]model.WeatherReading, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	client := &http.Client{Timeout: 15 * time.Second}
	var readings []model.WeatherReading

	for _, req := range reqs {
		endpoint, modelID := selectWeatherModel(req.Lat, req.Lng, req.At)
		r, err := fetchSingle(ctx, client, endpoint, modelID, req)
		if err != nil {
			return nil, fmt.Errorf("fetch weather (%s): %w", req.ReadingType, err)
		}
		readings = append(readings, *r)
	}
	return readings, nil
}

func fetchSingle(ctx context.Context, client *http.Client, endpoint, modelID string, req WeatherRequest) (*model.WeatherReading, error) {
	date := req.At.UTC().Format("2006-01-02")

	params := url.Values{}
	params.Set("latitude", strconv.FormatFloat(req.Lat, 'f', 4, 64))
	params.Set("longitude", strconv.FormatFloat(req.Lng, 'f', 4, 64))
	params.Set("start_date", date)
	params.Set("end_date", date)
	params.Set("hourly", weatherParams)
	if endpoint == forecastEndpoint {
		params.Set("model", modelID)
	}
	params.Set("wind_speed_unit", "ms")
	params.Set("timezone", "UTC")
	if req.ElevationM > 0 {
		params.Set("elevation", strconv.FormatFloat(req.ElevationM, 'f', 0, 64))
	}

	apiURL := endpoint + "?" + params.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo status %d (model=%s)", resp.StatusCode, modelID)
	}

	var data openMeteoResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	h := data.Hourly
	if len(h.Temperature2m) == 0 {
		return nil, fmt.Errorf("no hourly data (model=%s)", modelID)
	}

	idx := closestHourIndex(h.Time, req.At)

	condition, tags := mapWeatherCode(safeInt(h.WeatherCode, idx))
	cond := condition
	m := modelID

	reading := model.WeatherReading{
		ReadingType:     req.ReadingType,
		Label:           req.Label,
		RecordedAt:      req.At,
		Lat:             &req.Lat,
		Lng:             &req.Lng,
		Condition:       &cond,
		Tags:            tags,
		Model:           &m,
		TempC:           safeFloat64Ptr(h.Temperature2m, idx),
		ApparentTempC:   safeFloat64Ptr(h.ApparentTemp, idx),
		Humidity:        safeIntPtr(h.RelativeHumidity2m, idx),
		WindSpeedMS:     safeFloat64Ptr(h.WindSpeed10m, idx),
		WindGustsMS:     safeFloat64Ptr(h.WindGusts10m, idx),
		PrecipitationMM: safeFloat64Ptr(h.Precipitation, idx),
		UVIndex:         safeFloat64Ptr(h.UVIndex, idx),
		FreezingLevelM:  safeFloat64Ptr(h.FreezingLevel, idx),
	}
	if req.ElevationM > 0 {
		e := req.ElevationM
		reading.ElevationM = &e
	}
	if reading.TempC != nil {
		v := math.Round(*reading.TempC*10) / 10
		reading.TempC = &v
	}
	if reading.ApparentTempC != nil {
		v := math.Round(*reading.ApparentTempC*10) / 10
		reading.ApparentTempC = &v
	}
	return &reading, nil
}

// closestHourIndex finds the index in hourly time strings closest to t.
func closestHourIndex(times []string, t time.Time) int {
	target := t.UTC()
	best, bestDiff := 0, time.Duration(math.MaxInt64)
	for i, ts := range times {
		parsed, err := time.Parse("2006-01-02T15:04", ts)
		if err != nil {
			continue
		}
		d := target.Sub(parsed)
		if d < 0 {
			d = -d
		}
		if d < bestDiff {
			bestDiff = d
			best = i
		}
	}
	return best
}

func safeFloat64Ptr(arr []float64, idx int) *float64 {
	if idx >= len(arr) {
		return nil
	}
	v := arr[idx]
	if math.IsNaN(v) {
		return nil
	}
	return &v
}

func safeIntPtr(arr []int, idx int) *int {
	if idx >= len(arr) {
		return nil
	}
	v := arr[idx]
	return &v
}

func safeInt(arr []int, idx int) int {
	if idx >= len(arr) {
		return -1
	}
	return arr[idx]
}

// mapWeatherCode maps an Open-Meteo WMO code to a human-readable condition and tags.
func mapWeatherCode(code int) (string, []string) {
	switch {
	case code == 0:
		return "klar", []string{"sonnig"}
	case code == 1:
		return "überwiegend klar", []string{"sonnig"}
	case code == 2:
		return "teilweise bewölkt", []string{"bewölkt"}
	case code == 3:
		return "bedeckt", []string{"bewölkt"}
	case code == 45:
		return "Nebel", []string{"nebel"}
	case code == 48:
		return "Nebel", []string{"nebel"}
	case code >= 51 && code <= 55:
		return "Nieselregen", []string{"regen"}
	case code >= 56 && code <= 57:
		return "gefrierender Nieselregen", []string{"regen"}
	case code >= 61 && code <= 65:
		return "Regen", []string{"regen"}
	case code >= 66 && code <= 67:
		return "gefrierender Regen", []string{"regen"}
	case code >= 71 && code <= 75:
		return "Schneefall", []string{"schnee"}
	case code == 77:
		return "Schneegriesel", []string{"schnee"}
	case code >= 80 && code <= 82:
		return "Regenschauer", []string{"regen"}
	case code >= 85 && code <= 86:
		return "Schneeschauer", []string{"schnee"}
	case code == 95:
		return "Gewitter", []string{"gewitter", "regen"}
	case code == 96:
		return "Gewitter mit leichtem Hagel", []string{"gewitter", "regen"}
	case code == 99:
		return "Gewitter mit starkem Hagel", []string{"gewitter", "regen"}
	default:
		return "unbekannt", nil
	}
}

// BuildWeatherRequests constructs the 1–2 WeatherRequest values for an activity.
// When summit/midpoint lat/lng are provided (non-zero), they are used;
// otherwise falls back to start coordinates.
func BuildWeatherRequests(
	startLat, startLng float64,
	startedAt time.Time,
	elapsedSec int,
	minElevM, maxElevM *float64,
	summitLat, summitLng *float64, summitAlt float64,
	midLat, midLng, midMeanAlt float64,
	summitAt *time.Time, midAt *time.Time,
	prefs model.WeatherPrefs,
) []WeatherRequest {
	var minAlt, maxAlt float64
	if minElevM != nil {
		minAlt = *minElevM
	}
	if maxElevM != nil {
		maxAlt = *maxElevM
	}
	c := DetermineWeatherCase(elapsedSec, minAlt, maxAlt, prefs)

	startElevM := minAlt
	if minElevM == nil {
		startElevM = (minAlt + maxAlt) / 2
	}

	switch c {
	case CaseSingle:
		midTime := startedAt.Add(time.Duration(elapsedSec/2) * time.Second)
		meanElev := (minAlt + maxAlt) / 2
		lat, lng := startLat, startLng
		elev := meanElev
		if midLat != 0 {
			lat, lng, elev = midLat, midLng, midMeanAlt
		}
		if midAt != nil {
			midTime = *midAt
		}
		return []WeatherRequest{{
			ReadingType: "single",
			Lat:         lat, Lng: lng,
			ElevationM: elev,
			At:         midTime,
		}}

	case CaseMountain:
		label1 := fmt.Sprintf("▽ Start (%.0fm)", minAlt)
		label2 := fmt.Sprintf("▲ Berg (%.0fm)", maxAlt)
		sumLat, sumLng, sumAlt := startLat, startLng, maxAlt
		sumTime := startedAt.Add(time.Duration(elapsedSec/2) * time.Second)
		if summitLat != nil {
			sumLat, sumLng, sumAlt = *summitLat, *summitLng, summitAlt
		}
		if summitAt != nil {
			sumTime = *summitAt
		}
		return []WeatherRequest{
			{ReadingType: "start", Label: &label1, Lat: startLat, Lng: startLng, ElevationM: startElevM, At: startedAt},
			{ReadingType: "summit", Label: &label2, Lat: sumLat, Lng: sumLng, ElevationM: sumAlt, At: sumTime},
		}

	case CaseLong:
		label1 := "▲ Start"
		label2 := "⌀ Durchschnitt"
		midTime := startedAt.Add(time.Duration(elapsedSec/2) * time.Second)
		meanElev := (minAlt + maxAlt) / 2
		mLat, mLng, mElev := startLat, startLng, meanElev
		if midLat != 0 {
			mLat, mLng, mElev = midLat, midLng, midMeanAlt
		}
		if midAt != nil {
			midTime = *midAt
		}
		return []WeatherRequest{
			{ReadingType: "start", Label: &label1, Lat: startLat, Lng: startLng, ElevationM: startElevM, At: startedAt},
			{ReadingType: "midpoint", Label: &label2, Lat: mLat, Lng: mLng, ElevationM: mElev, At: midTime},
		}
	}
	return nil
}

// NeedsStreamRefinement reports whether an activity's weather should be re-fetched
// once streams are available.
func NeedsStreamRefinement(hasGPS bool, minElevM, maxElevM float64, elapsedSec int) bool {
	if !hasGPS || (minElevM == 0 && maxElevM == 0) {
		return false
	}
	c := DetermineWeatherCase(elapsedSec, minElevM, maxElevM, model.DefaultWeatherPrefs())
	return c == CaseMountain || c == CaseLong
}
