package komoot

import "time"

// LoginResponse from GET /v006/account/email/{email}/
type LoginResponse struct {
	Username string `json:"username"` // numeric user ID as string
	Password string `json:"password"` // session token
	User     struct {
		DisplayName string `json:"displayname"`
	} `json:"user"`
}

// ToursResponse from GET /v007/users/{uid}/tours/
type ToursResponse struct {
	Embedded struct {
		Tours []Tour `json:"tours"`
	} `json:"_embedded"`
	Page struct {
		Size          int `json:"size"`
		TotalElements int `json:"totalElements"`
		TotalPages    int `json:"totalPages"`
		Number        int `json:"number"`
	} `json:"page"`
}

// Tour represents a single Komoot tour (recorded or planned).
type Tour struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`          // "tour_recorded" | "tour_planned"
	Sport         string    `json:"sport"`          // "hike", "mtb", "jogging", etc.
	Date          time.Time `json:"date"`
	Distance      float64   `json:"distance"`      // meters
	Duration      int       `json:"duration"`      // seconds
	ElevationUp   float64   `json:"elevation_up"`
	ElevationDown float64   `json:"elevation_down"`
	TimeInMotion  float64   `json:"time_in_motion"`
	Status        string    `json:"status"`         // "public" | "private"
}

// TourDetailResponse from GET /v007/tours/{id}?_embedded=coordinates,timeline
type TourDetailResponse struct {
	Tour
	Embedded struct {
		Coordinates struct {
			Items []CoordinateItem `json:"items"`
		} `json:"coordinates"`
		Timeline struct {
			Embedded struct {
				Items []TimelineItem `json:"items"`
			} `json:"_embedded"`
		} `json:"timeline"`
	} `json:"_embedded"`
}

// CoordinateItem is a single GPS point in a tour track.
type CoordinateItem struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
	Alt float64 `json:"alt"`
	T   int64   `json:"t"` // milliseconds offset from tour start
}

// TimelineItem is a single entry in the tour timeline (photo, highlight, poi, etc.).
type TimelineItem struct {
	Type     string `json:"type"` // "photo", "highlight", "poi"
	Embedded struct {
		Reference struct {
			Src      string `json:"src"` // image CDN URL
			Location *struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			} `json:"location"`
		} `json:"reference"`
	} `json:"_embedded"`
}

// SportMapping maps a Komoot sport string to Skadia taxonomy fields.
type SportMapping struct {
	SportType string
	SubType   string
	IsEbike   bool
}

// KomootSportMapping maps Komoot sport strings to Skadia taxonomy.
// Covers all 26 known Komoot sport types.
var KomootSportMapping = map[string]SportMapping{
	"hike":             {"hiking", "", false},
	"mountaineering":   {"mountaineering", "", false},
	"jogging":          {"running", "", false},
	"racebike":         {"cycling", "road", false},
	"e_racebike":       {"cycling", "road", true},
	"touringbicycle":   {"cycling", "", false},
	"e_touringbicycle": {"cycling", "", true},
	"mtb":              {"cycling", "mountain", false},
	"e_mtb":            {"cycling", "mountain", true},
	"mtb_easy":         {"cycling", "gravel_cycling", false},
	"e_mtb_easy":       {"cycling", "gravel_cycling", true},
	"mtb_advanced":     {"cycling", "mountain", false},
	"e_mtb_advanced":   {"cycling", "mountain", true},
	"citybike":         {"cycling", "", false},
	"downhillbike":     {"cycling", "downhill", false},
	"climbing":         {"rock_climbing", "", false},
	"nordic":           {"cross_country_skiing", "", false},
	"nordicwalking":    {"walking", "speed_walking", false},
	"skaten":           {"inline_skating", "", false},
	"skialpin":         {"alpine_skiing", "resort", false},
	"skitour":          {"alpine_skiing", "backcountry", false},
	"sled":             {"sledding", "", false},
	"snowboard":        {"snowboarding", "", false},
	"snowshoe":         {"snowshoeing", "", false},
	"unicycle":         {"cycling", "unicycling", false},
	"other":            {"generic", "", false},
}
