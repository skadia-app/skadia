package bestefforts

import "time"

// ActivityInfo holds the activity metadata needed to compute best efforts.
// It is populated from the activities table by the database layer.
type ActivityInfo struct {
	ID            string
	UserID        string
	SportType     string
	SubType       string
	IsIndoor      bool
	IsVirtual     bool
	IsElectric    bool
	StartedAt     time.Time
	DistanceM     float64
	ElevationGainM float64
	ElapsedTimeS  int
}
