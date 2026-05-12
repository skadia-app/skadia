package agent

import (
	"regexp"
	"strconv"
	"strings"
)

type Template struct {
	Name     string
	SQL      string
	Format   string
	patterns []*regexp.Regexp
	params   func([]string) []any
}

type TemplateMatch struct {
	Name     string
	Template *Template
	Params   []any
}

var templates = []*Template{
	{
		Name:   "last_activities",
		SQL:    "SELECT id, title, sport_type, started_at, distance_m, duration_s FROM activities WHERE user_id = $1 ORDER BY started_at DESC LIMIT $2",
		Format: "Hier sind deine letzten {n} Aktivitäten:",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`letzte\s+(\d+)?\s*aktivität`),
			regexp.MustCompile(`recent\s+(\d+)?\s*activit`),
		},
		params: func(groups []string) []any {
			n := 5
			if len(groups) > 1 && groups[1] != "" {
				if parsed, err := strconv.Atoi(groups[1]); err == nil {
					n = parsed
				}
			}
			return []any{n}
		},
	},
	{
		Name: "weekly_summary",
		SQL: `
            SELECT CASE WHEN sub_type IS NULL OR sub_type = ''
                        THEN sport_type ELSE sport_type || '/' || sub_type END AS sport_tuple,
                   sport_type, sub_type, COUNT(*) AS count,
                   ROUND(SUM(distance_m)/1000.0, 1) AS km,
                   ROUND(SUM(duration_s)/3600.0, 1) AS hours
            FROM activities
            WHERE user_id = $1 AND started_at >= NOW() - INTERVAL '7 days'
            GROUP BY sport_type, sub_type ORDER BY count DESC
        `,
		Format: "Wochenzusammenfassung:",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`wochenzu`),
			regexp.MustCompile(`diese\s*woche`),
			regexp.MustCompile(`weekly\s*summary`),
		},
		params: func([]string) []any { return []any{} },
	},
	{
		Name: "hrv_trend",
		SQL: `
            SELECT measured_at_utc::date AS date, rmssd_ms, sdrr_ms
            FROM health_snapshots
            WHERE user_id = $1
            ORDER BY measured_at_utc DESC LIMIT 14
        `,
		Format: "HRV-Trend der letzten 14 Tage:",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`hrv.*(trend|verlauf)`),
			regexp.MustCompile(`herzratenvariab`),
		},
		params: func([]string) []any { return []any{} },
	},
	{
		Name: "readiness",
		SQL: `
            SELECT data FROM analytics_cache
            WHERE user_id = $1 AND cache_key = 'readiness'
            ORDER BY computed_at DESC LIMIT 1
        `,
		Format: "Aktuelle Readiness:",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`readiness|erholung|wie\s*(geht|fit|erholt)`),
		},
		params: func([]string) []any { return []any{} },
	},
}

func MatchTemplate(message string) *TemplateMatch {
	msg := strings.ToLower(strings.TrimSpace(message))
	for _, tmpl := range templates {
		for _, pattern := range tmpl.patterns {
			if match := pattern.FindStringSubmatch(msg); match != nil {
				return &TemplateMatch{Name: tmpl.Name, Template: tmpl, Params: tmpl.params(match)}
			}
		}
	}
	return nil
}
