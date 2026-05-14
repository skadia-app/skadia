package agent

import (
	"reflect"
	"testing"
)

func TestRouteQueryMatchesPythonRouterCases(t *testing.T) {
	small := GetProfile("small", "", nil)
	tests := []struct {
		query      string
		wantDomain string
	}{
		{"Wie viele Gipfel habe ich bestiegen?", "geo"},
		{"Wie ist mein HRV-Trend?", "health"},
		{"Wann war mein letzter Lauf?", "activities"},
		{"When was my last swim?", "activities"},
		{"Wie viele km haben meine Schuhe?", "gear"},
		{"Wie ist meine CTL?", "training"},
		{"Was ist meine Bestzeit auf dem Segment?", "segments"},
		{"Plane mir eine E-Bike Route", "routing"},
		{"Zeig mir Fotos mit Steinbock", "photos"},
		{"Suche in meinen Rechnungen nach GX Kette", "documents"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := RouteQuery(tt.query, &small)
			if !containsString(got.Domains, tt.wantDomain) {
				t.Fatalf("domains = %#v, want %q", got.Domains, tt.wantDomain)
			}
			if len(got.Domains) > 2 {
				t.Fatalf("domains = %#v, want max 2", got.Domains)
			}
		})
	}
}

func TestRouteQueryMatchesSubjectiveRoutingPhrases(t *testing.T) {
	small := GetProfile("small", "", nil)
	tests := []string{
		"Plane mir eine schoene Gravel-Runde mit wenig Verkehr",
		"Ich brauche eine Pendeln-Route",
		"Finde eine Trail-Route mit viel Wald",
		"Plane eine Radtour von Startort nach Zielort an einem See vorbei",
		"Ich brauche eine direkte Route von Dorf A zum Waldsee",
		"Finde eine Rundtour mit Fluss, Wald und moderatem Anstieg",
	}
	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			got := RouteQuery(query, &small)
			if !containsString(got.Domains, "routing") {
				t.Fatalf("domains = %#v, want routing", got.Domains)
			}
		})
	}
}

func TestRoutingDomainIncludesPlanningToolsWithoutWrites(t *testing.T) {
	small := GetProfile("small", "", nil)
	got := RouteQuery("Plane eine direkte Wanderroute von Startort zu einem See", &small)
	if len(got.Domains) == 0 || got.Domains[0] != "routing" {
		t.Fatalf("domains = %#v, want routing first", got.Domains)
	}

	tools := GetToolsForDomains(got.Domains, &small)
	for _, want := range []string{
		"resolve_route_place",
		"resolve_route_feature",
		"get_routing_coverage",
		"list_routing_profiles",
		"plan_route_between",
		"suggest_route_goal",
		"query_weather_at_point_time",
		"query_activity_terrain_summary",
		"plan_route",
		"query_coverage",
	} {
		if !containsString(tools, want) {
			t.Fatalf("tools = %#v, want %q", tools, want)
		}
	}
	if containsString(tools, "save_planned_route") {
		t.Fatalf("tools = %#v, did not want write tool save_planned_route", tools)
	}
}

func TestRouteQueryPrioritizesSegmentBestzeitOverActivityTitleSearch(t *testing.T) {
	small := GetProfile("small", "", nil)
	got := RouteQuery("Wann bin ich meine Bestzeit auf dem Neuland Trail gefahren?", &small)
	if len(got.Domains) == 0 || got.Domains[0] != "segments" {
		t.Fatalf("domains = %#v, want segments first", got.Domains)
	}
	tools := GetToolsForDomains(got.Domains, &small)
	if len(tools) == 0 || tools[0] != "query_segments" {
		t.Fatalf("tools = %#v, want query_segments first", tools)
	}
}

func TestRouteQueryPrioritizesSegmentPROverRoutingKeywords(t *testing.T) {
	small := GetProfile("small", "", nil)
	got := RouteQuery("Habe ich einen Segment PR auf der direkten Route am Waldsee?", &small)
	if len(got.Domains) == 0 || got.Domains[0] != "segments" {
		t.Fatalf("domains = %#v, want segments first", got.Domains)
	}
	tools := GetToolsForDomains(got.Domains, &small)
	if len(tools) == 0 || tools[0] != "query_segments" {
		t.Fatalf("tools = %#v, want query_segments first", tools)
	}
}

func TestRouteQueryFallbackAndLargeProfile(t *testing.T) {
	small := GetProfile("small", "", nil)
	if got := RouteQuery("Hallo", &small); !reflect.DeepEqual(got.Domains, []string{"activities", "health"}) || got.Confidence != 0.3 {
		t.Fatalf("fallback = %+v", got)
	}

	large := GetProfile("large", "", nil)
	got := RouteQuery("Wie viele Gipfel?", &large)
	if !reflect.DeepEqual(got.Domains, AllDomains) || got.Confidence != 1.0 {
		t.Fatalf("large route = %+v, want all domains", got)
	}
}

func TestGetToolsForDomainsAddsRunSQLOnlyWhenEnabled(t *testing.T) {
	small := GetProfile("small", "", nil)
	got := GetToolsForDomains([]string{"activities"}, &small)
	if !containsString(got, "query_activities") || !containsString(got, "run_sql") {
		t.Fatalf("tools = %#v, want query_activities and run_sql", got)
	}

	tiny := GetProfile("tiny", "", nil)
	got = GetToolsForDomains([]string{"activities"}, &tiny)
	if containsString(got, "run_sql") {
		t.Fatalf("tools = %#v, did not want run_sql", got)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
