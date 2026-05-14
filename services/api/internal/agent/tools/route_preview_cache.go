package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const routePreviewTTL = 30 * time.Minute

type routePreviewSnapshot struct {
	UserID          string
	Token           string
	Hash            string
	GeometryGeoJSON string
	ProfileKey      string
	BaseProfile     string
	ProfileVersion  string
	CreatedAt       time.Time
}

var routePreviewCache sync.Map

func attachRoutePreviewToken(userID string, result map[string]any, profileKey, baseProfile, profileVersion string) {
	geometry, err := routeGeometryGeoJSON(result)
	if err != nil {
		return
	}
	hash := routePreviewHash(userID, geometry, profileKey, baseProfile, profileVersion)
	token := uuid.NewString()
	routePreviewCache.Store(token, routePreviewSnapshot{
		UserID:          userID,
		Token:           token,
		Hash:            hash,
		GeometryGeoJSON: geometry,
		ProfileKey:      profileKey,
		BaseProfile:     baseProfile,
		ProfileVersion:  profileVersion,
		CreatedAt:       time.Now(),
	})
	result["route_preview_token"] = token
	result["route_preview_hash"] = hash
	result["geometry_geojson"] = geometry
	result["route_profile_version"] = profileVersion
}

func verifyRoutePreviewToken(userID, token, geometry, profile string) (*routePreviewSnapshot, error) {
	raw, ok := routePreviewCache.Load(token)
	if !ok {
		return nil, fmt.Errorf("route preview token not found or expired")
	}
	snapshot, ok := raw.(routePreviewSnapshot)
	if !ok || snapshot.UserID != userID {
		return nil, fmt.Errorf("route preview token not found or expired")
	}
	if time.Since(snapshot.CreatedAt) > routePreviewTTL {
		routePreviewCache.Delete(token)
		return nil, fmt.Errorf("route preview token expired")
	}
	if snapshot.GeometryGeoJSON != geometry {
		return nil, fmt.Errorf("route geometry does not match preview")
	}
	if snapshot.ProfileKey != profile && snapshot.BaseProfile != profile {
		return nil, fmt.Errorf("route profile does not match preview")
	}
	return &snapshot, nil
}

func consumeRoutePreviewToken(userID, token, geometry, profile string) (*routePreviewSnapshot, error) {
	raw, ok := routePreviewCache.LoadAndDelete(token)
	if !ok {
		return nil, fmt.Errorf("route preview token not found or expired")
	}
	snapshot, ok := raw.(routePreviewSnapshot)
	if !ok || snapshot.UserID != userID {
		return nil, fmt.Errorf("route preview token not found or expired")
	}
	if time.Since(snapshot.CreatedAt) > routePreviewTTL {
		return nil, fmt.Errorf("route preview token expired")
	}
	if snapshot.GeometryGeoJSON != geometry {
		return nil, fmt.Errorf("route geometry does not match preview")
	}
	if snapshot.ProfileKey != profile && snapshot.BaseProfile != profile {
		return nil, fmt.Errorf("route profile does not match preview")
	}
	return &snapshot, nil
}

func routePreviewHash(userID, geometry, profileKey, baseProfile, profileVersion string) string {
	sum := sha256.Sum256([]byte(userID + "\x00" + geometry + "\x00" + profileKey + "\x00" + baseProfile + "\x00" + profileVersion))
	return hex.EncodeToString(sum[:])
}

func routeGeometryGeoJSON(result map[string]any) (string, error) {
	if result == nil {
		return "", fmt.Errorf("route preview result missing")
	}
	if raw, ok := result["geometry_geojson"].(string); ok && raw != "" {
		if err := validateRouteGeometry(raw); err != nil {
			return "", err
		}
		return raw, nil
	}
	if geometry, ok := result["geometry"]; ok {
		raw, err := json.Marshal(geometry)
		if err != nil {
			return "", err
		}
		if err := validateRouteGeometry(string(raw)); err != nil {
			return "", err
		}
		return string(raw), nil
	}
	features, ok := result["features"].([]any)
	if !ok || len(features) == 0 {
		return "", fmt.Errorf("route preview geometry missing")
	}
	feature, ok := features[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("route preview feature invalid")
	}
	geometry, ok := feature["geometry"]
	if !ok {
		return "", fmt.Errorf("route preview geometry missing")
	}
	raw, err := json.Marshal(geometry)
	if err != nil {
		return "", err
	}
	if err := validateRouteGeometry(string(raw)); err != nil {
		return "", err
	}
	return string(raw), nil
}
