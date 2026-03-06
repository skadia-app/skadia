package enrichment

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"
)

// WeatherResult holds weather data for an activity.
type WeatherResult struct {
	TempC       float64
	Condition   string
	Humidity    int
	WindSpeedMS float64
	Tags        []string
}

// openMeteoResponse is the JSON structure from Open-Meteo archive API.
type openMeteoResponse struct {
	Hourly struct {
		Time              []string  `json:"time"`
		Temperature2m     []float64 `json:"temperature_2m"`
		RelativeHumidity2m []int    `json:"relative_humidity_2m"`
		WindSpeed10m      []float64 `json:"wind_speed_10m"`
		WeatherCode       []int    `json:"weather_code"`
	} `json:"hourly"`
}

// FetchWeather fetches historical weather from Open-Meteo archive API.
func FetchWeather(lat, lng float64, t time.Time) (*WeatherResult, error) {
	date := t.Format("2006-01-02")
	url := fmt.Sprintf(
		"https://archive-api.open-meteo.com/v1/archive?latitude=%.4f&longitude=%.4f&start_date=%s&end_date=%s&hourly=temperature_2m,relative_humidity_2m,wind_speed_10m,weather_code",
		lat, lng, date, date,
	)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch weather: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo returned status %d", resp.StatusCode)
	}

	var data openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode weather response: %w", err)
	}

	// Find the closest hour to the activity start time
	targetHour := t.Hour()
	if targetHour >= len(data.Hourly.Temperature2m) {
		targetHour = len(data.Hourly.Temperature2m) - 1
	}
	if len(data.Hourly.Temperature2m) == 0 {
		return nil, fmt.Errorf("no hourly data returned")
	}

	condition, tags := mapWeatherCode(data.Hourly.WeatherCode[targetHour])

	return &WeatherResult{
		TempC:       math.Round(data.Hourly.Temperature2m[targetHour]*10) / 10,
		Condition:   condition,
		Humidity:    data.Hourly.RelativeHumidity2m[targetHour],
		WindSpeedMS: math.Round(data.Hourly.WindSpeed10m[targetHour]/3.6*10) / 10, // km/h → m/s
		Tags:        tags,
	}, nil
}

// mapWeatherCode maps WMO weather codes to condition string and tags.
func mapWeatherCode(code int) (string, []string) {
	switch {
	case code == 0:
		return "klar", []string{"sonnig"}
	case code == 1:
		return "ueberwiegend klar", []string{"sonnig"}
	case code == 2:
		return "teilweise bewölkt", []string{"bewölkt"}
	case code == 3:
		return "bedeckt", []string{"bewölkt"}
	case code >= 45 && code <= 48:
		return "nebel", []string{"nebel"}
	case code >= 51 && code <= 55:
		return "nieselregen", []string{"regen"}
	case code >= 56 && code <= 57:
		return "gefrierender nieselregen", []string{"regen", "schnee"}
	case code >= 61 && code <= 65:
		return "regen", []string{"regen"}
	case code >= 66 && code <= 67:
		return "gefrierender regen", []string{"regen", "schnee"}
	case code >= 71 && code <= 75:
		return "schneefall", []string{"schnee"}
	case code == 77:
		return "schneegriesel", []string{"schnee"}
	case code >= 80 && code <= 82:
		return "regenschauer", []string{"regen"}
	case code >= 85 && code <= 86:
		return "schneeschauer", []string{"schnee"}
	case code >= 95 && code <= 99:
		return "gewitter", []string{"gewitter", "regen"}
	default:
		return "unbekannt", nil
	}
}
