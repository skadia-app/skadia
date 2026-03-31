package rules

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/planar"

	"github.com/skadia-app/skadia/services/api/internal/geo"
)

// compareValues vergleicht zwei Werte typbasiert ohne fmt.Sprintf-Allokation.
func compareValues(a, b any) bool {
	switch va := a.(type) {
	case string:
		if vb, ok := b.(string); ok {
			return va == vb
		}
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	case float64:
		switch vb := b.(type) {
		case float64:
			return va == vb
		case json.Number:
			if f, err := vb.Float64(); err == nil {
				return va == f
			}
		}
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	case json.Number:
		switch vb := b.(type) {
		case json.Number:
			return va.String() == vb.String()
		case float64:
			if f, err := va.Float64(); err == nil {
				return f == vb
			}
		case string:
			return va.String() == vb
		}
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	case bool:
		if vb, ok := b.(bool); ok {
			return va == vb
		}
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// toStringValue konvertiert einen Wert in einen String ohne fmt.Sprintf wenn möglich.
func toStringValue(v any) string {
	switch tv := v.(type) {
	case string:
		return tv
	case float64:
		return strconv.FormatFloat(tv, 'f', -1, 64)
	case json.Number:
		return tv.String()
	case bool:
		return strconv.FormatBool(tv)
	case int:
		return strconv.Itoa(tv)
	case int64:
		return strconv.FormatInt(tv, 10)
	}
	return fmt.Sprintf("%v", v)
}

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

// EvaluateGroups evaluates a slice of ConditionGroups against an activity map.
// Each group is evaluated independently by evalGroup (which respects the
// group's own "logic" field for AND/OR within the group). Multiple groups are
// always combined with AND: all groups must match for the overall result to be
// true. OR logic between groups is not supported; use a single group with
// logic="or" if you need any-condition semantics.
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
			want := toStringValue(c.Value) == "true"
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
	if c.Field == "time_of_day" || c.Field == "tags" || c.Field == "has_sensor_type" || c.Field == "sensor_device_ids" {
		return evalArrayContains(c, activity)
	}

	val, ok := activity[c.Field]
	if !ok {
		return false
	}

	switch c.Op {
	case "eq":
		return compareValues(val, c.Value)
	case "neq":
		return !compareValues(val, c.Value)
	case "gt":
		return toFloat(val) > toFloat(c.Value)
	case "lt":
		return toFloat(val) < toFloat(c.Value)
	case "gte":
		return toFloat(val) >= toFloat(c.Value)
	case "lte":
		return toFloat(val) <= toFloat(c.Value)
	case "contains":
		return strings.Contains(strings.ToLower(toStringValue(val)), strings.ToLower(toStringValue(c.Value)))
	case "not_contains":
		return !strings.Contains(strings.ToLower(toStringValue(val)), strings.ToLower(toStringValue(c.Value)))
	case "starts_with":
		return strings.HasPrefix(strings.ToLower(toStringValue(val)), strings.ToLower(toStringValue(c.Value)))
	case "ends_with":
		return strings.HasSuffix(strings.ToLower(toStringValue(val)), strings.ToLower(toStringValue(c.Value)))
	case "in":
		return evalIn(val, c.Value)
	case "not_in":
		return !evalIn(val, c.Value)
	case "is_null":
		return val == nil
	case "is_not_null":
		return val != nil
	case "between":
		parts := strings.SplitN(toStringValue(c.Value), ",", 2)
		if len(parts) != 2 {
			return false
		}
		fVal := toFloat(val)
		lo, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		hi, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 != nil || err2 != nil {
			return false
		}
		return fVal >= lo && fVal <= hi
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
	dist := HaversineM(toFloat(startLat), toFloat(startLon), lat, lon)
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
		inside = HaversineM(sLat, sLon, lat, lon) <= radiusM
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
	dist := HaversineM(toFloat(startLat), toFloat(startLon), lat, lon)
	inside := dist <= radiusM
	if mode == "outside" {
		return !inside
	}
	return inside
}

// pointInGeoJSONPolygon checks whether (lat, lon) lies inside a GeoJSON
// Polygon using paulmach/orb/planar.PolygonContains.
func pointInGeoJSONPolygon(lat, lon float64, geoJSONStr string) bool {
	g, err := geojson.UnmarshalGeometry([]byte(geoJSONStr))
	if err != nil {
		return false
	}
	poly, ok := g.Geometry().(orb.Polygon)
	if !ok {
		return false
	}
	return planar.PolygonContains(poly, orb.Point{lon, lat})
}

// HaversineM returns the great-circle distance in meters between two WGS-84 points.
// Delegates to geo.DistanceHaversine.
func HaversineM(lat1, lon1, lat2, lon2 float64) float64 {
	return geo.DistanceHaversine(lat1, lon1, lat2, lon2)
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
	want := toStringValue(c.Value)
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
		return c.Op == "neq" // no tags → tag is absent → neq is true
	}
	wantTag := toStringValue(c.Value)
	found := false
	switch ts := tags.(type) {
	case []string:
		found = slices.Contains(ts, wantTag)
	case []any:
		for _, t := range ts {
			if toStringValue(t) == wantTag {
				found = true
				break
			}
		}
	}
	if c.Op == "neq" {
		return !found
	}
	return found
}

func evalArrayContains(c Condition, activity map[string]any) bool {
	val, ok := activity[c.Field]
	if !ok {
		return false
	}
	want := toStringValue(c.Value)
	switch arr := val.(type) {
	case []string:
		if slices.Contains(arr, want) {
			return c.Op != "not_contains"
		}
	case []any:
		for _, item := range arr {
			if toStringValue(item) == want {
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
			if toStringValue(e["segment_id"]) == wantID {
				return true
			}
		}
		return false
	case "segment_effort_rank":
		wantID := segmentID(c.Value, params)
		want := toFloat(params["rank"])
		for _, e := range effortList {
			if toStringValue(e["segment_id"]) == wantID {
				return compareFloat(toFloat(e["rank"]), want, c.Op)
			}
		}
		return false
	case "segment_effort_time_s":
		wantID := segmentID(c.Value, params)
		want := toFloat(params["time_s"])
		for _, e := range effortList {
			if toStringValue(e["segment_id"]) == wantID {
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
			return toStringValue(id)
		}
	}
	return toStringValue(raw)
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

// actionParams is the consolidated struct for all action parameter fields.
// A single Unmarshal covers all action types, eliminating repeated per-case allocations.
type actionParams struct {
	GearID     string `json:"gear_id"`
	TagID      string `json:"tag_id"`
	Visibility string `json:"visibility"`
	SportType  string `json:"sport_type"`
	SubType    string `json:"sub_type"`
	Template   string `json:"template"`
	Intensity  string `json:"intensity"`
	Indoor     bool   `json:"indoor"`
	Field      string `json:"field"`
	Value      any    `json:"value"`
}

func Apply(actions []Action) []ActivityChange {
	var changes []ActivityChange
	for _, a := range actions {
		var p actionParams
		if err := json.Unmarshal(a.Params, &p); err != nil {
			continue
		}
		switch a.Action {
		case "set_gear":
			if p.GearID != "" {
				changes = append(changes, ActivityChange{Field: "gear_id", Value: p.GearID})
			}
		case "add_tag":
			if p.TagID != "" {
				changes = append(changes, ActivityChange{Field: "add_tag", Value: p.TagID})
			}
		case "remove_tag":
			if p.TagID != "" {
				changes = append(changes, ActivityChange{Field: "remove_tag", Value: p.TagID})
			}
		case "set_visibility":
			if p.Visibility != "" {
				changes = append(changes, ActivityChange{Field: "visibility", Value: p.Visibility})
			}
		case "set_sport_type":
			if p.SportType != "" {
				changes = append(changes, ActivityChange{Field: "sport_type", Value: p.SportType})
				if p.SubType != "" {
					changes = append(changes, ActivityChange{Field: "sub_type", Value: p.SubType})
				}
			}
		case "set_name":
			if p.Template != "" {
				changes = append(changes, ActivityChange{Field: "set_name_template", Value: p.Template})
			}
		case "set_intensity":
			if p.Intensity != "" {
				changes = append(changes, ActivityChange{Field: "intensity", Value: p.Intensity})
			}
		case "set_indoor":
			changes = append(changes, ActivityChange{Field: "is_indoor", Value: p.Indoor})
		case "set_field":
			if p.Field != "" {
				// Whitelist: nur erlaubte Felder
				switch p.Field {
				case "is_round_trip", "is_indoor":
					changes = append(changes, ActivityChange{Field: p.Field, Value: p.Value})
				}
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
	s := toStringValue(val)
	switch items := list.(type) {
	case []any:
		for _, item := range items {
			if toStringValue(item) == s {
				return true
			}
		}
	case []string:
		if slices.Contains(items, s) {
			return true
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
