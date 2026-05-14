package geosearch

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSearchServiceBuildsLocalQueriesWithProximity(t *testing.T) {
	opts := SearchOptions{
		Query: "Example",
		Type:  "all",
		Limit: 7,
		Near:  &Point{Lat: 1.2, Lng: 3.4},
	}

	streetSQL, streetArgs, err := buildStreetSearchQuery(opts)
	if err != nil {
		t.Fatalf("buildStreetSearchQuery returned error: %v", err)
	}
	if len(streetArgs) != 4 {
		t.Fatalf("street args len = %d, want 4", len(streetArgs))
	}
	if streetArgs[0] != "Example" || streetArgs[1] != 3.4 || streetArgs[2] != 1.2 || streetArgs[3] != 7 {
		t.Fatalf("street args = %#v", streetArgs)
	}
	if !containsAll(streetSQL, []string{
		"FROM osm_ways",
		"ST_Distance(geom::geography, ST_Point($2,$3)::geography) AS dist_m",
		"ORDER BY geom::geography <-> ST_Point($2,$3)::geography",
		"LIMIT $4",
	}) {
		t.Fatalf("street SQL missing expected proximity clauses:\n%s", streetSQL)
	}

	placeSQL, placeArgs, err := buildPlaceSearchQuery(opts)
	if err != nil {
		t.Fatalf("buildPlaceSearchQuery returned error: %v", err)
	}
	if len(placeArgs) != 2 || placeArgs[0] != "Example" || placeArgs[1] != 7 {
		t.Fatalf("place args = %#v", placeArgs)
	}
	if !containsAll(placeSQL, []string{
		"FROM osm_pois",
		"category IN ('settlement', 'summit', 'pass')",
		"ORDER BY similarity(name, $1) DESC",
	}) {
		t.Fatalf("place SQL missing expected clauses:\n%s", placeSQL)
	}
}

func TestSearchServiceExternalDisabledDoesNotCallProvider(t *testing.T) {
	provider := &recordingForwardProvider{}
	svc := NewService(ServiceConfig{
		Local:    emptyLocalSearcher{},
		External: provider,
		Policy:   ExternalPolicyDisabled,
		Cache:    NewCache(time.Minute),
		Quota:    NewLimiter(10, time.Hour, fixedClock()),
	})

	res, err := svc.Search(context.Background(), SearchOptions{
		Query:           "generic query",
		Type:            "all",
		Limit:           5,
		UserID:          "user-a",
		IncludeExternal: true,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if res.ExternalStatus != ExternalStatusDisabled {
		t.Fatalf("ExternalStatus = %q, want %q", res.ExternalStatus, ExternalStatusDisabled)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestSearchServiceManualConfirmDoesNotCallProvider(t *testing.T) {
	provider := &recordingForwardProvider{}
	svc := NewService(ServiceConfig{
		Local:    emptyLocalSearcher{},
		External: provider,
		Policy:   ExternalPolicyManualConfirm,
		Cache:    NewCache(time.Minute),
		Quota:    NewLimiter(10, time.Hour, fixedClock()),
	})

	res, err := svc.Search(context.Background(), SearchOptions{
		Query:           "generic query",
		Type:            "all",
		Limit:           5,
		UserID:          "user-a",
		IncludeExternal: true,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if res.ExternalStatus != ExternalStatusRequiresUserConfirmation {
		t.Fatalf("ExternalStatus = %q, want %q", res.ExternalStatus, ExternalStatusRequiresUserConfirmation)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
	if res.Quota != nil && res.Quota.Used {
		t.Fatalf("quota used on manual-confirm response: %#v", res.Quota)
	}
}

func TestSearchServiceExternalUnconfiguredReportsStatus(t *testing.T) {
	svc := NewService(ServiceConfig{
		Local:  emptyLocalSearcher{},
		Policy: ExternalPolicyAllowed,
		Cache:  NewCache(time.Minute),
		Quota:  NewLimiter(10, time.Hour, fixedClock()),
	})

	res, err := svc.Search(context.Background(), SearchOptions{
		Query:           "generic query",
		Type:            "all",
		Limit:           5,
		UserID:          "user-a",
		IncludeExternal: true,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if res.ExternalStatus != ExternalStatusUnconfigured {
		t.Fatalf("ExternalStatus = %q, want %q", res.ExternalStatus, ExternalStatusUnconfigured)
	}
	if res.Quota == nil || res.Quota.Used {
		t.Fatalf("quota = %#v, want visible state without quota use", res.Quota)
	}
}

func TestSearchServiceExternalRateLimitBlocksBeforeProvider(t *testing.T) {
	provider := &recordingForwardProvider{}
	clock := fixedClock()
	svc := NewService(ServiceConfig{
		Local:    emptyLocalSearcher{},
		External: provider,
		Policy:   ExternalPolicyAllowed,
		Cache:    NewCache(time.Minute),
		Quota:    NewLimiter(1, time.Hour, clock),
	})

	opts := SearchOptions{
		Query:           "generic query",
		Type:            "all",
		Limit:           5,
		UserID:          "user-a",
		IncludeExternal: true,
	}
	if _, err := svc.Search(context.Background(), opts); err != nil {
		t.Fatalf("first Search returned error: %v", err)
	}
	res, err := svc.Search(context.Background(), SearchOptions{
		Query:           "different query",
		Type:            "all",
		Limit:           5,
		UserID:          "user-a",
		IncludeExternal: true,
	})
	if err != nil {
		t.Fatalf("second Search returned error: %v", err)
	}
	if res.ExternalStatus != ExternalStatusRateLimited {
		t.Fatalf("ExternalStatus = %q, want %q", res.ExternalStatus, ExternalStatusRateLimited)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if res.Quota == nil || res.Quota.Remaining != 0 || res.Quota.BlockingReason != "rate_limited" {
		t.Fatalf("quota = %#v, want blocked remaining=0", res.Quota)
	}
}

func TestSearchServiceExternalCacheAvoidsQuotaBurn(t *testing.T) {
	provider := &recordingForwardProvider{results: []Result{{
		Name:   "Generic Result",
		Type:   "place",
		Lat:    1.3,
		Lng:    3.5,
		Source: SourceExternal,
	}}}
	svc := NewService(ServiceConfig{
		Local:    emptyLocalSearcher{},
		External: provider,
		Policy:   ExternalPolicyAllowed,
		Cache:    NewCache(time.Minute),
		Quota:    NewLimiter(1, time.Hour, fixedClock()),
	})
	opts := SearchOptions{
		Query:           "generic query",
		Type:            "all",
		Limit:           5,
		UserID:          "user-a",
		IncludeExternal: true,
	}

	first, err := svc.Search(context.Background(), opts)
	if err != nil {
		t.Fatalf("first Search returned error: %v", err)
	}
	second, err := svc.Search(context.Background(), opts)
	if err != nil {
		t.Fatalf("second Search returned error: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if first.Quota == nil || !first.Quota.Used {
		t.Fatalf("first quota = %#v, want used", first.Quota)
	}
	if second.Quota == nil || second.Quota.Used {
		t.Fatalf("second quota = %#v, want cache hit without quota use", second.Quota)
	}
	if len(second.Results) != 1 || second.Results[0].Name != "Generic Result" {
		t.Fatalf("second results = %#v", second.Results)
	}
	if second.ExternalStatus != ExternalStatusCached {
		t.Fatalf("ExternalStatus = %q, want %q", second.ExternalStatus, ExternalStatusCached)
	}
}

func TestSearchServiceExternalProviderErrorReturnsUnreachable(t *testing.T) {
	provider := &recordingForwardProvider{err: errors.New("network down")}
	svc := NewService(ServiceConfig{
		Local:    emptyLocalSearcher{},
		External: provider,
		Policy:   ExternalPolicyAllowed,
		Cache:    NewCache(time.Minute),
		Quota:    NewLimiter(10, time.Hour, fixedClock()),
	})

	res, err := svc.Search(context.Background(), SearchOptions{
		Query:           "generic query",
		Type:            "all",
		Limit:           5,
		UserID:          "user-a",
		IncludeExternal: true,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if res.ExternalStatus != ExternalStatusUnreachable {
		t.Fatalf("ExternalStatus = %q, want %q", res.ExternalStatus, ExternalStatusUnreachable)
	}
}

type emptyLocalSearcher struct{}

func (emptyLocalSearcher) SearchLocal(context.Context, SearchOptions) ([]Result, error) {
	return []Result{}, nil
}

type recordingForwardProvider struct {
	calls   int
	results []Result
	err     error
}

func (p *recordingForwardProvider) ForwardSearch(_ context.Context, opts SearchOptions) ([]Result, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.results != nil {
		return p.results, nil
	}
	return []Result{{
		Name:   opts.Query,
		Type:   "place",
		Lat:    1,
		Lng:    3,
		Source: SourceExternal,
	}}, nil
}

func fixedClock() func() time.Time {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return now }
}

func containsAll(s string, parts []string) bool {
	for _, part := range parts {
		if !stringsContains(s, part) {
			return false
		}
	}
	return true
}

func stringsContains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
