package tools

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConsumeRoutePreviewTokenIsSingleUse(t *testing.T) {
	userID := "00000000-0000-0000-0000-000000000123"
	token := uuid.NewString()
	geometry := `{"type":"LineString","coordinates":[[7,47],[7.1,47.1]]}`
	profile := "bike"
	routePreviewCache.Store(token, routePreviewSnapshot{
		UserID:          userID,
		Token:           token,
		Hash:            routePreviewHash(userID, geometry, profile, profile),
		GeometryGeoJSON: geometry,
		ProfileKey:      profile,
		BaseProfile:     profile,
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
