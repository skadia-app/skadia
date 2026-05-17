package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/geosearch"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResolveUserLocationOwnerIsolationAndLeakedRefRejected(t *testing.T) {
	pool := testdb.Open(t)
	userA := insertActivityToolUser(t, pool)
	userB := insertActivityToolUser(t, pool)
	insertRouteLocationTag(t, pool, userA, "Synthetic Depot", "standort", 48.10, 11.10)
	insertRouteLocationTag(t, pool, userB, "Synthetic Depot", "standort", 49.10, 12.10)

	result, err := executeResolveUserLocation(context.Background(), &Deps{DB: &database.DB{Pool: pool}}, userA, json.RawMessage(`{
		"query": "Synthetic Depot",
		"category": "standort"
	}`))
	if err != nil {
		t.Fatalf("executeResolveUserLocation: %v", err)
	}
	got := result.(map[string]any)
	candidates := got["candidates"].([]map[string]any)
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want exactly the current user's match", candidates)
	}
	if candidates[0]["lat"] != 48.10 || candidates[0]["lng"] != 11.10 {
		t.Fatalf("candidate coordinates = %#v, want user A tag only", candidates[0])
	}

	leakedRef := candidates[0]["ref"].(string)
	analysis, err := executeAnalyzeRoutePoints(context.Background(), nil, userB, json.RawMessage(`{
		"points": [{"ref": "`+leakedRef+`"}, {"lat": 48.2, "lng": 11.2}],
		"intent": "direct_route"
	}`))
	if err != nil {
		t.Fatalf("executeAnalyzeRoutePoints leaked ref: %v", err)
	}
	analysisMap := analysis.(map[string]any)
	if analysisMap["long_route_status"] != "invalid_points" {
		t.Fatalf("leaked ref analysis = %#v, want invalid_points", analysisMap)
	}
	if !containsAnyString(analysisMap["blocking_reasons"], "invalid_or_foreign_route_ref") {
		t.Fatalf("blocking_reasons = %#v, want invalid_or_foreign_route_ref", analysisMap["blocking_reasons"])
	}
}

func TestResolveUserLocationAmbiguousMissingCoordinatesAndNotFound(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	insertRouteLocationTag(t, pool, userID, "Synthetic Start North", "standort", 48.10, 11.10)
	insertRouteLocationTag(t, pool, userID, "Synthetic Start South", "standort", 48.20, 11.20)
	insertRouteLocationTagWithoutCoords(t, pool, userID, "Synthetic Start Empty", "standort")

	result, err := executeResolveUserLocation(context.Background(), &Deps{DB: &database.DB{Pool: pool}}, userID, json.RawMessage(`{
		"query": "Synthetic Start",
		"limit": 10
	}`))
	if err != nil {
		t.Fatalf("executeResolveUserLocation ambiguous: %v", err)
	}
	got := result.(map[string]any)
	if got["ambiguous"] != true {
		t.Fatalf("ambiguous = %#v, want true for two coordinate matches: %#v", got["ambiguous"], got)
	}
	if got["skipped_missing_coordinates"] != 1 {
		t.Fatalf("skipped_missing_coordinates = %#v, want 1", got["skipped_missing_coordinates"])
	}

	none, err := executeResolveUserLocation(context.Background(), &Deps{DB: &database.DB{Pool: pool}}, userID, json.RawMessage(`{
		"query": "No Such Synthetic Pin"
	}`))
	if err != nil {
		t.Fatalf("executeResolveUserLocation not found: %v", err)
	}
	noneMap := none.(map[string]any)
	if noneMap["not_found_reason"] == nil || !strings.Contains(noneMap["not_found_reason"].(string), "no matching user location") {
		t.Fatalf("not_found_reason = %#v, want user-location no-result", noneMap["not_found_reason"])
	}
}

func TestGeocodePlaceUsesSharedSearchAndReturnsRouteRefs(t *testing.T) {
	provider := &geocodePlaceForwardProvider{results: []geosearch.Result{{
		Name:   "Synthetic External Place",
		Type:   "place",
		Lat:    48.3,
		Lng:    11.3,
		Source: geosearch.SourceExternal,
	}}}
	deps := &Deps{
		GeoSearchExternal: provider,
		GeoSearchPolicy:   geosearch.ExternalPolicyAllowed,
		GeoSearchCache:    geosearch.NewCache(time.Minute),
		GeoSearchQuota:    geosearch.NewLimiter(10, time.Hour, geocodePlaceFixedClock()),
	}

	result, err := executeGeocodePlace(context.Background(), deps, "user-a", json.RawMessage(`{
		"query": "Synthetic External Place",
		"sources": "external",
		"limit": 5
	}`))
	if err != nil {
		t.Fatalf("executeGeocodePlace: %v", err)
	}
	got := result.(map[string]any)
	if got["external_status"] != geosearch.ExternalStatusAvailable {
		t.Fatalf("external_status = %#v, want available", got["external_status"])
	}
	if got["status"] != "ok" {
		t.Fatalf("status = %#v, want ok: %#v", got["status"], got)
	}
	if got["next_action"] != "use_candidate_ref_then_check_routing_coverage" {
		t.Fatalf("next_action = %#v, want route ref usage", got["next_action"])
	}
	candidates := got["candidates"].([]map[string]any)
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one external candidate", candidates)
	}
	if !strings.HasPrefix(candidates[0]["ref"].(string), "route-ref:") {
		t.Fatalf("candidate ref = %#v, want route ref", candidates[0]["ref"])
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
}

func TestGeocodePlaceLocalMapUsesPlaceSearch(t *testing.T) {
	sources := normalizeGeocodePlaceSources("local_map")
	if got := geosearchPlaceType(sources); got != "place" {
		t.Fatalf("geosearchPlaceType(local_map) = %q, want place", got)
	}

	allSources := normalizeGeocodePlaceSources("")
	if got := geosearchPlaceType(allSources); got != "place" {
		t.Fatalf("geosearchPlaceType(default) = %q, want place", got)
	}
	if !allSources["external"] {
		t.Fatalf("default geocode sources = %#v, want external included for all-source lookup", allSources)
	}

	externalSources := normalizeGeocodePlaceSources("external")
	if got := geosearchPlaceType(externalSources); got != "external" {
		t.Fatalf("geosearchPlaceType(external) = %q, want external", got)
	}
}

func TestGeocodePlaceExternalDisabledAndRateLimited(t *testing.T) {
	provider := &geocodePlaceForwardProvider{}
	disabled, err := executeGeocodePlace(context.Background(), &Deps{
		GeoSearchExternal: provider,
		GeoSearchPolicy:   geosearch.ExternalPolicyDisabled,
		GeoSearchCache:    geosearch.NewCache(time.Minute),
		GeoSearchQuota:    geosearch.NewLimiter(10, time.Hour, geocodePlaceFixedClock()),
	}, "user-a", json.RawMessage(`{"query":"Synthetic Place","sources":"external"}`))
	if err != nil {
		t.Fatalf("executeGeocodePlace disabled: %v", err)
	}
	disabledMap := disabled.(map[string]any)
	if disabledMap["external_status"] != geosearch.ExternalStatusDisabled {
		t.Fatalf("external_status = %#v, want disabled", disabledMap["external_status"])
	}
	if disabledMap["status"] != "unavailable" {
		t.Fatalf("status = %#v, want unavailable: %#v", disabledMap["status"], disabledMap)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 while disabled", provider.calls)
	}

	limitedDeps := &Deps{
		GeoSearchExternal: provider,
		GeoSearchPolicy:   geosearch.ExternalPolicyAllowed,
		GeoSearchCache:    geosearch.NewCache(time.Minute),
		GeoSearchQuota:    geosearch.NewLimiter(1, time.Hour, geocodePlaceFixedClock()),
	}
	if _, err := executeGeocodePlace(context.Background(), limitedDeps, "user-a", json.RawMessage(`{"query":"First Synthetic Place","sources":"external"}`)); err != nil {
		t.Fatalf("executeGeocodePlace first allowed: %v", err)
	}
	blocked, err := executeGeocodePlace(context.Background(), limitedDeps, "user-a", json.RawMessage(`{"query":"Second Synthetic Place","sources":"external"}`))
	if err != nil {
		t.Fatalf("executeGeocodePlace rate limited: %v", err)
	}
	blockedMap := blocked.(map[string]any)
	if blockedMap["external_status"] != geosearch.ExternalStatusRateLimited {
		t.Fatalf("external_status = %#v, want rate_limited", blockedMap["external_status"])
	}
	if blockedMap["status"] != "rate_limited" {
		t.Fatalf("status = %#v, want rate_limited: %#v", blockedMap["status"], blockedMap)
	}
	if blockedMap["next_action"] != "report_external_place_search_rate_limit" {
		t.Fatalf("next_action = %#v, want rate-limit report", blockedMap["next_action"])
	}
	rateLimit := blockedMap["rate_limit"].(*geosearch.QuotaState)
	if rateLimit.BlockingReason != "rate_limited" {
		t.Fatalf("rate_limit = %#v, want blocking reason", rateLimit)
	}
}

func TestGeocodePlaceRejectsUnsupportedBBoxAndInvalidNear(t *testing.T) {
	if _, err := executeGeocodePlace(context.Background(), nil, "user-a", json.RawMessage(`{
		"query": "Synthetic Place",
		"bbox": {"min_lat": 48.0, "min_lng": 11.0, "max_lat": 48.5, "max_lng": 11.5}
	}`)); err == nil || !strings.Contains(err.Error(), "bbox") {
		t.Fatalf("executeGeocodePlace bbox error = %v, want unsupported bbox error", err)
	}

	if _, err := executeGeocodePlace(context.Background(), nil, "user-a", json.RawMessage(`{
		"query": "Synthetic Place",
		"near": {"ref": "route-ref:not-json"}
	}`)); err == nil || !strings.Contains(err.Error(), "invalid near") {
		t.Fatalf("executeGeocodePlace near error = %v, want invalid near error", err)
	}
}

func insertRouteLocationTag(t *testing.T, pool *pgxpool.Pool, userID, name, category string, lat, lng float64) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO tags (id, user_id, name, color, text_color, category, lat, lng)
		VALUES ($1, $2, $3, '#6366F1', '#FFFFFF', $4, $5, $6)`,
		id, userID, name, category, lat, lng,
	)
	if err != nil {
		t.Fatalf("insert route location tag: %v", err)
	}
	return id
}

type geocodePlaceForwardProvider struct {
	calls   int
	results []geosearch.Result
}

func (p *geocodePlaceForwardProvider) ForwardSearch(_ context.Context, opts geosearch.SearchOptions) ([]geosearch.Result, error) {
	p.calls++
	if p.results != nil {
		return p.results, nil
	}
	return []geosearch.Result{{
		Name:   opts.Query,
		Type:   "place",
		Lat:    48.3,
		Lng:    11.3,
		Source: geosearch.SourceExternal,
	}}, nil
}

func geocodePlaceFixedClock() func() time.Time {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return now }
}

func insertRouteLocationTagWithoutCoords(t *testing.T, pool *pgxpool.Pool, userID, name, category string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO tags (id, user_id, name, color, text_color, category)
		VALUES ($1, $2, $3, '#6366F1', '#FFFFFF', $4)`,
		id, userID, name, category,
	)
	if err != nil {
		t.Fatalf("insert route location tag without coords: %v", err)
	}
	return id
}
