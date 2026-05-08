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
	Key        string // unique key, e.g. "dist_5km", "elev_1000hm", "power_5s"
	Type       CategoryType
	DistanceM  float64 // distance threshold in metres (TypeTimeLower)
	ElevationM float64 // elevation threshold in metres (TypeValueHigher for Hm categories)
	DurationS  int     // duration in seconds (TypeValueHigher for power categories)
	Label      string  // human-readable label, e.g. "5 km", "1000 Hm", "5 Sek"
}

// CategoriesFor returns the ordered list of best-efforts categories for the given sport group.
// Returns nil if the sport group is unknown or has no best-efforts semantics.
func CategoriesFor(sportGroup string) []Category {
	group, ok := RecordGroupByKey(sportGroup)
	if !ok {
		return nil
	}
	return group.Categories
}
