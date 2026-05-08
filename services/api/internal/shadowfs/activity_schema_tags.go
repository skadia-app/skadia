package shadowfs

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

type ActivitySchemaTagsInput struct {
	Format         string
	Source         string
	StartedAt      time.Time
	SportType      string
	SubType        string
	Title          string
	TitleAuto      string
	City           string
	Country        string
	DistanceM      float64
	ElapsedTimeS   int
	ElevationGainM float64
	IsElectric     bool
	IsIndoor       bool
	IsVirtual      bool
	IsCommute      bool
}

func ActivitySchemaTags(input ActivitySchemaTagsInput) map[string]string {
	tags := map[string]string{}
	if input.Format != "" {
		tags["format"] = input.Format
	}
	if input.Source != "" {
		tags["source"] = input.Source
	}
	if !input.StartedAt.IsZero() {
		tags["year"] = input.StartedAt.Format("2006")
		tags["month"] = input.StartedAt.Format("01")
		tags["day"] = input.StartedAt.Format("02")
		tags["time"] = input.StartedAt.Format("1504")
	}

	tuple, _, err := sport.NormalizeSportTupleForImport(input.SportType, input.SubType)
	if err != nil {
		tags["sport"] = "unknown"
	} else {
		tags["sport"] = tuple.SportType
		if tuple.SportType != "multisport" && tuple.SubType != "" {
			tags["subCategory"] = tuple.SubType
		}
	}

	if title := firstNonBlank(input.Title, input.TitleAuto); title != "" {
		tags["title"] = title
	}
	if city := strings.TrimSpace(input.City); city != "" {
		tags["city"] = city
	}
	if country := strings.TrimSpace(input.Country); country != "" {
		tags["country"] = country
	}
	if input.DistanceM > 0 && isFinite(input.DistanceM) {
		tags["distance_km"] = strconv.FormatFloat(input.DistanceM/1000.0, 'f', 1, 64)
	}
	if input.ElapsedTimeS > 0 {
		h := input.ElapsedTimeS / 3600
		m := (input.ElapsedTimeS % 3600) / 60
		tags["duration_h"] = fmt.Sprintf("%04d-%02d", h, m)
	}
	if input.ElevationGainM > 0 && isFinite(input.ElevationGainM) {
		tags["elevation_m"] = strconv.Itoa(int(input.ElevationGainM))
	}
	tags["modifiers"] = strings.Join(activitySchemaModifierTokens(input), "_")
	return tags
}

func activitySchemaModifierTokens(input ActivitySchemaTagsInput) []string {
	modifiers := []string{}
	for _, key := range ShadowfsApplicableModifierKeys {
		modifier, ok := sport.SportModifierByKey(key)
		if !ok || !activitySchemaModifierActive(input, modifier.GoField) {
			continue
		}
		modifiers = append(modifiers, modifier.ShadowFSToken)
	}
	return modifiers
}

func maxActivitySchemaModifiersTag() string {
	tokens := []string{}
	for _, key := range ShadowfsApplicableModifierKeys {
		modifier, ok := sport.SportModifierByKey(key)
		if ok {
			tokens = append(tokens, modifier.ShadowFSToken)
		}
	}
	return strings.Join(tokens, "_")
}

func activitySchemaModifierActive(input ActivitySchemaTagsInput, goField string) bool {
	field := reflect.ValueOf(input).FieldByName(goField)
	return field.IsValid() && field.Kind() == reflect.Bool && field.Bool()
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
