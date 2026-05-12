package tools

import (
	"fmt"
	"math"
	"time"
)

func formatDuration(seconds any) string {
	if !truthyNumber(seconds) {
		return "0min"
	}
	mins := int(numberValue(seconds)) / 60
	if mins >= 60 {
		return fmt.Sprintf("%dh%02dmin", mins/60, mins%60)
	}
	return fmt.Sprintf("%dmin", mins)
}

func formatActivityCompact(row map[string]any) map[string]any {
	distanceKM := math.Round((numberValue(row["distance_m"])/1000.0)*10) / 10
	durationS := row["elapsed_time_s"]
	if !truthyNumber(durationS) {
		durationS = row["duration_s"]
	}
	speed := numberValue(row["avg_speed_ms"])

	sportLabel := stringValue(row["sport_type"])
	if subType := stringValue(row["sub_type"]); subType != "" {
		sportLabel = fmt.Sprintf("%s/%s", sportLabel, subType)
	}
	modifiers := make([]string, 0, 4)
	for _, item := range []struct {
		field string
		label string
	}{
		{"is_electric", "electric"},
		{"is_indoor", "indoor"},
		{"is_virtual", "virtual"},
		{"is_commute", "commute"},
	} {
		if boolValue(row[item.field]) {
			modifiers = append(modifiers, item.label)
		}
	}
	if len(modifiers) > 0 {
		sportLabel = fmt.Sprintf("%s (%s)", sportLabel, joinLabels(modifiers))
	}

	result := map[string]any{
		"id":          stringValue(row["id"]),
		"date":        dateValue(row["started_at"]),
		"title":       stringValue(row["title"]),
		"sport":       sportLabel,
		"km":          distanceKM,
		"duration":    formatDuration(durationS),
		"hr":          row["avg_heart_rate"],
		"elevation_m": math.Round(numberValue(row["elevation_gain_m"])),
		"city":        stringValue(row["start_city"]),
	}

	if row["sport_type"] == "running" || row["sport_type"] == "hiking" || row["sport_type"] == "walking" {
		if speed > 0 {
			paceMinKM := (1000.0 / speed) / 60.0
			result["pace"] = fmt.Sprintf("%d:%02d min/km", int(paceMinKM), int(math.Mod(paceMinKM, 1)*60))
		}
	} else if speed > 0 {
		result["speed"] = fmt.Sprintf("%.1f km/h", math.Round(speed*36)/10)
	}

	return result
}

func numberValue(value any) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case float32:
		return float64(v)
	case float64:
		return v
	default:
		return 0
	}
}

func truthyNumber(value any) bool {
	return numberValue(value) != 0
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func boolValue(value any) bool {
	v, ok := value.(bool)
	return ok && v
}

func dateValue(value any) string {
	switch v := value.(type) {
	case time.Time:
		if v.IsZero() {
			return ""
		}
		return v.Format("02.01.2006")
	case *time.Time:
		if v == nil || v.IsZero() {
			return ""
		}
		return v.Format("02.01.2006")
	default:
		return ""
	}
}

func joinLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	out := labels[0]
	for _, label := range labels[1:] {
		out += ", " + label
	}
	return out
}
