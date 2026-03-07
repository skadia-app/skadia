package rules

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

type ConditionGroup struct {
	Logic      string      `json:"logic"`
	Conditions []Condition `json:"conditions"`
}

type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

type Action struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
}

type ActivityChange struct {
	Field string
	Value any
}

func EvaluateGroups(groups []ConditionGroup, activity map[string]any) bool {
	for _, g := range groups {
		if !evalGroup(g, activity) {
			return false
		}
	}
	return true
}

func Evaluate(conditions []Condition, activity map[string]any) bool {
	for _, c := range conditions {
		if !evalCondition(c, activity) {
			return false
		}
	}
	return true
}

func evalGroup(g ConditionGroup, activity map[string]any) bool {
	if len(g.Conditions) == 0 {
		return true
	}
	if g.Logic == "or" {
		for _, c := range g.Conditions {
			if evalCondition(c, activity) {
				return true
			}
		}
		return false
	}
	for _, c := range g.Conditions {
		if !evalCondition(c, activity) {
			return false
		}
	}
	return true
}

func evalCondition(c Condition, activity map[string]any) bool {
	if c.Field == "geofence" {
		return evalGeofence(c, activity)
	}
	if c.Field == "geofence_zone" {
		return evalGeofenceZone(c, activity)
	}
	if c.Field == "geofence_proximity" {
		return evalGeofenceProximity(c, activity)
	}
	if c.Field == "avg_pace_min_km" || c.Field == "min_pace_min_km" || c.Field == "max_pace_min_km" {
		return evalPace(c, activity)
	}
	if c.Field == "season" {
		return evalSeason(c, activity)
	}
	if c.Field == "has_lift" {
		liftCount := toFloat(activity["lift_count"])
		match := liftCount > 0
		if c.Op == "eq" {
			want := fmt.Sprintf("%v", c.Value) == "true"
			return match == want
		}
		return match
	}
	if c.Field == "has_tag" {
		return evalHasTag(c, activity)
	}
	if c.Field == "has_segment_effort" || c.Field == "segment_effort_rank" || c.Field == "segment_effort_time_s" {
		return evalSegment(c, activity)
	}
	if c.Field == "time_of_day" || c.Field == "tags" {
		return evalArrayContains(c, activity)
	}

	val, ok := activity[c.Field]
	if !ok {
		return false
	}

	switch c.Op {
	case "eq":
		return fmt.Sprintf("%v", val) == fmt.Sprintf("%v", c.Value)
	case "neq":
		return fmt.Sprintf("%v", val) != fmt.Sprintf("%v", c.Value)
	case "gt":
		return toFloat(val) > toFloat(c.Value)
	case "lt":
		return toFloat(val) < toFloat(c.Value)
	case "gte":
		return toFloat(val) >= toFloat(c.Value)
	case "lte":
		return toFloat(val) <= toFloat(c.Value)
	case "contains":
		return strings.Contains(strings.ToLower(fmt.Sprintf("%v", val)), strings.ToLower(fmt.Sprintf("%v", c.Value)))
	case "not_contains":
		return !strings.Contains(strings.ToLower(fmt.Sprintf("%v", val)), strings.ToLower(fmt.Sprintf("%v", c.Value)))
	case "starts_with":
		return strings.HasPrefix(strings.ToLower(fmt.Sprintf("%v", val)), strings.ToLower(fmt.Sprintf("%v", c.Value)))
	case "ends_with":
		return strings.HasSuffix(strings.ToLower(fmt.Sprintf("%v", val)), strings.ToLower(fmt.Sprintf("%v", c.Value)))
	case "in":
		return evalIn(val, c.Value)
	case "not_in":
		return !evalIn(val, c.Value)
	case "is_null":
		return val == nil
	case "is_not_null":
		return val != nil
	default:
		return false
	}
}

func evalGeofence(c Condition, activity map[string]any) bool {
	params, ok := toStringMap(c.Value)
	if !ok {
		return false
	}
	lat := toFloat(params["lat"])
	lon := toFloat(params["lon"])
	radiusM := toFloat(params["radius_m"])
	mode, _ := params["mode"].(string)
	startLat, latOK := activity["start_lat"]
	startLon, lonOK := activity["start_lon"]
	if !latOK || !lonOK || startLat == nil || startLon == nil {
		return false
	}
	dist := haversineM(toFloat(startLat), toFloat(startLon), lat, lon)
	inside := dist <= radiusM
	if mode == "outside" {
		return !inside
	}
	return inside
}

// evalGeofenceZone checks whether the activity start point is inside (or outside)
// a drawn zone. The condition value must be a map with:
//
//	mode        "inside" | "outside"
//	shape       "circle" | "polygon"
//	lat, lon    float64 — center (circle only)
//	radius_m    float64 — radius in metres (circle only)
//	polygon     string  — GeoJSON Polygon string (polygon only)
//
// For polygon shapes a ray-casting point-in-polygon algorithm is used (no PostGIS
// dependency). The polygon coordinates are expected as [lng, lat] pairs (GeoJSON).
func evalGeofenceZone(c Condition, activity map[string]any) bool {
	params, ok := toStringMap(c.Value)
	if !ok {
		return false
	}
	mode, _ := params["mode"].(string)
	shape, _ := params["shape"].(string)
	startLat, latOK := activity["start_lat"]
	startLon, lonOK := activity["start_lon"]
	if !latOK || !lonOK || startLat == nil || startLon == nil {
		return false
	}
	sLat := toFloat(startLat)
	sLon := toFloat(startLon)

	var inside bool
	switch shape {
	case "polygon":
		polyStr, _ := params["polygon"].(string)
		if polyStr == "" {
			return false
		}
		inside = pointInGeoJSONPolygon(sLat, sLon, polyStr)
	default: // "circle" and legacy default
		lat := toFloat(params["lat"])
		lon := toFloat(params["lon"])
		radiusM := toFloat(params["radius_m"])
		inside = haversineM(sLat, sLon, lat, lon) <= radiusM
	}

	if mode == "outside" {
		return !inside
	}
	return inside
}

// evalGeofenceProximity checks whether the activity start point is within (or
// outside) a given radius of a pin point. Identical to the legacy evalGeofence
// but registered under a separate field name so both can coexist.
// Condition value must be a map with: lat, lon (float64), radius_m (float64),
// mode ("inside" | "outside").
func evalGeofenceProximity(c Condition, activity map[string]any) bool {
	params, ok := toStringMap(c.Value)
	if !ok {
		return false
	}
	lat := toFloat(params["lat"])
	lon := toFloat(params["lon"])
	radiusM := toFloat(params["radius_m"])
	mode, _ := params["mode"].(string)
	startLat, latOK := activity["start_lat"]
	startLon, lonOK := activity["start_lon"]
	if !latOK || !lonOK || startLat == nil || startLon == nil {
		return false
	}
	dist := haversineM(toFloat(startLat), toFloat(startLon), lat, lon)
	inside := dist <= radiusM
	if mode == "outside" {
		return !inside
	}
	return inside
}

// pointInGeoJSONPolygon performs a ray-casting point-in-polygon test against
// the first (outer) ring of a GeoJSON Polygon. The polygon coordinates are in
// [longitude, latitude] order as per the GeoJSON spec. We treat them as (x, y)
// for the 2-D test which is accurate enough for the small areas used in
// geofencing (error is negligible compared to the zone sizes involved).
func pointInGeoJSONPolygon(lat, lon float64, geoJSONStr string) bool {
	var poly struct {
		Coordinates [][][2]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal([]byte(geoJSONStr), &poly); err != nil || len(poly.Coordinates) == 0 {
		return false
	}
	ring := poly.Coordinates[0]
	// Ray-casting: count crossings of a horizontal ray from (lon, lat)
	x, y := lon, lat
	inside := false
	n := len(ring)
	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := ring[i][0], ring[i][1]
		xj, yj := ring[j][0], ring[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}

func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0
	p1 := lat1 * math.Pi / 180
	p2 := lat2 * math.Pi / 180
	dp := (lat2 - lat1) * math.Pi / 180
	dl := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func evalPace(c Condition, activity map[string]any) bool {
	var speedField string
	switch c.Field {
	case "min_pace_min_km":
		speedField = "max_speed_ms"
	case "max_pace_min_km":
		speedField = "min_speed_ms"
	default:
		speedField = "avg_speed_ms"
	}
	speedMS := toFloat(activity[speedField])
	if speedMS <= 0 {
		return false
	}
	paceMinKm := 1000.0 / (speedMS * 60.0)
	want := toFloat(c.Value)
	switch c.Op {
	case "lte":
		return paceMinKm <= want
	case "lt":
		return paceMinKm < want
	case "gte":
		return paceMinKm >= want
	case "gt":
		return paceMinKm > want
	case "eq":
		return math.Abs(paceMinKm-want) < 0.01
	}
	return false
}

func evalSeason(c Condition, activity map[string]any) bool {
	month := int(toFloat(activity["month"]))
	var actual string
	switch {
	case month >= 3 && month <= 5:
		actual = "spring"
	case month >= 6 && month <= 8:
		actual = "summer"
	case month >= 9 && month <= 11:
		actual = "autumn"
	default:
		actual = "winter"
	}
	want := fmt.Sprintf("%v", c.Value)
	if c.Op == "eq" {
		return actual == want
	}
	if c.Op == "neq" {
		return actual != want
	}
	return false
}

func evalHasTag(c Condition, activity map[string]any) bool {
	tags, ok := activity["tags"]
	if !ok {
		return false
	}
	wantTag := fmt.Sprintf("%v", c.Value)
	switch ts := tags.(type) {
	case []string:
		for _, t := range ts {
			if t == wantTag {
				return true
			}
		}
	case []any:
		for _, t := range ts {
			if fmt.Sprintf("%v", t) == wantTag {
				return true
			}
		}
	}
	return false
}

func evalArrayContains(c Condition, activity map[string]any) bool {
	val, ok := activity[c.Field]
	if !ok {
		return false
	}
	want := fmt.Sprintf("%v", c.Value)
	switch arr := val.(type) {
	case []string:
		for _, s := range arr {
			if s == want {
				return c.Op != "not_contains"
			}
		}
	case []any:
		for _, item := range arr {
			if fmt.Sprintf("%v", item) == want {
				return c.Op != "not_contains"
			}
		}
	}
	return c.Op == "not_contains"
}

func evalSegment(c Condition, activity map[string]any) bool {
	efforts, ok := activity["segment_efforts"]
	if !ok {
		return false
	}
	var effortList []map[string]any
	switch e := efforts.(type) {
	case []map[string]any:
		effortList = e
	case []any:
		for _, item := range e {
			if m, ok := item.(map[string]any); ok {
				effortList = append(effortList, m)
			}
		}
	}
	params, _ := toStringMap(c.Value)
	switch c.Field {
	case "has_segment_effort":
		wantID := segmentID(c.Value, params)
		for _, e := range effortList {
			if fmt.Sprintf("%v", e["segment_id"]) == wantID {
				return true
			}
		}
		return false
	case "segment_effort_rank":
		wantID := segmentID(c.Value, params)
		want := toFloat(params["rank"])
		for _, e := range effortList {
			if fmt.Sprintf("%v", e["segment_id"]) == wantID {
				return compareFloat(toFloat(e["rank"]), want, c.Op)
			}
		}
		return false
	case "segment_effort_time_s":
		wantID := segmentID(c.Value, params)
		want := toFloat(params["time_s"])
		for _, e := range effortList {
			if fmt.Sprintf("%v", e["segment_id"]) == wantID {
				return compareFloat(toFloat(e["elapsed_time_s"]), want, c.Op)
			}
		}
		return false
	}
	return false
}

func segmentID(raw any, params map[string]any) string {
	if params != nil {
		if id, ok := params["segment_id"]; ok {
			return fmt.Sprintf("%v", id)
		}
	}
	return fmt.Sprintf("%v", raw)
}

func compareFloat(actual, want float64, op string) bool {
	switch op {
	case "eq":
		return actual == want
	case "lte":
		return actual <= want
	case "lt":
		return actual < want
	case "gte":
		return actual >= want
	case "gt":
		return actual > want
	}
	return false
}

func Apply(actions []Action) []ActivityChange {
	var changes []ActivityChange
	for _, a := range actions {
		switch a.Action {
		case "set_gear":
			var p struct{ GearID string `json:"gear_id"` }
			if json.Unmarshal(a.Params, &p) == nil && p.GearID != "" {
				changes = append(changes, ActivityChange{Field: "gear_id", Value: p.GearID})
			}
		case "add_tag":
			var p struct{ TagID string `json:"tag_id"` }
			if json.Unmarshal(a.Params, &p) == nil && p.TagID != "" {
				changes = append(changes, ActivityChange{Field: "add_tag", Value: p.TagID})
			}
		case "remove_tag":
			var p struct{ TagID string `json:"tag_id"` }
			if json.Unmarshal(a.Params, &p) == nil && p.TagID != "" {
				changes = append(changes, ActivityChange{Field: "remove_tag", Value: p.TagID})
			}
		case "set_visibility":
			var p struct{ Visibility string `json:"visibility"` }
			if json.Unmarshal(a.Params, &p) == nil && p.Visibility != "" {
				changes = append(changes, ActivityChange{Field: "visibility", Value: p.Visibility})
			}
		case "set_sport_type":
			var p struct {
				SportType string `json:"sport_type"`
				SubType   string `json:"sub_type"`
			}
			if json.Unmarshal(a.Params, &p) == nil && p.SportType != "" {
				changes = append(changes, ActivityChange{Field: "sport_type", Value: p.SportType})
				if p.SubType != "" {
					changes = append(changes, ActivityChange{Field: "sub_type", Value: p.SubType})
				}
			}
		case "set_name":
			var p struct{ Template string `json:"template"` }
			if json.Unmarshal(a.Params, &p) == nil && p.Template != "" {
				changes = append(changes, ActivityChange{Field: "set_name_template", Value: p.Template})
			}
		case "set_intensity":
			var p struct{ Intensity string `json:"intensity"` }
			if json.Unmarshal(a.Params, &p) == nil && p.Intensity != "" {
				changes = append(changes, ActivityChange{Field: "intensity", Value: p.Intensity})
			}
		case "set_indoor":
			var p struct{ Indoor bool `json:"indoor"` }
			if json.Unmarshal(a.Params, &p) == nil {
				changes = append(changes, ActivityChange{Field: "is_indoor", Value: p.Indoor})
			}
		}
	}
	return changes
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func evalIn(val, list any) bool {
	s := fmt.Sprintf("%v", val)
	switch items := list.(type) {
	case []any:
		for _, item := range items {
			if fmt.Sprintf("%v", item) == s {
				return true
			}
		}
	case []string:
		for _, item := range items {
			if item == s {
				return true
			}
		}
	}
	return false
}

func toStringMap(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, false
	}
	return m, true
}
