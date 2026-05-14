package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
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
