package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConsumeRoutePreviewTokenIsSingleUse(t *testing.T) {
	userID := "00000000-0000-0000-0000-000000000123"
	token := uuid.NewString()
	geometry := `{"type":"LineString","coordinates":[[7,47],[7.1,47.1]]}`
	profile := "bike"
	profileVersion := "test:1"
	routePreviewCache.Store(token, routePreviewSnapshot{
		UserID:          userID,
		Token:           token,
		Hash:            routePreviewHash(userID, geometry, profile, profile, profileVersion),
		GeometryGeoJSON: geometry,
		ProfileKey:      profile,
		BaseProfile:     profile,
		ProfileVersion:  profileVersion,
		CreatedAt:       time.Now(),
	})
	t.Cleanup(func() { routePreviewCache.Delete(token) })

	if _, err := verifyRoutePreviewToken(userID, token, geometry, profile); err != nil {
		t.Fatalf("verifyRoutePreviewToken before consume: %v", err)
	}
	if _, err := consumeRoutePreviewToken(userID, token, geometry, profile); err != nil {
		t.Fatalf("consumeRoutePreviewToken: %v", err)
	}
	if _, err := verifyRoutePreviewToken(userID, token, geometry, profile); err == nil {
		t.Fatal("verifyRoutePreviewToken succeeded after consume")
	}
}

func TestAttachRoutePreviewTokenNormalizesFeatureGeometryWithElevation(t *testing.T) {
	userID := "00000000-0000-0000-0000-000000000123"
	result := map[string]any{
		"type": "FeatureCollection",
		"features": []any{map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type": "LineString",
				"coordinates": []any{
					[]any{7.0, 47.0, 500.0},
					[]any{7.1, 47.1, 510.0},
				},
			},
			"properties": map[string]any{},
		}},
	}

	attachRoutePreviewToken(userID, result, "hiking-mountain", "hiking-mountain", "test:1")
	token, ok := result["route_preview_token"].(string)
	if !ok || token == "" {
		t.Fatalf("route_preview_token missing from %#v", result)
	}
	t.Cleanup(func() { routePreviewCache.Delete(token) })

	if got := result["geometry_geojson"]; got != `{"type":"LineString","coordinates":[[7,47],[7.1,47.1]]}` {
		t.Fatalf("geometry_geojson = %v", got)
	}
}

func TestPrepareSavePlannedRouteRejectsTamperedPreviewMetadata(t *testing.T) {
	userID := "00000000-0000-0000-0000-000000000123"
	geometry := `{"type":"LineString","coordinates":[[7,47],[7.1,47.1]]}`
	result := map[string]any{
		"type": "FeatureCollection",
		"features": []any{map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "LineString",
				"coordinates": []any{[]any{7.0, 47.0}, []any{7.1, 47.1}},
			},
			"properties": map[string]any{
				"segments": []any{map[string]any{
					"from_idx": 0,
					"to_idx":   1,
					"surface":  "gravel",
				}},
			},
		}},
	}
	attachRoutePreviewToken(userID, result, "hiking-mountain", "hiking-mountain", "test:1")
	token := result["route_preview_token"].(string)
	t.Cleanup(func() { routePreviewCache.Delete(token) })

	raw, err := json.Marshal(map[string]any{
		"preview_token":    token,
		"name":             "Test Route",
		"sport_type":       "hiking",
		"profile":          "hiking-mountain",
		"geometry_geojson": geometry,
		"segments": []any{map[string]any{
			"from_waypoint":  "start",
			"to_waypoint":    "end",
			"mode":           "routed",
			"smooth":         false,
			"distance_m":     100,
			"elevation_gain": 0,
			"elevation_loss": 0,
			"sub_segments": []any{map[string]any{
				"from_idx": 0,
				"to_idx":   1,
				"surface":  "asphalt",
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := prepareSavePlannedRoute(nil, nil, userID, raw); err == nil || !strings.Contains(err.Error(), "route metadata does not match preview") {
		t.Fatalf("prepareSavePlannedRoute err = %v, want route metadata mismatch", err)
	}
}
