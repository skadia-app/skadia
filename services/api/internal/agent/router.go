package agent

import (
	"sort"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

type RouteResult struct {
	Domains    []string
	Confidence float64
}

var DomainKeywords = map[string][]string{
	"activities": {
		"aktivität", "aktivitäten", "training", "trainings",
		"lauf", "laufen", "joggen", "gelaufen",
		"rad", "radfahren", "radtour", "radtouren", "biken", "bike", "fahrrad", "gefahren",
		"tour", "touren", "fahrt", "fahrten",
		"mtb", "mountainbike", "gravel",
		"e-bike", "ebike", "e bike", "electric", "elektrisch",
		"indoor", "virtuell", "virtual",
		"pendel", "pendeln", "pendelaktivität", "pendelaktivitäten", "commute",
		"trail",
		"wann war", "when was", "wie oft", "how often", "letzte", "letzten", "letztes", "last", "latest", "recent",
		"schnellste", "fastest", "längste", "longest", "kürzeste", "shortest", "weiteste", "furthest",
		"trend", "trends", "zeitreihe", "zeitreihen", "pro jahr", "pro monat", "jahresvergleich", "bestwert", "bestwerte",
		"regel", "regeln", "automation", "automatisierung", "dry-run", "vorschau",
	},
	"health": {
		"hrv", "herzratenvariabilität", "herzfrequenzvariabilität",
		"schlaf", "geschlafen", "schlafen",
		"erholung", "erholt", "erholungszeit",
		"puls", "ruhepuls", "herzfrequenz",
		"readiness", "bereitschaft",
		"müde", "müdigkeit", "erschöpft",
		"krank", "erkältung", "infekt",
		"stress", "gestresst",
		"body battery", "spo2", "sauerstoff",
		"gewicht", "körperfett",
	},
	"training": {
		"ctl", "atl", "tsb", "tss",
		"form", "fitness", "ermüdung",
		"belastung", "trainingsbelastung",
		"intervall", "tempo", "tempolauf",
		"zone", "zonen", "herzfrequenzzone",
		"acwr", "workload",
		"trainingsplan", "periodisierung",
		"übertraining", "überlastet",
	},
	"gear": {
		"schuh", "schuhe", "laufschuh", "laufschuhe",
		"rad", "fahrrad", "fahrräder", "räder", "rädern", "bike",
		"ausrüstung", "equipment", "gear",
		"verschleiß", "abnutzung",
		"km-stand", "kilometerstand",
		"helm", "brille", "uhr", "garmin",
		"batterie", "akku",
		"material", "reparatur", "wartung",
		"kette", "reifen", "kassette", "bremsbelag",
		"fällig", "überfällig", "service", "wartungshistorie", "komponente", "komponenten",
	},
	"documents": {
		"dokument", "dokumente", "rechnung", "rechnungen",
		"beleg", "belege", "quittung", "quittungen",
		"pdf", "ocr", "markdown", "textauszug", "extrahiert",
		"garantie", "handbuch", "manual", "anleitung",
	},
	"geo": {
		"gipfel", "berg", "berge", "summit",
		"summit", "summits",
		"pass", "pässe", "passüberquerung",
		"bestiegen", "besteigung", "besteigungen",
		"see", "seen", "fluss", "bach", "gewässer",
		"wald", "wälder", "forst",
		"hütte", "hütten", "alm", "alpen",
		"einkehr", "restaurant", "café", "gasthaus",
		"höhe", "höhenmeter", "aufstieg",
		"route", "routen", "strecke",
		"ort", "region", "stadt", "karte",
	},
	"segments": {
		"segment", "segmente",
		"bestzeit", "bestzeiten",
		"pr", "personal record",
		"rekord", "rekorde",
		"runde", "runden", "lap", "laps",
		"leaderboard", "rangliste",
		"effort", "efforts",
	},
	"photos": {
		"foto", "fotos", "bild", "bilder", "photo", "tier", "tiere",
		"landschaft", "gesehen", "steinbock", "murmeltier", "panorama",
	},
	"routing": {
		"route", "routen", "routenplanung", "navigation", "wegpunkt",
		"plane", "planen", "geplant", "wanderung planen", "radtour planen", "tour planen",
		"strecke", "weg", "pfad", "isochrone", "erreichbar",
		"wie komme ich", "kürzester weg", "schönste route", "rundtour",
		"schwierigkeit", "sac", "mtb skala", "steigung",
		"asphalt", "schotter", "profil", "gelände",
		"gravel", "trail", "wenig verkehr", "verkehrsarm", "schön", "schoen", "viel wald",
		"e-bike", "ebike", "e bike", "electric", "elektrisch",
		"indoor", "virtuell", "virtuelle", "virtual",
		"pendelroute", "pendeln", "commute",
	},
}

var DomainTools = map[string][]string{
	"activities": {"query_activities", "preview_activity_selection", "aggregate_stats", "aggregate_time_series", "query_records", "compare_periods", "get_activity_detail", "get_gear_for_activity", "dry_run_rule", "resolve_entity", "get_background_job_status"},
	"health":     {"get_health_status", "get_analytics"},
	"training":   {"get_training_load", "aggregate_time_series", "compare_periods", "get_analytics"},
	"gear": {
		"find_gear", "get_gear_stats", "get_gear_for_activity", "get_gear_documents", "search_documents",
		"gear_list", "gear_component_detail", "gear_service_events", "gear_summary", "query_gear_maintenance",
	},
	"documents": {"search_documents", "get_gear_documents", "find_gear", "resolve_entity"},
	"geo":       {"query_geo_features", "query_activity_geo_context", "search_routes", "resolve_entity"},
	"segments":  {"query_segments", "resolve_entity"},
	"photos":    {"search_photos_by_content"},
	"routing": {
		"search_pois_nearby", "search_trails_by_difficulty",
		"list_routing_profiles", "plan_route", "search_pois_on_route", "get_isochrone",
		"query_geo_features", "search_routes", "query_coverage", "resolve_entity",
	},
}

var GlobalTools = []string{"run_sql"}
var AllDomains = []string{"activities", "health", "training", "gear", "documents", "geo", "segments", "photos", "routing"}

func RouteQuery(query string, profile *ModelProfile) RouteResult {
	if profile == nil {
		p := GetProfile("small", "", nil)
		profile = &p
	}
	if !profile.UseRouter {
		return RouteResult{Domains: append([]string(nil), AllDomains...), Confidence: 1.0}
	}

	queryLower := strings.ToLower(query)
	scores := make(map[string]int)
	for domain, keywords := range DomainKeywords {
		for _, keyword := range keywords {
			if strings.Contains(queryLower, keyword) {
				scores[domain]++
			}
		}
	}
	applyIntentBoosts(queryLower, scores)
	if sport.QueryContainsKnownSport(query) {
		scores["activities"]++
	}
	if len(scores) == 0 {
		return RouteResult{Domains: []string{"activities", "health"}, Confidence: 0.3}
	}

	domains := make([]string, 0, len(scores))
	for domain := range scores {
		domains = append(domains, domain)
	}
	sort.SliceStable(domains, func(i, j int) bool {
		if scores[domains[i]] == scores[domains[j]] {
			return domains[i] < domains[j]
		}
		return scores[domains[i]] > scores[domains[j]]
	})
	if len(domains) > 2 {
		domains = domains[:2]
	}
	confidence := 0.7
	if scores[domains[0]] >= 2 {
		confidence = 0.9
	}
	return RouteResult{Domains: domains, Confidence: confidence}
}

func applyIntentBoosts(queryLower string, scores map[string]int) {
	if strings.Contains(queryLower, "bestzeit") ||
		strings.Contains(queryLower, "personal record") ||
		strings.Contains(queryLower, "persönlicher rekord") ||
		strings.Contains(queryLower, "schnellste zeit") {
		scores["segments"] += 5
	}
}

func GetToolsForDomains(domains []string, profile *ModelProfile) []string {
	if profile == nil {
		p := GetProfile("small", "", nil)
		profile = &p
	}

	toolSet := map[string]bool{}
	if !profile.UseRouter {
		for _, tools := range DomainTools {
			for _, tool := range tools {
				toolSet[tool] = true
			}
		}
		for _, tool := range GlobalTools {
			toolSet[tool] = true
		}
		return sortedKeys(toolSet)
	}

	for _, domain := range domains {
		for _, tool := range DomainTools[domain] {
			if !toolSet[tool] {
				toolSet[tool] = true
			}
		}
	}
	out := make([]string, 0, len(toolSet)+len(GlobalTools))
	for _, domain := range domains {
		for _, tool := range DomainTools[domain] {
			if toolSet[tool] {
				out = append(out, tool)
				toolSet[tool] = false
			}
		}
	}
	if profile.EnableSQLAgent {
		for _, tool := range GlobalTools {
			if !containsToolName(out, tool) {
				out = append(out, tool)
			}
		}
	}
	return out
}

func containsToolName(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
