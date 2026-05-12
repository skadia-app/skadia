package tools

import "fmt"

type ActivityFilters struct {
	TableAlias string
	SportType  string
	SubType    *string
	IsElectric *bool
	IsIndoor   *bool
	IsVirtual  *bool
	IsCommute  *bool
}

type SegmentEffortFilters struct {
	SportType     string
	SubType       *string
	IsElectric    *bool
	IsIndoor      *bool
	IsVirtual     *bool
	IsCommute     *bool
	InLeaderboard *bool
}

func applyActivityFilters(conditions []string, params []any, filters ActivityFilters) ([]string, []any) {
	prefix := ""
	if filters.TableAlias != "" {
		prefix = filters.TableAlias + "."
	}
	if filters.SportType != "" {
		params = append(params, filters.SportType)
		conditions = append(conditions, fmt.Sprintf("%ssport_type = $%d", prefix, len(params)))
	}
	if filters.SubType != nil {
		if *filters.SubType == "" {
			conditions = append(conditions, prefix+"sub_type IS NULL")
		} else {
			params = append(params, *filters.SubType)
			conditions = append(conditions, fmt.Sprintf("%ssub_type = $%d", prefix, len(params)))
		}
	}
	for _, item := range []struct {
		field string
		value *bool
	}{
		{"is_electric", filters.IsElectric},
		{"is_indoor", filters.IsIndoor},
		{"is_virtual", filters.IsVirtual},
		{"is_commute", filters.IsCommute},
	} {
		if item.value == nil {
			continue
		}
		params = append(params, *item.value)
		conditions = append(conditions, fmt.Sprintf("%s%s = $%d", prefix, item.field, len(params)))
	}
	return conditions, params
}

func applySegmentEffortFilters(conditions []string, params []any, filters SegmentEffortFilters) ([]string, []any) {
	if filters.SportType != "" {
		params = append(params, filters.SportType)
		conditions = append(conditions, fmt.Sprintf("se.sport_type = $%d", len(params)))
	}
	if filters.SubType != nil {
		if *filters.SubType == "" {
			conditions = append(conditions, "(se.sub_type IS NULL OR se.sub_type = 'generic')")
		} else {
			params = append(params, *filters.SubType)
			conditions = append(conditions, fmt.Sprintf("se.sub_type = $%d", len(params)))
		}
	}
	if filters.InLeaderboard != nil {
		params = append(params, *filters.InLeaderboard)
		conditions = append(conditions, fmt.Sprintf("se.in_leaderboard = $%d", len(params)))
	}
	return applyActivityFilters(conditions, params, ActivityFilters{
		TableAlias: "a",
		IsElectric: filters.IsElectric,
		IsIndoor:   filters.IsIndoor,
		IsVirtual:  filters.IsVirtual,
		IsCommute:  filters.IsCommute,
	})
}
