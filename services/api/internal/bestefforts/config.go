package bestefforts

// CategoryType describes how a best-efforts category is scored.
type CategoryType int

const (
	// TypeTimeLower means a shorter time for the distance is better.
	TypeTimeLower CategoryType = iota
	// TypeValueHigher means a larger value (e.g. elevation, power) is better.
	TypeValueHigher
	// TypeValueSuperlative means the all-time best in a derived metric (longest, highest, etc.).
	TypeValueSuperlative
)

// Category defines a single best-efforts measurement bucket.
type Category struct {
	Key         string       // unique key, e.g. "dist_5km", "elev_1000hm", "power_5s"
	Type        CategoryType
	DistanceM   float64 // distance threshold in metres (TypeTimeLower)
	ElevationM  float64 // elevation threshold in metres (TypeValueHigher for Hm categories)
	DurationS   int     // duration in seconds (TypeValueHigher for power categories)
	Label       string  // human-readable label, e.g. "5 km", "1000 Hm", "5 Sek"
}

// ---- distance helpers ----

func distCat(key, label string, metres float64) Category {
	return Category{Key: key, Type: TypeTimeLower, DistanceM: metres, Label: label}
}

func elevCat(key, label string, metres float64) Category {
	return Category{Key: key, Type: TypeTimeLower, ElevationM: metres, Label: label}
}

func powerCat(key, label string, seconds int) Category {
	return Category{Key: key, Type: TypeValueHigher, DurationS: seconds, Label: label}
}

func superlativeCat(key, label string) Category {
	return Category{Key: key, Type: TypeValueSuperlative, Label: label}
}

// ---- shared category slices ----

var distancesRunning = []Category{
	distCat("dist_400m", "400 m", 400),
	distCat("dist_800m", "800 m", 800),
	distCat("dist_1km", "1 km", 1000),
	distCat("dist_1mi", "1 Meile", 1609),
	distCat("dist_2mi", "2 Meilen", 3219),
	distCat("dist_5km", "5 km", 5000),
	distCat("dist_10km", "10 km", 10000),
	distCat("dist_15km", "15 km", 15000),
	distCat("dist_10mi", "10 Meilen", 16093),
	distCat("dist_20km", "20 km", 20000),
	distCat("dist_hm", "Halbmarathon", 21097),
	distCat("dist_30km", "30 km", 30000),
	distCat("dist_marathon", "Marathon", 42195),
	distCat("dist_50km", "50 km", 50000),
	distCat("dist_100km", "100 km", 100000),
}

var distancesCycling = []Category{
	distCat("dist_10km", "10 km", 10000),
	distCat("dist_20km", "20 km", 20000),
	distCat("dist_25mi", "25 Meilen", 40234),
	distCat("dist_40km", "40 km", 40000),
	distCat("dist_50km", "50 km", 50000),
	distCat("dist_50mi", "50 Meilen", 80467),
	distCat("dist_100km", "100 km", 100000),
	distCat("dist_100mi", "100 Meilen", 160934),
	distCat("dist_150km", "150 km", 150000),
	distCat("dist_200km", "200 km", 200000),
	distCat("dist_300km", "300 km", 300000),
}

var distancesHiking = []Category{
	distCat("dist_5km", "5 km", 5000),
	distCat("dist_10km", "10 km", 10000),
	distCat("dist_20km", "20 km", 20000),
	distCat("dist_30km", "30 km", 30000),
	distCat("dist_50km", "50 km", 50000),
}

var distancesSwimPool = []Category{
	distCat("dist_50m", "50 m", 50),
	distCat("dist_100m", "100 m", 100),
	distCat("dist_200m", "200 m", 200),
	distCat("dist_400m", "400 m", 400),
	distCat("dist_800m", "800 m", 800),
	distCat("dist_1500m", "1500 m", 1500),
}

var distancesSwimOW = []Category{
	distCat("dist_1km", "1 km", 1000),
	distCat("dist_2km", "2 km", 2000),
	distCat("dist_5km", "5 km", 5000),
}

var elevationCategories = []Category{
	elevCat("elev_500hm", "500 Hm", 500),
	elevCat("elev_1000hm", "1000 Hm", 1000),
	elevCat("elev_1500hm", "1500 Hm", 1500),
	elevCat("elev_2000hm", "2000 Hm", 2000),
}

var powerCategories = []Category{
	powerCat("power_5s", "5 Sek", 5),
	powerCat("power_30s", "30 Sek", 30),
	powerCat("power_1min", "1 Min", 60),
	powerCat("power_5min", "5 Min", 300),
	powerCat("power_10min", "10 Min", 600),
	powerCat("power_20min", "20 Min", 1200),
	powerCat("power_30min", "30 Min", 1800),
	powerCat("power_1h", "1 Std", 3600),
	powerCat("power_2h", "2 Std", 7200),
}

var superlativeCategories = []Category{
	superlativeCat("sup_longest_distance", "Längste Distanz"),
	superlativeCat("sup_most_elevation", "Meiste Höhenmeter"),
	superlativeCat("sup_biggest_climb", "Größter Anstieg"),
	superlativeCat("sup_longest_duration", "Längste Dauer"),
}

// concat returns a new slice containing all elements of the given slices.
func concat(slices ...[]Category) []Category {
	total := 0
	for _, s := range slices {
		total += len(s)
	}
	out := make([]Category, 0, total)
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}

// sportGroupCategories maps sport group keys to their ordered category list.
var sportGroupCategories = map[string][]Category{
	"running": concat(
		distancesRunning,
		elevationCategories,
		powerCategories,
		superlativeCategories,
	),
	"running_indoor": concat(
		distancesRunning,
		powerCategories,
		superlativeCategories,
	),
	"running_virtual": concat(
		distancesRunning,
		superlativeCategories,
	),
	"cycling": concat(
		distancesCycling,
		elevationCategories,
		powerCategories,
		superlativeCategories,
	),
	"cycling_electric": concat(
		distancesCycling,
		elevationCategories,
		superlativeCategories,
	),
	"cycling_virtual": concat(
		distancesCycling,
		superlativeCategories,
	),
	"hiking": concat(
		distancesHiking,
		elevationCategories,
		superlativeCategories,
	),
	"swim_pool": concat(
		distancesSwimPool,
		superlativeCategories,
	),
	"swim_ow": concat(
		distancesSwimOW,
		superlativeCategories,
	),
}

// CategoriesFor returns the ordered list of best-efforts categories for the given sport group.
// Returns nil if the sport group is unknown or has no best-efforts semantics.
func CategoriesFor(sportGroup string) []Category {
	cats, ok := sportGroupCategories[sportGroup]
	if !ok {
		return nil
	}
	result := make([]Category, len(cats))
	copy(result, cats)
	return result
}
