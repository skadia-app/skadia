package analytics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AnalyticsFilter struct {
	From             time.Time
	To               time.Time
	SportTypes       []string
	Tags             []string
	WeatherCondition *string
	TempMin          *float64
	TempMax          *float64
	TimeOfDay        []string
	Intensity        []string
	DistMin          *float64
	DistMax          *float64
	ElevMin          *float64
	ElevMax          *float64
	GearID           *uuid.UUID
	IsCommute        *bool
	IsRace           *bool
	PaceMinSecKm     *float64 // Pace in sec/km (Minimum = schnellste)
	PaceMaxSecKm     *float64 // Pace in sec/km (Maximum = langsamste)
	Countries        []string
	Seasons          []string
	Weekdays         []int
	HRMin            *int
	HRMax            *int
	PowerMin         *int
	PowerMax         *int
	IsElectric          *bool // nil=neutral, true=only ebike, false=exclude ebike
	IsVirtual        *bool // nil=neutral, true=only virtual, false=exclude virtual
	// Tri-state tag filters
	IncludeTags []uuid.UUID // activity must have ALL of these tags
	ExcludeTags []uuid.UUID // activity must have NONE of these tags
	// Tri-state gear filters
	IncludeGear []uuid.UUID // activity must use one of these gear items
	ExcludeGear []uuid.UUID // activity must not use any of these gear items
}

func ParseFilters(r *http.Request) AnalyticsFilter {
	q := r.URL.Query()
	f := AnalyticsFilter{}

	if v := q.Get("from"); v != "" {
		f.From, _ = time.Parse("2006-01-02", v)
	}
	if v := q.Get("to"); v != "" {
		f.To, _ = time.Parse("2006-01-02", v)
	}
	if f.To.IsZero() {
		f.To = time.Now()
	}
	if v := q.Get("sport_type"); v != "" {
		f.SportTypes = strings.Split(v, ",")
	}
	if v := q.Get("tags"); v != "" {
		f.Tags = strings.Split(v, ",")
	}
	if v := q.Get("weather"); v != "" {
		f.WeatherCondition = &v
	}
	f.TempMin = parseOptFloat(q.Get("temp_min"))
	f.TempMax = parseOptFloat(q.Get("temp_max"))
	if v := q.Get("time_of_day"); v != "" {
		f.TimeOfDay = strings.Split(v, ",")
	}
	if v := q.Get("intensity"); v != "" {
		f.Intensity = strings.Split(v, ",")
	}
	f.DistMin = parseOptFloat(q.Get("dist_min"))
	f.DistMax = parseOptFloat(q.Get("dist_max"))
	f.ElevMin = parseOptFloat(q.Get("elev_min"))
	f.ElevMax = parseOptFloat(q.Get("elev_max"))
	if v := q.Get("gear_id"); v != "" {
		if uid, err := uuid.Parse(v); err == nil {
			f.GearID = &uid
		}
	}
	f.IsCommute = parseOptBool(q.Get("is_commute"))
	f.IsRace = parseOptBool(q.Get("is_race"))
	f.IsElectric = parseOptBool(q.Get("is_electric"))
	f.IsVirtual = parseOptBool(q.Get("is_virtual"))
	f.PaceMinSecKm = parseOptFloat(q.Get("pace_min")) // sec/km
	f.PaceMaxSecKm = parseOptFloat(q.Get("pace_max")) // sec/km
	if v := q.Get("country"); v != "" {
		f.Countries = strings.Split(v, ",")
	}
	if v := q.Get("season"); v != "" {
		f.Seasons = strings.Split(v, ",")
	}
	if v := q.Get("weekday"); v != "" {
		for d := range strings.SplitSeq(v, ",") {
			if n, err := strconv.Atoi(d); err == nil {
				f.Weekdays = append(f.Weekdays, n)
			}
		}
	}
	f.HRMin = parseOptInt(q.Get("hr_min"))
	f.HRMax = parseOptInt(q.Get("hr_max"))
	f.PowerMin = parseOptInt(q.Get("power_min"))
	f.PowerMax = parseOptInt(q.Get("power_max"))

	if v := q.Get("include_tags"); v != "" {
		for s := range strings.SplitSeq(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				if uid, err := uuid.Parse(s); err == nil {
					f.IncludeTags = append(f.IncludeTags, uid)
				}
			}
		}
	}
	if v := q.Get("exclude_tags"); v != "" {
		for s := range strings.SplitSeq(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				if uid, err := uuid.Parse(s); err == nil {
					f.ExcludeTags = append(f.ExcludeTags, uid)
				}
			}
		}
	}
	if v := q.Get("include_gear"); v != "" {
		for s := range strings.SplitSeq(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				if uid, err := uuid.Parse(s); err == nil {
					f.IncludeGear = append(f.IncludeGear, uid)
				}
			}
		}
	}
	if v := q.Get("exclude_gear"); v != "" {
		for s := range strings.SplitSeq(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				if uid, err := uuid.Parse(s); err == nil {
					f.ExcludeGear = append(f.ExcludeGear, uid)
				}
			}
		}
	}

	return f
}

// BuildWhereClause erzeugt SQL WHERE-Bedingungen und zugehörige Parameter.
// startIdx gibt den Startindex für $N Placeholder an (1-basiert).
// Gibt clause string und args []any zurück.
func (f *AnalyticsFilter) BuildWhereClause(startIdx int) (string, []any) {
	var conditions []string
	var args []any
	idx := startIdx

	if !f.From.IsZero() {
		conditions = append(conditions, "a.started_at >= $"+strconv.Itoa(idx))
		args = append(args, f.From)
		idx++
	}
	if !f.To.IsZero() {
		conditions = append(conditions, "a.started_at <= $"+strconv.Itoa(idx))
		args = append(args, f.To)
		idx++
	}
	if len(f.SportTypes) > 0 {
		conditions = append(conditions, "a.sport_type = ANY($"+strconv.Itoa(idx)+")")
		args = append(args, f.SportTypes)
		idx++
	}
	if len(f.Tags) > 0 {
		conditions = append(conditions, "a.tags && $"+strconv.Itoa(idx))
		args = append(args, f.Tags)
		idx++
	}
	if f.IsCommute != nil {
		if *f.IsCommute {
			conditions = append(conditions, "a.id IN (SELECT at_c.activity_id FROM activity_tags at_c JOIN tags t_c ON at_c.tag_id = t_c.id WHERE t_c.name = 'Pendeln' AND t_c.user_id = a.user_id AND t_c.is_auto = true)")
		} else {
			conditions = append(conditions, "a.id NOT IN (SELECT at_c.activity_id FROM activity_tags at_c JOIN tags t_c ON at_c.tag_id = t_c.id WHERE t_c.name = 'Pendeln' AND t_c.user_id = a.user_id AND t_c.is_auto = true)")
		}
	}
	if f.IsRace != nil {
		if *f.IsRace {
			conditions = append(conditions, "a.id IN (SELECT at_r.activity_id FROM activity_tags at_r JOIN tags t_r ON at_r.tag_id = t_r.id WHERE t_r.name = 'Wettkampf' AND t_r.user_id = a.user_id AND t_r.is_auto = true)")
		} else {
			conditions = append(conditions, "a.id NOT IN (SELECT at_r.activity_id FROM activity_tags at_r JOIN tags t_r ON at_r.tag_id = t_r.id WHERE t_r.name = 'Wettkampf' AND t_r.user_id = a.user_id AND t_r.is_auto = true)")
		}
	}
	if f.PaceMinSecKm != nil {
		// Pace sec/km → avg_speed_ms: pace(sec/km) = 1000/speed(m/s)
		// pace_min (schnellste) → höchste Geschwindigkeit
		maxSpeed := 1000.0 / *f.PaceMinSecKm
		conditions = append(conditions, "a.avg_speed_ms <= $"+strconv.Itoa(idx))
		args = append(args, maxSpeed)
		idx++
	}
	if f.PaceMaxSecKm != nil {
		// pace_max (langsamste) → niedrigste Geschwindigkeit
		minSpeed := 1000.0 / *f.PaceMaxSecKm
		conditions = append(conditions, "a.avg_speed_ms >= $"+strconv.Itoa(idx))
		args = append(args, minSpeed)
		idx++
	}
	if f.GearID != nil {
		conditions = append(conditions, "a.gear_id = $"+strconv.Itoa(idx))
		args = append(args, *f.GearID)
		idx++
	}
	if len(f.Countries) > 0 {
		conditions = append(conditions, "a.start_country = ANY($"+strconv.Itoa(idx)+")")
		args = append(args, f.Countries)
		idx++
	}
	if len(f.Intensity) > 0 {
		conditions = append(conditions, "a.intensity = ANY($"+strconv.Itoa(idx)+")")
		args = append(args, f.Intensity)
		idx++
	}
	if f.DistMin != nil {
		conditions = append(conditions, "a.distance_m >= $"+strconv.Itoa(idx))
		args = append(args, *f.DistMin*1000)
		idx++
	}
	if f.DistMax != nil {
		conditions = append(conditions, "a.distance_m <= $"+strconv.Itoa(idx))
		args = append(args, *f.DistMax*1000)
		idx++
	}
	if f.ElevMin != nil {
		conditions = append(conditions, "a.elevation_gain_m >= $"+strconv.Itoa(idx))
		args = append(args, *f.ElevMin)
		idx++
	}
	if f.ElevMax != nil {
		conditions = append(conditions, "a.elevation_gain_m <= $"+strconv.Itoa(idx))
		args = append(args, *f.ElevMax)
		idx++
	}
	if f.HRMin != nil {
		conditions = append(conditions, "a.avg_heart_rate >= $"+strconv.Itoa(idx))
		args = append(args, *f.HRMin)
		idx++
	}
	if f.HRMax != nil {
		conditions = append(conditions, "a.avg_heart_rate <= $"+strconv.Itoa(idx))
		args = append(args, *f.HRMax)
		idx++
	}
	if f.PowerMin != nil {
		conditions = append(conditions, "a.avg_power_w >= $"+strconv.Itoa(idx))
		args = append(args, *f.PowerMin)
		idx++
	}
	if f.PowerMax != nil {
		conditions = append(conditions, "a.avg_power_w <= $"+strconv.Itoa(idx))
		args = append(args, *f.PowerMax)
		idx++
	}
	if len(f.Weekdays) > 0 {
		conditions = append(conditions, "EXTRACT(DOW FROM a.started_at) = ANY($"+strconv.Itoa(idx)+")")
		args = append(args, f.Weekdays)
		idx++
	}
	if len(f.Seasons) > 0 {
		// Frühling=3,4,5 Sommer=6,7,8 Herbst=9,10,11 Winter=12,1,2
		var months []int
		for _, s := range f.Seasons {
			switch s {
			case "fruehling":
				months = append(months, 3, 4, 5)
			case "sommer":
				months = append(months, 6, 7, 8)
			case "herbst":
				months = append(months, 9, 10, 11)
			case "winter":
				months = append(months, 12, 1, 2)
			}
		}
		if len(months) > 0 {
			conditions = append(conditions, "EXTRACT(MONTH FROM a.started_at) = ANY($"+strconv.Itoa(idx)+")")
			args = append(args, months)
			idx++
		}
	}
	// TODO(weather-improvements Task 8): TempMin/TempMax and WeatherCondition filters
	// should query activity_weather table instead of removed flat columns.
	// Temporarily disabled to avoid reference to dropped columns.
	_ = f.TempMin
	_ = f.TempMax
	_ = f.WeatherCondition
	if len(f.TimeOfDay) > 0 {
		conditions = append(conditions, "a.time_of_day && $"+strconv.Itoa(idx))
		args = append(args, f.TimeOfDay)
		idx++
	}

	// Tri-state tag filters: activity must have ALL included tags
	if len(f.IncludeTags) > 0 {
		conditions = append(conditions, "a.id IN ("+
			"SELECT at2.activity_id FROM activity_tags at2 "+
			"JOIN tags t ON t.id = at2.tag_id "+
			"WHERE t.id = ANY($"+strconv.Itoa(idx)+"::uuid[]) "+
			"GROUP BY at2.activity_id "+
			"HAVING COUNT(DISTINCT t.id) = "+strconv.Itoa(len(f.IncludeTags))+")")
		args = append(args, f.IncludeTags)
		idx++
	}
	// Activity must have NONE of the excluded tags
	if len(f.ExcludeTags) > 0 {
		conditions = append(conditions, "a.id NOT IN ("+
			"SELECT at2.activity_id FROM activity_tags at2 "+
			"WHERE at2.tag_id = ANY($"+strconv.Itoa(idx)+"::uuid[]))")
		args = append(args, f.ExcludeTags)
		idx++
	}
	// Tri-state gear filters: activity must use one of the included gear items
	if len(f.IncludeGear) > 0 {
		conditions = append(conditions, "a.gear_id = ANY($"+strconv.Itoa(idx)+"::uuid[])")
		args = append(args, f.IncludeGear)
		idx++
	}
	// Activity must not use any of the excluded gear items
	if len(f.ExcludeGear) > 0 {
		conditions = append(conditions, "(a.gear_id IS NULL OR a.gear_id != ALL($"+strconv.Itoa(idx)+"::uuid[]))")
		args = append(args, f.ExcludeGear)
		idx++
	}
	if f.IsElectric != nil {
		conditions = append(conditions, "a.is_electric = $"+strconv.Itoa(idx))
		args = append(args, *f.IsElectric)
		idx++
	}
	if f.IsVirtual != nil {
		conditions = append(conditions, "a.is_virtual = $"+strconv.Itoa(idx))
		args = append(args, *f.IsVirtual)
		idx++
	}
	// Always exclude activities marked as excluded from stats in analytics aggregations.
	conditions = append(conditions, "NOT a.exclude_from_stats")
	// Always exclude group-secondary activities (duplicate recordings in recording groups).
	conditions = append(conditions, "NOT a.is_group_secondary")

	if len(conditions) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(conditions, " AND "), args
}

func parseOptFloat(s string) *float64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseOptInt(s string) *int {
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &v
}

func parseOptBool(s string) *bool {
	if s == "" {
		return nil
	}
	v := s == "true" || s == "1"
	return &v
}
